package config

import (
	"strings"
	"testing"
)

func TestLoadFromBytes_ValidConfig(t *testing.T) {
	yamlData := `
version: 1
name: "demo-app"
runtime:
  php: "8.3"
  web_root: "public"
domain:
  name: "demo.test"
  tls: true
services:
  database:
    type: "mysql"
  redis:
    enabled: true
node:
  version: "20"
  dev_server:
    enabled: true
xdebug:
  enabled: true
environment:
  APP_ENV: "local"
`

	cfg, err := LoadFromBytes([]byte(yamlData), "default-name")
	if err != nil {
		t.Fatalf("unexpected error loading valid config: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}
	if cfg.Name != "demo-app" {
		t.Errorf("expected name demo-app, got %s", cfg.Name)
	}
	if cfg.Runtime.PHP != "8.3" {
		t.Errorf("expected PHP 8.3, got %s", cfg.Runtime.PHP)
	}
	if cfg.Runtime.WebRoot != "public" {
		t.Errorf("expected web_root public, got %s", cfg.Runtime.WebRoot)
	}
	if cfg.Domain.Name != "demo.test" {
		t.Errorf("expected domain demo.test, got %s", cfg.Domain.Name)
	}
	if !cfg.Domain.TLS {
		t.Errorf("expected TLS true")
	}
	if cfg.Services.Database.Version != "8.0" {
		t.Errorf("expected default mysql version 8.0, got %s", cfg.Services.Database.Version)
	}
	if cfg.Services.Redis.Version != "7-alpine" {
		t.Errorf("expected default redis version 7-alpine, got %s", cfg.Services.Redis.Version)
	}
	if cfg.Node.DevServer.Port != 5173 {
		t.Errorf("expected default dev server port 5173, got %d", cfg.Node.DevServer.Port)
	}
	if !cfg.Xdebug.Enabled {
		t.Errorf("expected xdebug enabled")
	}
	if cfg.Environment["APP_ENV"] != "local" {
		t.Errorf("expected APP_ENV local, got %s", cfg.Environment["APP_ENV"])
	}
}

func TestLoadFromBytes_DefaultsApplied(t *testing.T) {
	yamlData := `
version: 1
runtime:
  php: "8.2"
`

	cfg, err := LoadFromBytes([]byte(yamlData), "my-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Name != "my-service" {
		t.Errorf("expected default name 'my-service', got %s", cfg.Name)
	}
	if cfg.Runtime.WebRoot != "public" {
		t.Errorf("expected default web_root 'public', got %s", cfg.Runtime.WebRoot)
	}
	if cfg.Domain.Name != "my-service.test" {
		t.Errorf("expected default domain 'my-service.test', got %s", cfg.Domain.Name)
	}
	if cfg.Domain.TLS {
		t.Errorf("expected default TLS false")
	}
}

func TestLoadFromBytes_InvalidVersion(t *testing.T) {
	yamlData := `
version: 2
runtime:
  php: "8.3"
`

	_, err := LoadFromBytes([]byte(yamlData), "test")
	if err == nil {
		t.Fatalf("expected error for version != 1, but got nil")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("expected error message mentioning version, got: %v", err)
	}
}

func TestLoadFromBytes_MissingRequiredPHP(t *testing.T) {
	yamlData := `
version: 1
name: "no-php"
`

	_, err := LoadFromBytes([]byte(yamlData), "test")
	if err == nil {
		t.Fatalf("expected error for missing runtime.php, but got nil")
	}
	if !strings.Contains(err.Error(), "runtime.php is required") {
		t.Errorf("expected error to mention runtime.php, got: %v", err)
	}
}

func TestLoadFromBytes_UnsupportedPHP(t *testing.T) {
	yamlData := `
version: 1
runtime:
  php: "7.4"
`

	_, err := LoadFromBytes([]byte(yamlData), "test")
	if err == nil {
		t.Fatalf("expected error for unsupported PHP 7.4, but got nil")
	}
	if !strings.Contains(err.Error(), "unsupported PHP version") {
		t.Errorf("expected unsupported PHP error, got: %v", err)
	}
}

func TestLoadFromBytes_MalformedYAML(t *testing.T) {
	badYAML := `
version: 1
runtime:
  php: [unclosed list
`

	_, err := LoadFromBytes([]byte(badYAML), "test")
	if err == nil {
		t.Fatalf("expected error for malformed YAML, got nil")
	}
}

func TestResolvePHPConstraint(t *testing.T) {
	cases := []struct {
		constraint string
		expected   string
		shouldErr  bool
	}{
		{"^8.3", "8.3", false},
		{"~8.3.0", "8.3", false},
		{"^8.1", "8.1", false},
		{"8.2", "8.2", false},
		{">=8.2", "8.4", false},
		{"^7.4", "", true}, // Fatal error: no silent downgrade
		{"<8.0", "", true}, // Fatal error: no silent downgrade
		{"invalid", "", true},
	}

	for _, c := range cases {
		t.Run(c.constraint, func(t *testing.T) {
			got, err := ResolvePHPConstraint(c.constraint)
			if c.shouldErr {
				if err == nil {
					t.Errorf("expected error for constraint %q, got version %q", c.constraint, got)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for constraint %q: %v", c.constraint, err)
				}
				if got != c.expected {
					t.Errorf("ResolvePHPConstraint(%q) = %q; want %q", c.constraint, got, c.expected)
				}
			}
		})
	}
}
