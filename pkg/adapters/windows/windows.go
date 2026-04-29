package windows

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters"
	"github.com/supreme-majesty/supreme-local-dev/pkg/util"
)

type WindowsAdapter struct {
	runner *util.Runner
	log    *util.Logger
}

func NewWindowsAdapter() *WindowsAdapter {
	return &WindowsAdapter{
		runner: util.NewRunner(false),
		log:    &util.Logger{},
	}
}

// Service Management (sc.exe or simple process check)
func (w *WindowsAdapter) StartService(name string) error {
	return w.runner.Run("net", "start", name)
}

func (w *WindowsAdapter) StopService(name string) error {
	return w.runner.Run("net", "stop", name)
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
	w.log.Info("Automating PHP %s installation for Windows...", version)
	
	// 1. Define paths
	toolsDir := `C:\tools`
	phpDir := filepath.Join(toolsDir, "php"+version)
	os.MkdirAll(toolsDir, 0755)

	if _, err := os.Stat(phpDir); err == nil {
		w.log.Info("PHP %s is already installed in %s", version, phpDir)
		return nil
	}

	// 2. Map version to download URL (simplified mapping)
	// In a real pro app, we'd fetch the latest version numbers from windows.php.net/downloads/
	downloadURL := fmt.Sprintf("https://windows.php.net/downloads/releases/php-%s-Win32-vs16-x64.zip", version)
	if version == "8.2" {
		downloadURL = "https://windows.php.net/downloads/releases/php-8.2.12-Win32-vs16-x64.zip"
	}
	
	zipPath := filepath.Join(os.TempDir(), "php.zip")
	w.log.Info("Downloading PHP from %s...", downloadURL)
	if err := util.DownloadFile(zipPath, downloadURL); err != nil {
		return fmt.Errorf("failed to download PHP: %w", err)
	}

	// 3. Extract using PowerShell
	w.log.Info("Extracting PHP to %s...", phpDir)
	os.MkdirAll(phpDir, 0755)
	extractCmd := fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s' -Force", zipPath, phpDir)
	if err := w.runner.Run("powershell", "-Command", extractCmd); err != nil {
		return fmt.Errorf("failed to extract PHP: %w", err)
	}

	// 4. Create php.ini from development template
	iniPath := filepath.Join(phpDir, "php.ini")
	if _, err := os.Stat(iniPath); os.IsNotExist(err) {
		w.log.Info("Creating php.ini from template...")
		w.runner.Run("powershell", "-Command", fmt.Sprintf("Copy-Item -Path '%s' -Destination '%s'", filepath.Join(phpDir, "php.ini-development"), iniPath))
	}

	w.log.Success("PHP %s installed successfully!", version)
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
	fmt.Println("Updating Windows hosts file (requires Admin)...")
	hostsPath := `C:\Windows\System32\drivers\etc\hosts`
	
	// Prepare lines to add
	var lines []string
	for _, d := range domains {
		lines = append(lines, fmt.Sprintf("127.0.0.1 %s", d))
	}
	
	if len(lines) == 0 {
		return nil
	}

	// Use powershell to check and append
	for _, line := range lines {
		domain := strings.Fields(line)[1]
		// Check if domain already exists
		checkCmd := exec.Command("powershell", "-Command", fmt.Sprintf("Select-String -Path %s -Pattern '%s'", hostsPath, domain))
		if err := checkCmd.Run(); err != nil {
			// Not found, append
			fmt.Printf("Adding %s to hosts...\n", domain)
			appendCmd := exec.Command("powershell", "-Command", fmt.Sprintf("Add-Content -Path %s -Value '`n%s' -ErrorAction Stop", hostsPath, line))
			if err := appendCmd.Run(); err != nil {
				return fmt.Errorf("failed to update hosts: %w", err)
			}
		}
	}
	return nil
}

func (w *WindowsAdapter) InstallCertificates() error {
	return exec.Command("mkcert", "-install").Run()
}

func (w *WindowsAdapter) InstallMkcert() error {
	return w.installWingetPackage("FiloSottile.mkcert")
}

func (w *WindowsAdapter) GenerateCert(homeDir string, domains []string) error {
	appData := os.Getenv("APPDATA")
	finalDir := filepath.Join(appData, "sld", "certs")
	os.MkdirAll(finalDir, 0755)

	certPath := filepath.Join(finalDir, "dev.pem")
	keyPath := filepath.Join(finalDir, "dev-key.pem")

	args := []string{"-cert-file", certPath, "-key-file", keyPath, "*.test", "sld.test", "localhost", "127.0.0.1", "::1"}
	args = append(args, domains...)
	
	return exec.Command("mkcert", args...).Run()
}

func (w *WindowsAdapter) InstallBinary() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	destDir := `C:\Program Files\sld`
	dest := filepath.Join(destDir, "sld.exe")
	
	os.MkdirAll(destDir, 0755)
	fmt.Printf("Installing binary to %s...\n", dest)
	
	// Copy
	input, _ := os.ReadFile(exe)
	err = os.WriteFile(dest, input, 0755)
	if err != nil {
		// Might need admin/copy command
		return exec.Command("powershell", "-Command", fmt.Sprintf("Copy-Item -Path '%s' -Destination '%s' -Force", exe, dest)).Run()
	}
	
	// Add to PATH if not present
	exec.Command("powershell", "-Command", fmt.Sprintf("[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';%s', 'Machine')", destDir)).Run()
	
	return nil
}

func (w *WindowsAdapter) Uninstall() error {
	fmt.Println("Uninstalling SLD from Windows...")
	destDir := `C:\Program Files\sld`
	return os.RemoveAll(destDir)
}
func (w *WindowsAdapter) AddWebUserToGroup(group string) error                { return nil }
func (w *WindowsAdapter) RestartPHP() error                                   { return nil }
func (w *WindowsAdapter) CheckWifi() (bool, string) {
	out, err := exec.Command("netsh", "wlan", "show", "interfaces").Output()
	if err != nil {
		return false, "Not connected"
	}
	// Simple SSID extraction
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "SSID") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				return true, strings.TrimSpace(parts[1])
			}
		}
	}
	return false, "No SSID found"
}

func (w *WindowsAdapter) Doctor() error {
	fmt.Println("🏥 SLD Windows Health Check")
	// Check if nginx process is running
	out, _ := exec.Command("tasklist", "/FI", "IMAGENAME eq nginx.exe").Output()
	if strings.Contains(string(out), "nginx.exe") {
		fmt.Println("🟢 nginx.exe is running")
	} else {
		fmt.Println("🔴 nginx.exe is not running")
	}
	return nil
}
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
