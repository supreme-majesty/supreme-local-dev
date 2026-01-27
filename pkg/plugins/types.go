package plugins

type Status string

const (
	StatusRunning    Status = "running"
	StatusStopped    Status = "stopped"
	StatusError      Status = "error"
	StatusInstalling Status = "installing"
)

// Plugin defines the interface that all services/plugins must implement
type Plugin interface {
	// ID returns the unique identifier (e.g., "redis", "mailhog")
	ID() string

	// Name returns the display name
	Name() string

	// Description returns what the plugin does
	Description() string

	// Version returns the current installed version
	Version() string

	// Status returns the current running state
	Status() Status

	// Install downloads and sets up the plugin
	Install() error

	// Start launches the plugin process
	Start() error

	// Stop terminates the plugin process
	Stop() error

	// IsInstalled checks if the binary/resources exist
	IsInstalled() bool
}

// HealthChecker is an optional interface for plugins that can report health status
type HealthChecker interface {
	// Health returns whether the plugin is healthy and a status message
	Health() (ok bool, message string)
}

// LogProvider is an optional interface for plugins that can provide logs
type LogProvider interface {
	// Logs returns the last N lines of logs
	Logs(lines int) ([]string, error)
}

// UIProvider is an optional interface for plugins that have a web UI
type UIProvider interface {
	// UIPort returns the port where the UI is available
	UIPort() int
}

// NginxHook allows plugins to inject custom Nginx configurations
type NginxHook interface {
	// NginxConfig returns blocks of Nginx config to be included
	// Key is the name/identifier of the block
	NginxConfig() (map[string]string, error)
}

// PHPHook allows plugins to modify PHP configurations
type PHPHook interface {
	// PHPExtensions returns list of PHP extensions to be enabled
	PHPExtensions() []string
	// PHPConfig returns lines to be added to php.ini (or equivalent)
	PHPConfig() (map[string]string, error)
}
