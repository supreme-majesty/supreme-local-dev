package server

// Dockerizing workspace projects that SLD does not detect on its own (no
// Docker files, not parked or linked): the dashboard lists them, the user
// picks some, and SLD generates each one's Docker files from its detected
// stack (package dockerize), switches its SLD config to the new compose stack
// and, if asked, builds and starts it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/detect"
	"github.com/supreme-majesty/supreme-local-dev/internal/dockerize"
	v2 "github.com/supreme-majesty/supreme-local-dev/internal/v2"
)

func (s *Server) registerDockerize(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/candidates", s.handleCandidates)
	mux.HandleFunc("POST /api/v2/dockerize", s.handleDockerize)
}

// Candidate is a workspace project SLD does not detect yet.
type Candidate struct {
	Path   string       `json:"path"`   // folder inside the SLD container
	Folder string       `json:"folder"` // relative to its workspace
	Name   string       `json:"name"`
	Apps   []v2.AppInfo `json:"apps"`
	// Detected: SLD recognised the stack, so it can write the config itself.
	Detected bool `json:"detected"`
	// Configured: the folder already has a .sld/config.yaml.
	Configured bool     `json:"configured"`
	Problems   []string `json:"problems,omitempty"`
	// Reason explains why it cannot be dockerized on its own (a WordPress
	// plugin, an Apps Script add-on...); Detected is false then.
	Reason string `json:"reason,omitempty"`

	selectedApps []string // set for a dockerize request; empty = all apps
}

// candidates scans the workspaces for projects the normal scan skips,
// including app folders without .git (copies, WordPress sites...).
func (s *Server) candidates() ([]Candidate, error) {
	keep := s.keepProject()
	skipped := func(root string, apps []v2.AppInfo) bool { return !keep(root, apps) }
	out := []Candidate{}
	for _, ws := range s.opts.Workspaces {
		results, err := v2.ScanWith(ws, v2.ScanOptions{Keep: skipped, Manifests: true}) // never writes
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", ws, err)
		}
		for _, r := range results {
			c := Candidate{
				Path:       r.Root,
				Folder:     s.relFolder(r.Root),
				Name:       r.Name,
				Apps:       r.AppList,
				Configured: r.Status == "configured",
				Detected:   r.Status == "new" || r.Status == "configured",
				Problems:   r.Problems,
				Reason:     detect.NotServable(r.Root),
			}
			if c.Reason != "" {
				c.Detected = false
			}
			if c.Apps == nil {
				c.Apps = []v2.AppInfo{}
			}
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Folder < out[j].Folder })
	return out, nil
}

func (s *Server) handleCandidates(w http.ResponseWriter, r *http.Request) {
	list, err := s.candidates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleDockerize links the chosen candidates, writes their configs and,
// with "start", starts them one after another, all in one job.
func (s *Server) handleDockerize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		// Projects to dockerize, each with the apps (sub-folders) to include;
		// no apps means all of them.
		Projects []struct {
			Path string   `json:"path"`
			Apps []string `json:"apps"`
		} `json:"projects"`
		Paths []string `json:"paths"` // shorthand: whole projects
		Start bool     `json:"start"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	appsFor := map[string][]string{}
	for _, p := range req.Projects {
		req.Paths = append(req.Paths, p.Path)
		appsFor[p.Path] = p.Apps
	}
	if len(req.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "choose at least one project")
		return
	}
	list, err := s.candidates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Only folders the scan offered are accepted, never arbitrary paths.
	byPath := map[string]Candidate{}
	for _, c := range list {
		byPath[c.Path] = c
	}
	var chosen []Candidate
	for _, p := range req.Paths {
		c, ok := byPath[p]
		if !ok {
			writeError(w, http.StatusBadRequest, p+" is not a workspace project waiting to be dockerized")
			return
		}
		if c.Reason != "" {
			writeError(w, http.StatusBadRequest, c.Folder+" cannot be dockerized on its own: "+c.Reason)
			return
		}
		if !c.Detected {
			writeError(w, http.StatusBadRequest, "SLD could not detect the stack of "+c.Folder+"; add .sld/config.yaml to it by hand")
			return
		}
		for _, name := range appsFor[p] {
			found := false
			for _, a := range c.Apps {
				found = found || a.Name == name
			}
			if !found {
				writeError(w, http.StatusBadRequest, c.Folder+" has no app named "+name)
				return
			}
		}
		c.selectedApps = appsFor[p]
		chosen = append(chosen, c)
	}

	job, err := s.jobs.Start("dockerize", "", func(ctx context.Context, job *Job) error {
		defer s.invalidateScan()
		var failed []string
		for _, c := range chosen {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := s.dockerizeOne(ctx, job, c, req.Start); err != nil {
				fmt.Fprintf(job, "[SLD] ⚠️  %s: %v\n", c.Folder, err)
				failed = append(failed, c.Folder)
			}
		}
		if len(failed) > 0 {
			return fmt.Errorf("%d of %d failed: %s", len(failed), len(chosen), strings.Join(failed, ", "))
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, _ := job.Snapshot(0)
	writeJSON(w, http.StatusAccepted, view)
}

// dockerizeOne generates the project's Docker files (Dockerfile, compose.yaml,
// .dockerignore, DOCKER.md), points its SLD config at the new compose stack
// and, with start, builds and starts it.
func (s *Server) dockerizeOne(ctx context.Context, job *Job, c Candidate, start bool) error {
	fmt.Fprintf(job, "\n[SLD] ── %s\n", c.Folder)
	if len(c.selectedApps) > 0 {
		fmt.Fprintf(job, "[SLD] Only: %s\n", strings.Join(c.selectedApps, ", "))
	}
	plan, err := dockerize.PlanProjectWith(c.Path, dockerize.Options{Apps: c.selectedApps})
	if err != nil {
		return err
	}
	if err := plan.Write(); err != nil {
		return err
	}
	fmt.Fprintf(job, "[SLD] Profile: %s\n", plan.Profile)
	for _, r := range plan.Matrix {
		fmt.Fprintf(job, "  %-17s %s\n", r.Setting+":", r.Value)
	}
	for _, f := range plan.Files {
		fmt.Fprintf(job, "[SLD] Wrote %s\n", f.Path)
	}
	for _, a := range plan.Assumptions {
		fmt.Fprintf(job, "[SLD] Note: %s\n", a)
	}

	// SLD now runs the project through its own compose stack. An existing SLD
	// config describes the old setup: keep it as a backup and regenerate.
	cfgPath := filepath.Join(c.Path, ".sld", "config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		backup := cfgPath + ".pre-docker"
		if _, err := os.Stat(backup); err == nil {
			backup = fmt.Sprintf("%s.%d", backup, time.Now().Unix())
		}
		if err := os.Rename(cfgPath, backup); err != nil {
			return fmt.Errorf("could not back up .sld/config.yaml: %w", err)
		}
		fmt.Fprintf(job, "[SLD] Previous SLD config kept as .sld/%s\n", filepath.Base(backup))
	}
	results, err := v2.ScanWith(c.Path, v2.ScanOptions{
		Write: true,
		Keep:  func(root string, _ []v2.AppInfo) bool { return root == c.Path },
	})
	if err != nil {
		return err
	}
	written := false
	for _, res := range results {
		if res.Root == c.Path && res.Status == "written" {
			fmt.Fprintf(job, "[SLD] SLD config now uses the compose stack: %s\n", res.Apps)
			written = true
		}
		for _, p := range res.Problems {
			fmt.Fprintf(job, "[SLD] ⚠️  %s\n", p)
		}
	}
	if !written {
		return errors.New("the generated compose.yaml was not recognised; see DOCKER.md")
	}
	s.invalidateScan()
	if !start {
		return nil
	}
	id, err := s.opts.Env.ProjectIdentity(c.Path)
	if err != nil {
		return err
	}
	if err := s.newRunner(job).Start(ctx, c.Path); err != nil {
		return fmt.Errorf("starting: %w", err)
	}
	s.recordRunning(id.ID, "start")
	fmt.Fprintf(job, "[SLD] ✅ %s is running\n", c.Name)
	return nil
}
