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

// Health checks if Redis is responding
func (p *RedisPlugin) Health() (bool, string) {
	if p.Status() != plugins.StatusRunning {
		return false, "Redis is not running"
	}

	// Try redis-cli PING
	cmd := exec.Command("redis-cli", "PING")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Sprintf("Redis not responding: %v", err)
	}

	if strings.TrimSpace(string(output)) == "PONG" {
		return true, "Redis is healthy"
	}
	return false, "Redis PING failed"
}

// Logs returns the last N lines of Redis logs
func (p *RedisPlugin) Logs(lines int) ([]string, error) {
	logPath := filepath.Join(p.dataDir, "redis.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		// Try journalctl as fallback
		cmd := exec.Command("journalctl", "-u", "redis", "-n", strconv.Itoa(lines), "--no-pager")
		output, err := cmd.Output()
		if err != nil {
			return []string{"No logs available"}, nil
		}
		return strings.Split(string(output), "\n"), nil
	}

	// Read from log file
	content, err := os.ReadFile(logPath)
	if err != nil {
		return nil, err
	}

	allLines := strings.Split(string(content), "\n")
	if len(allLines) > lines {
		allLines = allLines[len(allLines)-lines:]
	}
	return allLines, nil
}
