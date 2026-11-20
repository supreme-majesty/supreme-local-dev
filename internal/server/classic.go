package server

// The endpoints behind the original dashboard pages (overview, sites,
// services, doctor, settings, metrics). V1 answered them from the host
// daemon (nginx, php-fpm, dnsmasq, state.json); here they are answered from
// Docker and the workspace scan, so the pages keep working in the SLD image.

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

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/detect"
	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/internal/tls"
	v2 "github.com/supreme-majesty/supreme-local-dev/internal/v2"
)

const composeProjectLabel = "com.docker.compose.project"
const composeServiceLabel = "com.docker.compose.service"

func (s *Server) registerClassic(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/sites", s.handleSites)
	mux.HandleFunc("POST /api/sites/update", s.handleSiteUpdate)
	mux.HandleFunc("POST /api/refresh", s.handleRefresh)

	// "Remove from SLD" = put a .sldignore in the folder; adding back removes it.
	mux.HandleFunc("POST /api/ignore", s.handleIgnore(true))
	mux.HandleFunc("POST /api/forget", s.handleIgnore(true))
	mux.HandleFunc("POST /api/unignore", s.handleIgnore(false))
	mux.HandleFunc("POST /api/unlink", s.handleUnlink)
	mux.HandleFunc("POST /api/park", s.handlePark)
	mux.HandleFunc("POST /api/link", s.handlePark)

	mux.HandleFunc("POST /api/secure", s.handleSecure)
	mux.HandleFunc("POST /api/unsecure", s.handleSecure)
	mux.HandleFunc("POST /api/restart", s.handleRestartRouter)
	mux.HandleFunc("POST /api/php", s.handlePHPSwitch)
	mux.HandleFunc("GET /api/php/versions", s.handlePHPVersions)

	mux.HandleFunc("GET /api/services", s.handleServices)
	mux.HandleFunc("GET /api/status", s.handleServices)
	mux.HandleFunc("POST /api/services/control", s.handleServiceControl)
	mux.HandleFunc("GET /api/system/doctor", s.handleDoctor)
	mux.HandleFunc("GET /api/metrics", s.handleMetrics)
}

// --- state & sites ---

// classicSite is a project in the shape of the original Sites page.
type classicSite struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Domain     string   `json:"domain"`
	PHPVersion string   `json:"phpVersion,omitempty"`
	Secure     bool     `json:"secure"`
	Type       string   `json:"type"` // "parked" (found in a workspace) or "docker" (compose stack)
	Tags       []string `json:"tags,omitempty"`
	Category   string   `json:"category,omitempty"`
	URL        string   `json:"url,omitempty"`
	ID         string   `json:"id,omitempty"`
	Status     string   `json:"status,omitempty"`
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	results, _ := s.scan(false)
	secure := false
	for _, res := range results {
		secure = secure || res.TLS
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tld":          config.DefaultTLD(),
		"paths":        s.opts.Workspaces,
		"links":        map[string]string{},
		"services":     map[string]string{},
		"certificates": []string{},
		"php_version":  config.SupportedPHPVersions[len(config.SupportedPHPVersions)-1],
		"secure":       secure,
		"port":         fmt.Sprint(s.opts.Env.HTTPPort()),
	})
}

func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	results, err := s.scan(r.URL.Query().Get("refresh") == "1")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	meta := s.settings.Get().SiteMeta
	sites := []classicSite{}
	for _, res := range results {
		if res.Status == "undetected" || len(res.AppList) == 0 {
			continue
		}
		view := s.view(r.Context(), res)
		app := res.AppList[0]
		site := classicSite{
			Name:   res.Name,
			Path:   res.Root,
			Domain: app.Domain,
			Secure: res.TLS,
			Type:   "parked",
			URL:    s.opts.Env.SiteURL(app.Domain, res.TLS),
			ID:     view.ID,
			Status: view.Status,
		}
		for _, a := range res.AppList {
			if a.Runtime == "php" && site.PHPVersion == "" {
				site.PHPVersion = a.Version
			}
			if a.Runtime == "compose" {
				site.Type = "docker"
			}
		}
		if m, ok := meta[site.Domain]; ok {
			site.Tags, site.Category = m.Tags, m.Category
		}
		sites = append(sites, site)
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].Name < sites[j].Name })
	writeJSON(w, http.StatusOK, sites)
}

func (s *Server) handleSiteUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain   string   `json:"domain"`
		Tags     []string `json:"tags"`
		Category string   `json:"category"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil || req.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}
	_, err := s.settings.Update(func(st *Settings) {
		if st.SiteMeta == nil {
			st.SiteMeta = map[string]SiteMeta{}
		}
		st.SiteMeta[req.Domain] = SiteMeta{Tags: req.Tags, Category: req.Category}
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if _, err := s.scan(true); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// --- adding / removing projects ---

// inWorkspace resolves path and checks it lies inside a workspace folder.
func (s *Server) inWorkspace(path string) (string, error) {
	if path == "" {
		return "", errors.New("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for _, ws := range s.opts.Workspaces {
		if rel, err := filepath.Rel(ws, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("%s is outside the workspace (%s). SLD finds projects in the workspace folders mounted into it",
		path, strings.Join(s.opts.Workspaces, ", "))
}

func (s *Server) handleIgnore(ignore bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req)
		path, err := s.inWorkspace(req.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := setIgnored(path, ignore); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.invalidateScan()
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	}
}

// setIgnored adds or removes the .sldignore marker of a folder.
func setIgnored(dir string, ignore bool) error {
	marker := filepath.Join(dir, detect.IgnoreFile)
	if ignore {
		return os.WriteFile(marker, []byte("# Hidden from SLD. Delete this file to show the project again.\n"), 0o644)
	}
	if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Server) handleUnlink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req)
	results, err := s.scan(false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, res := range results {
		if res.Name == req.Name {
			if err := setIgnored(res.Root, true); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			s.invalidateScan()
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
			return
		}
	}
	writeError(w, http.StatusNotFound, "no project named "+req.Name)
}

// handlePark covers V1's park and link: every folder in a workspace is
// already a project, so a path inside it only needs a rescan (and is shown
// again if it was hidden).
func (s *Server) handlePark(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req)
	path, err := s.inWorkspace(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := setIgnored(path, false); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.scan(true); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Projects in this folder are listed on the Projects page"})
}

func (s *Server) handleSecure(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusConflict,
		"HTTPS is set per project: open Projects, then the project's settings (domain.tls in .sld/config.yaml)")
}

func (s *Server) handlePHPSwitch(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusConflict,
		"Each project has its own PHP version (apps.<name>.version in .sld/config.yaml); there is no global PHP to switch")
}

func (s *Server) handlePHPVersions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, config.SupportedPHPVersions)
}

// --- services ---

// infraService is a container SLD itself runs: the router, the bundled
// stack's services (databases, mail ...) and plugin containers.
type infraService struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Version string `json:"version,omitempty"`
	State   string `json:"state,omitempty"`
	Kind    string `json:"kind,omitempty"` // router, stack, plugin
	id      string
}

// selfComposeProject is the compose project SLD was started from ("" when
// not started by compose), whose other services SLD shows as its own.
func (s *Server) selfComposeProject(ctx context.Context) string {
	if !s.opts.Env.InContainer() || s.opts.Env.ContainerID() == "" {
		return ""
	}
	info, err := s.opts.Engine.InspectContainer(ctx, s.opts.Env.ContainerID())
	if err != nil || info.Config == nil {
		return ""
	}
	return info.Config.Labels[composeProjectLabel]
}

func (s *Server) infraServices(ctx context.Context) ([]infraService, error) {
	containers, err := s.opts.Engine.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}
	stack := s.selfComposeProject(ctx)
	selfID := s.opts.Env.ContainerID()
	var out []infraService
	for _, c := range containers {
		if selfID != "" && strings.HasPrefix(c.ID, selfID) {
			continue
		}
		svc := infraService{Running: c.State == "running", State: c.State, Version: imageTag(c.Image), id: c.ID}
		switch {
		case c.Name == orchestrate.RouterContainer:
			svc.Name, svc.Kind = "router", "router"
		case c.Labels[pluginLabel] != "":
			svc.Name, svc.Kind = c.Labels[pluginLabel], "plugin"
		case stack != "" && c.Labels[composeProjectLabel] == stack:
			svc.Name, svc.Kind = c.Labels[composeServiceLabel], "stack"
		default:
			continue
		}
		out = append(out, svc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind > out[j].Kind // router, stack, plugin
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func imageTag(image string) string {
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		return image[i+1:]
	}
	return "latest"
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	services, err := s.infraServices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if services == nil {
		services = []infraService{}
	}
	writeJSON(w, http.StatusOK, services)
}

func (s *Server) handleServiceControl(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Service string `json:"service"`
		Action  string `json:"action"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
	services, err := s.infraServices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, svc := range services {
		if !strings.EqualFold(svc.Name, req.Service) {
			continue
		}
		if err := s.controlContainer(r.Context(), svc.id, req.Action); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}
	writeError(w, http.StatusNotFound, "unknown service "+req.Service)
}

func (s *Server) controlContainer(ctx context.Context, id, action string) error {
	switch action {
	case "start":
		return s.opts.Engine.StartContainer(ctx, id)
	case "stop":
		return s.opts.Engine.StopContainer(ctx, id, 10)
	case "restart":
		if err := s.opts.Engine.StopContainer(ctx, id, 10); err != nil {
			return err
		}
		return s.opts.Engine.StartContainer(ctx, id)
	}
	return fmt.Errorf("unknown action %q (start, stop or restart)", action)
}

// handleRestartRouter is the dashboard's "restart" button: it restarts the
// router, which reloads every route.
func (s *Server) handleRestartRouter(w http.ResponseWriter, r *http.Request) {
	services, err := s.infraServices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, svc := range services {
		if svc.Kind == "router" {
			if err := s.controlContainer(r.Context(), svc.id, "restart"); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Router restarted"})
			return
		}
	}
	writeError(w, http.StatusConflict, "the router is not set up yet: run the setup first")
}

// --- doctor ---

type healthCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // pass, warn, fail
	Message string `json:"message"`
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.doctor(r.Context()))
}

func (s *Server) doctor(ctx context.Context) []healthCheck {
	env := s.opts.Env
	var checks []healthCheck
	add := func(name, status, msg string) { checks = append(checks, healthCheck{name, status, msg}) }

	dockerOK := s.opts.Engine.Ping(ctx) == nil
	if dockerOK {
		add("Docker", "pass", "Docker Engine is reachable")
	} else {
		add("Docker", "fail", "Docker Engine is not reachable: is the Docker socket mounted into SLD?")
	}

	if dockerOK {
		router := "missing"
		if containers, err := s.opts.Engine.ListAllContainers(ctx); err == nil {
			for _, c := range containers {
				if c.Name == orchestrate.RouterContainer {
					router = c.State
				}
			}
		}
		switch router {
		case "running":
			add("Router", "pass", fmt.Sprintf("%s is running (HTTP %d, HTTPS %d)", orchestrate.RouterContainer, env.HTTPPort(), env.HTTPSPort()))
		case "missing":
			add("Router", "fail", "The router is not set up: open Setup and run it")
		default:
			add("Router", "fail", fmt.Sprintf("%s is %s: start it from Services", orchestrate.RouterContainer, router))
		}
	}

	for _, ws := range s.opts.Workspaces {
		results, err := v2.Scan(ws, false)
		switch {
		case err != nil:
			add("Workspace "+ws, "fail", err.Error())
		case len(results) == 0:
			add("Workspace "+ws, "warn", "No projects found: mount the folder that holds your projects")
		default:
			add("Workspace "+ws, "pass", fmt.Sprintf("%d project(s) found", len(results)))
		}
	}
	if len(s.opts.Workspaces) == 0 {
		add("Workspace", "fail", "No workspace folder: mount your projects folder (-v <projects>:/workspace)")
	}

	if certMgr, err := tls.NewManager(env.DataDir()); err == nil && certMgr.HasCA() {
		add("HTTPS", "pass", "SLD's certificate authority exists (trust it once to use https:// without warnings)")
	} else {
		add("HTTPS", "warn", "No certificate authority yet: run Setup to create one (HTTP works without it)")
	}

	tld := config.DefaultTLD()
	if config.IsAutoResolvingDomain("x." + tld) {
		add("Domains", "pass", fmt.Sprintf("*.%s resolves to this computer without any DNS setup", tld))
	} else {
		add("Domains", "warn", fmt.Sprintf("*.%s needs a DNS entry: add your domains to the hosts file, or use .localhost", tld))
	}
	return checks
}

// --- metrics ---

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	stats := map[string]any{"active_connections": 0}
	if v, err := mem.VirtualMemory(); err == nil {
		stats["ram_percent"] = v.UsedPercent
		stats["ram_usage"] = fmt.Sprintf("%.1f GB", float64(v.Used)/(1<<30))
		stats["ram_total"] = fmt.Sprintf("%.1f GB", float64(v.Total)/(1<<30))
	}
	if c, err := cpu.Percent(0, false); err == nil && len(c) > 0 {
		stats["cpu_percent"] = c[0]
	}

	results, _ := s.scan(false)
	running := 0
	for _, res := range results {
		if v := s.view(r.Context(), res); v.Status == "running" || v.Status == "partial" {
			running++
		}
	}
	stats["sites_parked"] = len(results) // projects found
	stats["sites_linked"] = running      // projects running

	services, _ := s.infraServices(r.Context())
	up := 0
	for _, svc := range services {
		if svc.Running {
			up++
		}
	}
	stats["services_running"] = up
	stats["services_total"] = len(services)
	writeJSON(w, http.StatusOK, stats)
}
