package macos

type MacOSAdapter struct{}

func NewMacOSAdapter() *MacOSAdapter {
	return &MacOSAdapter{}
}

func (m *MacOSAdapter) StartService(name string) error                      { return nil }
func (m *MacOSAdapter) StopService(name string) error                       { return nil }
func (m *MacOSAdapter) RestartService(name string) error                    { return nil }
func (m *MacOSAdapter) IsServiceRunning(name string) (bool, error)          { return false, nil }
func (m *MacOSAdapter) InstallDependencies() error                          { return nil }
func (m *MacOSAdapter) InstallCertificates() error                          { return nil }
func (m *MacOSAdapter) InstallMkcert() error                                { return nil }
func (m *MacOSAdapter) GenerateCert(homeDir string, domains []string) error { return nil }
func (m *MacOSAdapter) InstallBinary() error                                { return nil }
func (m *MacOSAdapter) WriteNginxConfig(config string) error                { return nil }
func (m *MacOSAdapter) GetNginxConfigPath() string                          { return "" }
func (m *MacOSAdapter) GetPHPVersion() string                               { return "" }
func (m *MacOSAdapter) CheckPHPSocket(version string) (string, error)       { return "", nil }
func (m *MacOSAdapter) ReloadNginx() error                                  { return nil }
func (m *MacOSAdapter) AddWebUserToGroup(group string) error                { return nil }
func (m *MacOSAdapter) RestartPHP() error                                   { return nil }
