package project

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// extractPHPVersion parses composer.json to find the required PHP version.
// It prefers the system's PHP version if it satisfies the constraint.
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

	// Get the system's PHP version
	systemPHP := getSystemPHPVersion()

	// If system PHP satisfies the constraint, use it
	if systemPHP != "" && satisfiesConstraint(systemPHP, constraint) {
		return systemPHP, nil
	}

	// Fallback: extract minimum version from constraint
	re := regexp.MustCompile(`(\d+\.\d+)`)
	matches := re.FindStringSubmatch(constraint)
	if len(matches) >= 2 {
		return matches[1], nil
	}

	return "", nil
}

// getSystemPHPVersion returns the system's current PHP version (e.g., "8.2")
func getSystemPHPVersion() string {
	out, err := exec.Command("php", "-r", "echo PHP_MAJOR_VERSION.'.'.PHP_MINOR_VERSION;").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// satisfiesConstraint checks if a PHP version satisfies a Composer constraint.
// Supports basic operators: ^, >=, >, |, and exact versions.
func satisfiesConstraint(version, constraint string) bool {
	// Parse version into major.minor
	vParts := strings.Split(version, ".")
	if len(vParts) < 2 {
		return false
	}
	vMajor, err1 := parseVersionPart(vParts[0])
	vMinor, err2 := parseVersionPart(vParts[1])
	if err1 != nil || err2 != nil {
		return false
	}

	// Handle OR constraints (e.g., "^8.0|^8.1")
	parts := strings.Split(constraint, "|")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if checkSingleConstraint(vMajor, vMinor, part) {
			return true
		}
	}
	return false
}

// checkSingleConstraint checks a single constraint part (no OR)
func checkSingleConstraint(vMajor, vMinor int, constraint string) bool {
	constraint = strings.TrimSpace(constraint)

	// Extract operator and version
	re := regexp.MustCompile(`^([><=^~]*)(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(constraint)
	if len(matches) < 4 {
		return false
	}

	op := matches[1]
	cMajor, _ := parseVersionPart(matches[2])
	cMinor, _ := parseVersionPart(matches[3])

	switch op {
	case "^":
		// ^8.1 means >=8.1.0 and <9.0.0
		if vMajor != cMajor {
			return false
		}
		return vMinor >= cMinor
	case ">=":
		if vMajor > cMajor {
			return true
		}
		if vMajor == cMajor && vMinor >= cMinor {
			return true
		}
		return false
	case ">":
		if vMajor > cMajor {
			return true
		}
		if vMajor == cMajor && vMinor > cMinor {
			return true
		}
		return false
	case "~":
		// ~8.1 means >=8.1.0 and <8.2.0 (next minor)
		return vMajor == cMajor && vMinor == cMinor
	case "", "=", "==":
		// Exact match (at major.minor level)
		return vMajor == cMajor && vMinor == cMinor
	default:
		// Unknown operator, try exact match
		return vMajor == cMajor && vMinor == cMinor
	}
}

// parseVersionPart parses a version part string to int
func parseVersionPart(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
