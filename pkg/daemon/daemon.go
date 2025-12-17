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
)

type Daemon struct {
	State   *state.Manager
	Events  *events.Bus
	Adapter adapters.SystemAdapter
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

	// 3. Detect OS and select Adapter
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
		State:   stateManager,
		Events:  eventBus,
		Adapter: adapter,
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
	templateName := "sld.conf"
	if d.State.Data.Secure {
		templateName = "sld-ssl.conf"
	}

	configStr, err := assets.ReadTemplate(templateName)
	if err != nil {
		return fmt.Errorf("failed to read embedded template %s: %w", templateName, err)
	}

	// 1. Replace Paths
	configStr = d.replacePaths(configStr)

	// 2. Replace Port
	port := d.State.Data.Port
	if port == "" {
		port = "80"
	}
	configStr = strings.ReplaceAll(configStr, "listen 80;", fmt.Sprintf("listen %s;", port))

	// 3. Replace PHP Socket if version is set
	if d.State.Data.PHPVersion != "" {
		socketPath, err := d.Adapter.CheckPHPSocket(d.State.Data.PHPVersion)
		if err == nil {
			configStr = replaceSocket(configStr, socketPath)
		}
	}

	return d.Adapter.WriteNginxConfig(configStr)
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

func (d *Daemon) Park(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	d.State.AddPath(absPath)
	return nil
}

func (d *Daemon) Forget(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	d.State.RemovePath(absPath)
	return nil
}

func (d *Daemon) Link(name, path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	d.State.AddLink(name, absPath)
	return nil
}

func (d *Daemon) Unlink(name string) error {
	d.State.RemoveLink(name)
	return nil
}

// HTTPS

func (d *Daemon) Secure() error {
	fmt.Println("Installing mkcert...")
	if err := d.Adapter.InstallMkcert(); err != nil {
		return fmt.Errorf("failed to install mkcert: %w", err)
	}

	fmt.Println("Generating wildcard certificate for *.test...")

	// Use global base directory. LinuxAdapter appends .sld/certs
	certBase := "/var/lib"
	// Ensure exists
	os.MkdirAll(filepath.Join(certBase, ".sld", "certs"), 0755)

	if err := d.Adapter.GenerateCert(certBase); err != nil {
		return fmt.Errorf("failed to generate certs: %w", err)
	}

	d.State.Data.Secure = true
	d.State.Save()

	fmt.Println("Updating Nginx configuration...")
	if err := d.refreshNginxConfig(); err != nil {
		return err // refreshNginxConfig wraps error
	}

	fmt.Println("HTTPS Enabled! 🔒")
	return nil
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
