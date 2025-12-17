package plugins

import (
	"log"
	"sync"

	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon/state"
)

type Manager struct {
	plugins      map[string]Plugin
	mu           sync.RWMutex
	DataDir      string
	StateManager *state.Manager
}

func NewManager(dataDir string, stateManager *state.Manager) *Manager {
	return &Manager{
		plugins:      make(map[string]Plugin),
		DataDir:      dataDir,
		StateManager: stateManager,
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

// SetEnabled persists the enabled state and starts/stops the plugin
func (m *Manager) SetEnabled(id string, enabled bool) error {
	p, ok := m.Get(id)
	if !ok {
		return nil
	}

	var err error
	if enabled {
		if p.IsInstalled() && p.Status() != StatusRunning {
			err = p.Start()
		}
	} else {
		if p.Status() == StatusRunning {
			err = p.Stop()
		}
	}

	if err == nil && m.StateManager != nil {
		m.StateManager.SetPluginEnabled(id, enabled)
	}

	return err
}

// StartEnabled starts all plugins that were marked as enabled in state
func (m *Manager) StartEnabled() {
	if m.StateManager == nil {
		return
	}

	enabledList := m.StateManager.GetEnabledPlugins()
	for _, id := range enabledList {
		p, ok := m.Get(id)
		if !ok {
			continue
		}
		if p.IsInstalled() && p.Status() != StatusRunning {
			if err := p.Start(); err != nil {
				log.Printf("Failed to auto-start plugin %s: %v", id, err)
			} else {
				log.Printf("Auto-started plugin: %s", id)
			}
		}
	}
}
