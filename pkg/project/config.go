package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the site-specific configuration
type Config struct {
	PHP    string `yaml:"php"`    // PHP version (e.g., "8.1")
	Node   string `yaml:"node"`   // Node version
	Public string `yaml:"public"` // Web root (e.g., "public")
}

// ComposerJSON represents a subset of composer.json
type ComposerJSON struct {
	Require map[string]string `json:"require"`
}

// Detect scans a directory for configuration files
func Detect(path string) (*Config, error) {
	config := &Config{}

	// 1. Check .sld.yaml (Highest Priority)
	sldYamlPath := filepath.Join(path, ".sld.yaml")
	if _, err := os.Stat(sldYamlPath); err == nil {
		data, err := os.ReadFile(sldYamlPath)
		if err == nil {
			if err := yaml.Unmarshal(data, config); err != nil {
				return nil, fmt.Errorf("failed to parse .sld.yaml: %w", err)
			}
		}
	}

	// 2. Check composer.json for PHP version if not already set
	if config.PHP == "" {
		composerPath := filepath.Join(path, "composer.json")
		if _, err := os.Stat(composerPath); err == nil {
			if phpVer, err := extractPHPVersion(composerPath); err == nil && phpVer != "" {
				config.PHP = phpVer
			}
		}
	}

	// 3. Check .nvmrc for Node version if not already set
	if config.Node == "" {
		nvmrcPath := filepath.Join(path, ".nvmrc")
		if _, err := os.Stat(nvmrcPath); err == nil {
			data, err := os.ReadFile(nvmrcPath)
			if err == nil {
				config.Node = strings.TrimSpace(string(data))
			}
		}
	}

	// 4. Auto-detect "public" directory (common in Laravel/Symfony/Modern Apps)
	if config.Public == "" {
		publicPath := filepath.Join(path, "public")
		if info, err := os.Stat(publicPath); err == nil && info.IsDir() {
			config.Public = "public"
		}
	}

	return config, nil
}

// extractPHPVersion parses composer.json to find the required PHP version constraint
func extractPHPVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var composer ComposerJSON
	if err := json.Unmarshal(data, &composer); err != nil {
		return "", err
	}

	// Extract version from "php": "^8.1"
	constraint, ok := composer.Require["php"]
	if !ok {
		return "", nil
	}

	return constraint, nil
}
