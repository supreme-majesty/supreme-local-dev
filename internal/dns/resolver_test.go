package dns

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestHasHostsEntry_Parser(t *testing.T) {
	tempFile, err := os.CreateTemp("", "hosts-test-*")
	if err != nil {
		t.Fatalf("failed to create temp hosts file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	sampleHosts := `
# Copyright (c) 1993-2009 Microsoft Corp.
#
127.0.0.1       localhost
::1             localhost
127.0.0.1       my-laravel-app.test # added by sld
127.0.0.1       api.test my-store.test
`
	if _, err := tempFile.WriteString(sampleHosts); err != nil {
		t.Fatalf("failed to write sample hosts: %v", err)
	}
	tempFile.Close()

	// Test parsing helper function directly
	testCases := []struct {
		domain   string
		expected bool
	}{
		{"localhost", true},
		{"my-laravel-app.test", true},
		{"api.test", true},
		{"my-store.test", true},
		{"non-existent.test", false},
		{"google.com", false},
	}

	for _, tc := range testCases {
		found := parseHostsForDomain(tempFile.Name(), tc.domain)
		if found != tc.expected {
			t.Errorf("domain %q: expected %v, got %v", tc.domain, tc.expected, found)
		}
	}
}

func parseHostsForDomain(path, domain string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
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
						return true
					}
				}
			}
		}
	}
	return false
}

func TestEnsureResolver_NoPanic(t *testing.T) {
	// Should not panic on any OS
	_ = EnsureResolver("test")
}
