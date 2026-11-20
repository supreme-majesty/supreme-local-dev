package v2

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"gopkg.in/yaml.v3"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/dns"
	docker "github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
	"github.com/supreme-majesty/supreme-local-dev/internal/secret"
	"github.com/supreme-majesty/supreme-local-dev/internal/tls"
)

// Runner provides the high-level CLI command implementations for SLD V2.
type Runner struct {
	orch *orchestrate.Orchestrator
}

// NewRunner creates a Runner from a Docker engine client. It assumes SLD runs
// on the host; use SetEnv to switch to container-mode path translation.
func NewRunner(engine docker.EngineClient) *Runner {
	return &Runner{
		orch: orchestrate.NewOrchestrator(engine),
	}
}

// SetEnv sets the runtime environment (host or container) used by the runner.
func (r *Runner) SetEnv(env *hostenv.Env) {
	r.orch.SetEnv(env)
}

// env returns the runtime environment, defaulting to host mode.
func (r *Runner) env() *hostenv.Env {
	if r.orch == nil {
		return hostenv.Host("")
	}
	return r.orch.Env()
}

// identity computes the project identity for a project directory, using the
// host path when SLD runs in a container so IDs are stable across mounts.
func (r *Runner) identity(projectRoot string) (*project.Identity, error) {
	id, err := r.env().ProjectIdentity(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to compute project identity: %w", err)
	}
	return id, nil
}

// manageHostsFile reports whether SLD should edit the hosts file for domain:
// never from inside a container, and never for *.localhost names.
func (r *Runner) manageHostsFile(domain string) bool {
	return domain != "" && !r.env().InContainer() && !config.IsAutoResolvingDomain(domain)
}

// Init performs first-time global setup:
// 1. Validates Docker Engine connectivity
// 2. Checks host ports 80/443 if router is not already running
// 3. Creates the sld-global network and starts the Traefik router
// 4. Configures OS DNS resolver helper where applicable
func (r *Runner) Init(ctx context.Context) error {
	fmt.Println("[SLD] Initializing SLD V2...")

	// 1. Check Docker
	fmt.Println("[SLD] Checking Docker Engine connectivity...")
	if err := r.orch.Engine().Ping(ctx); err != nil {
		return fmt.Errorf("Docker Engine is not reachable. Please ensure Docker is installed and running.\n  → %w", err)
	}
	fmt.Println("[SLD] ✅ Docker Engine is running.")

	// 2. Check the router's host ports if sld-router isn't already running.
	// From inside a container the host's ports cannot be probed; Docker
	// reports a conflict when the router is created instead.
	env := r.env()
	if !env.InContainer() && !r.isRouterRunning(ctx) {
		for _, port := range []int{env.HTTPPort(), env.HTTPSPort()} {
			if !isLocalPortAvailable(port) {
				fmt.Printf("[SLD] ⚠️  Host port %d appears to be in use (set SLD_HTTP_PORT / SLD_HTTPS_PORT to use other ports).\n", port)
			}
		}
	}

	// 3. Setup global infra (network + certs + router)
	fmt.Println("[SLD] Creating global network and starting router...")
	if err := r.orch.EnsureGlobalInfra(ctx); err != nil {
		return fmt.Errorf("failed to initialize global infrastructure: %w", err)
	}
	fmt.Println("[SLD] ✅ Global router is running on sld-global network.")

	// 4. Platform DNS resolver setup — only needed for TLDs browsers do not
	// resolve on their own, and impossible from inside a container.
	tld := config.DefaultTLD()
	if r.manageHostsFile("sld." + tld) {
		if err := dns.EnsureResolver(tld); err != nil {
			fmt.Printf("[SLD] ⚠️  DNS resolver setup skipped: %v\n", err)
		}
	}

	r.printTrustHint()

	fmt.Println()
	fmt.Println("[SLD] Initialization complete. SLD is ready. 🚀")
	fmt.Println("[SLD] Run 'sld v2 start' inside a project directory to begin.")
	return nil
}

// Start reads the project's .sld/config.yaml, resolves identity, starts all
// containers via the orchestrator, and ensures local DNS/hosts resolution.
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

	fmt.Printf("[SLD] Project: %s | Apps: %s | Domain: %s\n", cfg.Name, cfg.Describe(), cfg.Domain.Name)

	// 3. Compute identity
	identity, err := r.identity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	// 4. Build spec (absolute root: it is translated into container mounts)
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to resolve project path: %w", err)
	}
	spec := &orchestrate.ProjectSpec{
		Identity:    identity,
		Config:      cfg,
		ConfigHash:  config.ComputeConfigHash(configData),
		ProjectRoot: absRoot,
	}

	// 5. Services summary
	printServicesSummary(cfg)

	// 6. Start project
	fmt.Println("[SLD] Starting containers...")
	r.orch.SetBuildLog(os.Stdout)
	if err := r.orch.StartProject(ctx, spec); err != nil {
		return fmt.Errorf("failed to start project: %w", err)
	}

	// 7. Ensure local DNS resolution for every served host name
	for _, host := range cfg.Hostnames() {
		r.ensureProjectDomainDNS(host)
	}

	elapsed := time.Since(startTime)
	fmt.Println()
	fmt.Printf("[SLD] ✅ Project is running! (%.1fs)\n", elapsed.Seconds())

	env := r.env()
	for _, app := range cfg.Apps {
		fmt.Printf("🚀 %-10s %s\n", app.Name, env.SiteURL(app.Domain, cfg.Domain.TLS))
	}
	if cfg.Services.Mailpit.Enabled {
		fmt.Printf("📬 %-10s %s\n", "mailpit", env.SiteURL("mail."+cfg.Domain.Name, cfg.Domain.TLS))
	}
	for _, app := range cfg.Apps {
		switch app.Runtime {
		case config.RuntimeNode, config.RuntimePython, config.RuntimeRuby, config.RuntimeGo:
			fmt.Printf("[SLD] ℹ️  %s may still be installing dependencies — follow with: sld v2 logs %s -f\n", app.Name, app.Name)
		}
	}

	return nil
}

func (r *Runner) ensureProjectDomainDNS(domain string) {
	if !r.manageHostsFile(domain) {
		return
	}
	added, err := dns.EnsureDomain(domain)
	if err != nil {
		fmt.Printf("[SLD] ⚠️  DNS notice: %v\n", err)
		return
	}
	if added {
		fmt.Printf("[SLD] ✅ Added hosts entry: 127.0.0.1 %s\n", domain)
	}
}

// Stop stops and removes all containers for the project at the given path (preserving volumes & network).
func (r *Runner) Stop(ctx context.Context, projectRoot string) error {
	fmt.Printf("[SLD] Stopping project at %s...\n", projectRoot)

	identity, err := r.identity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

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

	fmt.Println("[SLD] ✅ Project stopped (volumes preserved).")
	return nil
}

// Destroy tears down project containers, project network, and optionally persistent volumes.
func (r *Runner) Destroy(ctx context.Context, projectRoot string, removeVolumes, skipConfirm bool) error {
	identity, err := r.identity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	if removeVolumes && !skipConfirm {
		fmt.Printf("⚠️  This will permanently delete database and Redis volumes for %s (%s).\n", identity.Slug, identity.ID)
		fmt.Print("Type 'yes' to confirm: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(answer)) != "yes" {
			fmt.Println("[SLD] Destroy cancelled.")
			return nil
		}
	}

	fmt.Printf("[SLD] Destroying project %s (%s)...\n", identity.Slug, identity.ID)
	if err := r.orch.DestroyProject(ctx, identity, removeVolumes); err != nil {
		return fmt.Errorf("failed to destroy project: %w", err)
	}

	// Best-effort cleanup of SLD-managed hosts entry
	if cfg, _, err := loadProjectConfig(projectRoot); err == nil && r.manageHostsFile(cfg.Domain.Name) {
		_ = dns.RemoveHostsEntry(cfg.Domain.Name)
		if cfg.Services.Mailpit.Enabled {
			_ = dns.RemoveHostsEntry("mail." + cfg.Domain.Name)
		}
	}

	if removeVolumes {
		fmt.Println("[SLD] ✅ Project containers, network, and volumes destroyed.")
	} else {
		fmt.Println("[SLD] ✅ Project containers and network destroyed (volumes preserved; pass --volumes to purge data).")
	}
	return nil
}

// ProjectStatusReport is the JSON-serializable representation of a project's container state.
type ProjectStatusReport struct {
	ProjectID   string                       `json:"project_id"`
	Slug        string                       `json:"slug,omitempty"`
	ProjectPath string                       `json:"project_path,omitempty"`
	Containers  []*reconcile.ActualContainer `json:"containers"`
}

// Status shows the current state of project containers (single project or --all, text or --json).
func (r *Runner) Status(ctx context.Context, projectRoot string, allProjects, asJSON bool) error {
	if allProjects {
		return r.statusAll(ctx, asJSON)
	}

	identity, err := r.identity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	containers, err := r.orch.ProjectStatus(ctx, identity.ID)
	if err != nil {
		return fmt.Errorf("failed to get project status: %w", err)
	}

	if asJSON {
		report := ProjectStatusReport{
			ProjectID:   identity.ID,
			Slug:        identity.Slug,
			ProjectPath: identity.CanonicalPath,
			Containers:  containers,
		}
		return printJSON(report)
	}

	if len(containers) == 0 {
		fmt.Println("No containers running for this project.")
		fmt.Println("Run 'sld v2 start' to launch the development environment.")
		return nil
	}

	fmt.Printf("Project: %s\n", identity.Slug)
	fmt.Printf("ID:      %s\n", identity.ID)
	fmt.Println()
	printContainerTable(containers)
	return nil
}

func (r *Runner) statusAll(ctx context.Context, asJSON bool) error {
	containers, err := r.orch.Engine().ListAllContainers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list SLD containers: %w", err)
	}

	grouped := make(map[string]*ProjectStatusReport)
	var globalContainers []*reconcile.ActualContainer

	for _, c := range containers {
		projID := c.Labels[docker.LabelProject]
		if projID == "" {
			globalContainers = append(globalContainers, c)
			continue
		}
		rep, ok := grouped[projID]
		if !ok {
			rep = &ProjectStatusReport{
				ProjectID:   projID,
				ProjectPath: c.Labels[docker.LabelProjectPath],
			}
			grouped[projID] = rep
		}
		rep.Containers = append(rep.Containers, c)
	}

	projectIDs := make([]string, 0, len(grouped))
	for id := range grouped {
		projectIDs = append(projectIDs, id)
	}
	sort.Strings(projectIDs)

	if asJSON {
		projects := make([]*ProjectStatusReport, 0, len(projectIDs))
		for _, id := range projectIDs {
			projects = append(projects, grouped[id])
		}
		return printJSON(map[string]any{
			"global":   globalContainers,
			"projects": projects,
		})
	}

	if len(globalContainers) == 0 && len(projectIDs) == 0 {
		fmt.Println("No SLD V2 containers found.")
		return nil
	}

	if len(globalContainers) > 0 {
		fmt.Println("Global Infrastructure")
		printContainerTable(globalContainers)
		fmt.Println()
	}

	for _, id := range projectIDs {
		rep := grouped[id]
		fmt.Printf("Project: %s", rep.ProjectID)
		if rep.ProjectPath != "" {
			fmt.Printf(" (%s)", rep.ProjectPath)
		}
		fmt.Println()
		printContainerTable(rep.Containers)
		fmt.Println()
	}
	return nil
}

// Inspect outputs detailed JSON metadata for the current project and its Docker containers.
func (r *Runner) Inspect(ctx context.Context, projectRoot string) error {
	identity, err := r.identity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	cfg, _, cfgErr := loadProjectConfig(projectRoot)
	containers, err := r.orch.ProjectStatus(ctx, identity.ID)
	if err != nil {
		return fmt.Errorf("failed to list project containers: %w", err)
	}

	type containerDetail struct {
		ID       string            `json:"id"`
		Name     string            `json:"name"`
		Service  string            `json:"service"`
		Image    string            `json:"image"`
		State    string            `json:"state"`
		Health   string            `json:"health,omitempty"`
		Networks []string          `json:"networks,omitempty"`
		Labels   map[string]string `json:"labels,omitempty"`
	}

	details := make([]containerDetail, 0, len(containers))
	for _, c := range containers {
		d := containerDetail{
			ID:      c.ID,
			Name:    c.Name,
			Service: c.Service,
			Image:   c.Image,
			State:   c.State,
			Labels:  c.Labels,
		}
		if info, err := r.orch.Engine().InspectContainer(ctx, c.ID); err == nil {
			if info.ContainerJSONBase != nil && info.State != nil && info.State.Health != nil {
				d.Health = info.State.Health.Status
			}
			if info.NetworkSettings != nil {
				for netName := range info.NetworkSettings.Networks {
					d.Networks = append(d.Networks, netName)
				}
				sort.Strings(d.Networks)
			}
		}
		details = append(details, d)
	}

	payload := map[string]any{
		"identity":   identity,
		"network":    identity.NetworkName(),
		"containers": details,
	}
	if cfgErr == nil {
		payload["config"] = cfg
	}

	return printJSON(payload)
}

// Secure ensures SLD's local CA and the router's default certificate exist,
// recreates the router, and explains how to trust the CA.
func (r *Runner) Secure(ctx context.Context) error {
	env := r.env()
	certMgr, err := tls.NewManager(env.DataDir())
	if err != nil {
		return fmt.Errorf("failed to initialize certificate manager: %w", err)
	}
	if !env.InContainer() {
		if imported, err := certMgr.ImportMkcertCA(); err != nil {
			return err
		} else if imported {
			fmt.Println("[SLD] Using mkcert's local CA for SLD certificates.")
		}
	}
	if err := certMgr.EnsureDefault(); err != nil {
		return fmt.Errorf("failed to prepare certificates: %w", err)
	}
	fmt.Printf("[SLD] ✅ Local CA ready: %s\n", certMgr.CACertPath())

	if r.orch != nil && r.orch.Engine() != nil {
		if err := r.orch.Engine().Ping(ctx); err == nil {
			fmt.Println("[SLD] Reloading sld-router...")
			if err := r.orch.RecreateRouter(ctx); err != nil {
				return fmt.Errorf("failed to reload router: %w", err)
			}
			fmt.Println("[SLD] ✅ Router updated.")
		}
	}

	r.printTrustHint()
	fmt.Println("[SLD] Set 'domain.tls: true' in .sld/config.yaml and run 'sld v2 start' to serve HTTPS.")
	return nil
}

// printTrustHint tells the user how to trust SLD's CA in their browsers.
func (r *Runner) printTrustHint() {
	env := r.env()
	certMgr, err := tls.NewManager(env.DataDir())
	if err != nil || !certMgr.HasCA() {
		return
	}
	caPath := certMgr.CACertPath()
	fmt.Println()
	fmt.Println("[SLD] HTTPS: trust SLD's local CA once so browsers accept project certificates.")
	if env.InContainer() {
		name := env.ContainerID()
		if len(name) > 12 {
			name = name[:12]
		}
		if name == "" {
			name = "<sld-container>"
		}
		fmt.Printf("  1. Copy it out:  docker cp %s:%s sld-ca.crt\n", name, caPath)
		caPath = "sld-ca.crt"
	} else {
		fmt.Printf("  CA certificate:  %s\n", caPath)
	}
	fmt.Printf("  • macOS:   sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s\n", caPath)
	fmt.Printf("  • Windows: certutil -addstore -f ROOT %s   (admin PowerShell)\n", caPath)
	fmt.Printf("  • Linux:   sudo cp %s /usr/local/share/ca-certificates/sld-ca.crt && sudo update-ca-certificates\n", caPath)
	fmt.Println("  • Firefox: Settings → Certificates → Import (Firefox keeps its own trust store)")
	fmt.Println("  Plain http:// works without this step.")
}

// ConfigValidate parses and validates .sld/config.yaml for the project.
func ConfigValidate(projectRoot string) error {
	cfg, _, err := loadProjectConfig(projectRoot)
	if err != nil {
		return err
	}
	fmt.Printf("[SLD] ✅ Configuration is valid (%s — %s, domain %s)\n", cfg.Name, cfg.Describe(), cfg.Domain.Name)
	if cfg.SourceVersion == 1 {
		fmt.Println("[SLD] ℹ️  This is a version 1 file; run 'sld v2 config migrate' to rewrite it as version 2.")
	}
	if err := orchestrate.CheckEngineSupport(cfg); err != nil {
		fmt.Printf("[SLD] ⚠️  Valid, but cannot be started yet: %v\n", err)
	}
	return nil
}

// ConfigMigrate rewrites a version 1 .sld/config.yaml as version 2, keeping
// the original as .sld/config.v1.yaml.bak. Container names, images and volumes
// do not change; containers are recreated once because the file changed.
func ConfigMigrate(projectRoot string) error {
	sldDir := filepath.Join(projectRoot, ".sld")
	configPath := filepath.Join(sldDir, "config.yaml")
	backupPath := filepath.Join(sldDir, "config.v1.yaml.bak")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}
	// Refuse to migrate a file that does not load: the user should see the real error first.
	if _, err := config.LoadFromBytes(data, filepath.Base(projectRoot)); err != nil {
		return fmt.Errorf("invalid .sld/config.yaml: %w", err)
	}

	migrated, err := config.MigrateV1(data)
	if errors.Is(err, config.ErrAlreadyCurrent) {
		fmt.Println("[SLD] ✅ .sld/config.yaml is already version 2.")
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := config.LoadFromBytes(migrated, filepath.Base(projectRoot)); err != nil {
		return fmt.Errorf("migrated configuration does not validate (please report this): %w", err)
	}

	if _, err := os.Stat(backupPath); err == nil {
		return fmt.Errorf("backup %s already exists; move it away and run migrate again", backupPath)
	}
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write backup: %w", err)
	}
	if err := os.WriteFile(configPath, migrated, 0o644); err != nil {
		return fmt.Errorf("failed to write migrated config (original kept at %s): %w", backupPath, err)
	}

	fmt.Printf("[SLD] ✅ Migrated .sld/config.yaml to version 2 (original saved as %s)\n", backupPath)
	fmt.Println("[SLD] Containers will be recreated once on the next 'sld v2 start'; data volumes are kept.")
	return nil
}

// ConfigShow prints the resolved .sld/config.yaml with all defaults applied.
func ConfigShow(projectRoot string) error {
	cfg, _, err := loadProjectConfig(projectRoot)
	if err != nil {
		return err
	}
	var out strings.Builder
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("failed to serialize resolved config: %w", err)
	}
	_ = enc.Close()
	fmt.Print(out.String())
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
		if runtime.GOOS == "linux" && strings.Contains(strings.ToLower(err.Error()), "permission denied") {
			fmt.Println("  → Fix: sudo usermod -aG docker $USER && newgrp docker")
		}
	} else {
		fmt.Println("✅ Running")
	}

	// 2. Global router & network
	routerRunning := false
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
					routerRunning = true
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

	// 3. Router host ports
	httpPort, httpsPort := r.env().HTTPPort(), r.env().HTTPSPort()
	label := fmt.Sprintf("Host Ports %d/%d:", httpPort, httpsPort)
	fmt.Printf("%-21s", label+" ")
	switch {
	case routerRunning:
		fmt.Println("✅ Bound by sld-router")
	case r.env().InContainer():
		fmt.Println("ℹ️  Cannot probe host ports from a container; Docker reports conflicts on 'sld v2 init'")
	default:
		var busy []string
		for _, p := range []int{httpPort, httpsPort} {
			if !isLocalPortAvailable(p) {
				busy = append(busy, strconv.Itoa(p))
			}
		}
		if len(busy) == 0 {
			fmt.Println("✅ Available")
		} else {
			fmt.Printf("⚠️  Port(s) %s in use by another process (stop it, or set SLD_HTTP_PORT / SLD_HTTPS_PORT)\n", strings.Join(busy, ", "))
		}
	}

	// 4. Local CA and certificates
	fmt.Print("TLS (local CA):      ")
	env := r.env()
	certMgr, err := tls.NewManager(env.DataDir())
	if err != nil {
		fmt.Printf("❌ %v\n", err)
	} else if certMgr.HasCA() {
		fmt.Printf("✅ Ready (%s) — trust it once for HTTPS\n", certMgr.CACertPath())
	} else {
		fmt.Println("⚠️  Not generated yet (run 'sld v2 init' or 'sld v2 secure')")
	}

	// 5. DNS Resolution
	tld := config.DefaultTLD()
	fmt.Printf("DNS (*.%s):%s", tld, strings.Repeat(" ", max(1, 13-len(tld))))
	switch {
	case config.IsAutoResolvingDomain("sld." + tld):
		fmt.Println("✅ Resolved by browsers automatically (no hosts file needed)")
	case env.InContainer():
		fmt.Printf("ℹ️  Add host entries for *.%s on your machine (SLD cannot edit the host's hosts file from a container)\n", tld)
	default:
		resolved, _ := dns.CheckResolution("sld." + tld)
		hasHosts, _ := dns.HasHostsEntry("sld." + tld)
		if resolved || hasHosts {
			fmt.Println("✅ Resolving to loopback")
		} else {
			fmt.Printf("⚠️  *.%s not resolving to loopback (hosts file: %s)\n", tld, dns.HostsFilePath())
		}
	}

	// 6. Runtime environment and mounts
	fmt.Print("SLD Runtime:         ")
	if env.InContainer() {
		fmt.Printf("🐳 Container (data: %s, ports bind %s)\n", env.DataDir(), env.BindHost())
		for _, m := range env.Mounts() {
			switch m.Type {
			case "bind":
				fmt.Printf("  • %-22s ← host %s\n", m.Destination, m.Source)
			case "volume":
				fmt.Printf("  • %-22s ← volume %s\n", m.Destination, m.Name)
			}
		}
		if _, err := env.MountFor(env.DataDir(), "/check", true); err != nil {
			fmt.Printf("  ❌ Data directory %s is not on a volume; the router cannot read certificates.\n", env.DataDir())
		}
	} else {
		fmt.Printf("Host (data: %s, ports bind %s)\n", env.DataDir(), env.BindHost())
	}

	// 7. WSL2 environment note
	if dns.IsWSL2() && !env.InContainer() {
		fmt.Println("WSL2 Environment:    ℹ️  Detected WSL2 — ensure Windows host ports 80/443 forward to WSL2 (mirrored networking recommended)")
	}

	return nil
}

// Logs streams container logs for a project service.
func (r *Runner) Logs(ctx context.Context, projectRoot, service string, follow bool, tail string) error {
	identity, err := r.identity(projectRoot)
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

	// Accept an app name or a service; include stopped containers so the logs
	// of a crashed dev server can still be read.
	cfg, _, _ := loadProjectConfig(projectRoot)
	targetContainer, err := pickContainer(containers, cfg, service, false)
	if err != nil {
		return err
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

	// SLD containers run without a TTY, so the stream is multiplexed stdout/stderr.
	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, reader)
	return err
}

// Open opens the project domain in the host default browser.
func (r *Runner) Open(ctx context.Context, projectRoot string) error {
	cfg, _, err := loadProjectConfig(projectRoot)
	if err != nil {
		return err
	}

	url := r.env().SiteURL(cfg.Domain.Name, cfg.Domain.TLS)
	if r.env().InContainer() {
		// No host browser is reachable from inside the SLD container.
		fmt.Printf("[SLD] Open %s in your browser.\n", url)
		return nil
	}
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

// Update pulls the latest SLD V2 Docker images from GHCR and recreates the global router if running.
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

	if r.isRouterRunning(ctx) {
		fmt.Println("[SLD] Recreating sld-router with updated image...")
		if err := r.orch.RecreateRouter(ctx); err != nil {
			fmt.Printf("  ⚠️  Failed to recreate router: %v\n", err)
		}
	}

	fmt.Println("[SLD] ✅ SLD V2 images updated successfully.")
	return nil
}

// MigrateV1 assists developers transitioning from host-based V1 to Docker V2 (alias for LegacyMigrate --execute).
func (r *Runner) MigrateV1(ctx context.Context, projectRoot string) error {
	return r.LegacyMigrate(ctx, projectRoot, true)
}

// LegacyStatus inspects the host for V1 installations, systemd services, and state files.
func (r *Runner) LegacyStatus(ctx context.Context) error {
	fmt.Println("SLD V1 Legacy Environment Inspection")
	fmt.Println("────────────────────────────────────")

	statePath := "/var/lib/sld/state.json"
	if data, err := os.ReadFile(statePath); err == nil {
		fmt.Printf("V1 State File:       ✅ Found (%s)\n", statePath)
		var state struct {
			Paths []string          `json:"paths"`
			Links map[string]string `json:"links"`
			TLD   string            `json:"tld"`
		}
		if json.Unmarshal(data, &state) == nil {
			fmt.Printf("  Parked paths:      %d\n", len(state.Paths))
			fmt.Printf("  Linked sites:      %d\n", len(state.Links))
			if state.TLD != "" {
				fmt.Printf("  TLD:               .%s\n", state.TLD)
			}
		}
	} else {
		fmt.Println("V1 State File:       ⚪ Not found (/var/lib/sld/state.json)")
	}

	unitPath := "/etc/systemd/system/sld-daemon.service"
	if _, err := os.Stat(unitPath); err == nil {
		fmt.Printf("V1 Systemd Service:  ⚠️  Present (%s)\n", unitPath)
	} else {
		fmt.Println("V1 Systemd Service:  ⚪ Not installed")
	}

	for _, bin := range []string{"nginx", "php-fpm", "mysql", "mysqldump", "dnsmasq"} {
		if path, err := exec.LookPath(bin); err == nil {
			fmt.Printf("Host %-15s ℹ️  Installed (%s)\n", bin+":", path)
		}
	}

	return nil
}

// LegacyMigrate migrates a V1 host project to V2 Docker (defaults to dry-run unless execute is true).
func (r *Runner) LegacyMigrate(ctx context.Context, projectRoot string, execute bool) error {
	fmt.Println("==================================================")
	fmt.Println("  SLD V1 → V2 Migration Assistant")
	fmt.Println("==================================================")
	fmt.Printf("Target project: %s\n", projectRoot)
	if !execute {
		fmt.Println("Mode:           DRY-RUN (pass --execute to apply changes)")
	} else {
		fmt.Println("Mode:           EXECUTE")
	}
	fmt.Println()

	configPath := filepath.Join(projectRoot, ".sld", "config.yaml")
	_, cfgStatErr := os.Stat(configPath)
	hasConfig := cfgStatErr == nil

	if !execute {
		if hasConfig {
			fmt.Printf("  [plan] Keep existing %s\n", configPath)
		} else {
			fmt.Printf("  [plan] Auto-detect framework and create %s\n", configPath)
		}
		if _, err := exec.LookPath("mysqldump"); err == nil {
			fmt.Println("  [plan] Export host MySQL database backup into ~/.sld/backup/ and .sld/migrations/v1_backup.sql")
			fmt.Println("  [plan] Start V2 containers and import SQL dump into isolated Docker database volume")
		} else {
			fmt.Println("  [plan] 'mysqldump' not found on host PATH — skip host DB export")
		}
		fmt.Println("\nRun with --execute to perform this migration.")
		return nil
	}

	// 1. Generate .sld/config.yaml if not already present
	if !hasConfig {
		fmt.Println("[SLD] Step 1: Auto-generating .sld/config.yaml...")
		if err := InitProject(projectRoot); err != nil {
			return fmt.Errorf("failed to initialize config: %w", err)
		}
		fmt.Println("  ✅ Generated .sld/config.yaml")
	} else {
		fmt.Println("[SLD] Step 1: .sld/config.yaml already exists. Skipping.")
	}

	// 2. Export host MySQL database if configured and mysqldump is available
	var dumpFile string
	cfg, _, err := loadProjectConfig(projectRoot)
	if err == nil && cfg.Services.Database.Type != "" {
		fmt.Printf("\n[SLD] Step 2: Database configured (%s %s)\n", cfg.Services.Database.Type, cfg.Services.Database.Version)
		dbName := cfg.Name
		dumpDir := filepath.Join(projectRoot, ".sld", "migrations")
		_ = os.MkdirAll(dumpDir, 0755)
		candidateDump := filepath.Join(dumpDir, "v1_backup.sql")

		if _, err := exec.LookPath("mysqldump"); err == nil {
			fmt.Printf("[SLD] Attempting to export local MySQL database '%s'...\n", dbName)
			dumpCmd := exec.Command("mysqldump", "-h", "127.0.0.1", "-u", "root", dbName)
			outFile, err := os.Create(candidateDump)
			if err == nil {
				dumpCmd.Stdout = outFile
				runErr := dumpCmd.Run()
				_ = outFile.Close()
				if runErr == nil {
					dumpFile = candidateDump
					fmt.Printf("  ✅ Exported local database to %s\n", dumpFile)
					// Also copy backup to ~/.sld/backup/
					if home, hErr := os.UserHomeDir(); hErr == nil {
						backupDir := filepath.Join(home, ".sld", "backup")
						if os.MkdirAll(backupDir, 0700) == nil {
							if data, rErr := os.ReadFile(dumpFile); rErr == nil {
								backupPath := filepath.Join(backupDir, fmt.Sprintf("%s-v1.sql", dbName))
								_ = os.WriteFile(backupPath, data, 0600)
							}
						}
					}
				} else {
					fmt.Printf("  ℹ️  Local database dump skipped (database '%s' not accessible on host).\n", dbName)
					_ = os.Remove(candidateDump)
				}
			}
		} else {
			fmt.Println("  ℹ️  'mysqldump' not in host PATH. Skipping database export.")
		}
	}

	// 3. If we have a SQL dump and Docker is reachable, offer/perform import into V2 DB container
	if dumpFile != "" && r.orch != nil && r.orch.Engine() != nil && r.orch.Engine().Ping(ctx) == nil {
		fmt.Println("\n[SLD] Step 3: Starting V2 containers and importing SQL backup...")
		if err := r.Start(ctx, projectRoot); err != nil {
			fmt.Printf("  ⚠️  Could not auto-start V2 stack for import (%v). Import manually after 'sld v2 start'.\n", err)
		} else if identity, idErr := r.identity(projectRoot); idErr == nil {
			if err := r.importSQLDump(ctx, projectRoot, identity, cfg, dumpFile); err != nil {
				fmt.Printf("  ⚠️  Automatic SQL import failed: %v\n", err)
			} else {
				fmt.Println("  ✅ Imported V1 database into isolated Docker volume.")
			}
		}
	}

	fmt.Println("\n[SLD] Migration complete! 🎉")
	fmt.Println("To manage this project, run: sld v2 start | status | logs")
	return nil
}

// importSQLDump streams a SQL dump into the project's database container via
// the Docker API (no host docker CLI required).
func (r *Runner) importSQLDump(ctx context.Context, projectRoot string, identity *project.Identity, cfg *config.Config, dumpFile string) error {
	executor, ok := r.orch.Engine().(docker.Executor)
	if !ok {
		return fmt.Errorf("docker engine does not support exec")
	}

	f, err := os.Open(dumpFile)
	if err != nil {
		return err
	}
	defer f.Close()

	dbPass, err := secret.GetOrGenerate(projectRoot, "db_password", 16)
	if err != nil {
		return err
	}

	opts := docker.ExecOptions{Stdin: f, Stdout: os.Stdout, Stderr: os.Stderr}
	switch strings.ToLower(cfg.Services.Database.Type) {
	case "postgres", "postgresql":
		opts.Cmd = []string{"psql", "-U", cfg.Name, cfg.Name}
		opts.Env = []string{"PGPASSWORD=" + dbPass}
	default:
		// MYSQL_PWD keeps the password out of the process list.
		opts.Cmd = []string{"mysql", "-u", "root", cfg.Name}
		opts.Env = []string{"MYSQL_PWD=" + dbPass}
	}

	code, err := executor.Exec(ctx, identity.DBContainerName(), opts)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("database import exited with code %d", code)
	}
	return nil
}

// LegacyRemove archives V1 state and prints/executes cleanup steps for the V1 daemon.
func (r *Runner) LegacyRemove(ctx context.Context, skipConfirm bool) error {
	if !skipConfirm {
		fmt.Println("⚠️  This will archive V1 state (/var/lib/sld/state.json) and stop sld-daemon.service if installed.")
		fmt.Print("Type 'yes' to confirm: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(answer)) != "yes" {
			fmt.Println("[SLD] Legacy removal cancelled.")
			return nil
		}
	}

	// Archive state.json if present
	statePath := "/var/lib/sld/state.json"
	if data, err := os.ReadFile(statePath); err == nil {
		home, hErr := os.UserHomeDir()
		if hErr == nil {
			backupDir := filepath.Join(home, ".sld", "backup")
			if err := os.MkdirAll(backupDir, 0700); err == nil {
				backupPath := filepath.Join(backupDir, "state.v1.json")
				if err := os.WriteFile(backupPath, data, 0600); err == nil {
					fmt.Printf("[SLD] ✅ Archived V1 state to %s\n", backupPath)
				}
			}
		}
	} else {
		fmt.Println("[SLD] ℹ️  No /var/lib/sld/state.json found.")
	}

	unitPath := "/etc/systemd/system/sld-daemon.service"
	if _, err := os.Stat(unitPath); err == nil {
		if os.Geteuid() == 0 {
			_ = exec.Command("systemctl", "disable", "--now", "sld-daemon.service").Run()
			_ = os.Remove(unitPath)
			_ = exec.Command("systemctl", "daemon-reload").Run()
			fmt.Println("[SLD] ✅ Stopped and removed sld-daemon.service.")
		} else {
			fmt.Println("[SLD] To stop and remove the V1 systemd daemon, run:")
			fmt.Println("  sudo systemctl disable --now sld-daemon.service")
			fmt.Println("  sudo rm -f /etc/systemd/system/sld-daemon.service && sudo systemctl daemon-reload")
		}
	} else {
		fmt.Println("[SLD] ℹ️  No sld-daemon.service unit found.")
	}

	fmt.Println("\n[SLD] Note: SLD V2 never invokes OS package managers automatically.")
	fmt.Println("If you wish to remove host V1 packages manually (Ubuntu/Debian):")
	fmt.Println("  sudo apt remove nginx php-fpm dnsmasq")
	return nil
}

// --- Helpers ---

func (r *Runner) isRouterRunning(ctx context.Context) bool {
	containers, err := r.orch.Engine().ListAllContainers(ctx)
	if err != nil {
		return false
	}
	for _, c := range containers {
		if c.Name == orchestrate.RouterContainer && c.State == "running" {
			return true
		}
	}
	return false
}

func isLocalPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func printContainerTable(containers []*reconcile.ActualContainer) {
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
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func loadProjectConfig(projectRoot string) (*config.Config, []byte, error) {
	configPath := filepath.Join(projectRoot, ".sld", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf(
				"no .sld/config.yaml found at %s\n"+
					"Create one or run 'sld v2 init-project' to auto-detect your framework", projectRoot)
		}
		return nil, nil, fmt.Errorf("failed to read config: %w", err)
	}

	defaultName := filepath.Base(projectRoot)
	cfg, err := config.LoadFromBytes(data, defaultName)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid .sld/config.yaml: %w", err)
	}

	return cfg, data, nil
}

func printServicesSummary(cfg *config.Config) {
	var services []string
	for _, app := range cfg.Apps {
		switch app.Runtime {
		case config.RuntimePHP:
			services = append(services, fmt.Sprintf("PHP %s + Nginx (%s)", app.Version, app.Name))
		default:
			label := app.Runtime
			if app.Version != "" {
				label += " " + app.Version
			}
			services = append(services, fmt.Sprintf("%s (%s)", label, app.Name))
		}
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

	fmt.Printf("[SLD] Services: %s\n", strings.Join(services, " • "))
}
