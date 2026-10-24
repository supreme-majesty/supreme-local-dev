package v2

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
)

func TestDetectFromComposer_Laravel(t *testing.T) {
	composerJSON := []byte(`{
		"require": {
			"php": "^8.2",
			"laravel/framework": "^11.0"
		}
	}`)

	cfg := &config.Config{}
	detectFromComposer(composerJSON, cfg)

	if cfg.Runtime.PHP != "8.2" {
		t.Errorf("expected PHP 8.2, got %s", cfg.Runtime.PHP)
	}
	if cfg.Runtime.WebRoot != "public" {
		t.Errorf("expected WebRoot public, got %s", cfg.Runtime.WebRoot)
	}
	if cfg.Services.Database.Type != "mysql" {
		t.Errorf("expected mysql, got %s", cfg.Services.Database.Type)
	}
	if !cfg.Services.Redis.Enabled {
		t.Errorf("expected Redis to be enabled for Laravel")
	}
	if !cfg.Services.Mailpit.Enabled {
		t.Errorf("expected Mailpit to be enabled for Laravel")
	}
}

func TestDetectFromComposer_Symfony(t *testing.T) {
	composerJSON := []byte(`{
		"require": {
			"php": "~8.3",
			"symfony/framework-bundle": "^7.0"
		}
	}`)

	cfg := &config.Config{}
	detectFromComposer(composerJSON, cfg)

	if cfg.Runtime.PHP != "8.3" {
		t.Errorf("expected PHP 8.3, got %s", cfg.Runtime.PHP)
	}
	if cfg.Runtime.WebRoot != "public" {
		t.Errorf("expected WebRoot public, got %s", cfg.Runtime.WebRoot)
	}
}

func TestDetectFromPackageJSON(t *testing.T) {
	pkgJSON := []byte(`{
		"engines": {
			"node": ">=20.0.0"
		},
		"scripts": {
			"dev": "vite"
		}
	}`)

	cfg := &config.Config{}
	detectFromPackageJSON(pkgJSON, cfg)

	if cfg.Node.Version != "20" {
		t.Errorf("expected Node version 20, got %s", cfg.Node.Version)
	}
	if !cfg.Node.DevServer.Enabled {
		t.Errorf("expected dev server to be enabled")
	}
}

func TestDetectFromDotEnv(t *testing.T) {
	dotEnv := `
APP_NAME=MyApp
DB_CONNECTION=pgsql
DB_HOST=127.0.0.1
CACHE_DRIVER=redis
`
	cfg := &config.Config{}
	detectFromDotEnv(dotEnv, cfg)

	if cfg.Services.Database.Type != "postgres" {
		t.Errorf("expected postgres, got %s", cfg.Services.Database.Type)
	}
	if cfg.Services.Database.Version != "16" {
		t.Errorf("expected postgres version 16, got %s", cfg.Services.Database.Version)
	}
	if !cfg.Services.Redis.Enabled {
		t.Errorf("expected redis enabled")
	}
}

func TestInitProject_CreatesConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sld-detect-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Write mock composer.json
	composerContent := `{"require":{"php":"^8.4","laravel/framework":"^11.0"}}`
	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(composerContent), 0644); err != nil {
		t.Fatalf("failed to write composer.json: %v", err)
	}

	if err := InitProject(tempDir); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	configFile := filepath.Join(tempDir, ".sld", "config.yaml")
	if _, err := os.Stat(configFile); err != nil {
		t.Fatalf("config file was not created at %s", configFile)
	}

	// Calling again should return error that config already exists
	if err := InitProject(tempDir); err == nil {
		t.Errorf("expected error when config already exists, got nil")
	}
}
