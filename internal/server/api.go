package server

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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/internal/tls"
	v2 "github.com/supreme-majesty/supreme-local-dev/internal/v2"
)

// Options configure the control-plane API.
type Options struct {
	Engine     docker.EngineClient
	Env        *hostenv.Env
	Auth       *Auth
	Workspaces []string // folders scanned for projects
	Version    string
}

// Server is the V2 control-plane API.
type Server struct {
	opts     Options
	jobs     *Jobs
	settings *settingsStore

	mu        sync.Mutex
	projects  map[string]v2.ScanResult // project id -> last scan result
	scannedAt time.Time
}

// New creates the API.
func New(opts Options) *Server {
	settings, err := loadSettings(opts.Env.DataDir())
	if err != nil {
		fmt.Printf("[SLD] ⚠️  %v — using default automation settings\n", err)
		settings = &settingsStore{path: filepath.Join(opts.Env.DataDir(), "settings.json"), s: defaultSettings()}
	}
	return &Server{opts: opts, jobs: NewJobs(), settings: settings, projects: map[string]v2.ScanResult{}}
}

// Register adds the /api/v2 routes to mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/auth/login", s.opts.Auth.handleLogin)
	mux.HandleFunc("/api/v2/auth/logout", s.opts.Auth.handleLogout)
	mux.HandleFunc("/api/v2/auth/status", s.opts.Auth.handleStatus)

	mux.HandleFunc("GET /api/v2/system", s.handleSystem)
	mux.HandleFunc("POST /api/v2/system/init", s.handleSystemInit)
	mux.HandleFunc("GET /api/v2/system/ca.crt", s.handleCACert)

	mux.HandleFunc("GET /api/v2/projects", s.handleProjects)
	mux.HandleFunc("GET /api/v2/projects/{id}", s.handleProject)
	mux.HandleFunc("POST /api/v2/projects/{id}/{action}", s.handleProjectAction)
	mux.HandleFunc("GET /api/v2/projects/{id}/logs", s.handleLogs)

	mux.HandleFunc("POST /api/v2/projects/{id}/autostart", s.handleAutostart)
	mux.HandleFunc("GET /api/v2/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/v2/settings", s.handlePutSettings)

	mux.HandleFunc("GET /api/v2/jobs/{id}", s.handleJob)
	mux.HandleFunc("GET /api/v2/jobs/{id}/events", s.handleJobEvents)

	// The original dashboard pages, backed by Docker.
	s.registerClassic(mux)
	s.registerPlugins(mux)
	s.registerShare(mux)
	s.registerCreate(mux)
	s.registerProjectFiles(mux)
	s.registerParking(mux)
	s.registerDockerize(mux)
	s.registerHTTPS(mux)
}

// newRunner creates a runner whose output goes to w. Each job gets its own
// runner so concurrent jobs don't share output or build-log settings.
func (s *Server) newRunner(w io.Writer) *v2.Runner {
	r := v2.NewRunner(s.opts.Engine)
	r.SetEnv(s.opts.Env)
	r.SetOutput(w)
	return r
}

// --- projects ---

// ProjectView is a project as shown on the dashboard.
type ProjectView struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Folder     string       `json:"folder"` // relative to its workspace
	Status     string       `json:"status"` // running, partial, stopped, not-configured, undetected, invalid
	Configured bool         `json:"configured"`
	Apps       []AppView    `json:"apps"`
	Containers int          `json:"containers"`
	Running    int          `json:"running"`
	Problems   []string     `json:"problems,omitempty"`
	JobID      string       `json:"job_id,omitempty"` // running operation, if any
	Autostart  bool         `json:"autostart"`        // starts whenever SLD starts
	Services   []ServiceRef `json:"services,omitempty"`
	Dockerized bool         `json:"dockerized"`          // own compose file or Dockerfile, or has containers
	ParkedIn   string       `json:"parked_in,omitempty"` // parked/linked folder that covers it, relative to its workspace
	TLS        bool         `json:"tls"`                 // served over HTTPS (domain.tls)
}

// AppView is one app of a project with its URL.
type AppView struct {
	v2.AppInfo
	URL string `json:"url"`
}

// ServiceRef is one container of a project.
type ServiceRef struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	App     string `json:"app,omitempty"`
	State   string `json:"state"`
}

func (s *Server) scan(force bool) ([]v2.ScanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !force && time.Since(s.scannedAt) < 10*time.Second && len(s.projects) > 0 {
		out := make([]v2.ScanResult, 0, len(s.projects))
		for _, p := range s.projects {
			out = append(out, p)
		}
		return out, nil
	}
	keep := s.keepProject() // only dockerized and parked/linked projects are detected
	var all []v2.ScanResult
	for _, ws := range s.opts.Workspaces {
		// With auto setup on, newly found projects get their config written now.
		results, err := v2.ScanWith(ws, v2.ScanOptions{Write: s.settings.Get().AutoSetup, Keep: keep})
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", ws, err)
		}
		all = append(all, results...)
	}
	s.projects = map[string]v2.ScanResult{}
	for _, r := range all {
		if id, err := s.opts.Env.ProjectIdentity(r.Root); err == nil {
			s.projects[id.ID] = r
		}
	}
	s.scannedAt = time.Now()
	return all, nil
}

func (s *Server) view(ctx context.Context, r v2.ScanResult) ProjectView {
	id, _ := s.opts.Env.ProjectIdentity(r.Root)
	v := ProjectView{Name: r.Name, Folder: s.relFolder(r.Root), Problems: r.Problems, TLS: r.TLS}
	if id != nil {
		v.ID = id.ID
		v.JobID = s.jobs.Running(id.ID)
		v.Autostart = s.settings.Get().Autostart[id.ID]
	}
	// "written" means auto setup created the config during this scan.
	v.Configured = r.Status == "configured" || r.Status == "written"
	for _, a := range r.AppList {
		v.Apps = append(v.Apps, AppView{AppInfo: a, URL: s.opts.Env.SiteURL(a.Domain, r.TLS)})
	}

	switch r.Status {
	case "configured", "written":
		v.Status = "stopped"
	case "new":
		v.Status = "not-configured"
	case "undetected":
		v.Status = "undetected"
	default:
		v.Status = "invalid"
	}
	if id != nil {
		if containers, err := s.opts.Engine.ListProjectContainers(ctx, id.ID); err == nil {
			for _, c := range containers {
				v.Containers++
				if c.State == "running" {
					v.Running++
				}
				v.Services = append(v.Services, ServiceRef{Name: c.Name, Service: c.Service, App: c.Labels[docker.LabelApp], State: c.State})
			}
			sort.Slice(v.Services, func(i, j int) bool { return v.Services[i].Name < v.Services[j].Name })
		}
	}
	// Having an SLD config is not enough: auto setup writes one for every
	// detected folder. Dockerized means the project brings its own Docker files
	// or SLD has actually created containers for it.
	v.Dockerized = v.Containers > 0 || dockerizedApps(r.AppList) || v2.HasDockerFiles(r.Root)
	if p := parkedIn(s.settings.Get().Parked, r.Root); p != "" {
		v.ParkedIn = s.relFolder(p)
	}
	if v.Configured && v.Containers > 0 {
		switch {
		case v.Running == v.Containers:
			v.Status = "running"
		case v.Running > 0:
			v.Status = "partial"
		}
	}
	return v
}

func (s *Server) relFolder(root string) string {
	for _, ws := range s.opts.Workspaces {
		if rel, err := filepath.Rel(ws, root); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return root
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	results, err := s.scan(r.URL.Query().Get("refresh") == "1")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]ProjectView, 0, len(results))
	for _, res := range results {
		views = append(views, s.view(r.Context(), res))
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Folder < views[j].Folder })
	writeJSON(w, http.StatusOK, views)
}

// lookup finds a scanned project by id, rescanning once if it is unknown.
func (s *Server) lookup(id string) (v2.ScanResult, bool) {
	if _, err := s.scan(false); err != nil {
		return v2.ScanResult{}, false
	}
	s.mu.Lock()
	p, ok := s.projects[id]
	s.mu.Unlock()
	if ok {
		return p, true
	}
	if _, err := s.scan(true); err != nil {
		return v2.ScanResult{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok = s.projects[id]
	return p, ok
}

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	view := s.view(r.Context(), p)
	resp := map[string]any{"project": view}
	if data, err := os.ReadFile(filepath.Join(p.Root, ".sld", "config.yaml")); err == nil {
		resp["config"] = string(data)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleProjectAction(w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	p, ok := s.lookup(id)
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var body struct {
		RemoveVolumes bool `json:"remove_volumes"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)

	var fn func(ctx context.Context, job *Job) error
	switch action {
	case "setup":
		fn = func(ctx context.Context, job *Job) error {
			results, err := v2.Scan(p.Root, true)
			if err != nil {
				return err
			}
			for _, res := range results {
				if res.Root == p.Root {
					fmt.Fprintf(job, "[SLD] %s: %s → %s\n", res.Name, res.Apps, res.Status)
					if res.Status == "undetected" {
						return errors.New("could not detect this project's stack; add .sld/config.yaml by hand")
					}
				}
			}
			return nil
		}
	case "start":
		fn = func(ctx context.Context, job *Job) error { return s.newRunner(job).Start(ctx, p.Root) }
	case "stop":
		fn = func(ctx context.Context, job *Job) error { return s.newRunner(job).Stop(ctx, p.Root) }
	case "restart":
		fn = func(ctx context.Context, job *Job) error {
			runner := s.newRunner(job)
			if err := runner.Stop(ctx, p.Root); err != nil {
				return err
			}
			return runner.Start(ctx, p.Root)
		}
	case "destroy":
		fn = func(ctx context.Context, job *Job) error {
			return s.newRunner(job).Destroy(ctx, p.Root, body.RemoveVolumes, true)
		}
	default:
		writeError(w, http.StatusNotFound, "unknown action "+action)
		return
	}

	job, err := s.jobs.Start(action, id, func(ctx context.Context, job *Job) error {
		err := fn(ctx, job)
		if err == nil {
			s.recordRunning(id, action)
		}
		s.mu.Lock()
		s.scannedAt = time.Time{} // status/config changed: rescan on next list
		s.mu.Unlock()
		return err
	})
	var busy ErrBusy
	if errors.As(err, &busy) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "job_id": busy.JobID})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, _ := job.Snapshot(0)
	writeJSON(w, http.StatusAccepted, view)
}

// handleLogs returns (or with follow=1 streams) the logs of a project container.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	p, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	id, err := s.opts.Env.ProjectIdentity(p.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	containers, err := s.opts.Engine.ListProjectContainers(r.Context(), id.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg, _, _ := v2.LoadProjectConfig(p.Root)
	target, err := v2.ResolveContainer(containers, cfg, r.URL.Query().Get("target"), true)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	tail := r.URL.Query().Get("tail")
	if n, err := strconv.Atoi(tail); err != nil || n <= 0 || n > 5000 {
		tail = "200"
	}
	follow := r.URL.Query().Get("follow") == "1"
	reader, err := s.opts.Engine.ContainerLogs(r.Context(), target.ID, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: follow, Tail: tail, Timestamps: r.URL.Query().Get("timestamps") == "1",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-SLD-Container", target.Name)
	out := io.Writer(w)
	if f, ok := w.(http.Flusher); ok && follow {
		out = flushWriter{w: w, f: f}
	}
	_, _ = stdcopy.StdCopy(out, out, reader)
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.f.Flush()
	return n, err
}

// --- jobs ---

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.jobs.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	view, _ := job.Snapshot(offset)
	writeJSON(w, http.StatusOK, view)
}

// handleJobEvents streams job output as Server-Sent Events: "output" events
// carry new text, a final "done" event carries the job's final state.
func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	job, ok := s.jobs.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	for {
		view, updated := job.Snapshot(offset)
		if view.Output != "" {
			data, _ := json.Marshal(view.Output)
			fmt.Fprintf(w, "event: output\ndata: %s\n\n", data)
		}
		offset = view.Offset
		if view.Status != JobRunning {
			view.Output = ""
			data, _ := json.Marshal(view)
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
			flusher.Flush()
			return
		}
		flusher.Flush()
		select {
		case <-updated:
		case <-r.Context().Done():
			return
		case <-time.After(15 * time.Second):
			fmt.Fprint(w, ": keep-alive\n\n")
		}
	}
}

// --- system ---

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	env := s.opts.Env
	info := map[string]any{
		"version":      s.opts.Version,
		"in_container": env.InContainer(),
		"data_dir":     env.DataDir(),
		"bind_host":    env.BindHost(),
		"http_port":    env.HTTPPort(),
		"https_port":   env.HTTPSPort(),
		"tld":          config.DefaultTLD(),
		"workspaces":   s.opts.Workspaces,
	}
	dockerOK := s.opts.Engine.Ping(r.Context()) == nil
	info["docker"] = dockerOK
	routerState := "missing"
	if dockerOK {
		if containers, err := s.opts.Engine.ListAllContainers(r.Context()); err == nil {
			for _, c := range containers {
				if c.Name == orchestrate.RouterContainer {
					routerState = c.State
				}
			}
		}
	}
	info["router"] = routerState
	if certMgr, err := tls.NewManager(env.DataDir()); err == nil {
		info["ca_available"] = certMgr.HasCA()
	}
	var mounts []map[string]string
	for _, m := range env.Mounts() {
		mounts = append(mounts, map[string]string{"type": m.Type, "destination": m.Destination, "source": m.Source, "volume": m.Name})
	}
	info["mounts"] = mounts
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleSystemInit(w http.ResponseWriter, r *http.Request) {
	job, err := s.jobs.Start("init", "", func(ctx context.Context, job *Job) error {
		return s.newRunner(job).Init(ctx)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, _ := job.Snapshot(0)
	writeJSON(w, http.StatusAccepted, view)
}

func (s *Server) handleCACert(w http.ResponseWriter, r *http.Request) {
	certMgr, err := tls.NewManager(s.opts.Env.DataDir())
	if err != nil || !certMgr.HasCA() {
		writeError(w, http.StatusNotFound, "no CA yet: initialise SLD first")
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="sld-ca.crt"`)
	http.ServeFile(w, r, certMgr.CACertPath())
}
