package v2

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
)

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

func TestInitProject_WritesValidVersion2(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-shop")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"composer.json": `{"require":{"php":"^8.4","laravel/framework":"^11.0"}}`,
		"package.json":  `{"engines":{"node":">=20"},"scripts":{"dev":"vite"}}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := InitProject(dir); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".sld", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromBytes(data, "my-shop")
	if err != nil {
		t.Fatalf("generated config does not load: %v\n%s", err, data)
	}
	if cfg.SourceVersion != config.CurrentVersion {
		t.Errorf("init-project should write version 2, got source version %d", cfg.SourceVersion)
	}
	if got := cfg.Describe(); got != "web (php 8.4), vite (node 20)" {
		t.Errorf("apps = %s", got)
	}
	if cfg.Services.Database.Type != "mysql" || !cfg.Services.Redis.Enabled {
		t.Errorf("Laravel services not detected: %+v", cfg.Services)
	}
}

func TestConfigMigrate(t *testing.T) {
	dir := t.TempDir()
	sld := filepath.Join(dir, ".sld")
	if err := os.MkdirAll(sld, 0o755); err != nil {
		t.Fatal(err)
	}
	v1 := "version: 1\nname: blog\nruntime:\n  php: \"8.2\"\ndomain:\n  name: blog.test\n"
	if err := os.WriteFile(filepath.Join(sld, "config.yaml"), []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ConfigMigrate(dir); err != nil {
		t.Fatalf("ConfigMigrate: %v", err)
	}
	backup, err := os.ReadFile(filepath.Join(sld, "config.v1.yaml.bak"))
	if err != nil || string(backup) != v1 {
		t.Fatalf("backup missing or altered: %v\n%s", err, backup)
	}
	cfg, _, err := loadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SourceVersion != config.CurrentVersion || cfg.Domain.Name != "blog.test" || cfg.Describe() != "web (php 8.2)" {
		t.Errorf("migrated config: source v%d domain %s apps %s", cfg.SourceVersion, cfg.Domain.Name, cfg.Describe())
	}

	// Running again is a no-op and must not touch the backup.
	if err := ConfigMigrate(dir); err != nil {
		t.Errorf("second migrate should be a no-op, got %v", err)
	}
}

func TestConfigMigrate_RefusesInvalidFile(t *testing.T) {
	dir := t.TempDir()
	sld := filepath.Join(dir, ".sld")
	if err := os.MkdirAll(sld, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sld, "config.yaml"), []byte("version: 1\nruntime:\n  php: \"7.4\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ConfigMigrate(dir); err == nil {
		t.Fatal("expected migrate to refuse an invalid config")
	}
	if _, err := os.Stat(filepath.Join(sld, "config.v1.yaml.bak")); !os.IsNotExist(err) {
		t.Error("no backup should be written when migration is refused")
	}
}
