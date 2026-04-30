package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Manifest tracks the installed state of the system dependencies.
type Manifest struct {
	InstalledPackages []string          `json:"installed_packages"`
	PHPVersions       []string          `json:"php_versions"`
	NodeVersions      []string          `json:"node_versions"`
	Certificates      map[string]string `json:"certificates"` // domain -> hash/timestamp
	path              string
}

func LoadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "manifest.json")
	m := &Manifest{
		Certificates: make(map[string]string),
		path:         path,
	}

	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, m); err != nil {
			return nil, err
		}
	}

	m.path = path // Ensure path is set
	return m, nil
}

func (m *Manifest) Save() error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0644)
}

func (m *Manifest) IsPackageInstalled(name string) bool {
	for _, p := range m.InstalledPackages {
		if p == name {
			return true
		}
	}
	return false
}

func (m *Manifest) AddPackage(name string) {
	if !m.IsPackageInstalled(name) {
		m.InstalledPackages = append(m.InstalledPackages, name)
	}
}
