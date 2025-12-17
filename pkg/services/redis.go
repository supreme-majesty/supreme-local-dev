package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/supreme-local-dev/pkg/plugins"
)

type RedisPlugin struct {
	dataDir string
	process *os.Process
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

func (p *RedisPlugin) Status() plugins.Status {
	if p.process != nil {
		// simple check if process is still alive
		if err := p.process.Signal(os.Signal(0)); err == nil {
			return plugins.StatusRunning
		}
		p.process = nil
	}
	return plugins.StatusStopped
}

func (p *RedisPlugin) IsInstalled() bool {
	// check if binary exists
	binPath := filepath.Join(p.dataDir, "redis-server")
	if _, err := os.Stat(binPath); err == nil {
		return true
	}
	return false
}

func (p *RedisPlugin) Install() error {
	// Mock install for now - in real world would download binary
	// For this Phase 2 MVP, let's assume system redis or just simulate
	if err := os.MkdirAll(p.dataDir, 0755); err != nil {
		return err
	}
	// Create a dummy file to mark as installed
	f, err := os.Create(filepath.Join(p.dataDir, "redis-server"))
	if err != nil {
		return err
	}
	f.Close()

	// Make executable
	os.Chmod(filepath.Join(p.dataDir, "redis-server"), 0755)

	return nil
}

func (p *RedisPlugin) Start() error {
	if !p.IsInstalled() {
		return fmt.Errorf("redis is not installed")
	}

	// Real implementation would exec the binary
	// here we just pretend
	cmd := exec.Command("sleep", "3600")
	if err := cmd.Start(); err != nil {
		return err
	}
	p.process = cmd.Process
	return nil
}

func (p *RedisPlugin) Stop() error {
	if p.process != nil {
		return p.process.Kill()
	}
	return nil
}
