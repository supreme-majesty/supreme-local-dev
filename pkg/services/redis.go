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

type RedisPlugin struct {
	dataDir string
}

func NewRedisPlugin(dataDir string) *RedisPlugin {
	return &RedisPlugin{
		dataDir: filepath.Join(dataDir, "redis"),
	}
}

func (p *RedisPlugin) ID() string          { return "redis" }
func (p *RedisPlugin) Name() string        { return "Redis" }
func (p *RedisPlugin) Description() string { return "In-memory data store" }
func (p *RedisPlugin) Version() string     { return "7.2.4" }

func (p *RedisPlugin) pidFile() string {
	return filepath.Join(p.dataDir, "redis.pid")
}

func (p *RedisPlugin) Status() plugins.Status {
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

func (p *RedisPlugin) IsInstalled() bool {
	// Check if system redis-server is available
	if _, err := exec.LookPath("redis-server"); err == nil {
		return true
	}
	// Or check our data directory for a downloaded binary
	binPath := filepath.Join(p.dataDir, "redis-server")
	if _, err := os.Stat(binPath); err == nil {
		return true
	}
	return false
}

func (p *RedisPlugin) Install() error {
	if err := os.MkdirAll(p.dataDir, 0755); err != nil {
		return err
	}

	// For MVP, we rely on system redis-server
	// Check if already available
	if _, err := exec.LookPath("redis-server"); err == nil {
		return nil // System redis available
	}

	// Create a marker file to indicate "installed" state
	f, err := os.Create(filepath.Join(p.dataDir, ".installed"))
	if err != nil {
		return err
	}
	f.Close()

	return fmt.Errorf("redis-server not found. Please install: sudo apt install redis-server")
}

func (p *RedisPlugin) Start() error {
	if !p.IsInstalled() {
		return fmt.Errorf("redis is not installed")
	}

	if p.Status() == plugins.StatusRunning {
		return nil // Already running
	}

	// Ensure data directory exists
	os.MkdirAll(p.dataDir, 0755)

	// Start redis-server in background
	cmd := exec.Command("redis-server", "--daemonize", "yes", "--pidfile", p.pidFile(), "--dir", p.dataDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start redis: %w", err)
	}

	return nil
}

func (p *RedisPlugin) Stop() error {
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
