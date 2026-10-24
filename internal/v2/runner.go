package v2

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/dns"
	docker "github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
	"github.com/supreme-majesty/supreme-local-dev/internal/tls"
)

// Runner provides the high-level CLI command implementations for SLD V2.
type Runner struct {
	orch *orchestrate.Orchestrator
}

// NewRunner creates a Runner from a Docker engine client.
func NewRunner(engine docker.EngineClient) *Runner {
	return &Runner{
		orch: orchestrate.NewOrchestrator(engine),
	}
}

// Init performs first-time global setup:
// 1. Validates Docker Engine connectivity
// 2. Creates the sld-global network
// 3. Starts the Traefik router
func (r *Runner) Init(ctx context.Context) error {
	fmt.Println("[SLD] Initializing SLD V2...")

	// 1. Check Docker
	fmt.Println("[SLD] Checking Docker Engine connectivity...")
	if err := r.orch.Engine().Ping(ctx); err != nil {
		return fmt.Errorf("Docker Engine is not reachable. Please ensure Docker is installed and running.\n  → %w", err)
	}
	fmt.Println("[SLD] ✅ Docker Engine is running.")

	// 2. Setup global infra (network + router)
	fmt.Println("[SLD] Creating global network and starting router...")
	if err := r.orch.EnsureGlobalInfra(ctx); err != nil {
		return fmt.Errorf("failed to initialize global infrastructure: %w", err)
	}
	fmt.Println("[SLD] ✅ Global router is running on sld-global network.")

	fmt.Println()
	fmt.Println("[SLD] Initialization complete. SLD is ready. 🚀")
	fmt.Println("[SLD] Run 'sld start' inside a project directory to begin.")
	return nil
}

// Start reads the project's .sld/config.yaml (or auto-detects), resolves
// identity, and starts all containers via the orchestrator.
func (r *Runner) Start(ctx context.Context, projectRoot string) error {
	startTime := time.Now()

	fmt.Printf("[SLD] Starting project at %s...\n", projectRoot)

	// 1. Ensure global infra is up
	if err := r.orch.EnsureGlobalInfra(ctx); err != nil {
		return fmt.Errorf("global infrastructure check failed: %w", err)
	}

	// 2. Load config
	cfg, configData, err := loadProjectConfig(projectRoot)
	if err != nil {
		return err
	}

	fmt.Printf("[SLD] Project: %s | PHP %s | Domain: %s\n", cfg.Name, cfg.Runtime.PHP, cfg.Domain.Name)

	// 3. Compute identity
	identity, err := project.ComputeIdentity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	// 4. Build spec
	spec := &orchestrate.ProjectSpec{
		Identity:    identity,
		Config:      cfg,
		ConfigHash:  config.ComputeConfigHash(configData),
		ProjectRoot: projectRoot,
	}

	// 5. Services summary
	printServicesSummary(cfg)

	// 6. Start project
	fmt.Println("[SLD] Starting containers...")
	if err := r.orch.StartProject(ctx, spec); err != nil {
		return fmt.Errorf("failed to start project: %w", err)
	}

	elapsed := time.Since(startTime)
	fmt.Println()
	fmt.Printf("[SLD] ✅ Project is running! (%.1fs)\n", elapsed.Seconds())

	protocol := "http"
	if cfg.Domain.TLS {
		protocol = "https"
	}
	fmt.Printf("🚀 Access your site at: %s://%s\n", protocol, cfg.Domain.Name)

	return nil
}

// Stop stops and removes all containers for the project at the given path.
func (r *Runner) Stop(ctx context.Context, projectRoot string) error {
	fmt.Printf("[SLD] Stopping project at %s...\n", projectRoot)

	identity, err := project.ComputeIdentity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	// Get status before stopping for a friendly summary
	containers, _ := r.orch.ProjectStatus(ctx, identity.ID)
	if len(containers) == 0 {
		fmt.Println("[SLD] No running containers found for this project.")
		return nil
	}

	for _, c := range containers {
		if c.State == "running" {
			fmt.Printf("[SLD] Stopping %s...\n", c.Service)
		}
	}

	if err := r.orch.StopProject(ctx, identity.ID); err != nil {
		return fmt.Errorf("failed to stop project: %w", err)
	}

	fmt.Println("[SLD] ✅ Project stopped.")
	return nil
}

// Status shows the current state of project containers.
func (r *Runner) Status(ctx context.Context, projectRoot string) error {
	identity, err := project.ComputeIdentity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	containers, err := r.orch.ProjectStatus(ctx, identity.ID)
	if err != nil {
		return fmt.Errorf("failed to get project status: %w", err)
	}

	if len(containers) == 0 {
		fmt.Println("No containers running for this project.")
		fmt.Println("Run 'sld start' to launch the development environment.")
		return nil
	}

	fmt.Printf("Project: %s\n", identity.Slug)
	fmt.Printf("ID:      %s\n", identity.ID)
	fmt.Println()
	fmt.Printf("%-20s %-40s %-10s\n", "SERVICE", "CONTAINER", "STATE")
	fmt.Println(strings.Repeat("─", 72))

	for _, c := range containers {
		state := c.State
		icon := "⚪"
		if state == "running" {
			icon = "🟢"
		} else if state == "exited" {
			icon = "🔴"
		}
		fmt.Printf("%-20s %-40s %s %s\n", c.Service, c.Name, icon, state)
	}

	return nil
}

// Doctor validates the Docker environment health.
func (r *Runner) Doctor(ctx context.Context) error {
	fmt.Println("SLD Doctor — Checking system health...")
	fmt.Println()

	// 1. Docker Engine
	fmt.Print("Docker Engine:       ")
	if err := r.orch.Engine().Ping(ctx); err != nil {
		fmt.Println("❌ Unreachable")
		fmt.Printf("  → %v\n", err)
	} else {
		fmt.Println("✅ Running")
	}

	// 2. Global router & network
	fmt.Print("Global Router:       ")
	containers, err := r.orch.Engine().ListAllContainers(ctx)
	if err != nil {
		fmt.Println("❌ Error listing containers")
	} else {
		routerFound := false
		for _, c := range containers {
			if c.Name == orchestrate.RouterContainer {
				routerFound = true
				if c.State == "running" {
					fmt.Println("✅ Running (sld-router)")
				} else {
					fmt.Printf("⚠️  Exists but %s\n", c.State)
				}
				break
			}
		}
		if !routerFound {
			fmt.Println("❌ Router not found (run 'sld v2 init')")
		}
	}

	// 3. TLS Wildcard Certificates
	fmt.Print("TLS Certificates:    ")
	certMgr, err := tls.NewCertManager("")
	if err != nil {
		fmt.Printf("❌ %v\n", err)
	} else {
		if _, err := os.Stat(certMgr.CertPath()); err == nil {
			fmt.Println("✅ Wildcard certificate ready (*.test)")
		} else {
			fmt.Println("⚠️  Certificate not yet generated (run 'sld v2 init')")
		}
	}

	// 4. DNS Resolution
	fmt.Print("DNS (*.test):        ")
	resolved, _ := dns.CheckResolution("sld.test")
	if resolved {
		fmt.Println("✅ Resolving to 127.0.0.1")
	} else {
		hasHosts, _ := dns.HasHostsEntry("sld.test")
		if hasHosts {
			fmt.Println("✅ Hosts entry configured")
		} else {
			fmt.Println("⚠️  *.test not resolving to loopback (configure hosts or dnsmasq)")
		}
	}

	return nil
}

// Logs streams container logs for a project service.
func (r *Runner) Logs(ctx context.Context, projectRoot, service string, follow bool, tail string) error {
	identity, err := project.ComputeIdentity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	containers, err := r.orch.ProjectStatus(ctx, identity.ID)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		return fmt.Errorf("no containers running for project %s. Run 'sld v2 start' first", identity.Slug)
	}

	// Filter by service if provided
	var targetContainer *reconcile.ActualContainer
	if service != "" {
		for _, c := range containers {
			if strings.EqualFold(c.Service, service) || strings.Contains(strings.ToLower(c.Name), strings.ToLower(service)) {
				targetContainer = c
				break
			}
		}
		if targetContainer == nil {
			var available []string
			for _, c := range containers {
				available = append(available, c.Service)
			}
			return fmt.Errorf("service %q not found. Available services: %s", service, strings.Join(available, ", "))
		}
	} else {
		// Default to PHP container, or the first running container
		for _, c := range containers {
			if c.Service == docker.ServicePHP {
				targetContainer = c
				break
			}
		}
		if targetContainer == nil {
			targetContainer = containers[0]
		}
	}

	if tail == "" {
		tail = "100"
	}

	fmt.Printf("[SLD] Tailing logs for %s (%s)...\n", targetContainer.Service, targetContainer.Name)

	reader, err := r.orch.Engine().ContainerLogs(ctx, targetContainer.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tail,
		Timestamps: false,
	})
	if err != nil {
		return fmt.Errorf("failed to read container logs: %w", err)
	}
	defer reader.Close()

	_, err = io.Copy(os.Stdout, reader)
	return err
}

// Open opens the project domain in the host default browser.
func (r *Runner) Open(ctx context.Context, projectRoot string) error {
	cfg, _, err := loadProjectConfig(projectRoot)
	if err != nil {
		return err
	}

	domain := cfg.Domain.Name
	if domain == "" {
		domain = fmt.Sprintf("%s.test", cfg.Name)
	}

	protocol := "http"
	if cfg.Domain.TLS {
		protocol = "https"
	}

	url := fmt.Sprintf("%s://%s", protocol, domain)
	fmt.Printf("[SLD] Opening %s in default browser...\n", url)

	return openBrowserURL(url)
}

func openBrowserURL(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}

// Update pulls the latest SLD V2 Docker images from GHCR and updates running services.
func (r *Runner) Update(ctx context.Context) error {
	fmt.Println("[SLD] Updating SLD V2 Docker images from GHCR...")

	images := []string{
		fmt.Sprintf("%s/sld-router:%s", orchestrate.ImageRegistry, orchestrate.RouterImageTag),
		fmt.Sprintf("%s/sld-nginx:%s", orchestrate.ImageRegistry, orchestrate.NginxImageTag),
		fmt.Sprintf("%s/sld-php:8.1", orchestrate.ImageRegistry),
		fmt.Sprintf("%s/sld-php:8.2", orchestrate.ImageRegistry),
		fmt.Sprintf("%s/sld-php:8.3", orchestrate.ImageRegistry),
		fmt.Sprintf("%s/sld-php:8.4", orchestrate.ImageRegistry),
	}

	for _, img := range images {
		fmt.Printf("[SLD] Pulling %s...\n", img)
		if err := r.orch.Engine().PullImage(ctx, img); err != nil {
			fmt.Printf("  ⚠️  Failed to pull %s: %v\n", img, err)
		} else {
			fmt.Printf("  ✅ Pulled %s\n", img)
		}
	}

	// Restart router if running to apply latest image
	containers, err := r.orch.Engine().ListAllContainers(ctx)
	if err == nil {
		for _, c := range containers {
			if c.Name == orchestrate.RouterContainer && c.State == "running" {
				fmt.Println("[SLD] Restarting sld-router with updated image...")
				_ = r.orch.EnsureGlobalInfra(ctx)
				break
			}
		}
	}

	fmt.Println("[SLD] ✅ SLD V2 images updated successfully.")
	return nil
}

// MigrateV1 assists developers transitioning from host-based V1 to Docker V2.
func (r *Runner) MigrateV1(ctx context.Context, projectRoot string) error {
	fmt.Println("==================================================")
	fmt.Println("  SLD V1 → V2 Migration Assistant")
	fmt.Println("==================================================")
	fmt.Printf("Target project: %s\n\n", projectRoot)

	// 1. Generate .sld/config.yaml if not already present
	configPath := filepath.Join(projectRoot, ".sld", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("[SLD] Step 1: Auto-generating .sld/config.yaml...")
		if err := InitProject(projectRoot); err != nil {
			return fmt.Errorf("failed to initialize config: %w", err)
		}
		fmt.Println("  ✅ Generated .sld/config.yaml")
	} else {
		fmt.Println("[SLD] Step 1: .sld/config.yaml already exists. Skipping.")
	}

	// 2. Check for local MySQL database dump
	cfg, _, err := loadProjectConfig(projectRoot)
	if err == nil && cfg.Services.Database.Type != "" {
		fmt.Printf("\n[SLD] Step 2: Database configured (%s %s)\n", cfg.Services.Database.Type, cfg.Services.Database.Version)
		dbName := cfg.Name
		dumpDir := filepath.Join(projectRoot, ".sld", "migrations")
		_ = os.MkdirAll(dumpDir, 0755)
		dumpFile := filepath.Join(dumpDir, "v1_backup.sql")

		if _, err := exec.LookPath("mysqldump"); err == nil {
			fmt.Printf("[SLD] Attempting to export local MySQL database '%s'...\n", dbName)
			dumpCmd := exec.Command("mysqldump", "-u", "root", dbName)
			outFile, err := os.Create(dumpFile)
			if err == nil {
				dumpCmd.Stdout = outFile
				if err := dumpCmd.Run(); err == nil {
					fmt.Printf("  ✅ Exported local database to %s\n", dumpFile)
				} else {
					fmt.Printf("  ℹ️  Local database dump failed (database '%s' may not exist on host). Skipping.\n", dbName)
					_ = outFile.Close()
					_ = os.Remove(dumpFile)
				}
				_ = outFile.Close()
			}
		} else {
			fmt.Println("  ℹ️  'mysqldump' not in host PATH. Skipping database export.")
		}
	}

	fmt.Println("\n[SLD] Step 3: Migration preparation complete! 🎉")
	fmt.Println("To launch this project with isolated Docker containers, run:")
	fmt.Println("  sld v2 start")

	return nil
}

// --- Helpers ---

func loadProjectConfig(projectRoot string) (*config.Config, []byte, error) {
	configPath := projectRoot + "/.sld/config.yaml"
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf(
				"no .sld/config.yaml found at %s\n"+
					"Create one or run 'sld init-project' to auto-detect your framework", projectRoot)
		}
		return nil, nil, fmt.Errorf("failed to read config: %w", err)
	}

	defaultName := ""
	if idx := strings.LastIndex(projectRoot, "/"); idx >= 0 {
		defaultName = projectRoot[idx+1:]
	} else if idx := strings.LastIndex(projectRoot, "\\"); idx >= 0 {
		defaultName = projectRoot[idx+1:]
	}

	cfg, err := config.LoadFromBytes(data, defaultName)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid .sld/config.yaml: %w", err)
	}

	return cfg, data, nil
}

func printServicesSummary(cfg *config.Config) {
	services := []string{
		fmt.Sprintf("PHP %s", cfg.Runtime.PHP),
		"Nginx",
	}

	if cfg.Services.Database.Type != "" {
		dbLabel := cfg.Services.Database.Type
		if len(dbLabel) > 0 {
			dbLabel = strings.ToUpper(dbLabel[:1]) + dbLabel[1:]
		}
		services = append(services, fmt.Sprintf("%s %s", dbLabel, cfg.Services.Database.Version))
	}
	if cfg.Services.Redis.Enabled {
		services = append(services, "Redis")
	}
	if cfg.Services.Mailpit.Enabled {
		services = append(services, "Mailpit")
	}
	if cfg.Node.DevServer.Enabled {
		services = append(services, fmt.Sprintf("Node %s (dev server)", cfg.Node.Version))
	}

	fmt.Printf("[SLD] Services: %s\n", strings.Join(services, " • "))
}
