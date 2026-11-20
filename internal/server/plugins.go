package server

// Plugins are shared services SLD runs as containers on the router network,
// reachable from every project by name (redis, mailpit, postgres). When SLD's
// own compose stack already runs the service (redis, mailpit), the plugin is
// that container rather than a second copy.

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

const pluginLabel = "com.supreme.sld.plugin"

type pluginDef struct {
	ID          string
	Name        string
	Description string
	Image       string
	Env         map[string]string
	DataDir     string // persisted in a named volume
	Healthcheck []string
	// StackService is the service name of the same thing in SLD's compose stack.
	StackService string
}

var pluginDefs = []pluginDef{
	{
		ID: "redis", Name: "Redis", Description: "In-memory data store and cache (host: sld-plugin-redis, port 6379)",
		Image: "redis:7-alpine", DataDir: "/data", StackService: "redis",
		Healthcheck: []string{"CMD", "redis-cli", "ping"},
	},
	{
		ID: "mailpit", Name: "Mailpit", Description: "Catches outgoing mail (SMTP: sld-plugin-mailpit:1025)",
		Image: "axllent/mailpit:v1.31.2", DataDir: "/data", StackService: "mailpit",
		Env: map[string]string{"MP_DATABASE": "/data/mailpit.db"},
	},
	{
		ID: "postgres", Name: "PostgreSQL", Description: "Shared PostgreSQL server (host: sld-plugin-postgres, user/password: postgres)",
		Image: "postgres:16-alpine", DataDir: "/var/lib/postgresql/data",
		Env:         map[string]string{"POSTGRES_PASSWORD": "postgres"},
		Healthcheck: []string{"CMD", "pg_isready", "-U", "postgres"},
	},
}

func findPlugin(id string) (pluginDef, bool) {
	for _, p := range pluginDefs {
		if p.ID == id {
			return p, true
		}
	}
	return pluginDef{}, false
}

func (s *Server) registerPlugins(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/plugins", s.handlePlugins)
	mux.HandleFunc("POST /api/plugins/install", s.handlePluginInstall)
	mux.HandleFunc("POST /api/plugins/toggle", s.handlePluginToggle)
	mux.HandleFunc("GET /api/plugins/logs", s.handlePluginLogs)
	mux.HandleFunc("GET /api/plugins/health", s.handlePluginHealth)
}

// pluginContainer finds the container serving a plugin: SLD's own plugin
// container, or the same service in SLD's compose stack.
func (s *Server) pluginContainer(ctx context.Context, p pluginDef) (*reconcile.ActualContainer, error) {
	containers, err := s.opts.Engine.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}
	stack := s.selfComposeProject(ctx)
	var fromStack *reconcile.ActualContainer
	for _, c := range containers {
		if c.Labels[pluginLabel] == p.ID {
			return c, nil
		}
		if stack != "" && p.StackService != "" && c.Labels[composeProjectLabel] == stack && c.Labels[composeServiceLabel] == p.StackService {
			fromStack = c
		}
	}
	return fromStack, nil
}

type pluginView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Status      string `json:"status"` // running, stopped, not_installed
	Installed   bool   `json:"installed"`
}

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	views := []pluginView{}
	for _, p := range pluginDefs {
		v := pluginView{ID: p.ID, Name: p.Name, Description: p.Description, Version: imageTag(p.Image), Status: "not_installed"}
		c, err := s.pluginContainer(r.Context(), p)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if c != nil {
			v.Installed = true
			v.Version = imageTag(c.Image)
			v.Status = "stopped"
			if c.State == "running" {
				v.Status = "running"
			}
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, views)
}

func decodeID(r *http.Request) (pluginDef, bool, bool) {
	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
	if req.ID == "" {
		req.ID = r.URL.Query().Get("id")
	}
	p, ok := findPlugin(req.ID)
	return p, req.Enabled, ok
}

// ensurePluginContainer creates the plugin's container (pulling its image)
// unless one exists already.
func (s *Server) ensurePluginContainer(ctx context.Context, p pluginDef) (*reconcile.ActualContainer, error) {
	if c, err := s.pluginContainer(ctx, p); err != nil || c != nil {
		return c, err
	}
	if _, err := s.opts.Engine.EnsureNetwork(ctx, orchestrate.GlobalNetwork, nil); err != nil {
		return nil, err
	}
	volume := "sld-plugin-" + p.ID + "-data"
	if _, err := s.opts.Engine.EnsureVolume(ctx, volume, map[string]string{pluginLabel: p.ID}); err != nil {
		return nil, err
	}
	desired := &reconcile.DesiredContainer{
		Name:        "sld-plugin-" + p.ID,
		Image:       p.Image,
		Environment: p.Env,
		Networks:    []string{orchestrate.GlobalNetwork},
		Volumes:     []string{volume + ":" + p.DataDir},
		Labels:      map[string]string{pluginLabel: p.ID},
	}
	if len(p.Healthcheck) > 0 {
		desired.Healthcheck = &reconcile.HealthcheckConfig{Test: p.Healthcheck}
	}
	if _, err := s.opts.Engine.CreateContainer(ctx, desired); err != nil {
		return nil, err
	}
	return s.pluginContainer(ctx, p)
}

func (s *Server) handlePluginInstall(w http.ResponseWriter, r *http.Request) {
	p, _, ok := decodeID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "plugin not found")
		return
	}
	if _, err := s.ensurePluginContainer(r.Context(), p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handlePluginToggle(w http.ResponseWriter, r *http.Request) {
	p, enabled, ok := decodeID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "plugin not found")
		return
	}
	var err error
	if enabled {
		var c *reconcile.ActualContainer
		if c, err = s.ensurePluginContainer(r.Context(), p); err == nil && c != nil && c.State != "running" {
			err = s.opts.Engine.StartContainer(r.Context(), c.ID)
		}
	} else {
		var c *reconcile.ActualContainer
		if c, err = s.pluginContainer(r.Context(), p); err == nil && c != nil && c.State == "running" {
			err = s.opts.Engine.StopContainer(r.Context(), c.ID, 10)
		}
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handlePluginLogs(w http.ResponseWriter, r *http.Request) {
	p, _, ok := decodeID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "plugin not found")
		return
	}
	lines := 100
	if n, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && n > 0 && n <= 5000 {
		lines = n
	}
	c, err := s.pluginContainer(r.Context(), p)
	if err != nil || c == nil {
		writeJSON(w, http.StatusOK, map[string]any{"logs": []string{p.Name + " is not installed"}})
		return
	}
	rc, err := s.opts.Engine.ContainerLogs(r.Context(), c.ID, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(lines),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rc.Close()
	pr, pw := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, rc)
		pw.CloseWithError(err)
	}()
	out := []string{}
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": out})
}

func (s *Server) handlePluginHealth(w http.ResponseWriter, r *http.Request) {
	p, _, ok := decodeID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "plugin not found")
		return
	}
	c, err := s.pluginContainer(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	healthy, msg := pluginHealth(r.Context(), s, c)
	writeJSON(w, http.StatusOK, map[string]any{"healthy": healthy, "message": msg})
}

func pluginHealth(ctx context.Context, s *Server, c *reconcile.ActualContainer) (bool, string) {
	if c == nil {
		return false, "not installed"
	}
	info, err := s.opts.Engine.InspectContainer(ctx, c.ID)
	if err != nil {
		return false, err.Error()
	}
	if info.State == nil || !info.State.Running {
		return false, "stopped"
	}
	if info.State.Health != nil {
		status := info.State.Health.Status
		return status == "healthy", status
	}
	return true, "running"
}
