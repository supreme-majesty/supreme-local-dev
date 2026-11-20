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
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	var results []ScanResult
	for _, root := range detect.FindProjects(abs, 3) {
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
			results = append(results, res)
			continue
		}

		detected, err := detect.Project(root, detect.Options{})
		if errors.Is(err, detect.ErrNothingDetected) {
			res.Name = filepath.Base(root)
			res.Status = "undetected"
			results = append(results, res)
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
			results = append(results, res)
			continue
		}
		res.fill(cfg)
		for _, n := range detected.Notes {
			if strings.Contains(n, "⚠️") {
				res.Problems = append(res.Problems, strings.TrimSpace(strings.ReplaceAll(n, "⚠️", "")))
			}
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
		fmt.Println("\nRun 'sld v2 scan --write' to create .sld/config.yaml for projects marked \"new\" (existing configs are never changed).")
	}
}
