package dns

import (
	"os"
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

}

func TestHostsLine(t *testing.T) {
	if got := HostsLine(" shop.test "); got != "127.0.0.1 shop.test" {
		t.Errorf("HostsLine = %q", got)
	}
}
