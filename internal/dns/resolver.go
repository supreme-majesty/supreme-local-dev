package dns

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultTLD   = "test"
	SLDTagMarker = "# added by sld"
)

// IsWSL2 reports whether the current Linux process is executing inside Windows Subsystem for Linux.
func IsWSL2() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	v := strings.ToLower(string(data))
	return strings.Contains(v, "microsoft") || strings.Contains(v, "wsl")
}

// HostsFilePath returns the path to the system's hosts file.
// On WSL2, prefers the Windows hosts file when accessible so host browsers resolve *.test domains.
func HostsFilePath() string {
	if runtime.GOOS == "windows" {
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = "C:\\Windows"
		}
		return fmt.Sprintf("%s\\System32\\drivers\\etc\\hosts", systemRoot)
	}
	if IsWSL2() {
		winHosts := "/mnt/c/Windows/System32/drivers/etc/hosts"
		if _, err := os.Stat(winHosts); err == nil {
			return winHosts
		}
	}
	return "/etc/hosts"
}

// CheckResolution tests whether a domain resolves to localhost (127.0.0.1 or ::1).
func CheckResolution(domain string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resolver := &net.Resolver{
		PreferGo: true,
	}

	addrs, err := resolver.LookupHost(ctx, domain)
	if err != nil {
		return false, err
	}

	for _, addr := range addrs {
		if addr == "127.0.0.1" || addr == "::1" {
			return true, nil
		}
	}

	return false, nil
}

// HasHostsEntry checks if the system hosts file contains a mapping for the domain.
func HasHostsEntry(domain string) (bool, error) {
	return HasHostsEntryInFile(HostsFilePath(), domain)
}

// HasHostsEntryInFile checks whether a specific hosts file maps domain to loopback.
func HasHostsEntryInFile(path, domain string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("failed to open hosts file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Strip inline comments before matching hosts
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 {
			ip := fields[0]
			if ip == "127.0.0.1" || ip == "::1" {
				for _, host := range fields[1:] {
					if strings.EqualFold(host, domain) {
						return true, nil
					}
				}
			}
		}
	}

	return false, scanner.Err()
}

// AddHostsEntry appends a loopback entry for the domain to the system hosts file.
func AddHostsEntry(domain string) error {
	return AddHostsEntryToFile(HostsFilePath(), domain)
}

// AddHostsEntryToFile appends a loopback entry for domain to the given hosts file if not present.
func AddHostsEntryToFile(path, domain string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	exists, err := HasHostsEntryInFile(path, domain)
	if err == nil && exists {
		return nil // Already present
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("cannot write %s (run with elevated privileges or add '127.0.0.1 %s' manually): %w", path, domain, err)
	}
	defer file.Close()

	entry := fmt.Sprintf("\n127.0.0.1 %s %s\n", domain, SLDTagMarker)
	_, err = file.WriteString(entry)
	return err
}

// RemoveHostsEntry removes SLD-managed hosts entries for the domain from the system hosts file.
func RemoveHostsEntry(domain string) error {
	return RemoveHostsEntryFromFile(HostsFilePath(), domain)
}

// RemoveHostsEntryFromFile removes SLD-managed loopback lines matching domain from the given hosts file.
func RemoveHostsEntryFromFile(path, domain string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	kept := make([]string, 0, len(lines))
	removed := false

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if strings.Contains(trimmed, SLDTagMarker) {
			noComment := trimmed
			if idx := strings.Index(noComment, "#"); idx >= 0 {
				noComment = strings.TrimSpace(noComment[:idx])
			}
			fields := strings.Fields(noComment)
			if len(fields) >= 2 && (fields[0] == "127.0.0.1" || fields[0] == "::1") && strings.EqualFold(fields[1], domain) {
				removed = true
				continue
			}
		}
		kept = append(kept, rawLine)
	}

	if !removed {
		return nil
	}

	return os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0644)
}

// EnsureDomain verifies that domain resolves to loopback, falling back to adding a hosts entry.
// Returns (true, nil) if a hosts entry was newly added.
func EnsureDomain(domain string) (bool, error) {
	if resolved, _ := CheckResolution(domain); resolved {
		return false, nil
	}
	if has, _ := HasHostsEntry(domain); has {
		return false, nil
	}
	if err := AddHostsEntry(domain); err != nil {
		return false, err
	}
	return true, nil
}

// EnsureResolver performs OS-specific setup for local *.test domain resolution.
func EnsureResolver(tld string) error {
	if tld == "" {
		tld = DefaultTLD
	}

	switch runtime.GOOS {
	case "darwin":
		// On macOS, create /etc/resolver/<tld> pointing to loopback
		resolverDir := "/etc/resolver"
		if err := os.MkdirAll(resolverDir, 0755); err != nil {
			return fmt.Errorf("failed to create %s (may require sudo): %w", resolverDir, err)
		}
		resolverFile := fmt.Sprintf("%s/%s", resolverDir, tld)
		content := "nameserver 127.0.0.1\nport 53\n"
		return os.WriteFile(resolverFile, []byte(content), 0644)

	case "linux", "windows":
		// On Linux and Windows/WSL2, MVP strategy uses per-domain hosts sync on project start
		// to avoid port 53 collisions with systemd-resolved or WSL2 NAT.
		return nil

	default:
		return nil
	}
}
