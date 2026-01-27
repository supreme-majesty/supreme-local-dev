package linux

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/pkg/adapters"
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
		// Base packages
		packages := []string{
			"nginx", "php-fpm", "dnsmasq", "zip", "unzip",
			"composer",
			"php-mysql", "php-mbstring", "php-xml", "php-curl",
			"php-zip", "php-sqlite3", "php-bcmath", "php-intl",
		}

		// Check specific packages to avoid conflicts or redundancies
		// Git
		if _, err := exec.LookPath("git"); err != nil {
			packages = append(packages, "git")
		}
		// Node.js (implies npm usually)
		if _, err := exec.LookPath("node"); err != nil {
			packages = append(packages, "nodejs")
		} else if _, err := exec.LookPath("npm"); err != nil {
			// Only install npm if node is there but npm isn't (rare, but possible on some distros)
			// Actually, let's just stick to nodejs, installing 'npm' explicit often conflicts
		}

		// Check for Database (MySQL or MariaDB)
		if _, err := exec.LookPath("mysql"); err != nil {
			if _, err := exec.LookPath("mariadb"); err != nil {
				fmt.Println("Database not found, adding mariadb-server...")
				packages = append(packages, "mariadb-server")
			}
		}

		// Check for Redis
		if _, err := exec.LookPath("redis-server"); err != nil {
			fmt.Println("Redis not found, adding redis-server...")
			packages = append(packages, "redis-server")
		}

		// Install packages
		args := append([]string{"apt-get", "install", "-y"}, packages...)
		cmd := exec.Command("sudo", args...)
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
address=/.test/::1
bind-interfaces
listen-address=127.0.0.1
listen-address=::1
no-resolv
local=/test/
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

func (l *LinuxAdapter) InstallPHP(version string) error {
	// 1. Check if PPA is needed (Ubuntu/Debian)
	// For simplicity, we assume user has add-apt-repository or similar,
	// checking if we can just install.
	// We'll proceed with direct install attempt.

	packageName := fmt.Sprintf("php%s-fpm", version)
	fmt.Printf("Attempting to install %s...\n", packageName)

	// Update apt cache first? Maybe too slow.
	// Let's rely on it being somewhat fresh or apt failing.

	cmd := exec.Command("sudo", "apt-get", "install", "-y",
		packageName,
		fmt.Sprintf("php%s-mysql", version),
		fmt.Sprintf("php%s-mbstring", version),
		fmt.Sprintf("php%s-xml", version),
		fmt.Sprintf("php%s-curl", version),
		fmt.Sprintf("php%s-zip", version),
		fmt.Sprintf("php%s-sqlite3", version),
		fmt.Sprintf("php%s-bcmath", version),
		fmt.Sprintf("php%s-intl", version),
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install %s: %w", packageName, err)
	}

	fmt.Printf("%s installed successfully! 🐘\n", packageName)
	return nil
}

// InstallNode installs a specific Node.js version using fnm
func (l *LinuxAdapter) InstallNode(version string) error {
	// Ensure fnm is installed
	if _, err := exec.LookPath("fnm"); err != nil {
		fmt.Println("Installing fnm (Fast Node Manager)...")
		// Install fnm via script to /usr/local/bin
		cmd := exec.Command("bash", "-c", "curl -fsSL https://fnm.vercel.app/install | bash -s -- --install-dir /usr/local/bin --skip-shell")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install fnm: %w", err)
		}
	}

	fmt.Printf("Installing Node.js v%s via fnm...\n", version)
	// fnm install <version>
	cmd := exec.Command("fnm", "install", version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install node %s: %w", version, err)
	}

	return nil
}

// GetNodePath returns the path to the node binary for a specific version
func (l *LinuxAdapter) GetNodePath(version string) (string, error) {
	// Check if version is installed
	cmd := exec.Command("fnm", "exec", "--using", version, "which", "node")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("node %s not found (or fnm error): %w", version, err)
	}
	return strings.TrimSpace(string(out)), nil
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

func (l *LinuxAdapter) UpdateHosts(domains []string) error {
	hostsPath := "/etc/hosts"
	startMarker := "# SLD-START"
	endMarker := "# SLD-END"

	// Read current hosts file
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}
	content := string(data)

	// Prepare new block
	var sb strings.Builder
	sb.WriteString(startMarker + "\n")
	// Always include sld.test
	sb.WriteString("127.0.0.1 sld.test\n")
	sb.WriteString("::1 sld.test\n")

	for _, d := range domains {
		if d == "sld.test" {
			continue
		}
		sb.WriteString(fmt.Sprintf("127.0.0.1 %s\n", d))
		sb.WriteString(fmt.Sprintf("::1 %s\n", d))
	}
	sb.WriteString(endMarker)
	newBlock := sb.String()

	// Replace or Append
	var newContent string
	startIndex := strings.Index(content, startMarker)
	endIndex := strings.Index(content, endMarker)

	if startIndex != -1 && endIndex != -1 && endIndex > startIndex {
		// Replace existing block
		// We need to look for newline after endMarker to keep it clean
		suffix := content[endIndex+len(endMarker):]
		newContent = content[:startIndex] + newBlock + suffix
	} else {
		// Append if not found (ensure newline before)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		newContent = content + newBlock + "\n"
	}

	fmt.Printf("Updating hosts file with %d domains...\n", len(domains))

	// Write to temp and mv
	tmpFile := "/tmp/sld-hosts-update"
	if err := os.WriteFile(tmpFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write temp hosts file: %w", err)
	}

	// Move file - check if we are root
	var cmd *exec.Cmd
	if os.Getuid() == 0 {
		cmd = exec.Command("mv", tmpFile, hostsPath)
	} else {
		cmd = exec.Command("sudo", "mv", tmpFile, hostsPath)
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update hosts file: %w (output: %s)", err, string(out))
	}

	fmt.Println("Hosts file updated successfully.")
	return nil
}

func (l *LinuxAdapter) InstallCertificates() error {
	// 1. Get mkcert Root CA path
	var cmd *exec.Cmd
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		cmd = exec.Command("sudo", "-u", sudoUser, "mkcert", "-CAROOT")
	} else {
		cmd = exec.Command("mkcert", "-CAROOT")
	}

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get mkcert CA path: %w", err)
	}
	caRoot := strings.TrimSpace(string(out))
	caFile := filepath.Join(caRoot, "rootCA.pem")

	// 2. Define search paths
	home := l.getRealUserHome()

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

func (l *LinuxAdapter) ListPHPVersions() ([]string, error) {
	// Use dpkg-query to find installed php*-fpm packages
	// We use a broad pattern and then filter numerically in Go
	cmd := "dpkg-query -W -f='${Package} ${Status}\n' 'php*-fpm' | grep ' ok installed' | cut -d' ' -f1"
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		// Fallback to checking sockets if dpkg fails or returns nothing
		files, _ := filepath.Glob("/run/php/php[0-9].[0-9]-fpm.sock")
		var versions []string
		for _, f := range files {
			ver := strings.TrimPrefix(filepath.Base(f), "php")
			ver = strings.TrimSuffix(ver, "-fpm.sock")
			versions = append(versions, ver)
		}
		return versions, nil
	}

	var versions []string
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Extract version: php8.1-fpm -> 8.1
		ver := strings.TrimPrefix(line, "php")
		ver = strings.TrimSuffix(ver, "-fpm")
		if ver != "" && ver != line {
			versions = append(versions, ver)
		}
	}

	// Sort versions descending (newest first)
	for i := 0; i < len(versions); i++ {
		for j := i + 1; j < len(versions); j++ {
			vI, _ := strconv.ParseFloat(versions[i], 64)
			vJ, _ := strconv.ParseFloat(versions[j], 64)
			if vJ > vI {
				versions[i], versions[j] = versions[j], versions[i]
			}
		}
	}

	return versions, nil
}

func (l *LinuxAdapter) ReloadNginx() error {
	// 1. Test configuration first
	if err := exec.Command("sudo", "nginx", "-t").Run(); err != nil {
		return fmt.Errorf("nginx configuration test failed: %w", err)
	}

	// 2. Try reload (safer)
	if err := exec.Command("sudo", "nginx", "-s", "reload").Run(); err != nil {
		fmt.Printf("Nginx reload failed: %v. Falling back to restart...\n", err)
		// 3. Fallback to restart if reload fails
		if err := l.RestartService("nginx"); err != nil {
			fmt.Println("Nginx restart failed. Attempting to free Port 80 and retry...")
			if err := l.FreePort80(); err != nil {
				fmt.Printf("Warning: Failed to free port 80: %v\n", err)
			}
			return l.RestartService("nginx")
		}
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
	home := l.getRealUserHome()
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

func (l *LinuxAdapter) getRealUserHome() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			return u.HomeDir
		}
		// Fallback for non-standard environments
		return filepath.Join("/home", sudoUser)
	}
	h, _ := os.UserHomeDir()
	return h
}
func (l *LinuxAdapter) CheckWifi() (bool, string) {
	// 1. Check nmcli for wifi status if available
	if path, err := exec.LookPath("nmcli"); err == nil && path != "" {
		out, err := exec.Command("nmcli", "radio", "wifi").Output()
		if err == nil {
			status := strings.TrimSpace(string(out))
			if status == "enabled" {
				// Check if connected to something
				out, err = exec.Command("nmcli", "-t", "-f", "ACTIVE,SSID", "dev", "wifi").Output()
				if err == nil {
					lines := strings.Split(string(out), "\n")
					for _, line := range lines {
						if strings.HasPrefix(line, "yes:") {
							return true, strings.TrimPrefix(line, "yes:")
						}
					}
				}
				return true, "Enabled but not connected"
			}
			return false, "WiFi Disabled"
		}
	}

	// 2. Fallback: check ip addr for wlan interface status
	out, err := exec.Command("ip", "addr").Output()
	if err == nil {
		content := string(out)
		if strings.Contains(content, "wlan") || strings.Contains(content, "wlp") {
			if strings.Contains(content, "state UP") {
				return true, "Connected (via IP link)"
			}
			return false, "Interface DOWN"
		}
	}

	return false, "No WiFi interface detected"
}

func (l *LinuxAdapter) Doctor() error {
	fmt.Println("🏥 SLD System Health Check")
	fmt.Println("--------------------------")

	// Check Services
	services := []string{"nginx", "dnsmasq", "systemd-resolved"}
	for _, s := range services {
		running, err := l.IsServiceRunning(s)
		status := "🔴 STOPPED"
		if err == nil && running {
			status = "🟢 RUNNING"
		}
		fmt.Printf("%-18s: %s\n", s, status)
	}

	// Check PHP-FPM
	phpVer := l.GetPHPVersion()
	phpSvc := fmt.Sprintf("php%s-fpm", phpVer)
	phpRunning, _ := l.IsServiceRunning(phpSvc)
	phpStatus := "🔴 STOPPED"
	if phpRunning {
		phpStatus = "🟢 RUNNING"
	}
	fmt.Printf("%-18s: %s (PHP %s)\n", phpSvc, phpStatus, phpVer)

	// Check Connectivity
	wifiAlive, wifiMsg := l.CheckWifi()
	wifiStatus := "🔴 OFFLINE"
	if wifiAlive {
		wifiStatus = "🟢 ONLINE"
	}
	fmt.Printf("%-18s: %s (%s)\n", "WiFi Status", wifiStatus, wifiMsg)

	// Check .test resolution
	cmd := exec.Command("resolvectl", "query", "sld.test")
	if err := cmd.Run(); err != nil {
		fmt.Printf("%-18s: 🔴 FAILED (systemd-resolved not resolving .test)\n", ".test Resolution")
	} else {
		fmt.Printf("%-18s: 🟢 WORKING\n", ".test Resolution")
	}

	return nil
}

func (l *LinuxAdapter) GetLogPaths() map[string]string {
	logs := make(map[string]string)
	logs["nginx_error"] = "/var/log/nginx/error.log"
	logs["nginx_access"] = "/var/log/nginx/access.log"

	// Try to find specific php log
	ver := l.GetPHPVersion()
	logs["php_fpm"] = fmt.Sprintf("/var/log/php%s-fpm.log", ver)

	return logs
}

// Structured Status Implementation

func (l *LinuxAdapter) GetServices() ([]adapters.ServiceStatus, error) {
	services := []adapters.ServiceStatus{}

	// Nginx
	nginxRunning, _ := l.IsServiceRunning("nginx")
	services = append(services, adapters.ServiceStatus{
		Name:    "Nginx",
		Running: nginxRunning,
		Version: "Unknown", // Could parse nginx -v
	})

	// PHP-FPM
	phpVer := l.GetPHPVersion()
	phpSvc := fmt.Sprintf("php%s-fpm", phpVer)
	phpRunning, _ := l.IsServiceRunning(phpSvc)
	services = append(services, adapters.ServiceStatus{
		Name:    "PHP-FPM",
		Running: phpRunning,
		Version: phpVer,
	})

	// DNSMasq
	dnsRunning, _ := l.IsServiceRunning("dnsmasq")
	services = append(services, adapters.ServiceStatus{
		Name:    "DNSMasq",
		Running: dnsRunning,
	})

	// Core Services
	core := []string{"nginx", "dnsmasq"}
	for _, name := range core {
		running, _ := l.IsServiceRunning(name)
		services = append(services, adapters.ServiceStatus{
			Name:    name,
			Running: running,
		})
	}

	// PHP Services
	phpVersions, _ := l.ListPHPVersions()
	for _, v := range phpVersions {
		svcName := fmt.Sprintf("php%s-fpm", v)
		running, _ := l.IsServiceRunning(svcName)
		services = append(services, adapters.ServiceStatus{
			Name:    svcName,
			Running: running,
			Version: v,
		})
	}

	return services, nil
}

func (l *LinuxAdapter) GetSystemHealth() ([]adapters.HealthCheck, error) {
	checks := []adapters.HealthCheck{}

	// 1. Services Check
	svcs, _ := l.GetServices()
	for _, s := range svcs {
		status := "fail"
		msg := "Stopped"
		if s.Running {
			status = "pass"
			msg = "Running"
		}
		checks = append(checks, adapters.HealthCheck{
			Name:    s.Name,
			Status:  status,
			Message: msg,
		})
	}

	// 2. Connectivity
	online, netMsg := l.CheckWifi()
	netStatus := "fail"
	if online {
		netStatus = "pass"
	}
	checks = append(checks, adapters.HealthCheck{
		Name:    "Network",
		Status:  netStatus,
		Message: netMsg,
	})

	return checks, nil
}
