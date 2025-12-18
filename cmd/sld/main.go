package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon/api"
)

var rootCmd = &cobra.Command{
	Use:     "sld",
	Short:   "Supreme Local Dev",
	Long:    `High-performance local development environment for PHP/Laravel.`,
	Version: Version,
}

var Version = "dev"

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install SLD dependencies and core services",
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("installation requires root privileges. Please run with sudo")
		}

		fmt.Println("Installing Supreme Local Dev...")

		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		if err := d.EnsureInstalled(); err != nil {
			return fmt.Errorf("installation failed: %w", err)
		}

		// Install daemon as systemd service for auto-start
		fmt.Println("Setting up daemon service...")
		if err := installDaemonService(); err != nil {
			fmt.Printf("Warning: Failed to install daemon service: %v\n", err)
			fmt.Println("You can manually start the daemon with: sld daemon")
		}

		fmt.Println("Supreme Local Dev installed successfully! 🚀")
		fmt.Println("")
		fmt.Println("The SLD daemon is now running and will auto-start on boot.")
		fmt.Println("Visit http://sld.test to access the dashboard.")
		return nil
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove SLD and all its configurations",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Uninstalling Supreme Local Dev...")

		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		if err := d.Uninstall(); err != nil {
			return fmt.Errorf("uninstall failed: %w", err)
		}

		fmt.Println("Supreme Local Dev uninstalled successfully. 👋")
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of services",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		// Simple status check
		running, err := d.Adapter.IsServiceRunning("nginx")
		status := "STOPPED"
		if err == nil && running {
			status = "RUNNING"
		}

		fmt.Printf("Nginx: %s\n", status)
		fmt.Printf("PHP:   %s\n", d.Adapter.GetPHPVersion())

		if len(d.State.Data.SiteConfigs) > 0 {
			fmt.Println("\nIsolated Sites:")
			for domain, conf := range d.State.Data.SiteConfigs {
				details := []string{}
				if conf.PHPVersion != "" {
					details = append(details, fmt.Sprintf("PHP %s", conf.PHPVersion))
				}
				if conf.WebRoot != "" {
					details = append(details, fmt.Sprintf("Root: %s", conf.WebRoot))
				}
				if len(details) > 0 {
					fmt.Printf(" - %s [%s]\n", domain, strings.Join(details, ", "))
				}
			}
		}

		return nil
	},
}

// isInstalled checks if SLD has been configured on the system
func isInstalled() bool {
	_, err := os.Stat("/var/lib/sld/state.json")
	return err == nil
}

// autoInstall attempts to run 'sudo sld install' with interactive password prompt
func autoInstall() bool {
	fmt.Println("⚠️  SLD is not configured on this system.")
	fmt.Println("🔧 Running automatic installation...")
	fmt.Println()

	// Get the current executable path
	exe, err := os.Executable()
	if err != nil {
		exe = "sld" // Fallback to PATH lookup
	}

	// Run sudo with interactive mode for password prompt
	cmd := exec.Command("sudo", exe, "install")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Println()
		fmt.Printf("❌ Installation failed: %v\n", err)
		fmt.Println("   Please run 'sudo sld install' manually.")
		return false
	}

	fmt.Println()
	fmt.Println("✅ Installation complete! Continuing with your command...")
	fmt.Println()
	return true
}

func main() {
	// Auto-detect missing installation for commands that need it
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		// Skip check for install, help, version, and completion commands
		skipCheck := cmd == "install" || cmd == "--help" || cmd == "-h" ||
			cmd == "--version" || cmd == "-v" || cmd == "help" || cmd == "completion"

		if !skipCheck && !isInstalled() {
			if !autoInstall() {
				os.Exit(1)
			}
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// installDaemonService installs and starts the SLD daemon as a systemd service
func installDaemonService() error {
	// Get executable path
	exePath, err := os.Executable()
	if err != nil {
		exePath = "/usr/bin/sld"
	}

	// Create systemd service file
	serviceContent := fmt.Sprintf(`[Unit]
Description=Supreme Local Dev Daemon
Documentation=https://github.com/supreme-majesty/supreme-local-dev
After=network.target nginx.service

[Service]
Type=simple
ExecStart=%s daemon
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, exePath)

	servicePath := "/etc/systemd/system/sld-daemon.service"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd and enable service
	exec.Command("systemctl", "daemon-reload").Run()
	if err := exec.Command("systemctl", "enable", "sld-daemon").Run(); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	// Start the service
	if err := exec.Command("systemctl", "start", "sld-daemon").Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(statusCmd)

	// Project Management Commands
	rootCmd.AddCommand(parkCmd)
	rootCmd.AddCommand(forgetCmd)
	rootCmd.AddCommand(pathsCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(unlinkCmd)
	rootCmd.AddCommand(linksCmd)
	rootCmd.AddCommand(secureCmd)
	rootCmd.AddCommand(phpCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(guiCmd)
	rootCmd.AddCommand(dashboardCmd)

	dashboardCmd.AddCommand(dashboardStartCmd)

	// Phase 1 Additional Commands
	rootCmd.AddCommand(unparkCmd)
	rootCmd.AddCommand(unsecureCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(refreshCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(pluginCmd)

	pluginCmd.AddCommand(pluginInstallCmd)
	pluginCmd.AddCommand(pluginEnableCmd)

	rootCmd.AddCommand(shareCmd)

	// Service management
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceStatusCmd)

}

// --- Commands ---

var unparkCmd = &cobra.Command{
	Use:   "unpark [path]",
	Short: "Remove a directory from parked paths (alias for forget)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Re-use forget logic
		return forgetCmd.RunE(cmd, args)
	},
}

var unsecureCmd = &cobra.Command{
	Use:   "unsecure",
	Short: "Disable HTTPS and revert to HTTP",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}
		return d.Unsecure()
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart Nginx, PHP, and Dnsmasq services",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}
		return d.Restart()
	},
}

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Re-scan all projects for configuration changes (.sld.yaml, composer.json)",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}
		return d.Refresh()
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and status",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}
		return d.Doctor()
	},
}

var logsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "View logs for a service (nginx, php)",
	Long:  `Available services: nginx-error, nginx-access, php-fpm`,
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		key := "nginx-error"
		if len(args) > 0 {
			key = args[0]
		}

		paths := d.GetLogPaths()
		logPath, ok := paths[key]
		if !ok {
			return fmt.Errorf("unknown log service: %s. Available: nginx-error, nginx-access, php-fpm", key)
		}

		fmt.Printf("Tailing log: %s\n", logPath)
		// Simple tail implementation
		cmdTail := exec.Command("tail", "-f", logPath)
		cmdTail.Stdout = os.Stdout
		cmdTail.Stderr = os.Stderr
		return cmdTail.Run()
	},
}

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage plugins",
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install [name]",
	Short: "Install a plugin (stub)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("plugin name required")
		}
		fmt.Printf("Installing plugin %s... (Not implemented in Phase 1)\n", args[0])
		return nil
	},
}

var pluginEnableCmd = &cobra.Command{
	Use:   "enable [name]",
	Short: "Enable a plugin (stub)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("plugin name required")
		}
		fmt.Printf("Enabling plugin %s... (Not implemented in Phase 1)\n", args[0])
		return nil
	},
}

// --- Commands ---

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the SLD API server and dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Ensure core is installed/ready?
		_, err := daemon.GetClient()
		if err != nil {
			return err
		}

		// Start Server
		srv := api.NewServer(2025)

		// Handle shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		go func() {
			<-sigChan
			fmt.Println("\nShutting down daemon... 🛑")
			d, _ := daemon.GetClient()
			if d.XRayService != nil {
				d.XRayService.Stop()
			}
			os.Exit(0)
		}()

		return srv.Start()
	},
}

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Manage the SLD Dashboard",
}

var dashboardStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Open the SLD Dashboard in your browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := "http://sld.test"
		fmt.Printf("Opening %s...\n", url)

		// Linux xdg-open
		exec.Command("xdg-open", url).Start()
		// TODO: Support Mac open, Windows start

		return nil
	},
}

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Open the SLD Dashboard in your browser (Deprecated: use 'dashboard start')",
	RunE: func(cmd *cobra.Command, args []string) error {
		return dashboardStartCmd.RunE(cmd, args)
	},
}

var phpCmd = &cobra.Command{
	Use:   "php [version]",
	Short: "Switch global PHP version (e.g. 8.1, 8.2)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("please specify a PHP version (e.g. 8.2)")
		}
		version := args[0]

		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		return d.SwitchPHP(version)
	},
}

var secureCmd = &cobra.Command{
	Use:   "secure",
	Short: "Enable HTTPS (installs mkcert and updates config)",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		return d.Secure()
	},
}

var parkCmd = &cobra.Command{
	Use:   "park [path]",
	Short: "Register a directory to serve projects from",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		if err := d.Park(path); err != nil {
			return err
		}
		fmt.Printf("Parked directory: %s\n", path)
		return nil
	},
}

var forgetCmd = &cobra.Command{
	Use:   "forget [path]",
	Short: "Remove a directory from parked paths",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		if err := d.Forget(path); err != nil {
			return err
		}
		fmt.Printf("Forgot directory: %s\n", path)
		return nil
	},
}

var pathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "List all parked directories",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		fmt.Println("Parked Paths:")
		for _, p := range d.State.Data.Paths {
			fmt.Printf(" - %s\n", p)
		}
		return nil
	},
}

var linkCmd = &cobra.Command{
	Use:   "link [name]",
	Short: "Link the current directory to a domain",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, _ := os.Getwd()
		name := ""

		if len(args) > 0 {
			name = args[0]
		} else {
			name = filepath.Base(path)
		}

		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		if err := d.Link(name, path); err != nil {
			return err
		}
		fmt.Printf("Linked http://%s.test to %s\n", name, path)
		return nil
	},
}

var unlinkCmd = &cobra.Command{
	Use:   "unlink [name]",
	Short: "Remove a link",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) > 0 {
			name = args[0]
		} else {
			cwd, _ := os.Getwd()
			name = filepath.Base(cwd)
		}

		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		if err := d.Unlink(name); err != nil {
			return err
		}
		fmt.Printf("Unlinked %s\n", name)
		return nil
	},
}

var linksCmd = &cobra.Command{
	Use:   "links",
	Short: "List all linked sites",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		fmt.Println("Linked Sites:")
		for name, path := range d.State.Data.Links {
			fmt.Printf(" - %s -> %s\n", name, path)
		}
		return nil
	},
}

var shareCmd = &cobra.Command{
	Use:   "share [site]",
	Short: "Share a site via public URL (Cloudflare Tunnel)",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) > 0 {
			name = args[0]
		} else {
			cwd, _ := os.Getwd()
			name = filepath.Base(cwd)
		}

		fmt.Printf("Starting tunnel for %s... 🚀\n", name)

		// Call API
		// We need a helper to call API from CLI properly
		// For now simple http post
		url := "http://localhost:2025/api/share/start"
		body := fmt.Sprintf(`{"site":"%s"}`, name)

		resp, err := http.Post(url, "application/json", strings.NewReader(body))
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		var res struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Error   string `json:"error"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return err
		}

		if !res.Success {
			return fmt.Errorf("failed to start tunnel: %s", res.Error)
		}

		fmt.Printf("✅ Tunnel active at: %s\n", res.Message)
		fmt.Println("Tunnel will run in background until you stop it.")
		return nil
	},
}

// --- Service Management Commands ---

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the SLD daemon as a system service",
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install SLD daemon as a systemd service (auto-start on boot)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("service installation requires root. Please run with sudo")
		}

		fmt.Println("Installing SLD daemon service...")

		// Get executable path
		exePath, err := os.Executable()
		if err != nil {
			exePath = "/usr/bin/sld"
		}

		// Create systemd service file
		serviceContent := fmt.Sprintf(`[Unit]
Description=Supreme Local Dev Daemon
Documentation=https://github.com/supreme-majesty/supreme-local-dev
After=network.target nginx.service

[Service]
Type=simple
ExecStart=%s daemon
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, exePath)

		servicePath := "/etc/systemd/system/sld-daemon.service"
		if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
			return fmt.Errorf("failed to write service file: %w", err)
		}

		// Reload systemd and enable service
		exec.Command("systemctl", "daemon-reload").Run()
		if err := exec.Command("systemctl", "enable", "sld-daemon").Run(); err != nil {
			return fmt.Errorf("failed to enable service: %w", err)
		}

		// Start the service
		if err := exec.Command("systemctl", "start", "sld-daemon").Run(); err != nil {
			return fmt.Errorf("failed to start service: %w", err)
		}

		fmt.Println("✅ SLD daemon service installed and started!")
		fmt.Println("   The daemon will now start automatically on boot.")
		fmt.Println("")
		fmt.Println("   Use 'sld service status' to check status")
		fmt.Println("   Use 'sld service stop' to stop the service")
		return nil
	},
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the SLD daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Starting SLD daemon service...")
		out, err := exec.Command("sudo", "systemctl", "start", "sld-daemon").CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to start service: %s", string(out))
		}
		fmt.Println("✅ SLD daemon started!")
		return nil
	},
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the SLD daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Stopping SLD daemon service...")
		out, err := exec.Command("sudo", "systemctl", "stop", "sld-daemon").CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to stop service: %s", string(out))
		}
		fmt.Println("✅ SLD daemon stopped!")
		return nil
	},
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of the SLD daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := exec.Command("systemctl", "status", "sld-daemon", "--no-pager").CombinedOutput()
		if err != nil {
			// Service might not be running, still show output
			fmt.Println(string(out))
			return nil
		}
		fmt.Println(string(out))
		return nil
	},
}
