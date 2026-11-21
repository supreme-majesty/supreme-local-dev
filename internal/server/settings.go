package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Settings are the control plane's automation preferences, stored in the
// data directory (settings.json).
type Settings struct {
	// AutoSetup writes .sld/config.yaml for newly found projects.
	AutoSetup bool `json:"auto_setup"`
	// RestoreRunning starts again, when SLD starts, the projects that were
	// running when it stopped.
	RestoreRunning bool `json:"restore_running"`
	// AutostartAll starts every set-up project when SLD starts.
	AutostartAll bool `json:"autostart_all"`
	// Autostart lists projects (by id) that always start with SLD.
	Autostart map[string]bool `json:"autostart,omitempty"`
	// Running remembers projects started through SLD and not stopped since.
	Running map[string]bool `json:"running,omitempty"`
	// SiteMeta holds the tags and category set on the Sites page, by domain.
	SiteMeta map[string]SiteMeta `json:"site_meta,omitempty"`
	// Parked lists folders (container paths) whose projects are detected even
	// without Docker files: a project folder itself (linked) or a folder of
	// projects (parked).
	Parked []string `json:"parked,omitempty"`
}

// SiteMeta is how a project is labelled on the Sites page.
type SiteMeta struct {
	Tags     []string `json:"tags,omitempty"`
	Category string   `json:"category,omitempty"`
}

// defaultSettings: start nothing on its own; projects are started by hand.
func defaultSettings() Settings {
	return Settings{Autostart: map[string]bool{}, Running: map[string]bool{}}
}

// settingsStore persists Settings as JSON.
type settingsStore struct {
	path string
	mu   sync.Mutex
	s    Settings
}

func loadSettings(dataDir string) (*settingsStore, error) {
	st := &settingsStore{path: filepath.Join(dataDir, "settings.json"), s: defaultSettings()}
	data, err := os.ReadFile(st.path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &st.s); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", st.path, err)
	}
	if st.s.Autostart == nil {
		st.s.Autostart = map[string]bool{}
	}
	if st.s.Running == nil {
		st.s.Running = map[string]bool{}
	}
	return st, nil
}

// Get returns a copy of the settings.
func (st *settingsStore) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.copyLocked()
}

func (st *settingsStore) copyLocked() Settings {
	c := st.s
	c.Autostart = make(map[string]bool, len(st.s.Autostart))
	for k, v := range st.s.Autostart {
		c.Autostart[k] = v
	}
	c.Running = make(map[string]bool, len(st.s.Running))
	for k, v := range st.s.Running {
		c.Running[k] = v
	}
	c.Parked = append([]string(nil), st.s.Parked...)
	c.SiteMeta = make(map[string]SiteMeta, len(st.s.SiteMeta))
	for k, v := range st.s.SiteMeta {
		c.SiteMeta[k] = SiteMeta{Tags: append([]string(nil), v.Tags...), Category: v.Category}
	}
	return c
}

// Update applies fn to the settings and saves them.
func (st *settingsStore) Update(fn func(*Settings)) (Settings, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	fn(&st.s)
	for id, on := range st.s.Autostart { // keep the file small
		if !on {
			delete(st.s.Autostart, id)
		}
	}
	for id, on := range st.s.Running {
		if !on {
			delete(st.s.Running, id)
		}
	}
	data, err := json.MarshalIndent(st.s, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
		return Settings{}, err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return Settings{}, err
	}
	if err := os.Rename(tmp, st.path); err != nil {
		return Settings{}, err
	}
	return st.copyLocked(), nil
}

// startupTargets picks, in a stable order, the set-up projects to start when
// SLD starts. Projects already running are skipped.
func startupTargets(projects []ProjectView, s Settings) []ProjectView {
	var out []ProjectView
	for _, p := range projects {
		if !p.Configured || p.ID == "" || p.Status == "running" {
			continue
		}
		if s.AutostartAll || s.Autostart[p.ID] || (s.RestoreRunning && s.Running[p.ID]) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Folder < out[j].Folder })
	return out
}
