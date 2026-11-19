package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
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
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ContainerName string   `json:"container_name,omitempty"`
	Driver        string   `json:"driver"` // "mysql" or "postgres"
	Host          string   `json:"host"`
	Port          string   `json:"port"`
	User          string   `json:"user"`
	Password      string   `json:"-"`
	AltUser       string   `json:"-"`
	AltPassword   string   `json:"-"`
	Database      string   `json:"database"`
	Status        string   `json:"status"`
	Image         string   `json:"image"`
	Networks      []string `json:"-"`
	SharedNetwork bool     `json:"-"`
	HasPublicPort bool     `json:"-"`
}

type DockerDiscoveryService struct {
	cli         *client.Client
	mu          sync.RWMutex
	unavailable sync.Once
	unavailErr  error
}

func NewDockerDiscoveryService() *DockerDiscoveryService {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("Docker discovery: client init failed: %v", err)
		return &DockerDiscoveryService{unavailErr: err}
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
	if err != nil {
		d.unavailable.Do(func() {
			d.unavailErr = err
			log.Printf("Docker discovery unavailable (check docker.sock permissions): %v", err)
		})
		return false
	}
	return true
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

	upsert := func(key string, proj *DiscoveredProject) *DiscoveredProject {
		if existing, ok := projectsMap[key]; ok {
			if proj.Status == "running" {
				existing.Status = "running"
			}
			if existing.Path == "" && proj.Path != "" {
				existing.Path = proj.Path
			}
			if existing.Domain == "" && proj.Domain != "" {
				existing.Domain = proj.Domain
			}
			if existing.PHPVersion == "" && proj.PHPVersion != "" {
				existing.PHPVersion = proj.PHPVersion
			}
			if proj.Secure {
				existing.Secure = true
			}
			if existing.Container == "" {
				existing.Container = proj.Container
			}
			return existing
		}
		projectsMap[key] = proj
		return proj
	}

	for _, c := range containers {
		name := containerName(c.Names)
		if isSLDInfra(name, c.Labels["com.docker.compose.project"]) {
			continue
		}

		isSLD := c.Labels["com.supreme.sld"] == "true"
		projectID := c.Labels["com.supreme.sld.project"]
		projectPath := c.Labels["com.supreme.sld.project-path"]
		service := c.Labels["com.supreme.sld.service"]
		composeProject := c.Labels["com.docker.compose.project"]
		workingDir := c.Labels["com.docker.compose.project.working_dir"]
		status := normalizeStatus(c.State)

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

		phpVer := phpVersionFromImage(c.Image)
		secure := hasTLSPort(c.Status)
		var privatePorts []uint16
		for _, p := range c.Ports {
			privatePorts = append(privatePorts, p.PrivatePort)
			if p.PrivatePort == 443 || p.PublicPort == 443 {
				secure = true
			}
		}

		if isSLD && projectID != "" {
			projName := projectID
			parts := strings.Split(projectID, "-")
			if len(parts) > 1 && len(parts[len(parts)-1]) == 8 {
				projName = strings.Join(parts[:len(parts)-1], "-")
			}
			if domain == "" {
				domain = projName + "." + defaultTLD
			}
			proj := upsert(projectID, &DiscoveredProject{
				Name:       projName,
				Path:       projectPath,
				Domain:     domain,
				PHPVersion: phpVer,
				Secure:     secure,
				Type:       "docker",
				Status:     status,
				Container:  name,
				Tags:       []string{"docker", "v2"},
			})
			if service == "php" && phpVer != "" {
				proj.PHPVersion = phpVer
			}
			continue
		}

		if composeProject != "" {
			if domain == "" {
				domain = sanitizeDomainLabel(composeProject) + "." + defaultTLD
			}
			upsert("compose:"+composeProject, &DiscoveredProject{
				Name:       composeProject,
				Path:       workingDir,
				Domain:     domain,
				PHPVersion: phpVer,
				Secure:     secure,
				Type:       "docker",
				Status:     status,
				Container:  name,
				Tags:       []string{"compose", "docker"},
			})
			continue
		}

		if domain != "" && name != "" {
			upsert("host:"+domain, &DiscoveredProject{
				Name:      name,
				Domain:    domain,
				Type:      "docker",
				Status:    status,
				Container: name,
				Tags:      []string{"docker"},
			})
			continue
		}

		if looksLikeAppContainer(name, c.Image, privatePorts) {
			if domain == "" {
				domain = sanitizeDomainLabel(name) + "." + defaultTLD
			}
			upsert("name:"+name, &DiscoveredProject{
				Name:       name,
				Domain:     domain,
				PHPVersion: phpVer,
				Secure:     secure,
				Type:       "docker",
				Status:     status,
				Container:  name,
				Tags:       []string{"docker"},
			})
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

	var ownNetworks map[string]bool
	if isInsideDocker {
		ownNetworks = d.currentContainerNetworks(ctx)
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	for _, c := range containers {
		c := c
		name := containerName(c.Names)
		composeProject := c.Labels["com.docker.compose.project"]
		if isSLDInfra(name, composeProject) {
			continue
		}

		svc := c.Labels["com.supreme.sld.service"]
		if !mightBeDatabaseContainer(svc, c.Image, name) {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

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

			driver := detectDatabaseDriver(svc, c.Image, name, envMap)
			if driver == "" {
				return
			}

			user, password, dbname := resolveDatabaseCredentials(driver, envMap)
			var altUser, altPassword string
			if driver == "mysql" {
				if u := firstNonEmpty(envMap["MYSQL_USER"], envMap["MARIADB_USER"]); u != "" && u != user {
					altUser = u
					altPassword = firstNonEmpty(envMap["MYSQL_PASSWORD"], envMap["MARIADB_PASSWORD"])
				}
			}

			sharedNet := false
			containerIP := ""
			var netNames []string
			if err == nil && inspect.NetworkSettings != nil {
				for netName, ep := range inspect.NetworkSettings.Networks {
					netNames = append(netNames, netName)
					if ep != nil && containerIP == "" && ep.IPAddress != "" {
						containerIP = ep.IPAddress
					}
					if ownNetworks[netName] {
						sharedNet = true
					}
				}
			}

			internalPort := uint16(3306)
			if driver == "postgres" {
				internalPort = 5432
			}
			hasPublicPort := false
			for _, p := range c.Ports {
				if p.PrivatePort == internalPort && p.PublicPort > 0 {
					hasPublicPort = true
					break
				}
			}

			host, port := resolveDatabaseEndpoint(driver, name, isInsideDocker, sharedNet, c.Ports, containerIP)

			displayName := name
			if proj := c.Labels["com.supreme.sld.project"]; proj != "" {
				displayName = fmt.Sprintf("%s (%s DB)", proj, strings.ToUpper(driver))
			} else if composeProject != "" {
				displayName = fmt.Sprintf("%s (%s)", composeProject, name)
			}

			id := c.ID
			if len(id) > 12 {
				id = id[:12]
			}

			mu.Lock()
			databases = append(databases, DiscoveredDatabase{
				ID:            id,
				Name:          displayName,
				ContainerName: name,
				Driver:        driver,
				Host:          host,
				Port:          port,
				User:          user,
				Password:      password,
				AltUser:       altUser,
				AltPassword:   altPassword,
				Database:      dbname,
				Status:        normalizeStatus(c.State),
				Image:         c.Image,
				Networks:      netNames,
				SharedNetwork: sharedNet,
				HasPublicPort: hasPublicPort,
			})
			mu.Unlock()
		}()
	}

	wg.Wait()

	return databases, nil
}

// EnsureNetworkAccess attaches the current SLD container to the target DB's bridge network
// when running inside Docker and the target has neither a shared network nor a published host port.
func (d *DockerDiscoveryService) EnsureNetworkAccess(ctx context.Context, target *DiscoveredDatabase) (string, string, error) {
	if target == nil {
		return "", "", fmt.Errorf("nil database target")
	}
	isInsideDocker := os.Getenv("SLD_STATE_DIR") != "" || fileExists("/.dockerenv")
	if !isInsideDocker || target.SharedNetwork || target.HasPublicPort || len(target.Networks) == 0 || d.cli == nil {
		return target.Host, target.Port, nil
	}

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return target.Host, target.Port, nil
	}

	defaultPort := "3306"
	if target.Driver == "postgres" {
		defaultPort = "5432"
	}

	for _, netName := range target.Networks {
		if netName == "" || netName == "none" || netName == "host" {
			continue
		}
		err := d.cli.NetworkConnect(ctx, netName, hostname, nil)
		if err == nil || strings.Contains(err.Error(), "already exists in network") {
			log.Printf("Docker discovery: connected SLD container to network %s to reach %s", netName, target.ContainerName)
			return target.ContainerName, defaultPort, nil
		}
	}

	return target.Host, target.Port, nil
}

func (d *DockerDiscoveryService) currentContainerNetworks(ctx context.Context) map[string]bool {
	nets := make(map[string]bool)
	hostname, err := os.Hostname()
	if err != nil || hostname == "" || d.cli == nil {
		return nets
	}
	inspect, err := d.cli.ContainerInspect(ctx, hostname)
	if err != nil || inspect.NetworkSettings == nil {
		return nets
	}
	for netName := range inspect.NetworkSettings.Networks {
		nets[netName] = true
	}
	return nets
}

func mightBeDatabaseContainer(serviceLabel, image, name string) bool {
	n := strings.ToLower(name)
	if strings.Contains(n, "-backup") || strings.Contains(n, "_backup") {
		return false
	}
	if serviceLabel == "db" {
		return true
	}
	return detectDatabaseDriver("", image, name, nil) != ""
}

func detectDatabaseDriver(serviceLabel, image, name string, env map[string]string) string {
	img := strings.ToLower(image)
	n := strings.ToLower(name)

	// Check Postgres first so SLD V2 Postgres containers (labeled service=db) are not misclassified as MySQL.
	if strings.Contains(img, "postgres") || strings.Contains(img, "postgis") || strings.Contains(n, "postgres") || strings.Contains(n, "_pg_") {
		return "postgres"
	}
	if strings.Contains(img, "mariadb") || strings.Contains(img, "mysql") || strings.Contains(n, "mariadb") || strings.Contains(n, "mysql") {
		return "mysql"
	}
	if serviceLabel == "db" {
		if env["POSTGRES_DB"] != "" || env["POSTGRES_USER"] != "" || env["POSTGRES_PASSWORD"] != "" {
			return "postgres"
		}
		return "mysql"
	}
	return ""
}

func resolveDatabaseCredentials(driver string, env map[string]string) (user, password, dbname string) {
	if driver == "postgres" {
		user = firstNonEmpty(env["POSTGRES_USER"], "postgres")
		password = env["POSTGRES_PASSWORD"]
		dbname = firstNonEmpty(env["POSTGRES_DB"], user)
		return user, password, dbname
	}

	// MySQL / MariaDB: prefer root + root password for full server database listing,
	// falling back to MYSQL_USER + MYSQL_PASSWORD when root password is not set.
	dbname = firstNonEmpty(env["MYSQL_DATABASE"], env["MARIADB_DATABASE"])
	if rootPass := firstNonEmpty(env["MYSQL_ROOT_PASSWORD"], env["MARIADB_ROOT_PASSWORD"]); rootPass != "" {
		return "root", rootPass, dbname
	}
	if isTruthy(firstNonEmpty(env["MYSQL_ALLOW_EMPTY_PASSWORD"], env["MARIADB_ALLOW_EMPTY_PASSWORD"])) {
		return "root", "", dbname
	}
	if u := firstNonEmpty(env["MYSQL_USER"], env["MARIADB_USER"]); u != "" {
		return u, firstNonEmpty(env["MYSQL_PASSWORD"], env["MARIADB_PASSWORD"]), dbname
	}
	return "root", "sld", dbname
}

func resolveDatabaseEndpoint(driver, name string, isInsideDocker, sharedNetwork bool, ports []types.Port, containerIP string) (host, port string) {
	internalPort := uint16(3306)
	defaultPort := "3306"
	if driver == "postgres" {
		internalPort = 5432
		defaultPort = "5432"
	}

	var pubPort uint16
	for _, p := range ports {
		if p.PrivatePort == internalPort && p.PublicPort > 0 {
			pubPort = p.PublicPort
			break
		}
	}

	if isInsideDocker {
		if sharedNetwork {
			return name, defaultPort
		}
		if pubPort > 0 {
			return "host.docker.internal", fmt.Sprintf("%d", pubPort)
		}
		if containerIP != "" {
			return containerIP, defaultPort
		}
		return name, defaultPort
	}

	if pubPort > 0 {
		return "127.0.0.1", fmt.Sprintf("%d", pubPort)
	}
	return "127.0.0.1", defaultPort
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (d *DockerDiscoveryService) FindDatabase(ctx context.Context, id string) (*DiscoveredDatabase, error) {
	dbs, err := d.DiscoverDatabaseContainers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range dbs {
		if dbs[i].ID == id {
			return &dbs[i], nil
		}
	}
	return nil, fmt.Errorf("database target %s not found", id)
}

func containerName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

func isSLDInfra(name, composeProject string) bool {
	if strings.EqualFold(composeProject, "sld") {
		return true
	}
	n := strings.ToLower(name)
	if n == "sld-router" || strings.HasPrefix(n, "sld-app") || strings.HasPrefix(n, "sld_app") {
		return true
	}
	return false
}

func normalizeStatus(state string) string {
	if strings.EqualFold(state, "running") {
		return "running"
	}
	return "stopped"
}

func phpVersionFromImage(image string) string {
	if !strings.Contains(strings.ToLower(image), "php") {
		return ""
	}
	parts := strings.Split(image, ":")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return ""
}

func hasTLSPort(status string) bool {
	return strings.Contains(status, "443")
}

func looksLikeAppContainer(name, image string, privatePorts []uint16) bool {
	n := strings.ToLower(name + " " + image)
	skip := []string{"redis", "mariadb", "mysql", "postgres", "postgis", "mailpit", "minio", "rclone"}
	for _, s := range skip {
		if strings.Contains(n, s) {
			return false
		}
	}
	webHints := []string{"nginx", "apache", "httpd", "php", "node", "caddy", "traefik", "-app", "_app", "-web", "_web", "frontend"}
	for _, h := range webHints {
		if strings.Contains(n, h) {
			return true
		}
	}
	for _, port := range privatePorts {
		switch port {
		case 80, 443, 8080, 3000, 5173, 8000, 8081:
			return true
		}
	}
	return false
}

func sanitizeDomainLabel(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "docker"
	}
	return name
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
