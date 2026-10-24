package v2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
)

// InitProject auto-detects the project framework and generates .sld/config.yaml.
func InitProject(projectRoot string) error {
	configDir := filepath.Join(projectRoot, ".sld")
	configPath := filepath.Join(configDir, "config.yaml")

	// Check if already exists
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf(".sld/config.yaml already exists at %s\n  Edit it directly or delete it to regenerate", configPath)
	}

	projectName := filepath.Base(projectRoot)
	fmt.Printf("[SLD] Detecting project type for: %s\n", projectName)

	cfg := &config.Config{
		Version: 1,
		Name:    projectName,
		Runtime: config.RuntimeConfig{
			WebRoot: "public",
		},
		Domain: config.DomainConfig{
			Name: fmt.Sprintf("%s.test", projectName),
		},
	}

	// Auto-detect from composer.json
	composerPath := filepath.Join(projectRoot, "composer.json")
	if data, err := os.ReadFile(composerPath); err == nil {
		detectFromComposer(data, cfg)
	}

	// Auto-detect from package.json
	packagePath := filepath.Join(projectRoot, "package.json")
	if data, err := os.ReadFile(packagePath); err == nil {
		detectFromPackageJSON(data, cfg)
	}

	// Default PHP if not detected
	if cfg.Runtime.PHP == "" {
		cfg.Runtime.PHP = "8.3"
		fmt.Println("[SLD] No PHP version detected, defaulting to 8.3")
	}

	// Default services for Laravel
	if cfg.Services.Database.Type == "" {
		// Check if .env exists with DB config
		envPath := filepath.Join(projectRoot, ".env")
		if data, err := os.ReadFile(envPath); err == nil {
			detectFromDotEnv(string(data), cfg)
		}
	}

	// Apply defaults
	config.ApplyDefaults(cfg, projectName)

	// Write config
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create .sld directory: %w", err)
	}

	output, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	header := "# SLD V2 Project Configuration\n# Auto-generated — edit as needed\n# Documentation: https://github.com/supreme-majesty/supreme-local-dev\n\n"

	if err := os.WriteFile(configPath, []byte(header+string(output)), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Println()
	fmt.Printf("[SLD] ✅ Created %s\n", configPath)
	fmt.Println()
	printConfigSummary(cfg)
	fmt.Println()
	fmt.Println("[SLD] Run 'sld v2 start' to launch the development environment.")

	return nil
}

// detectFromComposer reads composer.json to detect PHP version and framework.
func detectFromComposer(data []byte, cfg *config.Config) {
	var composer struct {
		Require map[string]string `json:"require"`
	}

	if err := json.Unmarshal(data, &composer); err != nil {
		return
	}

	// PHP version
	if phpConstraint, ok := composer.Require["php"]; ok {
		resolved, err := config.ResolvePHPConstraint(phpConstraint)
		if err == nil {
			cfg.Runtime.PHP = resolved
			fmt.Printf("[SLD] Detected PHP %s (from composer.json: %s)\n", resolved, phpConstraint)
		} else {
			fmt.Printf("[SLD] ⚠️  %v\n", err)
		}
	}

	// Laravel detection
	if _, ok := composer.Require["laravel/framework"]; ok {
		fmt.Println("[SLD] Detected Laravel project")
		cfg.Runtime.WebRoot = "public"

		// Enable standard Laravel services
		if cfg.Services.Database.Type == "" {
			cfg.Services.Database.Type = "mysql"
			cfg.Services.Database.Version = "8.0"
		}
		cfg.Services.Redis.Enabled = true
		cfg.Services.Mailpit.Enabled = true
	}

	// Symfony detection
	if _, ok := composer.Require["symfony/framework-bundle"]; ok {
		fmt.Println("[SLD] Detected Symfony project")
		cfg.Runtime.WebRoot = "public"
	}

	// WordPress detection
	if _, ok := composer.Require["roots/wordpress"]; ok {
		fmt.Println("[SLD] Detected WordPress (Bedrock) project")
		cfg.Runtime.WebRoot = "web"
		if cfg.Services.Database.Type == "" {
			cfg.Services.Database.Type = "mysql"
			cfg.Services.Database.Version = "8.0"
		}
	}
}

// detectFromPackageJSON reads package.json to detect Node.js requirements.
func detectFromPackageJSON(data []byte, cfg *config.Config) {
	var pkg struct {
		Engines struct {
			Node string `json:"node"`
		} `json:"engines"`
		Scripts map[string]string `json:"scripts"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return
	}

	// Node version
	if pkg.Engines.Node != "" {
		version := strings.TrimLeft(pkg.Engines.Node, ">=^~v ")
		version = strings.Split(version, ".")[0] // Major version only
		cfg.Node.Version = version
		fmt.Printf("[SLD] Detected Node %s requirement\n", version)
	}

	// Dev server detection (Vite, Next.js, etc.)
	if devCmd, ok := pkg.Scripts["dev"]; ok {
		if strings.Contains(devCmd, "vite") || strings.Contains(devCmd, "next") {
			cfg.Node.DevServer.Enabled = true
			cfg.Node.DevServer.Command = "npm run dev"
			fmt.Println("[SLD] Detected dev server script")
		}
	}
}

// detectFromDotEnv reads .env to detect database configuration.
func detectFromDotEnv(content string, cfg *config.Config) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "DB_CONNECTION":
			switch value {
			case "mysql":
				cfg.Services.Database.Type = "mysql"
				cfg.Services.Database.Version = "8.0"
			case "pgsql":
				cfg.Services.Database.Type = "postgres"
				cfg.Services.Database.Version = "16"
			case "mariadb":
				cfg.Services.Database.Type = "mariadb"
				cfg.Services.Database.Version = "10.11"
			}
		case "CACHE_DRIVER", "CACHE_STORE":
			if value == "redis" {
				cfg.Services.Redis.Enabled = true
			}
		}
	}
}

// printConfigSummary displays a human-readable summary of the generated config.
func printConfigSummary(cfg *config.Config) {
	fmt.Printf("  Project:   %s\n", cfg.Name)
	fmt.Printf("  Domain:    %s\n", cfg.Domain.Name)
	fmt.Printf("  PHP:       %s\n", cfg.Runtime.PHP)
	fmt.Printf("  Web Root:  %s\n", cfg.Runtime.WebRoot)

	if cfg.Services.Database.Type != "" {
		fmt.Printf("  Database:  %s %s\n", cfg.Services.Database.Type, cfg.Services.Database.Version)
	}
	if cfg.Services.Redis.Enabled {
		fmt.Println("  Redis:     enabled")
	}
	if cfg.Services.Mailpit.Enabled {
		fmt.Println("  Mailpit:   enabled")
	}
	if cfg.Node.DevServer.Enabled {
		fmt.Printf("  Node:      %s (dev server)\n", cfg.Node.Version)
	}
}
