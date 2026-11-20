package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// recordRunning remembers whether a project should be running, so it can be
// started again when SLD restarts (RestoreRunning).
func (s *Server) recordRunning(projectID, action string) {
	var running, known bool
	switch action {
	case "start", "restart":
		running, known = true, true
	case "stop", "destroy":
		running, known = false, true
	}
	if !known {
		return
	}
	if _, err := s.settings.Update(func(st *Settings) { st.Running[projectID] = running }); err != nil {
		fmt.Printf("[SLD] ⚠️  could not save project state: %v\n", err)
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, publicSettings(s.settings.Get()))
}

// handlePutSettings updates the global automation switches. Fields that are
// omitted keep their value.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AutoSetup      *bool `json:"auto_setup"`
		RestoreRunning *bool `json:"restore_running"`
		AutostartAll   *bool `json:"autostart_all"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	updated, err := s.settings.Update(func(st *Settings) {
		if req.AutoSetup != nil {
			st.AutoSetup = *req.AutoSetup
		}
		if req.RestoreRunning != nil {
			st.RestoreRunning = *req.RestoreRunning
		}
		if req.AutostartAll != nil {
			st.AutostartAll = *req.AutostartAll
		}
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req.AutoSetup != nil && *req.AutoSetup {
		s.invalidateScan() // set up newly found projects on the next listing
	}
	writeJSON(w, http.StatusOK, publicSettings(updated))
}

// handleAutostart turns "start with SLD" on or off for one project.
func (s *Server) handleAutostart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.lookup(id); !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if _, err := s.settings.Update(func(st *Settings) { st.Autostart[id] = req.Enabled }); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "autostart": req.Enabled})
}

// publicSettings is the settings as shown to the dashboard (without the
// per-project bookkeeping maps).
func publicSettings(st Settings) map[string]any {
	return map[string]any{
		"auto_setup":      st.AutoSetup,
		"restore_running": st.RestoreRunning,
		"autostart_all":   st.AutostartAll,
	}
}

func (s *Server) invalidateScan() {
	s.mu.Lock()
	s.scannedAt = time.Time{}
	s.mu.Unlock()
}

// RunStartupTasks runs once when SLD starts: it sets up new projects (when
// auto setup is on) and starts the projects that should run, one at a time so
// a laptop is not asked to boot every stack at once. Each start is a normal
// job, visible in the dashboard.
func (s *Server) RunStartupTasks(ctx context.Context) {
	if s.opts.Engine.Ping(ctx) != nil {
		fmt.Println("[SLD] Docker is not reachable yet; skipping automatic project starts.")
		return
	}
	s.invalidateScan()
	results, err := s.scan(true) // also writes configs for new projects when auto setup is on
	if err != nil {
		fmt.Printf("[SLD] ⚠️  startup scan failed: %v\n", err)
		return
	}
	views := make([]ProjectView, 0, len(results))
	for _, r := range results {
		views = append(views, s.view(ctx, r))
	}
	// Projects already running (e.g. started from the CLI) count as "was running".
	for _, v := range views {
		if v.Configured && v.ID != "" && (v.Status == "running" || v.Status == "partial") {
			s.recordRunning(v.ID, "start")
		}
	}

	targets := startupTargets(views, s.settings.Get())
	if len(targets) == 0 {
		return
	}
	fmt.Printf("[SLD] Starting %d project(s) automatically...\n", len(targets))
	for _, p := range targets {
		if ctx.Err() != nil {
			return
		}
		res, ok := s.lookup(p.ID)
		if !ok {
			continue
		}
		root := res.Root
		job, err := s.jobs.Start("start", p.ID, func(ctx context.Context, job *Job) error {
			fmt.Fprintf(job, "[SLD] Automatic start of %s (SLD started)\n", p.Name)
			err := s.newRunner(job).Start(ctx, root)
			if err == nil {
				s.recordRunning(p.ID, "start")
			}
			s.invalidateScan()
			return err
		})
		if err != nil {
			fmt.Printf("[SLD] ⚠️  could not start %s: %v\n", p.Name, err)
			continue
		}
		view := waitJob(ctx, job)
		if view.Status == JobFailed {
			fmt.Printf("[SLD] ⚠️  %s failed to start: %s\n", p.Name, view.Error)
		} else {
			fmt.Printf("[SLD] ✅ %s started\n", p.Name)
		}
	}
}

// waitJob blocks until a job finishes (or ctx ends) and returns its state.
func waitJob(ctx context.Context, job *Job) JobView {
	for {
		view, updated := job.Snapshot(0)
		if view.Status != JobRunning {
			return view
		}
		select {
		case <-updated:
		case <-ctx.Done():
			return view
		}
	}
}
