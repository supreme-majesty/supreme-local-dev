package v2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectConfig_Success(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sld-runner-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sldDir := filepath.Join(tempDir, ".sld")
	if err := os.MkdirAll(sldDir, 0755); err != nil {
		t.Fatalf("failed to create .sld dir: %v", err)
	}

	configContent := `version: 1
name: "my-test-app"
runtime:
  php: "8.3"
  web_root: "public"
domain:
  name: "my-test-app.test"
  tls: true
services:
  database:
    type: "mysql"
    version: "8.0"
  redis:
    enabled: true
  mailpit:
    enabled: true
`
	if err := os.WriteFile(filepath.Join(sldDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	cfg, data, err := loadProjectConfig(tempDir)
	if err != nil {
		t.Fatalf("loadProjectConfig failed: %v", err)
	}

	if cfg.Name != "my-test-app" {
		t.Errorf("expected name my-test-app, got %s", cfg.Name)
	}
	if cfg.Runtime.PHP != "8.3" {
		t.Errorf("expected PHP 8.3, got %s", cfg.Runtime.PHP)
	}
	if !cfg.Domain.TLS {
		t.Errorf("expected TLS true")
	}
	if len(data) == 0 {
		t.Errorf("expected raw data, got empty")
	}
}

func TestLoadProjectConfig_NotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sld-runner-empty-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	_, _, err = loadProjectConfig(tempDir)
	if err == nil {
		t.Errorf("expected error for missing config, got nil")
	}
}
