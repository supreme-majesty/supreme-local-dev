package services

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/supreme-majesty/supreme-local-dev/pkg/plugins"
)

type PostgresPlugin struct {
	DataDir string
	Port    int
}

func NewPostgresPlugin(dataDir string) *PostgresPlugin {
	return &PostgresPlugin{
		DataDir: dataDir,
		Port:    5432,
	}
}

func (p *PostgresPlugin) Name() string {
	return "PostgreSQL"
}

func (p *PostgresPlugin) ID() string {
	return "postgres"
}

func (p *PostgresPlugin) Description() string {
	return "Advanced Open Source Relational Database"
}

func (p *PostgresPlugin) Version() string {
	return "14.x" // Default valid version for now
}

func (p *PostgresPlugin) Install() error {
	fmt.Println("Installing PostgreSQL...")

	// Delegate to system adapter?
	// Or run commands directly. Better to delegate or adapt based on OS.
	// For now, assuming apt/brew.

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("sudo", "apt-get", "install", "-y", "postgresql", "postgresql-contrib")
	case "darwin":
		cmd = exec.Command("brew", "install", "postgresql@14")
	case "windows":
		// Managed by adapter or manual
		return fmt.Errorf("manual installation required on Windows")
	}

	if cmd != nil {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return nil
}

func (p *PostgresPlugin) Uninstall() error {
	return nil
}

func (p *PostgresPlugin) Start() error {
	// PostgreSQL is usually a system service
	// We can try to start it via service manager
	switch runtime.GOOS {
	case "linux":
		return exec.Command("sudo", "systemctl", "start", "postgresql").Run()
	case "darwin":
		return exec.Command("brew", "services", "start", "postgresql@14").Run()
	}
	return nil
}

func (p *PostgresPlugin) Stop() error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("sudo", "systemctl", "stop", "postgresql").Run()
	case "darwin":
		return exec.Command("brew", "services", "stop", "postgresql@14").Run()
	}
	return nil
}

func (p *PostgresPlugin) Status() plugins.Status {
	// Check port
	conn, err := exec.Command("nc", "-z", "localhost", fmt.Sprintf("%d", p.Port)).Output()
	if err == nil && len(conn) == 0 { // nc returns 0 on success
		return plugins.StatusRunning
	}

	// Alternative check via systemctl/brew
	return plugins.StatusStopped
}

func (p *PostgresPlugin) IsInstalled() bool {
	// Simple check for psql binary
	if _, err := exec.LookPath("psql"); err == nil {
		return true
	}
	return false
}

func (p *PostgresPlugin) Logs(lines int) ([]string, error) {
	// Trivial implementation for now
	return []string{"Logs not implemented yet"}, nil
}

func (p *PostgresPlugin) Health() (bool, string) {
	if p.Status() == plugins.StatusRunning {
		return true, "Running on port 5432"
	}
	return false, "Stopped"
}
