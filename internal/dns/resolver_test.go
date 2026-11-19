package dns

import (
	"os"
	"strings"
	"testing"
)

func TestHostsEntryLifecycle(t *testing.T) {
	tempFile, err := os.CreateTemp("", "hosts-test-*")
	if err != nil {
		t.Fatalf("failed to create temp hosts file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	sampleHosts := `# Sample hosts
127.0.0.1       localhost
::1             localhost
127.0.0.1       my-laravel-app.test # added by sld
127.0.0.1       api.test my-store.test
`
	if _, err := tempFile.WriteString(sampleHosts); err != nil {
		t.Fatalf("failed to write sample hosts: %v", err)
	}
	tempFile.Close()

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
		found, err := HasHostsEntryInFile(tempFile.Name(), tc.domain)
		if err != nil {
			t.Fatalf("HasHostsEntryInFile(%q) error: %v", tc.domain, err)
		}
		if found != tc.expected {
			t.Errorf("domain %q: expected %v, got %v", tc.domain, tc.expected, found)
		}
	}

	// Add new entry (idempotent)
	if err := AddHostsEntryToFile(tempFile.Name(), "shop.test"); err != nil {
		t.Fatalf("AddHostsEntryToFile failed: %v", err)
	}
	if err := AddHostsEntryToFile(tempFile.Name(), "shop.test"); err != nil {
		t.Fatalf("second AddHostsEntryToFile failed: %v", err)
	}
	found, err := HasHostsEntryInFile(tempFile.Name(), "shop.test")
	if err != nil || !found {
		t.Fatalf("expected shop.test to exist after add (found=%v, err=%v)", found, err)
	}

	// Ensure duplicate wasn't written twice
	content, _ := os.ReadFile(tempFile.Name())
	if strings.Count(string(content), "shop.test") != 1 {
		t.Errorf("expected exactly 1 occurrence of shop.test, got %d", strings.Count(string(content), "shop.test"))
	}

	// Remove SLD-managed entry
	if err := RemoveHostsEntryFromFile(tempFile.Name(), "shop.test"); err != nil {
		t.Fatalf("RemoveHostsEntryFromFile failed: %v", err)
	}
	found, _ = HasHostsEntryInFile(tempFile.Name(), "shop.test")
	if found {
		t.Errorf("expected shop.test to be removed")
	}

	// Non-SLD entries should NOT be removed
	if err := RemoveHostsEntryFromFile(tempFile.Name(), "api.test"); err != nil {
		t.Fatalf("RemoveHostsEntryFromFile(api.test) error: %v", err)
	}
	found, _ = HasHostsEntryInFile(tempFile.Name(), "api.test")
	if !found {
		t.Errorf("expected non-SLD managed api.test to remain untouched")
	}
}

func TestEnsureResolver_NoPanic(t *testing.T) {
	_ = EnsureResolver("test")
}
