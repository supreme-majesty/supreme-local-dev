package config

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// CurrentVersion is the configuration schema version SLD writes.
const CurrentVersion = 2

// Runtime names an app runtime driver.
const (
	RuntimePHP        = "php"
	RuntimeStatic     = "static"
	RuntimeNode       = "node"
	RuntimePython     = "python"
	RuntimeRuby       = "ruby"
	RuntimeGo         = "go"
	RuntimeImage      = "image"
	RuntimeDockerfile = "dockerfile"
	RuntimeProxy      = "proxy"
)

// Runtimes lists every supported runtime, in documentation order.
var Runtimes = []string{
	RuntimePHP, RuntimeStatic, RuntimeNode, RuntimePython, RuntimeRuby,
	RuntimeGo, RuntimeImage, RuntimeDockerfile, RuntimeProxy,
}

// Config is the desired state of a project, defined in .sld/config.yaml.
// Version 1 files are upgraded to this shape when loaded.
type Config struct {
	Version     int               `yaml:"version"`
	Name        string            `yaml:"name,omitempty"`
	Domain      DomainConfig      `yaml:"domain,omitempty"`
	Apps        Apps              `yaml:"apps"`
	Services    ServicesConfig    `yaml:"services,omitempty"`
	Tools       ToolsConfig       `yaml:"tools,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`

	// SourceVersion is the schema version of the file this config was loaded
	// from (1 when it was upgraded in memory). Not serialized.
	SourceVersion int `yaml:"-"`
}

// DomainConfig defines the project's root domain and TLS settings.
type DomainConfig struct {
	Name    string   `yaml:"name,omitempty"`    // e.g. "shop.localhost"
	TLS     bool     `yaml:"tls,omitempty"`     // serve HTTPS via the router
	Aliases []string `yaml:"aliases,omitempty"` // extra names for the primary app
}

// App is one HTTP-serving part of a project (a PHP site, a Node dev server, an API...).
type App struct {
	// Name is the key under `apps:`. Not serialized as a field.
	Name string `yaml:"-"`

	Runtime        string            `yaml:"runtime"`
	Version        string            `yaml:"version,omitempty"`         // runtime version / image tag, e.g. "8.3", "22", "3.13"
	Command        string            `yaml:"command,omitempty"`         // long-running process (node/python/ruby/go/image)
	Install        string            `yaml:"install,omitempty"`         // dependency install command, e.g. "npm ci"
	PackageManager string            `yaml:"package_manager,omitempty"` // node: npm | pnpm | yarn | bun
	Port           int               `yaml:"port,omitempty"`            // port the app listens on inside its container (host port for proxy)
	ExposePort     int               `yaml:"expose_port,omitempty"`     // optional extra host port mapping (loopback only)
	WebRoot        string            `yaml:"web_root,omitempty"`        // php / static: document root relative to the project
	Xdebug         bool              `yaml:"xdebug,omitempty"`          // php only
	Image          string            `yaml:"image,omitempty"`           // image runtime
	Dockerfile     string            `yaml:"dockerfile,omitempty"`      // dockerfile runtime, relative to context
	Context        string            `yaml:"context,omitempty"`         // dockerfile runtime build context
	Target         string            `yaml:"target,omitempty"`          // dockerfile runtime build stage
	BuildArgs      map[string]string `yaml:"build_args,omitempty"`      // dockerfile runtime
	Domain         string            `yaml:"domain,omitempty"`          // default: root domain (first app) or <app>.<root domain>
	Healthcheck    string            `yaml:"healthcheck,omitempty"`     // HTTP path checked for readiness, e.g. "/health"
	Environment    map[string]string `yaml:"environment,omitempty"`     // app-specific variables (merged over project environment)
}

// appFields is the set of YAML keys an app may use, derived from App's tags.
var appFields = func() map[string]bool {
	fields := map[string]bool{}
	t := reflect.TypeOf(App{})
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("yaml"), ",")[0]
		if name != "" && name != "-" {
			fields[name] = true
		}
	}
	return fields
}()

// Apps is an ordered set of apps. In YAML it is a mapping from app name to
// app definition; order is preserved because the first app owns the root domain.
type Apps []App

// UnmarshalYAML decodes a mapping while preserving key order.
func (a *Apps) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: apps must be a mapping of app name to app definition", node.Line)
	}
	seen := make(map[string]bool, len(node.Content)/2)
	out := make(Apps, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if seen[key.Value] {
			return fmt.Errorf("line %d: duplicate app %q", key.Line, key.Value)
		}
		seen[key.Value] = true
		var app App
		if value.Kind == yaml.MappingNode {
			// node.Decode does not inherit the decoder's KnownFields setting,
			// so reject unknown keys (typos) here explicitly.
			for j := 0; j+1 < len(value.Content); j += 2 {
				field := value.Content[j]
				if !appFields[field.Value] {
					return fmt.Errorf("line %d: apps.%s: unknown field %q", field.Line, key.Value, field.Value)
				}
			}
		}
		if value.Kind == yaml.MappingNode || value.Tag != "!!null" {
			if err := value.Decode(&app); err != nil {
				return fmt.Errorf("apps.%s: %w", key.Value, err)
			}
		}
		app.Name = key.Value
		out = append(out, app)
	}
	*a = out
	return nil
}

// MarshalYAML encodes apps as a mapping in their stored order.
func (a Apps) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, app := range a {
		var value yaml.Node
		if err := value.Encode(app); err != nil {
			return nil, fmt.Errorf("apps.%s: %w", app.Name, err)
		}
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: app.Name},
			&value,
		)
	}
	return node, nil
}

// ServicesConfig defines auxiliary infrastructure containers.
type ServicesConfig struct {
	Database DatabaseConfig `yaml:"database,omitempty"`
	Redis    RedisConfig    `yaml:"redis,omitempty"`
	Mailpit  MailpitConfig  `yaml:"mailpit,omitempty"`
}

// DatabaseConfig selects either a dedicated database container (Type/Version)
// or a database on an SLD-managed shared server (Shared/Name).
type DatabaseConfig struct {
	Type       string `yaml:"type,omitempty"`        // "mysql", "mariadb", "postgres"
	Version    string `yaml:"version,omitempty"`     // e.g. "8.0", "11.4", "16"
	ExposePort int    `yaml:"expose_port,omitempty"` // host port mapping (0 = none)
	Shared     string `yaml:"shared,omitempty"`      // shared server name, e.g. "sld-shared-postgres-16"
	Name       string `yaml:"name,omitempty"`        // database name on the shared server (default: project name)
}

// RedisConfig defines the key-value cache service.
type RedisConfig struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	Version string `yaml:"version,omitempty"` // default "7-alpine"
}

// MailpitConfig defines the local email testing service.
type MailpitConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
}

// ToolsConfig pins versions of one-off tool containers (`sld npm`, ...) used
// when the project has no app of that runtime.
type ToolsConfig struct {
	Node string `yaml:"node,omitempty"`
}

// App returns the app with the given name.
func (c *Config) App(name string) (*App, bool) {
	for i := range c.Apps {
		if c.Apps[i].Name == name {
			return &c.Apps[i], true
		}
	}
	return nil, false
}

// AppsByRuntime returns the apps using runtime, in config order.
func (c *Config) AppsByRuntime(runtime string) []*App {
	var out []*App
	for i := range c.Apps {
		if c.Apps[i].Runtime == runtime {
			out = append(out, &c.Apps[i])
		}
	}
	return out
}

// PrimaryApp is the app served on the project's root domain (the first app).
func (c *Config) PrimaryApp() *App {
	if len(c.Apps) == 0 {
		return nil
	}
	for i := range c.Apps {
		if c.Apps[i].Domain != "" && c.Apps[i].Domain == c.Domain.Name {
			return &c.Apps[i]
		}
	}
	return &c.Apps[0]
}

// NodeVersion is the Node.js version for one-off tool containers: the first
// node app's version, then tools.node, then the default.
func (c *Config) NodeVersion() string {
	for _, app := range c.AppsByRuntime(RuntimeNode) {
		if app.Version != "" {
			return app.Version
		}
	}
	if c.Tools.Node != "" {
		return c.Tools.Node
	}
	return DefaultVersions[RuntimeNode]
}

// Hostnames returns every host name the project serves: each app's domain,
// the root-domain aliases and, when enabled, the Mailpit UI.
func (c *Config) Hostnames() []string {
	var hosts []string
	seen := map[string]bool{}
	add := func(h string) {
		if h != "" && !seen[h] {
			seen[h] = true
			hosts = append(hosts, h)
		}
	}
	add(c.Domain.Name)
	for _, app := range c.Apps {
		add(app.Domain)
	}
	for _, alias := range c.Domain.Aliases {
		add(alias)
	}
	if c.Services.Mailpit.Enabled {
		add("mail." + c.Domain.Name)
	}
	return hosts
}

// Describe summarises the apps for CLI output, e.g. "web (php 8.3), api (python 3.13)".
func (c *Config) Describe() string {
	s := ""
	for i, app := range c.Apps {
		if i > 0 {
			s += ", "
		}
		s += app.Name + " (" + app.Runtime
		if app.Version != "" {
			s += " " + app.Version
		}
		s += ")"
	}
	return s
}
