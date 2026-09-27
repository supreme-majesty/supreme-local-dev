package config

// Config represents the project desired state defined in .sld/config.yaml.
type Config struct {
	Version     int               `yaml:"version"`
	Name        string            `yaml:"name,omitempty"`
	Runtime     RuntimeConfig     `yaml:"runtime"`
	Domain      DomainConfig      `yaml:"domain,omitempty"`
	Services    ServicesConfig    `yaml:"services,omitempty"`
	Node        NodeConfig        `yaml:"node,omitempty"`
	Xdebug      XdebugConfig      `yaml:"xdebug,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
}

// RuntimeConfig defines the application language and web root.
type RuntimeConfig struct {
	PHP     string `yaml:"php"`                // e.g. "8.1", "8.2", "8.3", "8.4"
	WebRoot string `yaml:"web_root,omitempty"` // e.g. "public"
}

// DomainConfig defines the HTTP routing and TLS settings.
type DomainConfig struct {
	Name string `yaml:"name,omitempty"` // e.g. "my-project.test"
	TLS  bool   `yaml:"tls,omitempty"`  // true enables HTTPS via Traefik
}

// ServicesConfig defines auxiliary infrastructure containers.
type ServicesConfig struct {
	Database DatabaseConfig `yaml:"database,omitempty"`
	Redis    RedisConfig    `yaml:"redis,omitempty"`
	Mailpit  MailpitConfig  `yaml:"mailpit,omitempty"`
}

// DatabaseConfig defines the database engine and port settings.
type DatabaseConfig struct {
	Type       string `yaml:"type,omitempty"`        // "mysql", "postgres", "mariadb"
	Version    string `yaml:"version,omitempty"`     // e.g. "8.0", "16", "10.11"
	ExposePort int    `yaml:"expose_port,omitempty"` // host port mapping (0 = none)
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

// NodeConfig defines the Node.js runtime and dev server settings.
type NodeConfig struct {
	Version   string          `yaml:"version,omitempty"` // e.g. "18", "20", "22"
	DevServer DevServerConfig `yaml:"dev_server,omitempty"`
}

// DevServerConfig defines settings for a persistent dev server (Vite, Next.js).
type DevServerConfig struct {
	Enabled    bool   `yaml:"enabled,omitempty"`
	Command    string `yaml:"command,omitempty"`     // e.g. "npm run dev"
	Port       int    `yaml:"port,omitempty"`        // container port (default 5173)
	ExposePort int    `yaml:"expose_port,omitempty"` // host port mapping (default 5173)
}

// XdebugConfig defines PHP debugging settings.
type XdebugConfig struct {
	Enabled bool `yaml:"enabled,omitempty"` // sets XDEBUG_MODE=debug when true
}
