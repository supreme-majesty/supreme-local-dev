package v2

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/detect"
	appruntime "github.com/supreme-majesty/supreme-local-dev/internal/runtime"
)

// ScanResult describes one project found by Scan.
type ScanResult struct {
	Root     string
	Name     string
	Domains  []string
	Apps     string // summary, e.g. "web (php 8.3), api (python 3.13)"
	AppList  []AppInfo
	TLS      bool
	Status   string // "configured", "new", "written", "undetected"
	Problems []string
}

// Scan finds every project under dir, detects its stack and, with write,
// creates .sld/config.yaml for projects that do not have one yet. Existing
// configs are never overwritten.
func Scan(dir string, write bool) ([]ScanResult, error) {
	return ScanWith(dir, ScanOptions{Write: write})
}

// ScanOptions tune ScanWith.
type ScanOptions struct {
	// Write creates .sld/config.yaml for detected projects that lack one.
	Write bool
	// Keep, when set, decides which projects are reported (and written). It
	// sees the project folder and its apps (empty when the stack is unknown).
	Keep func(root string, apps []AppInfo) bool
	// Manifests also treats folders with an app manifest (composer.json,
	// package.json, wp-config.php, ...) as projects, even without .git.
	Manifests bool
}

// ScanWith is Scan with options.
func ScanWith(dir string, opts ScanOptions) ([]ScanResult, error) {
	write := opts.Write
	keep := func(r ScanResult) bool { return opts.Keep == nil || opts.Keep(r.Root, r.AppList) }
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	var results []ScanResult
	for _, root := range detect.FindProjectsWith(abs, 3, opts.Manifests) {
		res := ScanResult{Root: root}
		configPath := filepath.Join(root, ".sld", "config.yaml")

		if data, err := os.ReadFile(configPath); err == nil {
			cfg, err := config.LoadFromBytes(data, filepath.Base(root))
			if err != nil {
				res.Status = "invalid config"
				res.Problems = append(res.Problems, err.Error())
			} else {
				res.fill(cfg)
				res.Status = "configured"
			}
			if keep(res) {
				results = append(results, res)
			}
			continue
		}

		detected, err := detect.Project(root, detect.Options{})
		if errors.Is(err, detect.ErrNothingDetected) {
			res.Name = filepath.Base(root)
			res.Status = "undetected"
			if keep(res) {
				results = append(results, res)
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		output, err := config.Marshal(detected.Config)
		if err != nil {
			return nil, err
		}
		cfg, err := config.LoadFromBytes(output, filepath.Base(root))
		if err != nil {
			res.Name = detected.Config.Name
			res.Status = "undetected"
			res.Problems = append(res.Problems, err.Error())
			if keep(res) {
				results = append(results, res)
			}
			continue
		}
		res.fill(cfg)
		for _, n := range detected.Notes {
			if strings.Contains(n, "⚠️") {
				res.Problems = append(res.Problems, strings.TrimSpace(strings.ReplaceAll(n, "⚠️", "")))
			}
		}
		if !keep(res) {
			continue // not reported, and no config is written for it
		}
		res.Status = "new"
		if write {
			if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(configPath, output, 0o644); err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", configPath, err)
			}
			res.Status = "written"
		}
		results = append(results, res)
	}
	return results, nil
}

func (r *ScanResult) fill(cfg *config.Config) {
	r.Name = cfg.Name
	r.Apps = cfg.Describe()
	r.TLS = cfg.Domain.TLS
	for _, app := range cfg.Apps {
		r.Domains = append(r.Domains, app.Domain)
		r.AppList = append(r.AppList, AppInfo{Name: app.Name, Runtime: app.Runtime, Version: app.Version, Domain: app.Domain, Path: app.Path, Service: app.Service})
	}
}

// AppInfo summarises one app of a scanned project.
type AppInfo struct {
	Name    string `json:"name"`
	Runtime string `json:"runtime"`
	Version string `json:"version,omitempty"`
	Domain  string `json:"domain"`
	Path    string `json:"path,omitempty"`
	Service string `json:"service,omitempty"`
}

// PrintScan prints scan results as a table.
func PrintScan(base string, results []ScanResult, write bool) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tFOLDER\tDOMAIN\tAPPS\tSTATUS")
	for _, r := range results {
		rel, err := filepath.Rel(base, r.Root)
		if err != nil {
			rel = r.Root
		}
		domain := "-"
		if len(r.Domains) > 0 {
			domain = r.Domains[0]
			if len(r.Domains) > 1 {
				domain += fmt.Sprintf(" (+%d)", len(r.Domains)-1)
			}
		}
		apps := r.Apps
		if apps == "" {
			apps = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Name, filepath.ToSlash(rel), domain, apps, r.Status)
	}
	_ = w.Flush()

	for _, r := range results {
		for _, p := range r.Problems {
			fmt.Printf("  ⚠️  %s: %s\n", r.Name, p)
		}
	}
	if !write {
		fmt.Println("\nRun 'sld scan --write' to create .sld/config.yaml for projects marked \"new\" (existing configs are never changed).")
	}
}

// dockerFileSkip are folders never searched for Docker files.
var dockerFileSkip = map[string]bool{"node_modules": true, "vendor": true, "venv": true, ".venv": true, "dist": true, "build": true, "target": true}

// HasDockerFiles reports whether root has its own compose file or Dockerfile,
// at its top level or in a sub-folder up to two levels down (stacks such as
// sunport-apps/backend/compose.yaml).
func HasDockerFiles(root string) bool {
	var look func(dir string, depth int) bool
	look = func(dir string, depth int) bool {
		if len(appruntime.DefaultComposeFiles(dir)) > 0 {
			return true
		}
		if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err == nil {
			return true
		}
		if depth >= 2 {
			return false
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return false
		}
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() || strings.HasPrefix(name, ".") || dockerFileSkip[strings.ToLower(name)] {
				continue
			}
			if look(filepath.Join(dir, name), depth+1) {
				return true
			}
		}
		return false
	}
	return look(root, 0)
}
