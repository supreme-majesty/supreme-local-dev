package daemon

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"runtime"

	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters"
	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters/linux"
	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters/macos"
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
	databaseService := services.NewDatabaseService()
	// Use user's home/Developments as default base?
	// We need a sensible default.
	home, _ := os.UserHomeDir()
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		home = filepath.Join("/home", sudoUser)
	}
	baseDir := filepath.Join(home, "Developments")
	// Ensure it exists? Or let create handle it.
	projectManager := services.NewProjectManager(baseDir)

	// Start X-Ray immediately
	go xrayService.Start()

	// Register default plugins
	pluginManager.Register(services.NewRedisPlugin(pluginManager.DataDir))
	pluginManager.Register(services.NewMailHogPlugin(pluginManager.DataDir))

	// Auto-start enabled plugins from persisted state
	pluginManager.StartEnabled()

	// 4. Detect OS and select Adapter
	var adapter adapters.SystemAdapter
	switch runtime.GOOS {
	case "linux":
		adapter = linux.NewLinuxAdapter()
	case "darwin":
		adapter = macos.NewMacOSAdapter()
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	instance = &Daemon{
		State:           stateManager,
		Events:          eventBus,
		Adapter:         adapter,
		PluginManager:   pluginManager,
		TunnelManager:   tunnelManager,
		XRayService:     xrayService,
		DatabaseService: databaseService,
		ProjectManager:  projectManager,
	}

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
	baseConfig = strings.ReplaceAll(baseConfig, "listen 80;", fmt.Sprintf("listen %s;", port))

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
					var block string
					if d.State.Data.Secure {
						block = fmt.Sprintf(`
server {
    listen %s;
    server_name %s;
    return 301 https://$host$request_uri;
}
`, port, domain)
					} else {
						block = fmt.Sprintf(`
server {
    listen %s;
    server_name %s;
    root "%s";
    
    index index.html index.htm index.php;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        fastcgi_pass unix:%s;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
        include fastcgi_params;
        fastcgi_param PHP_VALUE "error_reporting=E_ALL & ~E_DEPRECATED";
        fastcgi_buffers 16 32k;
        fastcgi_buffer_size 64k;
        fastcgi_busy_buffers_size 64k;
    }
}
`, port, domain, webRoot, socket)
					}

					// If secure, add SSL block too (using snakeoil for simplicity or same certs)
					// But wait, the wildcard cert works for these!
					// If d.State.Data.Secure is true, we should generate an SSL block.
					if d.State.Data.Secure {
						// We assume certs are at /var/lib/sld/certs/dev.pem
						certPath := "/var/lib/sld/certs/dev.pem"
						keyPath := "/var/lib/sld/certs/dev-key.pem"

						block += fmt.Sprintf(`
server {
    listen 443 ssl;
    server_name %s;
    root "%s";
    
    ssl_certificate %s;
    ssl_certificate_key %s;

    index index.html index.htm index.php;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        fastcgi_pass unix:%s;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
        include fastcgi_params;
        fastcgi_param HTTPS on;
        fastcgi_buffers 16 32k;
        fastcgi_buffer_size 64k;
        fastcgi_busy_buffers_size 64k;
    }
}
`, domain, webRoot, certPath, keyPath, socket)
					}

					isolationBlocks += block
				} else {
					fmt.Printf("Warning: PHP socket for %s not found. Skipping isolation for %s.\n", config.PHPVersion, domain)
				}
			}
		}
	}

	// Append isolation blocks to config
	finalConfig := baseConfig + "\n# --- Isolated Sites ---\n" + isolationBlocks

	return d.Adapter.WriteNginxConfig(finalConfig)
}

func getRealUserHome() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		// This is a naive way, but works for standard setups.
		// Ideally lookup /etc/passwd
		return filepath.Join("/home", sudoUser)
	}
	h, _ := os.UserHomeDir()
	return h
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
					d.State.SetSiteConfig(domain, state.SiteConfig{
						PHPVersion:  conf.PHP,
						WebRoot:     conf.Public,
						NodeVersion: conf.Node,
					})
					fmt.Printf("Detected config for %s: PHP %s\n", domain, conf.PHP)
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
		d.State.SetSiteConfig(domain, state.SiteConfig{
			PHPVersion:  conf.PHP,
			WebRoot:     conf.Public,
			NodeVersion: conf.Node,
		})
		fmt.Printf("Detected config for %s: PHP %s\n", domain, conf.PHP)
	}
	return nil
}

func (d *Daemon) Link(name, path string) error {
	if err := d.linkInternal(name, path); err != nil {
		return err
	}

	if d.State.Data.Secure {
		if err := d.regenerateCerts(); err != nil {
			return err
		}
		// Reload nginx to pick up the new certificate
		return d.Adapter.ReloadNginx()
	}
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

	if d.State.Data.Secure {
		return d.regenerateCerts()
	}
	return d.refreshNginxConfig()
}

// GetSites returns a list of all available sites (parked + linked)
func (d *Daemon) GetSites() ([]Site, error) {
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

				sites = append(sites, Site{
					Name:       name,
					Path:       fullPath,
					Domain:     name + "." + tld,
					PHPVersion: d.State.Data.PHPVersion,
					Secure:     d.State.Data.Secure,
					Type:       "parked",
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

		sites = append(sites, Site{
			Name:       name,
			Path:       path,
			Domain:     name + "." + tld,
			PHPVersion: d.State.Data.PHPVersion,
			Secure:     d.State.Data.Secure,
			Type:       "linked",
		})
	}

	return sites, nil
}

func (d *Daemon) Ignore(path string) error {
	d.State.AddIgnore(path)
	fmt.Printf("Ignored path: %s\n", path)
	return nil
}

func (d *Daemon) Unignore(path string) error {
	d.State.RemoveIgnore(path)
	fmt.Printf("Unignored path: %s\n", path)
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
	fmt.Println("Running diagnostic checks... 🩺")

	// 1. Check Services
	services := []string{"nginx", "dnsmasq"}
	allGood := true

	for _, s := range services {
		running, err := d.Adapter.IsServiceRunning(s)
		if err != nil {
			fmt.Printf("❌ %s check failed: %v\n", s, err)
			allGood = false
		} else if !running {
			fmt.Printf("❌ %s is NOT running.\n", s)
			allGood = false
		} else {
			fmt.Printf("✅ %s is running.\n", s)
		}
	}

	// 2. Check PHP
	phpV := d.Adapter.GetPHPVersion()
	if phpV == "" {
		fmt.Println("❌ No PHP version detected.")
		allGood = false
	} else {
		fmt.Printf("✅ PHP version: %s\n", phpV)
		// Check socket
		_, err := d.Adapter.CheckPHPSocket(phpV)
		if err != nil {
			fmt.Printf("❌ PHP socket not found: %v\n", err)
			allGood = false
		} else {
			fmt.Println("✅ PHP socket found.")
		}
	}

	// 3. Check Permissions
	// TODO: Add permission checks

	if allGood {
		fmt.Println("\nEverything looks good! 🎉")
	} else {
		fmt.Println("\nSome issues were found. Please check logs.")
	}

	return nil
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
		return err
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
