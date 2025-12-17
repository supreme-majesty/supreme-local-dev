package linux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type LinuxAdapter struct {
	// We can store configuration paths here
}

func NewLinuxAdapter() *LinuxAdapter {
	return &LinuxAdapter{}
}

// Service Management using systemctl

func (l *LinuxAdapter) StartService(name string) error {
	return exec.Command("sudo", "systemctl", "start", name).Run()
}

func (l *LinuxAdapter) StopService(name string) error {
	return exec.Command("sudo", "systemctl", "stop", name).Run()
}

func (l *LinuxAdapter) RestartService(name string) error {
	return exec.Command("sudo", "systemctl", "restart", name).Run()
}

func (l *LinuxAdapter) IsServiceRunning(name string) (bool, error) {
	cmd := exec.Command("systemctl", "is-active", name)
	err := cmd.Run()
	if err != nil {
		return false, nil // Not active
	}
	return true, nil
}

// Installation

func (l *LinuxAdapter) InstallDependencies() error {
	// Check for apt-get
	path, err := exec.LookPath("apt-get")
	if err == nil && path != "" {
		// Install nginx, php-fpm, dnsmasq, and essential PHP extensions
		cmd := exec.Command("sudo", "apt-get", "install", "-y",
			"nginx", "php-fpm", "dnsmasq",
			"php-mysql", "php-mbstring", "php-xml", "php-curl",
			"php-zip", "php-sqlite3", "php-bcmath", "php-intl",
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}

		// Configure dnsmasq for .test domain
		dnsConf := "address=/.test/127.0.0.1"
		tmpFile := "/tmp/sld-dnsmasq.conf"
		os.WriteFile(tmpFile, []byte(dnsConf), 0644)
		exec.Command("sudo", "mv", tmpFile, "/etc/dnsmasq.d/sld.conf").Run()
		exec.Command("sudo", "systemctl", "restart", "dnsmasq").Run()

		return nil
	}
	return fmt.Errorf("package manager not supported (only apt-get implemented for now)")
}

func (l *LinuxAdapter) InstallCertificates() error {
	// Placeholder for mkcert
	return nil
}

// Configuration

func (l *LinuxAdapter) WriteNginxConfig(config string) error {
	path := l.GetNginxConfigPath()
	// Write to a temporary file first then move with sudo
	tmpFile := "/tmp/sld-nginx.conf"
	if err := os.WriteFile(tmpFile, []byte(config), 0644); err != nil {
		return err
	}

	// Move to /etc/nginx/sites-available/sld.conf
	cmd := exec.Command("sudo", "mv", tmpFile, path)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Create symlink if not exists
	linkPath := "/etc/nginx/sites-enabled/sld.conf"
	if _, err := os.Stat(linkPath); os.IsNotExist(err) {
		exec.Command("sudo", "ln", "-s", path, linkPath).Run()
	}

	return l.ReloadNginx()
}

func (l *LinuxAdapter) GetNginxConfigPath() string {
	return "/etc/nginx/sites-available/sld.conf"
}

func (l *LinuxAdapter) GetPHPVersion() string {
	// Attempt to detect generic php version
	out, err := exec.Command("php", "-r", "echo PHP_VERSION;").Output()
	if err != nil {
		return "8.1" // Fallback
	}
	ver := strings.TrimSpace(string(out))
	// Parse "8.4.1" -> "8.4"
	parts := strings.Split(ver, ".")
	if len(parts) >= 2 {
		return fmt.Sprintf("%s.%s", parts[0], parts[1])
	}
	return ver
}

func (l *LinuxAdapter) ReloadNginx() error {
	if err := l.RestartService("nginx"); err != nil {
		fmt.Println("Nginx restart failed. Attempting to free Port 80 and retry...")
		// If restart fails, it might be due to port conflict.
		// Try to free port 80 and start again.
		if err := l.FreePort80(); err != nil {
			fmt.Printf("Warning: Failed to free port 80: %v\n", err)
		}
		// Try start instead of restart, or restart again
		return l.RestartService("nginx")
	}
	return nil
}

func (l *LinuxAdapter) AddWebUserToGroup(group string) error {
	// 1. Identify web user (usually www-data)
	webUser := "www-data"
	// In some distros it might be 'http' or 'nginx', but for Ubuntu/Debian it is www-data.
	// We could check /etc/passwd but let's stick to standard for now.

	// 2. Add to group
	// usermod -aG group user
	cmd := exec.Command("sudo", "usermod", "-aG", group, webUser)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add %s to group %s: %w", webUser, group, err)
	}
	return nil
}

func (l *LinuxAdapter) RestartPHP() error {
	// Restart all php-fpm services we can find
	// This is a bit brute-force but ensures the group change is picked up

	// List running units matching php*-fpm
	// systemctl list-units --type=service --state=running | grep php
	// Simplified: just try restarting common versions
	versions := []string{"8.4", "8.3", "8.2", "8.1", "8.0", "7.4"}

	for _, v := range versions {
		service := fmt.Sprintf("php%s-fpm", v)
		if running, _ := l.IsServiceRunning(service); running {
			fmt.Printf("Restarting %s...\n", service)
			l.RestartService(service)
		}
	}
	return nil
}

func (l *LinuxAdapter) FreePort80() error {
	// 1. Stop Apache2 if running (common conflict)
	// We don't check for error because it might not be installed or running
	exec.Command("sudo", "systemctl", "stop", "apache2").Run()

	// 2. Stop XAMPP if present (common user conflict)
	if _, err := os.Stat("/opt/lampp/lampp"); err == nil {
		exec.Command("sudo", "/opt/lampp/lampp", "stopapache").Run()
	}

	// 3. Kill any rogue process on port 80
	// Try using 'fuser' if available
	if path, err := exec.LookPath("fuser"); err == nil && path != "" {
		exec.Command("sudo", "fuser", "-k", "80/tcp").Run()
	} else {
		// Fallback to killall for common web servers
		exec.Command("sudo", "killall", "nginx").Run()
		exec.Command("sudo", "killall", "apache2").Run()
		exec.Command("sudo", "killall", "httpd").Run()
	}

	// Brief pause to allow OS to release socket
	time.Sleep(1 * time.Second)
	return nil
}

// HTTPS Support

func (l *LinuxAdapter) InstallMkcert() error {
	path, err := exec.LookPath("mkcert")
	if err == nil && path != "" {
		return nil // already installed
	}

	// Try installing via apt (if available) or suggest user install it
	// On Ubuntu/Debian 'mkcert' is in recent repos or via brew.
	// For simplicity, let's assume apt install works or fail with message.
	// Actually, `libnss3-tools` is needed for mkcert.

	cmd := exec.Command("sudo", "apt-get", "install", "-y", "mkcert", "libnss3-tools")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (l *LinuxAdapter) GenerateCert(homeDir string) error {
	// 1. Install CA
	installCmd := exec.Command("mkcert", "-install")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("failed to install local CA: %w", err)
	}

	// 2. Generate certs for *.test
	certPath := filepath.Join(homeDir, ".sld", "certs", "current.pem")
	keyPath := filepath.Join(homeDir, ".sld", "certs", "current-key.pem")

	genCmd := exec.Command("mkcert", "-cert-file", certPath, "-key-file", keyPath, "*.test", "test.test", "localhost", "127.0.0.1", "::1")
	genCmd.Stdout = os.Stdout
	genCmd.Stderr = os.Stderr
	return genCmd.Run()
}

func (l *LinuxAdapter) InstallBinary() error {
	// Get current binary path
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	dest := "/usr/local/bin/sld"

	// Check if already installed (optional optimization, but cp is fast)

	// Copy binary
	fmt.Printf("Installing binary to %s...\n", dest)
	cmd := exec.Command("sudo", "cp", exe, dest)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Ensure executable
	return exec.Command("sudo", "chmod", "+x", dest).Run()
}

func (l *LinuxAdapter) CheckPHPSocket(version string) (string, error) {
	// Check common paths
	// Ubuntu/Debian: /run/php/phpX.Y-fpm.sock
	socketPath := fmt.Sprintf("/run/php/php%s-fpm.sock", version)

	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		// Try without /run/ (older systems?)
		// unlikely, but let's stick to /run/php for now or /var/run/php
		socketPathVar := fmt.Sprintf("/var/run/php/php%s-fpm.sock", version)
		if _, err := os.Stat(socketPathVar); os.IsNotExist(err) {
			return "", fmt.Errorf("PHP %s socket not found at %s. Is php%s-fpm installed and running?", version, socketPath, version)
		}
		socketPath = socketPathVar
	}

	return socketPath, nil
}
