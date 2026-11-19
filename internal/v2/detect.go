package v2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
)

// InitProject auto-detects the project framework and generates a version 2
// .sld/config.yaml. Detection currently covers PHP projects and their Node
// dev server; detectors for other stacks arrive with Phase 15 M4.
func InitProject(projectRoot string) error {
	configDir := filepath.Join(projectRoot, ".sld")
	configPath := filepath.Join(configDir, "config.yaml")

	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf(".sld/config.yaml already exists at %s\n  Edit it directly or delete it to regenerate", configPath)
	}

	projectName := filepath.Base(projectRoot)
	fmt.Printf("[SLD] Detecting project type for: %s\n", projectName)

	web := config.App{Name: "web", Runtime: config.RuntimePHP, WebRoot: "public"}
	cfg := &config.Config{
		Version: config.CurrentVersion,
		Name:    projectName,
		Domain: config.DomainConfig{
			Name: fmt.Sprintf("%s.%s", project.Slugify(projectName), config.DefaultTLD()),
		},
	}

	if data, err := os.ReadFile(filepath.Join(projectRoot, "composer.json")); err == nil {
		detectFromComposer(data, cfg, &web)
	}
	if web.Version == "" {
		web.Version = "8.3"
		fmt.Println("[SLD] No PHP version detected, defaulting to 8.3")
	}
	cfg.Apps = config.Apps{web}

	if data, err := os.ReadFile(filepath.Join(projectRoot, "package.json")); err == nil {
		detectFromPackageJSON(data, cfg)
	}

	if cfg.Services.Database.Type == "" {
		if data, err := os.ReadFile(filepath.Join(projectRoot, ".env")); err == nil {
			detectFromDotEnv(string(data), cfg)
		}
	}

	output, err := config.Marshal(cfg)
	if err != nil {
		return err
	}
	// Never write a file SLD itself would reject.
	resolved, err := config.LoadFromBytes(output, projectName)
	if err != nil {
		return fmt.Errorf("generated configuration is invalid (please report this): %w", err)
	}

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .sld directory: %w", err)
	}
	if err := os.WriteFile(configPath, output, 0o644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Println()
	fmt.Printf("[SLD] ✅ Created %s\n", configPath)
	fmt.Println()
	printConfigSummary(resolved)
	fmt.Println()
	fmt.Println("[SLD] Run 'sld v2 start' to launch the development environment.")
	return nil
}

// detectFromComposer reads composer.json to detect the PHP version and framework.
func detectFromComposer(data []byte, cfg *config.Config, web *config.App) {
	var composer struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(data, &composer); err != nil {
		return
	}

	if phpConstraint, ok := composer.Require["php"]; ok {
		resolved, err := config.ResolvePHPConstraint(phpConstraint)
		if err == nil {
			web.Version = resolved
			fmt.Printf("[SLD] Detected PHP %s (from composer.json: %s)\n", resolved, phpConstraint)
		} else {
			fmt.Printf("[SLD] ⚠️  %v\n", err)
		}
	}

	if _, ok := composer.Require["laravel/framework"]; ok {
		fmt.Println("[SLD] Detected Laravel project")
		web.WebRoot = "public"
		if cfg.Services.Database.Type == "" {
			cfg.Services.Database.Type = "mysql"
			cfg.Services.Database.Version = "8.0"
		}
		cfg.Services.Redis.Enabled = true
		cfg.Services.Mailpit.Enabled = true
	}

	if _, ok := composer.Require["symfony/framework-bundle"]; ok {
		fmt.Println("[SLD] Detected Symfony project")
		web.WebRoot = "public"
	}

	if _, ok := composer.Require["roots/wordpress"]; ok {
		fmt.Println("[SLD] Detected WordPress (Bedrock) project")
		web.WebRoot = "web"
		if cfg.Services.Database.Type == "" {
			cfg.Services.Database.Type = "mysql"
			cfg.Services.Database.Version = "8.0"
		}
	}
}

// detectFromPackageJSON reads package.json and adds a Node dev server app
// (Vite, Next.js) or pins the Node version used by `sld npm`.
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

	version := ""
	if pkg.Engines.Node != "" {
		version = strings.TrimLeft(pkg.Engines.Node, ">=^~v ")
		version = strings.Split(version, ".")[0] // major version only
		fmt.Printf("[SLD] Detected Node %s requirement\n", version)
	}

	devCmd, ok := pkg.Scripts["dev"]
	if ok && (strings.Contains(devCmd, "vite") || strings.Contains(devCmd, "next")) {
		fmt.Println("[SLD] Detected dev server script")
		cfg.Apps = append(cfg.Apps, config.App{
			Name:       "vite",
			Runtime:    config.RuntimeNode,
			Version:    version,
			Command:    "npm run dev",
			Port:       5173,
			ExposePort: 5173,
		})
		return
	}
	if version != "" {
		cfg.Tools.Node = version
	}
}

// detectFromDotEnv reads .env to detect database and cache configuration.
func detectFromDotEnv(content string, cfg *config.Config) {
	for _, line := range strings.Split(content, "\n") {
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

// printConfigSummary displays a human-readable summary of a resolved config.
func printConfigSummary(cfg *config.Config) {
	fmt.Printf("  Project:   %s\n", cfg.Name)
	fmt.Printf("  Domain:    %s\n", cfg.Domain.Name)
	for _, app := range cfg.Apps {
		detail := app.Runtime
		if app.Version != "" {
			detail += " " + app.Version
		}
		if app.WebRoot != "" {
			detail += ", web root " + app.WebRoot
		}
		fmt.Printf("  App %-6s %s → %s\n", app.Name+":", detail, app.Domain)
	}
	if cfg.Services.Database.Type != "" {
		fmt.Printf("  Database:  %s %s\n", cfg.Services.Database.Type, cfg.Services.Database.Version)
	}
	if cfg.Services.Redis.Enabled {
		fmt.Println("  Redis:     enabled")
	}
	if cfg.Services.Mailpit.Enabled {
		fmt.Println("  Mailpit:   enabled")
	}
}
