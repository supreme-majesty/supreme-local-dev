package daemon

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"runtime"

	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters"
	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters/linux"
	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters/macos"
	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters/windows"
	"github.com/supreme-majesty/supreme-local-dev/pkg/assets"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon/state"
	"github.com/supreme-majesty/supreme-local-dev/pkg/events"
	"github.com/supreme-majesty/supreme-local-dev/pkg/plugins"
	"github.com/supreme-majesty/supreme-local-dev/pkg/project"
	"github.com/supreme-majesty/supreme-local-dev/pkg/services"
)

type Daemon struct {
	State           *state.Manager
	Events          *events.Bus
	Adapter         adapters.SystemAdapter
	PluginManager   *plugins.Manager
	TunnelManager   *services.TunnelManager
	XRayService     *services.XRayService
	DatabaseService *services.DatabaseService
	ProjectManager  *services.ProjectManager
	LogWatcher      *services.LogWatcher
	EnvManager      *services.EnvManager
	ArtisanService  *services.ArtisanService
	HealerService   *services.HealerService
}

var instance *Daemon

// Initialize sets up the global daemon instance
func Initialize() (*Daemon, error) {
	if instance != nil {
		return instance, nil
	}

	// 1. Load State
	stateManager, err := state.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}
	if err := stateManager.Load(); err != nil {
		// Log warning but continue if just empty
		log.Printf("Warning loading state: %v", err)
	}

	// 2. Initialize Event Bus
	eventBus := events.NewBus()

	// 3. Initialize Plugin Manager
	// We use /var/lib/sld/plugins for shared plugin data/binaries
	pluginManager := plugins.NewManager("/var/lib/sld/plugins", stateManager)
	tunnelManager := services.NewTunnelManager("/var/lib/sld")
	xrayService := services.NewXRayService(eventBus)
	// LogWatcher moved down to depend on adapter
	databaseService := services.NewDatabaseService()
	home := getRealUserHome()
	baseDir := findBestDevDir(home)
	projectManager := services.NewProjectManager(baseDir)

	// Start X-Ray immediately
	go xrayService.Start()

	// Register default plugins
	pluginManager.Register(services.NewRedisPlugin(pluginManager.DataDir))
	pluginManager.Register(services.NewMailHogPlugin(pluginManager.DataDir))
	pluginManager.Register(services.NewPostgresPlugin(pluginManager.DataDir))

	// Auto-start enabled plugins from persisted state
	pluginManager.StartEnabled()

	// 4. Detect OS and select Adapter
	var adapter adapters.SystemAdapter
	switch runtime.GOOS {
	case "linux":
		adapter = linux.NewLinuxAdapter()
	case "darwin":
		adapter = macos.NewMacOSAdapter()
	case "windows":
		adapter = windows.NewWindowsAdapter()
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	logWatcher := services.NewLogWatcher(eventBus, adapter.GetLogPaths)

	instance = &Daemon{
		State:           stateManager,
		Events:          eventBus,
		Adapter:         adapter,
		PluginManager:   pluginManager,
		TunnelManager:   tunnelManager,
		XRayService:     xrayService,
		DatabaseService: databaseService,
		ProjectManager:  projectManager,
		LogWatcher:      logWatcher,
		EnvManager:      services.NewEnvManager(),
		ArtisanService:  services.NewArtisanService(eventBus),
		HealerService:   services.NewHealerService(eventBus),
	}

	// Start Healer
	instance.HealerService.Start()

	return instance, nil
}

// GetClient returns the running daemon instance.
func GetClient() (*Daemon, error) {
	if instance == nil {
		return Initialize()
	}
	return instance, nil
}

// EnsureInstalled checks if dependencies are met.
func (d *Daemon) EnsureInstalled() error {
	fmt.Println("Installing system packages...")
	if err := d.Adapter.InstallDependencies(); err != nil {
		return err
	}

	if err := d.Adapter.InstallBinary(); err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}

	// Extract to /var/lib/sld (globally accessible for Nginx)
	sldBase := "/var/lib/sld"
	fmt.Printf("Extracting runtime assets to %s...\n", sldBase)
	if err := assets.Extract(sldBase); err != nil {
		return fmt.Errorf("failed to extract assets: %w", err)
	}

	// Create config.inc.php pointing to user state
	realHome := getRealUserHome()
	userState := filepath.Join(realHome, ".sld", "state.json")

	// Create .sld directory for user if not exists and fix permissions
	userSld := filepath.Join(realHome, ".sld")
	if _, err := os.Stat(userSld); os.IsNotExist(err) {
		// We are sudo, so we must be careful with ownership
		// For now, let's create it with 755 permissions.
		os.MkdirAll(userSld, 0755)
		// We should chown it to real user, but Go `os.Chown` requires Uid/Gid lookup.
		// exec "chown" is easier.
		// sudoUser := os.Getenv("SUDO_USER")
		// exec.Command("chown", "-R", sudoUser+":"+sudoUser, userSld).Run()
	}
	// Make sure it is world readable (or at least Nginx readable)
	exec.Command("chmod", "755", userSld).Run()
	// And state.json if exists
	exec.Command("chmod", "644", userState).Run()

	// 3a. Fix Permissions for Web Server (Add www-data to user group)
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser != "" {
		fmt.Printf("Adding web user to group %s...\n", sudoUser)
		if err := d.Adapter.AddWebUserToGroup(sudoUser); err != nil {
			fmt.Printf("Warning: Failed to add web user to group: %v\n", err)
		}

		// Ensure home dir allows group traversal
		// chmod g+x /home/user
		exec.Command("chmod", "g+x", realHome).Run()

		// Restart PHP to pick up group changes
		d.Adapter.RestartPHP()
	}

	// 4. Global State Setup for Multi-User Support
	globalState := "/var/lib/sld/state.json"

	// Create state if not exists
	if _, err := os.Stat(globalState); os.IsNotExist(err) {
		emptyState := `{"services":{},"certificates":[],"php_version":"","secure":false,"tld":"test","paths":[],"links":{}}`
		os.WriteFile(globalState, []byte(emptyState), 0666)
	}

	// Ensure state is world writable so any user can park paths
	exec.Command("chmod", "666", globalState).Run()
	// Ensure directory is world writable/executable
	exec.Command("chmod", "777", "/var/lib/sld").Run()

	configFile := filepath.Join(sldBase, "runtime", "config.inc.php")
	phpConfig := fmt.Sprintf("<?php $sld_state_path = '%s'; ?>", globalState)
	os.WriteFile(configFile, []byte(phpConfig), 0644)

	// Set PHP Version in State if detection succeeds
	if v := d.Adapter.GetPHPVersion(); v != "" && d.State.Data.PHPVersion == "" {
		fmt.Printf("Detected PHP %s. Setting as default.\n", v)
		d.State.Data.PHPVersion = v
		d.State.Save()
	}

	fmt.Println("Configuring Nginx...")

	// Install any missing PHP versions required by projects
	d.ensureProjectPHPVersions()
	// Install any missing Node versions required by projects
	d.ensureProjectNodeVersions()

	// Remove default nginx site to avoid conflicts
	defSite := "/etc/nginx/sites-enabled/default"
	if _, err := os.Stat(defSite); err == nil {
		fmt.Println("Removing default Nginx site...")
		exec.Command("rm", defSite).Run()
	}

	// Use helper to write config
	if err := d.refreshNginxConfig(); err != nil {
		return fmt.Errorf("failed to configure nginx: %w", err)
	}

	// Create TLD in State if not exists (default test)
	// This saves to the loaded state path (user's home, if Daemon loaded correctly).
	// Since we run as sudo, `d.State` might be pointing to /root/.sld/state.json if initialized naïvely.
	// But `daemon.Initialize` calls `state.NewManager` which uses `os.UserHomeDir`.
	// If running as sudo, `os.UserHomeDir` is /root.
	// So `d.State` is modifying ROOT's state.
	// But `router.php` is configured to read REAL USER's state.
	// We need to `Switch` the state manager to the real user's path?
	// Or just copy the TLD init logic.
	// Actually, `sld install` is mostly for SYSTEM setup.
	// The USER will run `sld park` later (as user).
	// `sld park` (as user) will initialize `~/.sld/state.json`.
	// So we don't strictly need to populate `~/.sld/state.json` here.
	// BUT `router.php` will fail if file doesn't exist.
	// So we should initialize an empty state for the user.

	if _, err := os.Stat(userState); os.IsNotExist(err) {
		emptyState := `{"services":{},"certificates":[],"php_version":"","secure":false,"tld":"test","paths":[],"links":{}}`
		os.WriteFile(userState, []byte(emptyState), 0644)
		// Fix ownership
		sudoUser := os.Getenv("SUDO_USER")
		if sudoUser != "" {
			exec.Command("chown", sudoUser, userState).Run()
		}
	}

	// Sync hosts initially
	if err := instance.syncHosts(); err != nil {
		fmt.Printf("Warning: Failed to initial sync hosts: %v\n", err)
	}

	return nil
}

func (d *Daemon) syncHosts() error {
	// Reverted: User requested to not hardcode projects in /etc/hosts
	return nil
}

func replaceSocket(config, newSocket string) string {
	// Our templates use this default socket path
	defaultSocket := "unix:/run/php/php-fpm.sock"
	// newSocket usually is "/run/php/php8.1-fpm.sock"
	target := "unix:" + newSocket
	return strings.ReplaceAll(config, defaultSocket, target)
}

func (d *Daemon) replacePaths(config string) string {
	// Global runtime path
	runtimePath := "/var/lib/sld/runtime"
	config = strings.ReplaceAll(config, "{{SLD_RUNTIME_PATH}}", runtimePath)

	// Certs path: /var/lib/.sld/certs
	// Template has {{HOME}}/.sld/certs
	// We map {{HOME}}/.sld -> /var/lib/.sld
	config = strings.ReplaceAll(config, "{{HOME}}/.sld", "/var/lib/.sld")

	// Just in case {{HOME}} is used elsewhere
	config = strings.ReplaceAll(config, "{{HOME}}", "/var/lib")

	return config
}

// Helper to write Nginx config with current state (PHP version, etc)
func (d *Daemon) refreshNginxConfig() error {
	// 1. Get Base Config
	templateName := "sld.conf"
	if d.State.Data.Secure {
		templateName = "sld-ssl.conf"
	}

	baseConfig, err := assets.ReadTemplate(templateName)
	if err != nil {
		return fmt.Errorf("failed to read embedded template %s: %w", templateName, err)
	}

	// 2. Perform Standard Replacements on Base Config
	baseConfig = d.replacePaths(baseConfig)

	port := d.State.Data.Port
	if port == "" {
		port = "80"
	}
	baseConfig = strings.ReplaceAll(baseConfig, "listen 80;", fmt.Sprintf("listen %s;\n    listen [::]:%s;", port, port))
	baseConfig = strings.ReplaceAll(baseConfig, "listen 443 ssl http2;", "listen 443 ssl http2;\n    listen [::]:443 ssl http2;")

	if d.State.Data.PHPVersion != "" {
		socketPath, err := d.Adapter.CheckPHPSocket(d.State.Data.PHPVersion)
		if err == nil {
			baseConfig = replaceSocket(baseConfig, socketPath)
		}
	}

	// 3. Generate Isolated Server Blocks
	isolationBlocks := ""
	for domain, config := range d.State.Data.SiteConfigs {
		if config.PHPVersion != "" {
			// Find path for this domain
			projectPath := ""
			// Check Links
			linkPath, ok := d.State.Data.Links[strings.TrimSuffix(domain, "."+d.State.Data.TLD)]
			if ok {
				projectPath = linkPath
			} else {
				// Check Parked Paths (Scan again? Optimization needed for real app)
				// For now, let's assume if it's in SiteConfigs, it exists.
				// But we need the PATH to set root/router.
				// Wait, router.php logic handles path routing dynamically.
				// But for isolation, we are bypassing the wildcard server block.
				// So we need to set `root` correctly in the isolated block.

				// Re-scanning parked paths to find where this domain lives
				name := strings.TrimSuffix(domain, "."+d.State.Data.TLD)
				for _, p := range d.State.Data.Paths {
					if _, err := os.Stat(filepath.Join(p, name)); err == nil {
						projectPath = filepath.Join(p, name)
						break
					}
				}
			}

			if projectPath != "" {
				socket, err := d.Adapter.CheckPHPSocket(config.PHPVersion)
				if err == nil {
					// Use WebRoot override if present
					webRoot := projectPath
					if config.WebRoot != "" {
						webRoot = filepath.Join(projectPath, config.WebRoot)
					}

					// Basic Server Block Template for Isolation
					// We add support for Cloudflare Tunnel headers (X-Forwarded-Host, X-Forwarded-Proto)
					// to ensure Laravel/PHP generates correct public URLs and handles SSL correctly behind the tunnel.

					proxyLogic := `
    # Proxy Header Support for Cloudflare Tunnels
    set $proxy_host $host;
    if ($http_x_forwarded_host) {
        set $proxy_host $http_x_forwarded_host;
    }
    
    set $proxy_https $https;
    if ($http_x_forwarded_proto = "https") {
        set $proxy_https "on";
    }
`

					var block string
					if d.State.Data.Secure {
						block = fmt.Sprintf(`
server {
    listen %s;
    listen [::]:%s;
    server_name %s;
    return 301 https://$host$request_uri;
}
`, port, port, domain)
					} else {
						block = fmt.Sprintf(`
server {
    listen %s;
    listen [::]:%s;
    server_name %s;
    root "%s";
    
    index index.html index.htm index.php;

    %s

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        fastcgi_pass unix:%s;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
        include fastcgi_params;
        
        # Override Host/Proto for Tunnel
        fastcgi_param HTTP_HOST $proxy_host;
        fastcgi_param SERVER_NAME $proxy_host;
        fastcgi_param HTTPS $proxy_https;

        fastcgi_param PHP_VALUE "error_reporting=E_ALL & ~E_DEPRECATED";
        fastcgi_buffers 16 32k;
        fastcgi_buffer_size 64k;
        fastcgi_busy_buffers_size 64k;
    }
}
`, port, port, domain, webRoot, proxyLogic, socket)
					}

					// If secure, add SSL block too
					if d.State.Data.Secure {
						// We assume certs are at /var/lib/sld/certs/dev.pem
						certPath := "/var/lib/sld/certs/dev.pem"
						keyPath := "/var/lib/sld/certs/dev-key.pem"

						block += fmt.Sprintf(`
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name %s;
    root "%s";
    
    ssl_certificate %s;
    ssl_certificate_key %s;

    index index.html index.htm index.php;

    %s

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        fastcgi_pass unix:%s;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
        include fastcgi_params;
        
        # Override Host/Proto for Tunnel
        fastcgi_param HTTP_HOST $proxy_host;
        fastcgi_param SERVER_NAME $proxy_host;
        fastcgi_param HTTPS $proxy_https;  # Prioritize proxy logic, fallback to explicit HTTPS on

        fastcgi_buffers 16 32k;
        fastcgi_buffer_size 64k;
        fastcgi_busy_buffers_size 64k;
    }
}
`, domain, webRoot, certPath, keyPath, proxyLogic, socket)
					}

					isolationBlocks += block
				} else {
					// Only warn if version is >= 7.4
					shouldWarn := true
					if v, err := strconv.ParseFloat(config.PHPVersion, 64); err == nil {
						if v < 7.4 {
							shouldWarn = false
						}
					}
					if shouldWarn {
						fmt.Printf("Warning: PHP socket for %s not found. Skipping isolation for %s.\n", config.PHPVersion, domain)
					}
				}
			}
		}
	}

	// 4. Collect Plugin Configs
	pluginBlocks := ""
	if d.PluginManager != nil {
		for _, p := range d.PluginManager.GetAll() {
			if d.State.IsPluginEnabled(p.ID()) {
				if hook, ok := p.(plugins.NginxHook); ok {
					configs, err := hook.NginxConfig()
					if err == nil {
						for name, cfg := range configs {
							pluginBlocks += fmt.Sprintf("\n# --- Plugin: %s (%s) ---\n%s\n", p.Name(), name, cfg)
						}
					}
				}
			}
		}
	}

	// Append isolation blocks to config
	finalConfig := baseConfig + "\n# --- Plugin Blocks ---\n" + pluginBlocks + "\n# --- Isolated Sites ---\n" + isolationBlocks

	return d.Adapter.WriteNginxConfig(finalConfig)
}

func getRealUserHome() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			return u.HomeDir
		}
		// Fallback if lookup fails (e.g. static binary issues)
		return filepath.Join("/home", sudoUser)
	}
	h, _ := os.UserHomeDir()
	return h
}

// ensureProjectPHPVersions installs any PHP versions required by projects but not yet installed
func (d *Daemon) ensureProjectPHPVersions() {
	versions := make(map[string]bool)
	for _, config := range d.State.Data.SiteConfigs {
		if config.PHPVersion != "" {
			versions[config.PHPVersion] = true
		}
	}

	for version := range versions {
		// Filter out versions below 7.4
		if v, err := strconv.ParseFloat(version, 64); err == nil {
			if v < 7.4 {
				continue
			}
		}

		if _, err := d.Adapter.CheckPHPSocket(version); err != nil {
			fmt.Printf("Installing PHP %s for project isolation...\n", version)
			if installErr := d.Adapter.InstallPHP(version); installErr != nil {
				fmt.Printf("Warning: Failed to install PHP %s: %v\n", version, installErr)
			}
		}
	}
}

// ensureProjectNodeVersions installs Node.js versions required by projects
func (d *Daemon) ensureProjectNodeVersions() {
	// Scan all projects to find node requirements
	// For simplicity, we iterate known sites. Ideally, we scan all paths.
	// But `SiteConfigs` might be empty initially.
	// Let's rely on parked paths.
	for _, path := range d.State.Data.Paths {
		version, err := d.ProjectManager.ScanNodeRequirement(path)
		if err != nil {
			fmt.Printf("Warning: Failed to scan node version for %s: %v\n", path, err)
			continue
		}

		if version != "" {
			// Clean version string (e.g. ">=18.0.0" -> "18", "v20" -> "20")
			// This is a naive cleaner. fnm handles some semver, but let's be safe.
			// If it contains specific version, we try to use it.
			// For now, let's assume valid semver or simple version.
			// fnm supports "18", "20", "lts", etc.
			// We remove >=, ^, ~ chars for better matching if simple
			cleanVer := strings.TrimLeft(version, ">=^~v")
			cleanVer = strings.Split(cleanVer, " ")[0] // Take first part if range

			fmt.Printf("Project at %s requires Node %s (clean: %s). Ensuring installed...\n", path, version, cleanVer)
			if err := d.Adapter.InstallNode(cleanVer); err != nil {
				fmt.Printf("Warning: Failed to install Node %s: %v\n", cleanVer, err)
			}
		}
	}
}

// Project Management

// HTTPS

func (d *Daemon) regenerateCerts() error {
	fmt.Println("Regenerating certificates...")

	// Collect domains from state
	domains := []string{"sld.test", "*.test"} // Explicitly add system domains
	// Linked sites
	for name := range d.State.Data.Links {
		domains = append(domains, name+".test")
	}
	// Parked sites (scan directories)
	for _, p := range d.State.Data.Paths {
		entries, err := os.ReadDir(p)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
					domains = append(domains, entry.Name()+".test")
				}
			}
		}
	}

	if err := d.Adapter.GenerateCert("", domains); err != nil {
		return fmt.Errorf("failed to generate certs: %w", err)
	}

	return d.refreshNginxConfig()
}

func (d *Daemon) Secure() error {
	fmt.Println("Installing mkcert...")
	if err := d.Adapter.InstallMkcert(); err != nil {
		return fmt.Errorf("failed to install mkcert: %w", err)
	}

	d.State.Data.Secure = true
	d.State.Save()

	if err := d.regenerateCerts(); err != nil {
		return err
	}

	// Trust certificates in Snap browsers etc
	if err := d.Adapter.InstallCertificates(); err != nil {
		fmt.Printf("Warning: Failed to install certificates to browsers: %v\n", err)
	}

	fmt.Println("HTTPS Enabled! 🔒")
	return nil
}

func (d *Daemon) Unsecure() error {
	fmt.Println("Disabling HTTPS...")

	d.State.Data.Secure = false
	d.State.Save()

	fmt.Println("Updating Nginx configuration...")
	if err := d.refreshNginxConfig(); err != nil {
		return err
	}

	// We don't uninstall mkcert, just switch config.
	fmt.Println("HTTPS Disabled. Switched back to HTTP. 🔓")
	return nil
}

// Project Management

func (d *Daemon) scanPath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	d.State.AddPath(absPath)

	entries, err := os.ReadDir(absPath)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				subPath := filepath.Join(absPath, entry.Name())
				// Detect config
				if conf, err := project.Detect(subPath); err == nil && (conf.PHP != "" || conf.Public != "") {
					domain := fmt.Sprintf("%s.%s", entry.Name(), d.State.Data.TLD)
					resolvedPHP := d.resolvePHPVersion(conf.PHP)
					d.State.SetSiteConfig(domain, state.SiteConfig{
						PHPVersion:  resolvedPHP,
						WebRoot:     conf.Public,
						NodeVersion: conf.Node,
					})
					if resolvedPHP != "" {
						fmt.Printf("Detected config for %s: PHP %s (from %s)\n", domain, resolvedPHP, conf.PHP)
					} else {
						fmt.Printf("Detected config for %s: Using default PHP (satisfied %s)\n", domain, conf.PHP)
					}
				}
			}
		}
	}
	return nil
}

func (d *Daemon) Park(path string) error {
	if err := d.scanPath(path); err != nil {
		return err
	}

	d.Events.Publish(events.Event{Type: events.SitesUpdated})

	if err := d.syncHosts(); err != nil {
		fmt.Printf("Warning: Failed to sync hosts: %v\n", err)
	}

	if d.State.Data.Secure {
		return d.regenerateCerts()
	}
	return d.refreshNginxConfig()
}

func (d *Daemon) Forget(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	d.State.RemovePath(absPath)

	d.Events.Publish(events.Event{Type: events.SitesUpdated})

	if err := d.syncHosts(); err != nil {
		fmt.Printf("Warning: Failed to sync hosts: %v\n", err)
	}

	if d.State.Data.Secure {
		return d.regenerateCerts()
	}
	return d.refreshNginxConfig()
}

func (d *Daemon) linkInternal(name, path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	d.State.AddLink(name, absPath)

	// Detect config
	if conf, err := project.Detect(absPath); err == nil && (conf.PHP != "" || conf.Public != "") {
		domain := fmt.Sprintf("%s.%s", name, d.State.Data.TLD)
		resolvedPHP := d.resolvePHPVersion(conf.PHP)
		d.State.SetSiteConfig(domain, state.SiteConfig{
			PHPVersion:  resolvedPHP,
			WebRoot:     conf.Public,
			NodeVersion: conf.Node,
		})
		if resolvedPHP != "" {
			fmt.Printf("Detected config for %s: PHP %s (from %s)\n", domain, resolvedPHP, conf.PHP)
		}
	}
	return nil
}

func (d *Daemon) Link(name, path string) error {
	if err := d.linkInternal(name, path); err != nil {
		return err
	}

	if err := d.syncHosts(); err != nil {
		fmt.Printf("Warning: Failed to sync hosts: %v\n", err)
	}

	if d.State.Data.Secure {
		if err := d.regenerateCerts(); err != nil {
			return err
		}
		// Reload nginx to pick up the new certificate
		return d.Adapter.ReloadNginx()
	}

	d.Events.Publish(events.Event{Type: events.SitesUpdated})
	return d.refreshNginxConfig()
}

func (d *Daemon) Unlink(name string) error {
	d.State.RemoveLink(name)
	// Remove config if any
	domain := fmt.Sprintf("%s.%s", name, d.State.Data.TLD)
	if _, ok := d.State.Data.SiteConfigs[domain]; ok {
		delete(d.State.Data.SiteConfigs, domain)
		d.State.Save()
	}

	d.Events.Publish(events.Event{Type: events.SitesUpdated})

	if err := d.syncHosts(); err != nil {
		fmt.Printf("Warning: Failed to sync hosts: %v\n", err)
	}

	if d.State.Data.Secure {
		return d.regenerateCerts()
	}
	return d.refreshNginxConfig()
}

// Refresh re-scans all projects for configuration changes
func (d *Daemon) Refresh() error {
	fmt.Println("Scanning parked paths...")
	for _, p := range d.State.Data.Paths {
		d.scanPath(p) // Re-scan internal
	}

	fmt.Println("Scanning linked sites...")
	for name, path := range d.State.Data.Links {
		d.linkInternal(name, path) // Re-scan internal
	}

	if err := d.syncHosts(); err != nil {
		fmt.Printf("Warning: Failed to sync hosts: %v\n", err)
	}

	if d.State.Data.Secure {
		return d.regenerateCerts()
	}
	return d.refreshNginxConfig()
}

// GetSites returns a list of all available sites (parked + linked)
func (d *Daemon) GetSites() ([]Site, error) {
	// Reload state from disk to ensure we have CLI changes
	d.State.Load()

	sites := []Site{}
	tld := d.State.Data.TLD
	if tld == "" {
		tld = "test"
	}

	// Helper to check if ignored
	isIgnored := func(path string) bool {
		for _, ignored := range d.State.Data.Ignored {
			if ignored == path {
				return true
			}
		}
		return false
	}

	// Build a set of linked paths for deduplication
	// This prevents projects from appearing twice if they are both
	// in a parked directory AND explicitly linked
	linkedPaths := make(map[string]bool)
	for _, linkPath := range d.State.Data.Links {
		linkedPaths[linkPath] = true
	}

	// 1. Scan Parked Paths
	for _, path := range d.State.Data.Paths {
		entries, err := os.ReadDir(path)
		if err != nil {
			// Log error but continue? Or skip
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				name := entry.Name()
				fullPath := filepath.Join(path, name)

				if isIgnored(fullPath) {
					continue
				}

				// Skip if this path is also linked (avoid duplicates)
				if linkedPaths[fullPath] {
					continue
				}

				// PHP Version override?
				domain := name + "." + tld
				phpVer := d.State.Data.PHPVersion
				var tags []string
				var category string
				if conf, ok := d.State.Data.SiteConfigs[domain]; ok {
					if conf.PHPVersion != "" {
						phpVer = conf.PHPVersion
					}
					tags = conf.Tags
					category = conf.Category
				}

				sites = append(sites, Site{
					Name:       name,
					Path:       fullPath,
					Domain:     domain,
					PHPVersion: phpVer,
					Secure:     d.State.Data.Secure,
					Type:       "parked",
					Tags:       tags,
					Category:   category,
				})
			}
		}
	}

	// 2. Add Linked Sites
	for name, path := range d.State.Data.Links {
		// Verify path exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		// PHP Version override?
		domain := name + "." + tld
		phpVer := d.State.Data.PHPVersion
		var tags []string
		var category string
		if conf, ok := d.State.Data.SiteConfigs[domain]; ok {
			if conf.PHPVersion != "" {
				phpVer = conf.PHPVersion
			}
			tags = conf.Tags
			category = conf.Category
		}

		sites = append(sites, Site{
			Name:       name,
			Path:       path,
			Domain:     domain,
			PHPVersion: phpVer,
			Secure:     d.State.Data.Secure,
			Type:       "linked",
			Tags:       tags,
			Category:   category,
		})
	}

	return sites, nil
}

func (d *Daemon) Ignore(path string) error {
	d.State.AddIgnore(path)
	fmt.Printf("Ignored path: %s\n", path)
	d.Events.Publish(events.Event{Type: events.SitesUpdated})
	return nil
}

func (d *Daemon) Unignore(path string) error {
	d.State.RemoveIgnore(path)
	fmt.Printf("Unignored path: %s\n", path)
	d.Events.Publish(events.Event{Type: events.SitesUpdated})
	return nil
}

// Uninstall removes SLD from the system
func (d *Daemon) Uninstall() error {
	return d.Adapter.Uninstall()
}

// Service Management

func (d *Daemon) Restart() error {
	fmt.Println("Restarting services...")

	if err := d.Adapter.RestartService("nginx"); err != nil {
		fmt.Printf("Warning: Failed to restart Nginx: %v\n", err)
	} else {
		fmt.Println("Nginx restarted.")
	}

	if err := d.Adapter.RestartPHP(); err != nil {
		fmt.Printf("Warning: Failed to restart PHP: %v\n", err)
	} else {
		fmt.Println("PHP restarted.")
	}

	// Dnsmasq might be separate, but let's try
	if err := d.Adapter.RestartService("dnsmasq"); err != nil {
		// Log but don't fail, dnsmasq might be managed differently on some OS
		fmt.Printf("Warning: Failed to restart dnsmasq: %v\n", err)
	} else {
		fmt.Println("Dnsmasq restarted.")
	}

	return nil
}

// Diagnostics

func (d *Daemon) Doctor() error {
	return d.Adapter.Doctor()
}

// Logs returns map of log names to paths
func (d *Daemon) GetLogPaths() map[string]string {
	// Ideally adapter gives these paths as they vary by OS.
	// For now, assuming standard Linux/Nginx locations or getting from config.
	// TODO: move to Adapter.GetLogPaths()

	logs := make(map[string]string)
	logs["nginx-error"] = "/var/log/nginx/error.log"
	logs["nginx-access"] = "/var/log/nginx/access.log"
	// PHP logs vary
	logs["php-fpm"] = "/var/log/php-fpm.log" // Generic fallback

	return logs
}

// Multi-PHP

func (d *Daemon) SwitchPHP(version string) error {
	fmt.Printf("Switching to PHP %s...\n", version)

	// 1. Verify existence
	socketPath, err := d.Adapter.CheckPHPSocket(version)
	if err != nil {
		fmt.Printf("Socket for PHP %s not found. Attempting automatic installation...\n", version)
		if installErr := d.Adapter.InstallPHP(version); installErr != nil {
			return fmt.Errorf("failed to install PHP %s: %w", version, installErr)
		}

		// Re-check after installation
		socketPath, err = d.Adapter.CheckPHPSocket(version)
		if err != nil {
			return fmt.Errorf("failed to locate socket after installation: %w", err)
		}
	}
	fmt.Printf("Found socket: %s\n", socketPath)

	// 2. Update State
	d.State.Data.PHPVersion = version
	d.State.Save()

	// 3. Update Config
	if err := d.refreshNginxConfig(); err != nil {
		return err
	}

	fmt.Printf("Switched to PHP %s successfully! 🐘\n", version)
	return nil
}

func findBestDevDir(home string) string {
	defaults := []string{"Developments", "Projects", "Sites", "code", "codes", "dev"}
	for _, d := range defaults {
		path := filepath.Join(home, d)
		if st, err := os.Stat(path); err == nil && st.IsDir() {
			return path
		}
	}
	return home // Fallback to home if none found
}
func (d *Daemon) resolvePHPVersion(constraint string) string {
	if constraint == "" {
		return ""
	}

	// 1. Extract base version using regex (e.g. 8.1 from ^8.1)
	re := regexp.MustCompile(`(\d+\.\d+)`)
	matches := re.FindStringSubmatch(constraint)
	if len(matches) < 2 {
		return ""
	}
	baseVer := matches[1]

	// 2. Get all installed PHP versions
	installed, err := d.Adapter.ListPHPVersions()
	if err != nil {
		return ""
	}

	// 3. Find the highest compatible version
	// installed is already sorted descending (newest first) by the adapter.
	for _, v := range installed {
		vNum, _ := strconv.ParseFloat(v, 64)
		baseNum, _ := strconv.ParseFloat(baseVer, 64)

		isCompatible := false
		if strings.Contains(constraint, "^") || strings.Contains(constraint, ">=") {
			// Compatible if same major or if installed is higher (major check avoids 7.x vs 8.x unless >= used)
			vMajor := int(vNum)
			baseMajor := int(baseNum)

			if strings.Contains(constraint, ">=") {
				if vNum >= baseNum {
					isCompatible = true
				}
			} else { // caret ^
				if vMajor == baseMajor && vNum >= baseNum {
					isCompatible = true
				}
			}
		} else {
			// Exact or range fallback
			if v == baseVer {
				isCompatible = true
			}
		}

		if isCompatible {
			// Prefer system default if it's the one we found
			if v == d.State.Data.PHPVersion {
				return "" // Use system default (implies Nginx base config)
			}
			return v
		}
	}

	return "" // Fallback to default if no compatibility found
}
