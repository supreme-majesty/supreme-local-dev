package dns

import (
	"bufio"
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

// HostsFilePath returns the path to the system's hosts file.
func HostsFilePath() string {
	if runtime.GOOS == "windows" {
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = "C:\\Windows"
		}
		return fmt.Sprintf("%s\\System32\\drivers\\etc\\hosts", systemRoot)
	}
	return "/etc/hosts"
}

// CheckResolution tests whether a domain resolves to localhost (127.0.0.1 or ::1).
func CheckResolution(domain string) (bool, error) {
	// Use a short timeout resolver
	resolver := &net.Resolver{
		PreferGo: true,
	}

	addrs, err := resolver.LookupHost(contextWithTimeout(2*time.Second), domain)
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
	path := HostsFilePath()
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
	exists, err := HasHostsEntry(domain)
	if err == nil && exists {
		return nil // Already present
	}

	path := HostsFilePath()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open hosts file for writing (run with administrative privileges): %w", err)
	}
	defer file.Close()

	entry := fmt.Sprintf("\n127.0.0.1 %s # added by sld\n", domain)
	_, err = file.WriteString(entry)
	return err
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

	case "linux":
		// On Linux with systemd-resolved, or dnsmasq, or hosts
		return nil

	case "windows":
		// On Windows, test domain resolution
		return nil

	default:
		return nil
	}
}

func contextWithTimeout(d time.Duration) netContext {
	ctx, cancel := newTimeoutContext(d)
	_ = cancel
	return ctx
}

type netContext interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
}

type timeoutCtx struct {
	deadline time.Time
	done     chan struct{}
	err      error
}

func newTimeoutContext(d time.Duration) (*timeoutCtx, func()) {
	c := &timeoutCtx{
		deadline: time.Now().Add(d),
		done:     make(chan struct{}),
	}
	timer := time.AfterFunc(d, func() {
		c.err = fmt.Errorf("context deadline exceeded")
		close(c.done)
	})
	return c, func() { timer.Stop() }
}

func (c *timeoutCtx) Deadline() (time.Time, bool) { return c.deadline, true }
func (c *timeoutCtx) Done() <-chan struct{}       { return c.done }
func (c *timeoutCtx) Err() error                  { return c.err }
func (c *timeoutCtx) Value(key any) any           { return nil }
