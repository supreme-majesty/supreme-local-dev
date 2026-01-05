package adapters

// SystemAdapter defines the interface for OS-specific interactions.
type SystemAdapter interface {
	// Service Management
	StartService(serviceName string) error
	StopService(serviceName string) error
	RestartService(serviceName string) error
	IsServiceRunning(serviceName string) (bool, error)

	// Installation & Setup
	InstallDependencies() error
	InstallPHP(version string) error
	InstallNode(version string) error
	GetNodePath(version string) (string, error)
	InstallCertificates() error
	InstallMkcert() error
	GenerateCert(homeDir string, domains []string) error
	InstallBinary() error
	Uninstall() error

	// Configuration
	WriteNginxConfig(config string) error
	GetNginxConfigPath() string

	// Runtime
	GetPHPVersion() string
	ListPHPVersions() ([]string, error)
	CheckPHPSocket(version string) (string, error)
	ReloadNginx() error

	// Permissions & User Management
	AddWebUserToGroup(group string) error
	RestartPHP() error
	UpdateHosts(domains []string) error
	// Health & Connectivity
	CheckWifi() (bool, string)
	Doctor() error
	GetLogPaths() map[string]string

	// Structured Status
	GetServices() ([]ServiceStatus, error)
	GetSystemHealth() ([]HealthCheck, error)
}

// Shared Types
type ServiceStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Version string `json:"version,omitempty"`
}

type HealthCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // pass, fail, warn
	Message string `json:"message"`
}
