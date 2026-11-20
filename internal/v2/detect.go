package v2

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/detect"
)

// InitOptions adjust `sld init-project`.
type InitOptions struct {
	Runtime string // keep only apps of this runtime
	DryRun  bool   // print the configuration instead of writing it
}

// InitProject detects the project's stack and writes .sld/config.yaml.
func InitProject(projectRoot string) error {
	return InitProjectWith(projectRoot, InitOptions{})
}

// InitProjectWith is InitProject with options.
func InitProjectWith(projectRoot string, opts InitOptions) error {
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = abs
	}
	configDir := filepath.Join(projectRoot, ".sld")
	configPath := filepath.Join(configDir, "config.yaml")

	if _, err := os.Stat(configPath); err == nil && !opts.DryRun {
		return fmt.Errorf(".sld/config.yaml already exists at %s\n  Edit it directly or delete it to regenerate", configPath)
	}

	projectName := filepath.Base(projectRoot)
	fmt.Printf("[SLD] Detecting project type for: %s\n", projectName)

	result, err := detect.Project(projectRoot, detect.Options{Runtime: opts.Runtime})
	if errors.Is(err, detect.ErrNothingDetected) {
		return fmt.Errorf("%w in %s.\n  SLD looks for composer.json/artisan, package.json, manage.py/requirements.txt/pyproject.toml,\n"+
			"  Gemfile, go.mod, a Dockerfile or index.html. Create .sld/config.yaml by hand\n"+
			"  (see 'sld config show' on another project for the format)", err, projectRoot)
	}
	if err != nil {
		return err
	}
	for _, n := range result.Notes {
		fmt.Printf("[SLD] • %s\n", n)
	}

	output, err := config.Marshal(result.Config)
	if err != nil {
		return err
	}
	// Never write a file SLD itself would reject.
	resolved, err := config.LoadFromBytes(output, projectName)
	if err != nil {
		return fmt.Errorf("generated configuration is invalid (please report this): %w\n%s", err, output)
	}

	if opts.DryRun {
		fmt.Println()
		fmt.Print(string(output))
		return nil
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
	fmt.Println("[SLD] Review the file, then run 'sld start' to launch the development environment.")
	return nil
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
		var extras []string
		if app.WebRoot != "" {
			extras = append(extras, "web root "+app.WebRoot)
		}
		if app.Command != "" {
			extras = append(extras, "runs `"+app.Command+"`")
		}
		if len(extras) > 0 {
			detail += " (" + strings.Join(extras, ", ") + ")"
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
