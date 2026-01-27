package windows

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters"
)

type WindowsAdapter struct{}

func NewWindowsAdapter() *WindowsAdapter {
	return &WindowsAdapter{}
}

// Service Management (sc.exe or simple process check)
func (w *WindowsAdapter) StartService(name string) error {
	// Windows services usually handled via 'net start' or 'sc start'
	return exec.Command("net", "start", name).Run()
}

func (w *WindowsAdapter) StopService(name string) error {
	return exec.Command("net", "stop", name).Run()
}

func (w *WindowsAdapter) RestartService(name string) error {
	w.StopService(name)
	return w.StartService(name)
}

func (w *WindowsAdapter) IsServiceRunning(name string) (bool, error) {
	// sc query "name"
	out, err := exec.Command("sc", "query", name).Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), "RUNNING"), nil
}

// Installation
func (w *WindowsAdapter) InstallDependencies() error {
	fmt.Println("Checking for Winget...")
	if _, err := exec.LookPath("winget"); err != nil {
		fmt.Println("Winget not found. Please install App Installer from Microsoft Store.")
		return fmt.Errorf("winget required")
	}

	packages := []string{
		"Nginx.Nginx",
		"Schniz.fnm",
		"FiloSottile.mkcert",
	}

	fmt.Println("Installing core packages via Winget...")
	for _, pkg := range packages {
		if err := w.installWingetPackage(pkg); err != nil {
			// Continue on error? users might have things installed manually
			fmt.Printf("Warning: Failed to install %s: %v\n", pkg, err)
		}
	}

	return nil
}

func (w *WindowsAdapter) installWingetPackage(pkg string) error {
	// winget list check is slow, maybe verify binary existence?
	// Keep it simple: install --accept-path-agreements
	fmt.Printf("Installing %s...\n", pkg)
	// winget install -e --id <pkg> --accept-source-agreements --accept-package-agreements
	cmd := exec.Command("winget", "install", "-e", "--id", pkg, "--accept-source-agreements", "--accept-package-agreements")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (w *WindowsAdapter) InstallPHP(version string) error {
	// Windows PHP installation is tricky. Usually "php" is one version.
	// We might need "tools" for multi-version.
	// For now, let's just warn or use a scope if available.
	// There is no standard "php switch" on Windows without tools like Laragon or manual PATH manipulation.
	// But we can extract zips to C:\tools\php<ver>
	fmt.Println("Windows PHP multi-version installation not yet automated. Please install PHP manually.")
	return nil
}

func (w *WindowsAdapter) InstallNode(version string) error {
	// Ensure fnm
	if _, err := exec.LookPath("fnm"); err != nil {
		w.installWingetPackage("Schniz.fnm")
	}

	fmt.Printf("Installing Node.js v%s via fnm...\n", version)
	// On Windows, fnm usually needs setup in profile.
	cmd := exec.Command("fnm", "install", version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (w *WindowsAdapter) GetNodePath(version string) (string, error) {
	// fnm exec ... where node
	cmd := exec.Command("fnm", "exec", "--using", version, "where", "node")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("node %s not found: %w", version, err)
	}
	// 'where' might return multiple lines, take first
	lines := strings.Split(strings.TrimSpace(string(out)), "\r\n")
	if len(lines) > 0 {
		return lines[0], nil
	}
	return "", fmt.Errorf("node binary path parse failed")
}

// Config & Runtime
func (w *WindowsAdapter) GetNginxConfigPath() string {
	// Guess standard location or define one
	return "C:\\Program Files\\nginx\\conf\\sld-sites.conf"
}

func (w *WindowsAdapter) WriteNginxConfig(config string) error {
	path := w.GetNginxConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(config), 0644)
}

func (w *WindowsAdapter) ReloadNginx() error {
	return exec.Command("nginx", "-s", "reload").Run()
}

func (w *WindowsAdapter) CheckPHPSocket(version string) (string, error) {
	// Windows uses TCP ports usually, e.g. 127.0.0.1:9000
	// We assume manually managed PHP-CGI processes
	// Return typical port mapping
	// 8.2 -> 9082
	verFloat := 0.0
	fmt.Sscanf(version, "%f", &verFloat)
	compact := strings.ReplaceAll(version, ".", "")
	port := "90" + compact

	// Verification logic missing on Windows for processes listening on ports
	// but assuming standard config
	return "127.0.0.1:" + port, nil
}

func (w *WindowsAdapter) GetPHPVersion() string {
	out, err := exec.Command("php", "-v").Output()
	if err == nil {
		return string(out)
	}
	return ""
}

func (w *WindowsAdapter) ListPHPVersions() ([]string, error) {
	return []string{"8.2", "8.1"}, nil // Stub
}

// System
func (w *WindowsAdapter) UpdateHosts(domains []string) error {
	// Requires Admin
	// Read, check, append
	// ...
	return nil
}

func (w *WindowsAdapter) InstallCertificates() error                          { return nil }
func (w *WindowsAdapter) InstallMkcert() error                                { return nil }
func (w *WindowsAdapter) GenerateCert(homeDir string, domains []string) error { return nil }
func (w *WindowsAdapter) InstallBinary() error                                { return nil }
func (w *WindowsAdapter) Uninstall() error                                    { return nil }
func (w *WindowsAdapter) AddWebUserToGroup(group string) error                { return nil }
func (w *WindowsAdapter) RestartPHP() error                                   { return nil }
func (w *WindowsAdapter) CheckWifi() (bool, string)                           { return true, "Unknown" }
func (w *WindowsAdapter) Doctor() error                                       { return nil }
func (w *WindowsAdapter) GetLogPaths() map[string]string {
	// Assuming standard install paths or derived from env
	nginxHome := os.Getenv("NGINX_HOME")
	if nginxHome == "" {
		nginxHome = `C:\Program Files\nginx`
	}
	return map[string]string{
		"nginx_access": filepath.Join(nginxHome, "logs", "access.log"),
		"nginx_error":  filepath.Join(nginxHome, "logs", "error.log"),
		"php_error":    `C:\tools\php\error.log`, // Example
	}
}
func (w *WindowsAdapter) GetServices() ([]adapters.ServiceStatus, error) {
	services := []adapters.ServiceStatus{}

	// Core
	// Nginx on Windows is often just a process "nginx.exe", but if installed via valid tools it might be a service "nginx".
	// Let's check both or assume service for now as per StartService implementation.
	running, _ := w.IsServiceRunning("nginx")
	services = append(services, adapters.ServiceStatus{
		Name:    "nginx",
		Running: running,
	})

	// PHP
	// On Windows, PHP is often run as FastCGI process, not a service.
	// But we can check if "php-cgi.exe" is running or a named service exists.
	// For consistency with other adapters, we'll list versions.
	phpVersions, _ := w.ListPHPVersions()
	for _, v := range phpVersions {
		svcName := fmt.Sprintf("php-%s", v) // e.g. php-8.2
		running, _ := w.IsServiceRunning(svcName)
		services = append(services, adapters.ServiceStatus{
			Name:    svcName,
			Running: running,
			Version: v,
		})
	}

	return services, nil
}

func (w *WindowsAdapter) GetSystemHealth() ([]adapters.HealthCheck, error) {
	// Stub
	return []adapters.HealthCheck{}, nil
}
