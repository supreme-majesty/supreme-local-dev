package tls

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCertManager_EnsureWildcardCert_SelfSigned(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sld-cert-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm, err := NewCertManager(tempDir)
	if err != nil {
		t.Fatalf("NewCertManager failed: %v", err)
	}

	// Ensure wildcard certificate for *.test
	if err := cm.EnsureWildcardCert("test"); err != nil {
		t.Fatalf("EnsureWildcardCert failed: %v", err)
	}

	// Verify cert file exists
	certPath := cm.CertPath()
	if !fileExists(certPath) {
		t.Fatalf("cert file does not exist at %s", certPath)
	}

	// Verify key file exists
	keyPath := cm.KeyPath()
	if !fileExists(keyPath) {
		t.Fatalf("key file does not exist at %s", keyPath)
	}

	// Verify traefik config file exists
	traefikConf := cm.TraefikConfigPath()
	if !fileExists(traefikConf) {
		t.Fatalf("traefik config file does not exist at %s", traefikConf)
	}

	// Verify cert contents
	certData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("failed to read cert file: %v", err)
	}

	block, _ := pem.Decode(certData)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("invalid certificate PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	if cert.Subject.CommonName != "*.test" {
		t.Errorf("expected CommonName *.test, got %s", cert.Subject.CommonName)
	}

	if !cert.NotAfter.After(time.Now()) {
		t.Errorf("certificate is already expired")
	}

	// Check idempotency - calling again should reuse valid cert
	if err := cm.EnsureWildcardCert("test"); err != nil {
		t.Fatalf("subsequent EnsureWildcardCert call failed: %v", err)
	}
}

func TestCertManager_TraefikConfigContent(t *testing.T) {
	tempDir := t.TempDir()
	cm, err := NewCertManager(tempDir)
	if err != nil {
		t.Fatalf("NewCertManager failed: %v", err)
	}

	if err := cm.writeTraefikConfig(); err != nil {
		t.Fatalf("writeTraefikConfig failed: %v", err)
	}

	data, err := os.ReadFile(cm.TraefikConfigPath())
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	content := string(data)
	if filepath.Ext(cm.TraefikConfigPath()) != ".yml" {
		t.Errorf("expected .yml extension")
	}

	expectedCertRef := "/etc/traefik/certs/wildcard.crt"
	if !containsString(content, expectedCertRef) {
		t.Errorf("expected config to reference %s, got:\n%s", expectedCertRef, content)
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || (len(haystack) > len(needle) && hasSubstring(haystack, needle)))
}

func hasSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
