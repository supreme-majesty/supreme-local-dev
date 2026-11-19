package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"gopkg.in/yaml.v3"
)

// SupportedPHPVersions are the PHP versions with official SLD images.
var SupportedPHPVersions = []string{"8.1", "8.2", "8.3", "8.4"}

// DefaultVersions is the runtime version used when an app does not set one.
var DefaultVersions = map[string]string{
	RuntimeNode:   "22",
	RuntimePython: "3.13",
	RuntimeRuby:   "3.4",
	RuntimeGo:     "1.25",
}

// ErrAlreadyCurrent is returned by MigrateV1 for files already on the current schema.
var ErrAlreadyCurrent = errors.New("configuration is already version 2")

// FallbackTLD is the default top-level domain for project domains. Browsers
// resolve *.localhost to loopback without any DNS or hosts-file setup.
const FallbackTLD = "localhost"

// DefaultTLD returns the TLD used for project domains that do not set one
// explicitly: SLD_TLD when set, otherwise "localhost".
func DefaultTLD() string {
	if tld := strings.Trim(strings.ToLower(strings.TrimSpace(os.Getenv("SLD_TLD"))), "."); tld != "" {
		return tld
	}
	return FallbackTLD
}

// IsAutoResolvingDomain reports whether domain resolves to loopback without
// hosts-file or DNS configuration (RFC 6761 "localhost" names).
func IsAutoResolvingDomain(domain string) bool {
	d := strings.TrimSuffix(strings.ToLower(domain), ".")
	return d == "localhost" || strings.HasSuffix(d, ".localhost")
}

// LoadFromBytes parses, upgrades (version 1), defaults and validates a config.
func LoadFromBytes(data []byte, defaultName string) (*Config, error) {
	version, err := peekVersion(data)
	if err != nil {
		return nil, err
	}

	var cfg *Config
	switch version {
	case 1:
		var old configV1
		if err := yaml.Unmarshal(data, &old); err != nil {
			return nil, fmt.Errorf("malformed YAML configuration: %w", err)
		}
		if cfg, err = upgradeV1(&old); err != nil {
			return nil, err
		}
	case CurrentVersion:
		cfg = &Config{}
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true) // catch typos such as "comand:" instead of silently ignoring them
		if err := dec.Decode(cfg); err != nil {
			return nil, fmt.Errorf("invalid YAML configuration: %w", err)
		}
		cfg.SourceVersion = CurrentVersion
	default:
		return nil, unsupportedVersion(version)
	}

	ApplyDefaults(cfg, defaultName)
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadFromFile reads and parses the .sld/config.yaml file from a project root.
func LoadFromFile(projectRoot string) (*Config, error) {
	configPath := filepath.Join(projectRoot, ".sld", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", configPath, err)
	}
	return LoadFromBytes(data, filepath.Base(projectRoot))
}

func peekVersion(data []byte) (int, error) {
	var head struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return 0, fmt.Errorf("malformed YAML configuration: %w", err)
	}
	return head.Version, nil
}

func unsupportedVersion(v int) error {
	if v == 0 {
		return fmt.Errorf("missing configuration version: add 'version: %d' to .sld/config.yaml", CurrentVersion)
	}
	return fmt.Errorf("unsupported configuration version %d (supported: 1, %d)", v, CurrentVersion)
}

// ApplyDefaults fills optional fields with SLD's defaults.
func ApplyDefaults(cfg *Config, defaultName string) {
	cfg.Version = CurrentVersion
	if cfg.Name == "" {
		if defaultName != "" {
			cfg.Name = defaultName
		} else {
			cfg.Name = "project"
		}
	}
	if cfg.Domain.Name == "" {
		cfg.Domain.Name = fmt.Sprintf("%s.%s", project.Slugify(cfg.Name), DefaultTLD())
	}

	for i := range cfg.Apps {
		app := &cfg.Apps[i]
		app.Runtime = strings.ToLower(strings.TrimSpace(app.Runtime))
		applyAppDefaults(app)
		if app.Domain == "" {
			if i == 0 {
				app.Domain = cfg.Domain.Name
			} else {
				app.Domain = app.Name + "." + cfg.Domain.Name
			}
		}
	}

	db := &cfg.Services.Database
	if db.Type != "" && db.Version == "" {
		switch strings.ToLower(db.Type) {
		case "mysql":
			db.Version = "8.0"
		case "postgres", "postgresql":
			db.Version = "16"
		case "mariadb":
			db.Version = "10.11"
		}
	}
	if db.Shared != "" && db.Name == "" {
		db.Name = strings.ReplaceAll(project.Slugify(cfg.Name), "-", "_")
	}

	if cfg.Services.Redis.Enabled && cfg.Services.Redis.Version == "" {
		cfg.Services.Redis.Version = "7-alpine"
	}
}

func applyAppDefaults(app *App) {
	if app.Version == "" {
		app.Version = DefaultVersions[app.Runtime]
	}
	switch app.Runtime {
	case RuntimePHP:
		if app.WebRoot == "" {
			app.WebRoot = "public"
		}
	case RuntimeStatic:
		if app.WebRoot == "" {
			app.WebRoot = "."
		}
	case RuntimeNode:
		if app.Command == "" {
			app.Command = "npm run dev"
		}
		if app.Port == 0 {
			app.Port = 3000
		}
	case RuntimeDockerfile:
		if app.Dockerfile == "" {
			app.Dockerfile = "Dockerfile"
		}
		if app.Context == "" {
			app.Context = "."
		}
	}
}

var (
	appNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,19}$`)
	versionPattern  = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$`)
	hostnamePattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

// Validate checks a defaulted config and reports every problem it finds.
func Validate(cfg *Config) error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if cfg.Version != CurrentVersion {
		fail("version: must be %d after upgrade, got %d", CurrentVersion, cfg.Version)
	}
	if !hostnamePattern.MatchString(cfg.Domain.Name) {
		fail("domain.name: %q is not a valid host name", cfg.Domain.Name)
	}
	if len(cfg.Apps) == 0 {
		fail("apps: define at least one app, e.g.\n  apps:\n    web:\n      runtime: php\n      version: \"8.3\"")
	}

	domains := map[string]string{}
	claim := func(host, owner string) {
		if prev, ok := domains[host]; ok {
			fail("%s: domain %q is already used by %s", owner, host, prev)
			return
		}
		domains[host] = owner
	}

	for _, app := range cfg.Apps {
		path := "apps." + app.Name
		if !appNamePattern.MatchString(app.Name) {
			fail("%s: app names must be lowercase letters, digits or '-', start with a letter, max 20 chars", path)
		}
		validateApp(path, &app, fail)
		if app.Domain != "" {
			if !hostnamePattern.MatchString(app.Domain) {
				fail("%s.domain: %q is not a valid host name", path, app.Domain)
			} else {
				claim(app.Domain, path)
			}
		}
	}
	for i, alias := range cfg.Domain.Aliases {
		if !hostnamePattern.MatchString(alias) {
			fail("domain.aliases[%d]: %q is not a valid host name", i, alias)
		} else {
			claim(alias, fmt.Sprintf("domain.aliases[%d]", i))
		}
	}

	db := cfg.Services.Database
	switch {
	case db.Type != "" && db.Shared != "":
		fail("services.database: set either 'type' (dedicated container) or 'shared' (shared server), not both")
	case db.Type != "":
		switch strings.ToLower(db.Type) {
		case "mysql", "postgres", "postgresql", "mariadb":
		default:
			fail("services.database.type: unsupported %q (supported: mysql, mariadb, postgres)", db.Type)
		}
	case db.Name != "" && db.Shared == "":
		fail("services.database.name: only used together with 'shared'")
	}
	if db.ExposePort != 0 && !validPort(db.ExposePort) {
		fail("services.database.expose_port: %d is not a valid port", db.ExposePort)
	}

	return errors.Join(errs...)
}

func validateApp(path string, app *App, fail func(string, ...any)) {
	rt := app.Runtime
	known := false
	for _, r := range Runtimes {
		if r == rt {
			known = true
			break
		}
	}
	if !known {
		if rt == "" {
			fail("%s.runtime: required (one of: %s)", path, strings.Join(Runtimes, ", "))
		} else {
			fail("%s.runtime: unknown runtime %q (one of: %s)", path, rt, strings.Join(Runtimes, ", "))
		}
		return
	}

	// Fields that only make sense for some runtimes.
	only := func(set bool, field string, runtimes ...string) {
		if !set {
			return
		}
		for _, r := range runtimes {
			if r == rt {
				return
			}
		}
		fail("%s.%s: not supported for runtime %s", path, field, rt)
	}
	only(app.WebRoot != "", "web_root", RuntimePHP, RuntimeStatic)
	only(app.Xdebug, "xdebug", RuntimePHP)
	only(app.Image != "", "image", RuntimeImage)
	only(app.Dockerfile != "" || app.Context != "" || app.Target != "" || len(app.BuildArgs) > 0, "dockerfile/context/target/build_args", RuntimeDockerfile)
	only(app.PackageManager != "", "package_manager", RuntimeNode)
	only(app.Command != "", "command", RuntimeNode, RuntimePython, RuntimeRuby, RuntimeGo, RuntimeImage, RuntimeDockerfile)
	only(app.Install != "", "install", RuntimeNode, RuntimePython, RuntimeRuby, RuntimeGo)
	only(app.Port != 0, "port", RuntimeNode, RuntimePython, RuntimeRuby, RuntimeGo, RuntimeImage, RuntimeDockerfile, RuntimeProxy)
	only(app.Version != "", "version", RuntimePHP, RuntimeNode, RuntimePython, RuntimeRuby, RuntimeGo)

	switch rt {
	case RuntimePHP:
		if app.Version == "" {
			fail("%s.version: required for php (supported: %s)", path, strings.Join(SupportedPHPVersions, ", "))
		} else if !contains(SupportedPHPVersions, app.Version) {
			fail("%s.version: unsupported PHP version %q (supported: %s)", path, app.Version, strings.Join(SupportedPHPVersions, ", "))
		}
	case RuntimePython, RuntimeRuby, RuntimeGo:
		if app.Command == "" {
			fail("%s.command: required for runtime %s (the dev server command, listening on 0.0.0.0)", path, rt)
		}
		if app.Port == 0 {
			fail("%s.port: required for runtime %s", path, rt)
		}
	case RuntimeImage:
		if app.Image == "" {
			fail("%s.image: required for runtime image", path)
		}
		if app.Port == 0 {
			fail("%s.port: required for runtime image", path)
		}
	case RuntimeDockerfile:
		if app.Port == 0 {
			fail("%s.port: required for runtime dockerfile", path)
		}
	case RuntimeProxy:
		if app.Port == 0 {
			fail("%s.port: required for runtime proxy (the port your app listens on, on the host)", path)
		}
	}

	if app.Version != "" && !versionPattern.MatchString(app.Version) {
		fail("%s.version: %q is not a valid version", path, app.Version)
	}
	if app.PackageManager != "" && !contains([]string{"npm", "pnpm", "yarn", "bun"}, app.PackageManager) {
		fail("%s.package_manager: unsupported %q (npm, pnpm, yarn, bun)", path, app.PackageManager)
	}
	if app.Port != 0 && !validPort(app.Port) {
		fail("%s.port: %d is not a valid port", path, app.Port)
	}
	if app.ExposePort != 0 && !validPort(app.ExposePort) {
		fail("%s.expose_port: %d is not a valid port", path, app.ExposePort)
	}
	for _, p := range []struct{ field, value string }{{"web_root", app.WebRoot}, {"context", app.Context}, {"dockerfile", app.Dockerfile}} {
		if p.value != "" && (filepath.IsAbs(p.value) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(p.value)), "../") || filepath.Clean(p.value) == "..") {
			fail("%s.%s: %q must be a path inside the project", path, p.field, p.value)
		}
	}
	if app.Healthcheck != "" && !strings.HasPrefix(app.Healthcheck, "/") {
		fail("%s.healthcheck: must be an HTTP path starting with '/'", path)
	}
}

func validPort(p int) bool { return p > 0 && p < 65536 }

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
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
