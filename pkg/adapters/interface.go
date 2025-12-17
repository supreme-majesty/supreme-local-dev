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
	CheckPHPSocket(version string) (string, error)
	ReloadNginx() error

	// Permissions & User Management
	AddWebUserToGroup(group string) error
	RestartPHP() error
}
