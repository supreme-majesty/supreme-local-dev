package macos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters"
)

type MacOSAdapter struct{}

func NewMacOSAdapter() *MacOSAdapter {
	return &MacOSAdapter{}
}

// Service Management (brew services)
func (m *MacOSAdapter) StartService(name string) error {
	return exec.Command("brew", "services", "start", name).Run()
}

func (m *MacOSAdapter) StopService(name string) error {
	return exec.Command("brew", "services", "stop", name).Run()
}

func (m *MacOSAdapter) RestartService(name string) error {
	return exec.Command("brew", "services", "restart", name).Run()
}

func (m *MacOSAdapter) IsServiceRunning(name string) (bool, error) {
	out, err := exec.Command("brew", "services", "list").Output()
	if err != nil {
		return false, err
	}
	// Output format: name status ...
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, name+" ") {
			return strings.Contains(line, "started"), nil
		}
	}
	return false, nil
}

// Installation
func (m *MacOSAdapter) InstallDependencies() error {
	fmt.Println("Checking Homebrew...")
	if _, err := exec.LookPath("brew"); err != nil {
		return fmt.Errorf("homebrew is required but not found. please install it: https://brew.sh")
	}

	packages := []string{
		"nginx",
		"dnsmasq",
		"mkcert",
		"nss", // for mkcert firefox support
		"fnm",
	}

	fmt.Println("Installing core packages via Homebrew...")
	for _, pkg := range packages {
		if err := m.installBrewPackage(pkg); err != nil {
			return err
		}
	}

	// Helper for PHP tap
	exec.Command("brew", "tap", "shivammathur/php").Run()

	return nil
}

func (m *MacOSAdapter) installBrewPackage(pkg string) error {
	// Check if installed
	if err := exec.Command("brew", "list", pkg).Run(); err == nil {
		return nil // Already installed
	}
	fmt.Printf("Installing %s...\n", pkg)
	cmd := exec.Command("brew", "install", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *MacOSAdapter) InstallPHP(version string) error {
	// brew install shivammathur/php/php@version
	pkg := fmt.Sprintf("shivammathur/php/php@%s", version)
	return m.installBrewPackage(pkg)
}

func (m *MacOSAdapter) InstallNode(version string) error {
	// Ensure fnm is managed
	if _, err := exec.LookPath("fnm"); err != nil {
		if err := m.installBrewPackage("fnm"); err != nil {
			return err
		}
	}

	fmt.Printf("Installing Node.js v%s via fnm...\n", version)
	cmd := exec.Command("fnm", "install", version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *MacOSAdapter) GetNodePath(version string) (string, error) {
	cmd := exec.Command("fnm", "exec", "--using", version, "which", "node")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("node %s not found: %w", version, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *MacOSAdapter) InstallCertificates() error                          { return nil }
func (m *MacOSAdapter) InstallMkcert() error                                { return nil }
func (m *MacOSAdapter) GenerateCert(homeDir string, domains []string) error { return nil }
func (m *MacOSAdapter) InstallBinary() error                                { return nil }
func (m *MacOSAdapter) Uninstall() error                                    { return nil }

// Config Paths
func (m *MacOSAdapter) getBrewPrefix() string {
	out, err := exec.Command("brew", "--prefix").Output()
	if err != nil {
		return "/usr/local" // Fallback
	}
	return strings.TrimSpace(string(out))
}

func (m *MacOSAdapter) GetNginxConfigPath() string {
	return filepath.Join(m.getBrewPrefix(), "etc", "nginx", "sld-sites.conf")
}

func (m *MacOSAdapter) WriteNginxConfig(config string) error {
	path := m.GetNginxConfigPath()

	// Create directory if missing
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(config), 0644); err != nil {
		return err
	}

	// Ensure this file is included in the main nginx.conf
	mainConfig := filepath.Join(m.getBrewPrefix(), "etc", "nginx", "nginx.conf")

	// Check if include exists
	content, _ := os.ReadFile(mainConfig)
	if !strings.Contains(string(content), "sld-sites.conf") {
		fmt.Println("Adding include to Nginx config...")
		// Append to http block (simplistic approach, ideally parse config)
		// Assuming standard brew config structure requires manual intervention or smart regex
		// For now, let's just warn or try simple append before last }
		fmt.Printf("Warning: Please manually add 'include %s;' to your http block in %s\n", path, mainConfig)
	}

	return nil
}

func (m *MacOSAdapter) ReloadNginx() error {
	return exec.Command("sudo", "nginx", "-s", "reload").Run()
}

func (m *MacOSAdapter) CheckPHPSocket(version string) (string, error) {
	// macOS with brew doesn't use sockets by default, usually 127.0.0.1:90xx
	// But shivammathur/php uses sockets in usual locations or ports.
	// Let's assume we use ports for macOS to avoid permission issues with socket files
	// Or check if service is running

	// Check brew services
	running, _ := m.IsServiceRunning("php@" + version)
	if !running {
		return "", fmt.Errorf("php@%s not running", version)
	}

	// Return a stub socket address or valid port string that our Nginx generator understands
	// If daemon.go uses this string for `fastcgi_pass`, we must return format `127.0.0.1:90xx`
	// Typically php@8.2 -> 127.0.0.1:9082
	// We need a mapping logic similar to valet
	verFloat := 0.0
	fmt.Sscanf(version, "%f", &verFloat)
	// Example: 8.1 -> 9081, 7.4 -> 9074
	// Remove dots: 8.1 -> 81
	compact := strings.ReplaceAll(version, ".", "")
	port := "90" + compact

	return "127.0.0.1:" + port, nil
}

func (m *MacOSAdapter) GetPHPVersion() string {
	out, err := exec.Command("php", "-v").Output()
	if err != nil {
		return ""
	}
	return string(out) // Parse if needed
}

func (m *MacOSAdapter) ListPHPVersions() ([]string, error) {
	// brew list | grep php
	out, err := exec.Command("bash", "-c", "brew list | grep 'php@'").Output()
	if err != nil {
		return []string{}, nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var versions []string
	for _, line := range lines {
		// php@8.2 -> 8.2
		parts := strings.Split(line, "@")
		if len(parts) == 2 {
			versions = append(versions, parts[1])
		}
	}
	return versions, nil
}

func (m *MacOSAdapter) UpdateHosts(domains []string) error {
	// Simple implementation: Append to /etc/hosts if not present
	// Requires sudo

	for _, domain := range domains {
		// Check invalid chars
		if strings.Contains(domain, "/") || strings.Contains(domain, " ") {
			continue
		}

		// Grep check
		checkCmd := exec.Command("grep", domain, "/etc/hosts")
		if err := checkCmd.Run(); err != nil {
			// Not found (exit status 1), append it
			fmt.Printf("Adding %s to /etc/hosts (requires sudo)...\n", domain)
			line := fmt.Sprintf("127.0.0.1 %s", domain)
			// sudo sh -c "echo '...' >> /etc/hosts"
			updateCmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo '%s' >> /etc/hosts", line))
			updateCmd.Stdout = os.Stdout
			updateCmd.Stderr = os.Stderr
			if err := updateCmd.Run(); err != nil {
				return fmt.Errorf("failed to update hosts for %s: %w", domain, err)
			}
		}
	}
	return nil
}

func (m *MacOSAdapter) AddWebUserToGroup(group string) error {
	// On macOS, users are usually in staff/admin groups already
	// Nginx runs as user or root.
	// We can stub this or try `dseditgroup` but usually not needed for valet-like setup
	return nil
}

func (m *MacOSAdapter) RestartPHP() error {
	// Restart all php services
	versions, _ := m.ListPHPVersions()
	for _, v := range versions {
		exec.Command("brew", "services", "restart", "php@"+v).Run()
	}
	return nil
}

func (m *MacOSAdapter) CheckWifi() (bool, string) { return true, "Unknown" }
func (m *MacOSAdapter) Doctor() error             { return nil }
func (m *MacOSAdapter) GetLogPaths() map[string]string {
	prefix := "/usr/local"
	if runtime.GOARCH == "arm64" {
		prefix = "/opt/homebrew"
	}
	return map[string]string{
		"nginx_access": filepath.Join(prefix, "var/log/nginx/access.log"),
		"nginx_error":  filepath.Join(prefix, "var/log/nginx/error.log"),
		"php_fpm":      filepath.Join(prefix, "var/log/php-fpm.log"),
	}
}
func (m *MacOSAdapter) GetServices() ([]adapters.ServiceStatus, error) {
	services := []adapters.ServiceStatus{}

	// Core Services
	core := []string{"nginx", "dnsmasq"}
	for _, name := range core {
		// On macOS, it might be just 'nginx' or 'nginx-full' depending on install,
		// but IsServiceRunning handles "brew services list" checks generally?
		// Actually IsServiceRunning implementation needs to be robust.
		// Assuming "nginx" works for homebrew formula name.
		running, _ := m.IsServiceRunning(name)
		services = append(services, adapters.ServiceStatus{
			Name:    name,
			Running: running,
		})
	}

	// PHP Services
	phpVersions, _ := m.ListPHPVersions()
	for _, v := range phpVersions {
		// Brew service name is usually php@8.2
		svcName := "php@" + v
		running, _ := m.IsServiceRunning(svcName)
		services = append(services, adapters.ServiceStatus{
			Name:    svcName,
			Running: running,
			Version: v,
		})
	}

	return services, nil
}
func (m *MacOSAdapter) GetSystemHealth() ([]adapters.HealthCheck, error) {
	return []adapters.HealthCheck{}, nil
}
