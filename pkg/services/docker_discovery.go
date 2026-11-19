package services

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type DiscoveredProject struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Domain     string   `json:"domain"`
	PHPVersion string   `json:"phpVersion,omitempty"`
	Secure     bool     `json:"secure"`
	Type       string   `json:"type"`   // "docker"
	Status     string   `json:"status"` // "running", "stopped"
	Container  string   `json:"container"`
	Tags       []string `json:"tags,omitempty"`
}

type DiscoveredDatabase struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Driver   string `json:"driver"` // "mysql" or "postgres"
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	Status   string `json:"status"`
	Image    string `json:"image"`
}

type DockerDiscoveryService struct {
	cli *client.Client
	mu  sync.RWMutex
}

func NewDockerDiscoveryService() *DockerDiscoveryService {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return &DockerDiscoveryService{}
	}
	return &DockerDiscoveryService{cli: cli}
}

func (d *DockerDiscoveryService) IsAvailable() bool {
	if d == nil || d.cli == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err := d.cli.Ping(ctx)
	return err == nil
}

func (d *DockerDiscoveryService) DiscoverProjectContainers(ctx context.Context, defaultTLD string) ([]DiscoveredProject, error) {
	if !d.IsAvailable() {
		return []DiscoveredProject{}, nil
	}
	if defaultTLD == "" {
		defaultTLD = "test"
	}

	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list docker containers: %w", err)
	}

	hostRegex := regexp.MustCompile("Host\\(`([^`]+)`\\)")
	projectsMap := make(map[string]*DiscoveredProject)

	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		// Skip internal infra containers
		if name == "sld-router" || name == "sld-app" || strings.HasPrefix(name, "sld_app") {
			continue
		}

		// 1. Check for SLD V2 Project Labels
		isSLD := c.Labels["com.supreme.sld"] == "true"
		projectID := c.Labels["com.supreme.sld.project"]
		projectPath := c.Labels["com.supreme.sld.project-path"]
		service := c.Labels["com.supreme.sld.service"]
		composeProject := c.Labels["com.docker.compose.project"]

		// Check Traefik domain
		domain := ""
		for k, v := range c.Labels {
			if strings.HasPrefix(k, "traefik.http.routers.") && strings.HasSuffix(k, ".rule") {
				match := hostRegex.FindStringSubmatch(v)
				if len(match) > 1 {
					domain = match[1]
					break
				}
			}
		}

		if isSLD && projectID != "" {
			proj, exists := projectsMap[projectID]
			if !exists {
				projName := projectID
				parts := strings.Split(projectID, "-")
				if len(parts) > 1 && len(parts[len(parts)-1]) == 8 {
					projName = strings.Join(parts[:len(parts)-1], "-")
				}
				if domain == "" {
					domain = projName + "." + defaultTLD
				}
				proj = &DiscoveredProject{
					Name:      projName,
					Path:      projectPath,
					Domain:    domain,
					Type:      "docker",
					Status:    c.State,
					Container: name,
					Tags:      []string{"docker", "v2"},
				}
				projectsMap[projectID] = proj
			}
			if c.State == "running" {
				proj.Status = "running"
			}
			if service == "php" {
				if imgParts := strings.Split(c.Image, ":"); len(imgParts) > 1 {
					proj.PHPVersion = imgParts[len(imgParts)-1]
				}
			}
			if strings.Contains(c.Status, "443") {
				proj.Secure = true
			}
			continue
		}

		// 2. Compose Project with domain
		if composeProject != "" && domain != "" {
			if _, exists := projectsMap[composeProject]; !exists {
				phpVer := ""
				if strings.Contains(c.Image, "php") {
					parts := strings.Split(c.Image, ":")
					if len(parts) > 1 {
						phpVer = parts[len(parts)-1]
					}
				}
				projectsMap[composeProject] = &DiscoveredProject{
					Name:       composeProject,
					Path:       c.Labels["com.docker.compose.project.working_dir"],
					Domain:     domain,
					PHPVersion: phpVer,
					Type:       "docker",
					Status:     c.State,
					Container:  name,
					Tags:       []string{"compose", "docker"},
				}
			}
			continue
		}

		// 3. Generic web container with Traefik Host routing rule
		if domain != "" && name != "" {
			if _, exists := projectsMap[name]; !exists {
				projectsMap[name] = &DiscoveredProject{
					Name:      name,
					Domain:    domain,
					Type:      "docker",
					Status:    c.State,
					Container: name,
					Tags:      []string{"docker"},
				}
			}
		}
	}

	result := make([]DiscoveredProject, 0, len(projectsMap))
	for _, p := range projectsMap {
		result = append(result, *p)
	}
	return result, nil
}

func (d *DockerDiscoveryService) DiscoverDatabaseContainers(ctx context.Context) ([]DiscoveredDatabase, error) {
	if !d.IsAvailable() {
		return []DiscoveredDatabase{}, nil
	}

	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list docker containers: %w", err)
	}

	databases := make([]DiscoveredDatabase, 0)
	isInsideDocker := os.Getenv("SLD_STATE_DIR") != "" || fileExists("/.dockerenv")

	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		img := strings.ToLower(c.Image)
		svc := c.Labels["com.supreme.sld.service"]

		var driver string
		if svc == "db" || strings.Contains(img, "mariadb") || strings.Contains(img, "mysql") || strings.Contains(name, "mariadb") || strings.Contains(name, "mysql") {
			driver = "mysql"
		} else if strings.Contains(img, "postgres") || strings.Contains(name, "postgres") || strings.Contains(name, "_pg_") {
			driver = "postgres"
		} else {
			continue
		}

		inspect, err := d.cli.ContainerInspect(ctx, c.ID)
		envMap := make(map[string]string)
		if err == nil && inspect.Config != nil {
			for _, e := range inspect.Config.Env {
				parts := strings.SplitN(e, "=", 2)
				if len(parts) == 2 {
					envMap[parts[0]] = parts[1]
				}
			}
		}

		host := "127.0.0.1"
		port := "3306"
		user := "root"
		password := "sld"
		dbname := ""

		if driver == "mysql" {
			port = "3306"
			if u := envMap["MYSQL_USER"]; u != "" {
				user = u
			}
			if p := envMap["MYSQL_ROOT_PASSWORD"]; p != "" {
				password = p
			} else if p := envMap["MYSQL_PASSWORD"]; p != "" {
				password = p
			}
			if db := envMap["MYSQL_DATABASE"]; db != "" {
				dbname = db
			}
		} else if driver == "postgres" {
			port = "5432"
			user = "postgres"
			if u := envMap["POSTGRES_USER"]; u != "" {
				user = u
			}
			if p := envMap["POSTGRES_PASSWORD"]; p != "" {
				password = p
			}
			if db := envMap["POSTGRES_DB"]; db != "" {
				dbname = db
			}
		}

		if isInsideDocker {
			host = name
		} else {
			for _, p := range c.Ports {
				if (driver == "mysql" && p.PrivatePort == 3306) || (driver == "postgres" && p.PrivatePort == 5432) {
					if p.PublicPort > 0 {
						port = fmt.Sprintf("%d", p.PublicPort)
					}
					break
				}
			}
		}

		displayName := name
		if proj := c.Labels["com.supreme.sld.project"]; proj != "" {
			displayName = fmt.Sprintf("%s (%s DB)", proj, strings.ToUpper(driver))
		}

		databases = append(databases, DiscoveredDatabase{
			ID:       c.ID[:12],
			Name:     displayName,
			Driver:   driver,
			Host:     host,
			Port:     port,
			User:     user,
			Password: password,
			Database: dbname,
			Status:   c.State,
			Image:    c.Image,
		})
	}

	return databases, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
