package plugins

import (
	"sync"
)

type Manager struct {
	plugins map[string]Plugin
	mu      sync.RWMutex
	DataDir string
}

func NewManager(dataDir string) *Manager {
	return &Manager{
		plugins: make(map[string]Plugin),
		DataDir: dataDir,
	}
}

func (m *Manager) Register(p Plugin) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plugins[p.ID()] = p
}

func (m *Manager) Get(id string) (Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[id]
	return p, ok
}

func (m *Manager) GetAll() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []Plugin
	for _, p := range m.plugins {
		list = append(list, p)
	}
	return list
}

func (m *Manager) StartAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.plugins {
		if p.IsInstalled() && p.Status() != StatusRunning {
			// In a real implementation, we might respect a "enabled" flag
			// For now, if it's installed, we don't auto-start unless explicitly told?
			// Actually Phase 2 plan says "Start/Stop/Restart".
			// Use a persistence mechanism later to know what to auto-start.
		}
	}
}
