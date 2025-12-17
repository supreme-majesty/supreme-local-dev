package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// State represents the persistent configuration of the SLD environment.
// State represents the persistent configuration of the SLD environment.
type State struct {
	TLD          string            `json:"tld"`
	Paths        []string          `json:"paths"`        // Parked paths
	Links        map[string]string `json:"links"`        // Linked projects (siteName -> path)
	Services     map[string]string `json:"services"`     // Service status/config
	Certificates []string          `json:"certificates"` // Secured domains
	PHPVersion   string            `json:"php_version"`  // Default PHP version
	Secure       bool              `json:"secure"`       // Is global HTTPS enabled?
	Port         string            `json:"port"`         // Main HTTP Port (default 80)
	Ignored      []string          `json:"ignored"`      // Ignored project paths
}

type Manager struct {
	mu       sync.RWMutex
	filePath string
	Data     *State
}

// NewManager creates a new State Manager pointing to the global config path.
func NewManager() (*Manager, error) {
	// Global path for multi-user support
	configDir := "/var/lib/sld"

	// Ensure directory exists (usually created by installer, but good safety)
	if err := os.MkdirAll(configDir, 0777); err != nil {
		return nil, err
	}

	return &Manager{
		filePath: filepath.Join(configDir, "state.json"),
		Data: &State{
			TLD:      "test",
			Paths:    []string{},
			Links:    make(map[string]string),
			Services: make(map[string]string),
			Port:     "80", // Default port
			Ignored:  []string{},
		},
	}, nil
}

// Load reads the state from disk.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if os.IsNotExist(err) {
		return m.Save() // Initialize new file
	}
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, m.Data); err != nil {
		return err
	}

	// Ensure default port if missing (e.g. old state file)
	if m.Data.Port == "" {
		m.Data.Port = "80"
	}

	// Ensure Initialized slices
	if m.Data.Paths == nil {
		m.Data.Paths = []string{}
	}
	if m.Data.Ignored == nil {
		m.Data.Ignored = []string{}
	}

	return nil
}

// Save writes the current state to disk.
func (m *Manager) Save() error {
	// Lock is assumed to be held by caller or we lock here if this is public
	// but usually we want atomic operations. For simplicity, we lock.
	// NOTE: In a real concurrent daemon, we might need finer grained locking.

	// For now, let's just write.
	data, err := json.MarshalIndent(m.Data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.filePath, data, 0644)
}

func (m *Manager) AddPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.Data.Paths {
		if p == path {
			return
		}
	}
	m.Data.Paths = append(m.Data.Paths, path)
	m.Save()
}

func (m *Manager) RemovePath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newPaths := []string{}
	for _, p := range m.Data.Paths {
		if p != path {
			newPaths = append(newPaths, p)
		}
	}
	m.Data.Paths = newPaths
	m.Save()
}

func (m *Manager) AddLink(name, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Data.Links == nil {
		m.Data.Links = make(map[string]string)
	}
	m.Data.Links[name] = path
	m.Save()
}

func (m *Manager) RemoveLink(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.Data.Links, name)
	m.Save()
}

func (m *Manager) AddIgnore(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.Data.Ignored {
		if p == path {
			return
		}
	}
	m.Data.Ignored = append(m.Data.Ignored, path)
	m.Save()
}

func (m *Manager) RemoveIgnore(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newPaths := []string{}
	for _, p := range m.Data.Ignored {
		if p != path {
			newPaths = append(newPaths, p)
		}
	}
	m.Data.Ignored = newPaths
	m.Save()
}
