package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var SupportedPHPVersions = []string{"8.1", "8.2", "8.3", "8.4"}

// LoadFromBytes parses and validates raw YAML configuration bytes.
func LoadFromBytes(data []byte, defaultName string) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("malformed YAML configuration: %w", err)
	}

	if cfg.Version != 1 {
		return nil, fmt.Errorf("invalid or missing configuration version: %d (must be 1)", cfg.Version)
	}

	ApplyDefaults(&cfg, defaultName)

	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadFromFile reads and parses the .sld/config.yaml file from a project root.
func LoadFromFile(projectRoot string) (*Config, error) {
	configPath := filepath.Join(projectRoot, ".sld", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	defaultName := filepath.Base(projectRoot)
	return LoadFromBytes(data, defaultName)
}

// ApplyDefaults applies standard sensible defaults to optional fields.
func ApplyDefaults(cfg *Config, defaultName string) {
	if cfg.Name == "" {
		if defaultName != "" {
			cfg.Name = defaultName
		} else {
			cfg.Name = "project"
		}
	}

	if cfg.Runtime.WebRoot == "" {
		cfg.Runtime.WebRoot = "public"
	}

	if cfg.Domain.Name == "" {
		cfg.Domain.Name = fmt.Sprintf("%s.test", cfg.Name)
	}

	if cfg.Services.Database.Type != "" && cfg.Services.Database.Version == "" {
		switch strings.ToLower(cfg.Services.Database.Type) {
		case "mysql":
			cfg.Services.Database.Version = "8.0"
		case "postgres", "postgresql":
			cfg.Services.Database.Version = "16"
		case "mariadb":
			cfg.Services.Database.Version = "10.11"
		}
	}

	if cfg.Services.Redis.Enabled && cfg.Services.Redis.Version == "" {
		cfg.Services.Redis.Version = "7-alpine"
	}

	if cfg.Node.DevServer.Enabled {
		if cfg.Node.DevServer.Command == "" {
			cfg.Node.DevServer.Command = "npm run dev"
		}
		if cfg.Node.DevServer.Port == 0 {
			cfg.Node.DevServer.Port = 5173
		}
		if cfg.Node.DevServer.ExposePort == 0 {
			cfg.Node.DevServer.ExposePort = 5173
		}
	}
}

// Validate ensures all configuration fields adhere to SLD V2 rules.
func Validate(cfg *Config) error {
	if cfg.Runtime.PHP == "" {
		return fmt.Errorf("runtime.php is required. Supported versions: %s", strings.Join(SupportedPHPVersions, ", "))
	}

	supported := false
	for _, v := range SupportedPHPVersions {
		if cfg.Runtime.PHP == v {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Errorf("unsupported PHP version: %q. Supported versions: %s", cfg.Runtime.PHP, strings.Join(SupportedPHPVersions, ", "))
	}

	if cfg.Services.Database.Type != "" {
		switch strings.ToLower(cfg.Services.Database.Type) {
		case "mysql", "postgres", "postgresql", "mariadb":
			// Valid
		default:
			return fmt.Errorf("unsupported database type: %q. Supported: mysql, postgres, mariadb", cfg.Services.Database.Type)
		}
	}

	return nil
}

// ResolvePHPConstraint maps a composer.json PHP constraint to an approved SLD image version.
// Under NO circumstance will SLD silently downgrade an unsupported version.
func ResolvePHPConstraint(constraint string) (string, error) {
	c := strings.TrimSpace(constraint)
	if c == "" {
		return "", fmt.Errorf("empty PHP constraint")
	}

	// Direct matching
	for _, v := range SupportedPHPVersions {
		if c == v || c == "^"+v || c == "~"+v || strings.HasPrefix(c, "^"+v+".") || strings.HasPrefix(c, "~"+v+".") {
			return v, nil
		}
	}

	// Greater-than / range expressions
	if strings.Contains(c, ">=8.2") || strings.Contains(c, ">= 8.2") {
		return "8.4", nil // highest supported compatible version
	}
	if strings.Contains(c, ">=8.1") || strings.Contains(c, ">= 8.1") {
		return "8.4", nil
	}
	if strings.Contains(c, ">=8.3") || strings.Contains(c, ">= 8.3") {
		return "8.4", nil
	}

	// Explicit check for legacy/unsupported versions (< 8.0, ^7.4, etc.)
	if strings.Contains(c, "7.") || strings.Contains(c, "5.") || strings.HasPrefix(c, "<8") || strings.HasPrefix(c, "< 8") {
		return "", fmt.Errorf("project requires PHP %s, but SLD V2 only supports PHP 8.1, 8.2, 8.3, 8.4. Please upgrade project dependencies", c)
	}

	return "", fmt.Errorf("unable to resolve PHP constraint %q to supported SLD versions (%s)", c, strings.Join(SupportedPHPVersions, ", "))
}

// ComputeConfigHash generates a deterministic SHA-256 hash of the configuration content.
func ComputeConfigHash(data []byte) string {
	hasher := sha256.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}
