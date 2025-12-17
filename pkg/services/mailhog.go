package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/supreme-majesty/supreme-local-dev/pkg/plugins"
)

type MailHogPlugin struct {
	dataDir string
}

func NewMailHogPlugin(dataDir string) *MailHogPlugin {
	return &MailHogPlugin{
		dataDir: filepath.Join(dataDir, "mailhog"),
	}
}

func (p *MailHogPlugin) ID() string          { return "mailhog" }
func (p *MailHogPlugin) Name() string        { return "MailHog" }
func (p *MailHogPlugin) Description() string { return "Email testing tool for capturing SMTP emails" }
func (p *MailHogPlugin) Version() string     { return "1.0.1" }

func (p *MailHogPlugin) pidFile() string {
	return filepath.Join(p.dataDir, "mailhog.pid")
}

func (p *MailHogPlugin) Status() plugins.Status {
	pidData, err := os.ReadFile(p.pidFile())
	if err != nil {
		return plugins.StatusStopped
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return plugins.StatusStopped
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return plugins.StatusStopped
	}

	// Signal 0 checks if process exists without killing it
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process doesn't exist, clean up stale PID file
		os.Remove(p.pidFile())
		return plugins.StatusStopped
	}

	return plugins.StatusRunning
}

func (p *MailHogPlugin) IsInstalled() bool {
	// Check if system mailhog is available
	if _, err := exec.LookPath("mailhog"); err == nil {
		return true
	}
	// Alternative name used by some package managers
	if _, err := exec.LookPath("MailHog"); err == nil {
		return true
	}
	// Check our data directory
	binPath := filepath.Join(p.dataDir, "mailhog")
	if _, err := os.Stat(binPath); err == nil {
		return true
	}
	return false
}

func (p *MailHogPlugin) Install() error {
	if err := os.MkdirAll(p.dataDir, 0755); err != nil {
		return err
	}

	// Check if already available
	if _, err := exec.LookPath("mailhog"); err == nil {
		return nil
	}
	if _, err := exec.LookPath("MailHog"); err == nil {
		return nil
	}

	// Create marker file
	f, err := os.Create(filepath.Join(p.dataDir, ".installed"))
	if err != nil {
		return err
	}
	f.Close()

	return fmt.Errorf("mailhog not found. Install with: go install github.com/mailhog/MailHog@latest")
}

func (p *MailHogPlugin) Start() error {
	if !p.IsInstalled() {
		return fmt.Errorf("mailhog is not installed")
	}

	if p.Status() == plugins.StatusRunning {
		return nil // Already running
	}

	// Ensure data directory exists
	os.MkdirAll(p.dataDir, 0755)

	// Find mailhog binary
	binName := "mailhog"
	if _, err := exec.LookPath("MailHog"); err == nil {
		binName = "MailHog"
	}

	// MailHog doesn't daemonize itself, so we start it in background
	cmd := exec.Command(binName)
	cmd.Dir = p.dataDir
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mailhog: %w", err)
	}

	// Write PID file
	if err := os.WriteFile(p.pidFile(), []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	return nil
}

func (p *MailHogPlugin) Stop() error {
	pidData, err := os.ReadFile(p.pidFile())
	if err != nil {
		return nil // Not running
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		os.Remove(p.pidFile())
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(p.pidFile())
		return nil
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Force kill if SIGTERM fails
		process.Kill()
	}

	os.Remove(p.pidFile())
	return nil
}
