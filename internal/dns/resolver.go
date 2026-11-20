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
	DefaultTLD = "test"
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

// HostsLine is the hosts-file line that makes domain resolve to this
// computer. SLD never edits the hosts file itself; it prints this line.
func HostsLine(domain string) string {
	return "127.0.0.1 " + strings.TrimSpace(domain)
}

// NeedsHostsEntry reports whether domain neither resolves to loopback nor has
// a hosts entry, i.e. whether the user should add HostsLine(domain).
func NeedsHostsEntry(domain string) bool {
	if resolved, _ := CheckResolution(domain); resolved {
		return false
	}
	has, _ := HasHostsEntry(domain)
	return !has
}
