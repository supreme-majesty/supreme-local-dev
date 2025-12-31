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
		// strict-order: query strict order (not needed if only one upstream)
		// bind-interfaces: listen only on specified address (crucial for systemd-resolved coexistence)
		// listen-address: 127.0.0.1 (avoid binding specific interface or 0.0.0.0)
		// resolv-file: usage of real upstream to avoid loop with systemd-resolved stub
		// Use static upstream DNS servers instead of /run/systemd/resolve/resolv.conf
		// This allows .test domains to resolve even when offline
		dnsConf := `address=/.test/127.0.0.1
bind-interfaces
listen-address=127.0.0.1
no-resolv
server=8.8.8.8
server=1.1.1.1
`
		tmpFile := "/tmp/sld-dnsmasq.conf"
		os.WriteFile(tmpFile, []byte(dnsConf), 0644)
		exec.Command("sudo", "mv", tmpFile, "/etc/dnsmasq.d/sld.conf").Run()
		exec.Command("sudo", "systemctl", "restart", "dnsmasq").Run()

		// Configure systemd-resolved to route .test to 127.0.0.1
		// We use a drop-in file
		resolvedConf := `[Resolve]
DNS=127.0.0.1
Domains=~test
`
		tmpResolved := "/tmp/sld-resolved.conf"
		os.WriteFile(tmpResolved, []byte(resolvedConf), 0644)

		exec.Command("sudo", "mkdir", "-p", "/etc/systemd/resolved.conf.d").Run()
		exec.Command("sudo", "mv", tmpResolved, "/etc/systemd/resolved.conf.d/sld.conf").Run()
		exec.Command("sudo", "systemctl", "restart", "systemd-resolved").Run()

		// Add sld.test to /etc/hosts for reliable offline access
		// /etc/hosts is consulted first, bypassing DNS entirely
		if err := l.ensureHostsEntry("sld.test"); err != nil {
			fmt.Printf("Warning: Failed to add sld.test to /etc/hosts: %v\n", err)
		}

		return nil
	}
	return fmt.Errorf("package manager not supported (only apt-get implemented for now)")
}

// ensureHostsEntry adds a hostname to /etc/hosts if not already present
func (l *LinuxAdapter) ensureHostsEntry(hostname string) error {
	hostsPath := "/etc/hosts"
	entry := fmt.Sprintf("127.0.0.1 %s", hostname)

	// Read current hosts file
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	// Check if entry already exists
	if strings.Contains(string(data), hostname) {
		return nil // Already present
	}

	// Append entry using sudo
	tmpFile := "/tmp/sld-hosts-entry"
	newContent := string(data)
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += entry + "\n"

	if err := os.WriteFile(tmpFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write temp hosts file: %w", err)
	}

	if err := exec.Command("sudo", "mv", tmpFile, hostsPath).Run(); err != nil {
		return fmt.Errorf("failed to update hosts file: %w", err)
	}

	fmt.Printf("Added %s to /etc/hosts for offline access\n", hostname)
	return nil
}

func (l *LinuxAdapter) InstallCertificates() error {
	// 1. Get mkcert Root CA path
	out, err := exec.Command("mkcert", "-CAROOT").Output()
	if err != nil {
		return fmt.Errorf("failed to get mkcert CA path: %w", err)
	}
	caRoot := strings.TrimSpace(string(out))
	caFile := filepath.Join(caRoot, "rootCA.pem")

	// 2. Define search paths
	home, _ := os.UserHomeDir()
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		home = filepath.Join("/home", sudoUser)
	}

	searchPaths := []string{
		filepath.Join(home, ".pki"),
		filepath.Join(home, ".mozilla"),
		filepath.Join(home, "snap"),
	}

	fmt.Println("Scanning for browser databases to trust...")

	// 3. Walk and find cert9.db
	for _, root := range searchPaths {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip errors
			}
			if info.Name() == "cert9.db" {
				dbDir := filepath.Dir(path)
				fmt.Printf("Trusting CA in %s\n", dbDir)

				// 3a. Try to delete existing cert (to avoid SEC_ERROR_ADDING_CERT)
				exec.Command("certutil", "-d", "sql:"+dbDir, "-D", "-n", "supremelocaldev").Run()

				// 3b. Add new cert
				// certutil -d sql:DIR -A -t "C,," -n "supremelocaldev" -i CAFILE
				// We must use "sql:" prefix
				cmd := exec.Command("certutil", "-d", "sql:"+dbDir, "-A", "-t", "C,,", "-n", "supremelocaldev", "-i", caFile)
				// Certutil might fail if DB is locked or read-only, we try best effort
				if out, err := cmd.CombinedOutput(); err != nil {
					fmt.Printf("Warning: Failed to trust in %s: %v (%s)\n", dbDir, err, string(out))
				}
			}
			return nil
		})
	}
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

func (l *LinuxAdapter) GenerateCert(homeDir string, domains []string) error {
	sudoUser := os.Getenv("SUDO_USER")

	// 1. Install CA if needed (mkcert -install checks itself)
	// Run as user if possible to hit user's browsers
	var installCmd *exec.Cmd
	if sudoUser != "" {
		installCmd = exec.Command("sudo", "-u", sudoUser, "mkcert", "-install")
	} else {
		installCmd = exec.Command("mkcert", "-install")
		if os.Getuid() != 0 {
			installCmd.Stdin = os.Stdin
		}
	}

	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr

	if err := installCmd.Run(); err != nil {
		fmt.Printf("Warning: mkcert -install failed: %v\n", err)
	}

	// 2. Generate certs to temporary location
	tempDir, err := os.MkdirTemp("", "sld-certs")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir) // cleanup

	// Fix ownership of tempDir so user can write to it
	if sudoUser != "" {
		exec.Command("chown", "-R", sudoUser, tempDir).Run()
	}

	certName := "dev.pem"
	keyName := "dev-key.pem"
	certPath := filepath.Join(tempDir, certName)
	keyPath := filepath.Join(tempDir, keyName)

	// Base domains
	// Note: *.test wildcard doesn't match "sld.test" itself, only subdomains like "app.test"
	// We must explicitly include sld.test for the dashboard to work over HTTPS
	args := []string{"mkcert", "-cert-file", certPath, "-key-file", keyPath, "*.test", "sld.test", "test.test", "localhost", "127.0.0.1", "::1"}
	// Append custom domains
	args = append(args, domains...)

	var genCmd *exec.Cmd
	if sudoUser != "" {
		// Prepend sudo -u user
		genArgs := append([]string{"-u", sudoUser}, args...)
		genCmd = exec.Command("sudo", genArgs...)
	} else {
		// args included "mkcert", remove it for direct call?
		// exec.Command takes (name, args...). "mkcert" is in args[0].
		// wait, args defined above has "mkcert" as first element? Yes.
		genCmd = exec.Command(args[0], args[1:]...)
	}

	genCmd.Stdout = os.Stdout
	genCmd.Stderr = os.Stderr
	genCmd.Stdin = os.Stdin
	if err := genCmd.Run(); err != nil {
		return fmt.Errorf("mkcert generation failed: %w", err)
	}

	// 3. Move to system location
	finalDir := "/var/lib/sld/certs"
	fmt.Printf("Installing certificates to %s...\n", finalDir)

	// mkdir -p
	if err := exec.Command("sudo", "mkdir", "-p", finalDir).Run(); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	// copy files
	if err := exec.Command("sudo", "cp", certPath, filepath.Join(finalDir, certName)).Run(); err != nil {
		return fmt.Errorf("failed to install cert: %w", err)
	}
	if err := exec.Command("sudo", "cp", keyPath, filepath.Join(finalDir, keyName)).Run(); err != nil {
		return fmt.Errorf("failed to install key: %w", err)
	}

	// chmod valid for nginx reading
	exec.Command("sudo", "chmod", "644", filepath.Join(finalDir, certName)).Run()
	exec.Command("sudo", "chmod", "644", filepath.Join(finalDir, keyName)).Run()

	return nil
}

func (l *LinuxAdapter) InstallBinary() error {
	// Get current binary path
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	dest := "/usr/local/bin/sld"

	// Check if already installed (optional optimization, but cp is fast)
	if exe == dest {
		fmt.Println("Binary already installed in " + dest)
		return nil
	}

	// Copy binary
	fmt.Printf("Installing binary to %s...\n", dest)
	cmd := exec.Command("sudo", "cp", exe, dest)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	// Ensure executable
	return exec.Command("sudo", "chmod", "+x", dest).Run()
}

func (l *LinuxAdapter) Uninstall() error {
	fmt.Println("Removing configuration files...")

	files := []string{
		"/usr/local/bin/sld",
		"/etc/dnsmasq.d/sld.conf",
		"/etc/systemd/resolved.conf.d/sld.conf",
		"/etc/nginx/sites-enabled/sld.conf",
		"/etc/nginx/sites-enabled/sld-ssl.conf",
	}

	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			fmt.Printf("Removing %s...\n", f)
			exec.Command("sudo", "rm", f).Run()
		}
	}

	fmt.Println("Removing data directories...")
	exec.Command("sudo", "rm", "-rf", "/var/lib/sld").Run()

	// Remove user config
	home, _ := os.UserHomeDir()
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		home = filepath.Join("/home", sudoUser)
	}
	exec.Command("rm", "-rf", filepath.Join(home, ".sld")).Run()

	fmt.Println("Restarting services to apply changes...")
	exec.Command("sudo", "systemctl", "restart", "dnsmasq").Run()
	exec.Command("sudo", "systemctl", "restart", "systemd-resolved").Run()
	exec.Command("sudo", "systemctl", "restart", "nginx").Run()

	return nil
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
