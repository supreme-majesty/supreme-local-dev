package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/pkg/assets"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon/metrics"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon/state"
	"github.com/supreme-majesty/supreme-local-dev/pkg/events"
	"github.com/supreme-majesty/supreme-local-dev/pkg/services"
)

type Server struct {
	Port int
}

func NewServer(port int) *Server {
	return &Server{Port: port}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/status", s.handleServices) // Alias for frontend
	mux.HandleFunc("/api/park", s.handlePark)
	mux.HandleFunc("/api/forget", s.handleForget)
	mux.HandleFunc("/api/link", s.handleLink)
	mux.HandleFunc("/api/unlink", s.handleUnlink)
	mux.HandleFunc("/api/php", s.handlePHP)
	mux.HandleFunc("/api/php/versions", s.handlePHPVersions)
	mux.HandleFunc("/api/secure", s.handleSecure)
	mux.HandleFunc("/api/restart", s.handleRestart)
	mux.HandleFunc("/api/sites", s.handleSites)
	mux.HandleFunc("/api/sites/update", s.handleSiteUpdate)
	mux.HandleFunc("/api/ignore", s.handleIgnore)
	mux.HandleFunc("/api/unignore", s.handleUnignore)
	mux.HandleFunc("/api/plugins", s.handlePlugins)
	mux.HandleFunc("/api/plugins/install", s.handlePluginInstall)
	mux.HandleFunc("/api/plugins/toggle", s.handlePluginToggle)
	mux.HandleFunc("/api/plugins/logs", s.handlePluginLogs)
	mux.HandleFunc("/api/plugins/health", s.handlePluginHealth)
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/share/start", s.handleShareStart)
	mux.HandleFunc("/api/share/stop", s.handleShareStop)
	mux.HandleFunc("/api/share/status", s.handleShareStatus)

	// Database Manager
	mux.HandleFunc("/api/db/status", s.handleDBStatus)
	mux.HandleFunc("/api/db/databases", s.handleDBDatabases)
	mux.HandleFunc("/api/db/create", s.handleDBCreate)
	mux.HandleFunc("/api/db/delete", s.handleDBDelete)
	mux.HandleFunc("/api/db/tables", s.handleDBTables)
	mux.HandleFunc("/api/db/table", s.handleDBTableData)
	mux.HandleFunc("/api/db/schema", s.handleDBSchema)
	mux.HandleFunc("/api/db/relationships", s.handleDBRelationships)
	mux.HandleFunc("/api/db/snapshots", s.handleDBSnapshots)
	mux.HandleFunc("/api/db/snapshots/download", s.handleDBDownload)
	mux.HandleFunc("/api/db/snapshots/restore", s.handleDBRestore)
	mux.HandleFunc("/api/db/import", s.handleDBImport)
	mux.HandleFunc("/api/db/query", s.handleDBQuery)
	mux.HandleFunc("/api/db/clone", s.handleDBClone)
	mux.HandleFunc("/api/db/rewind", s.handleDBRewind)
	mux.HandleFunc("/api/db/foreign-values", s.handleDBForeignValues)

	// Service Status & Health
	mux.HandleFunc("/api/services", s.handleServices)
	mux.HandleFunc("/api/services/control", s.handleServiceControl)
	mux.HandleFunc("/api/system/doctor", s.handleSystemDoctor)

	// Logging
	mux.HandleFunc("/api/logs/sources", s.handleLogSources)
	mux.HandleFunc("/api/logs/watch", s.handleLogWatch)
	mux.HandleFunc("/api/logs/unwatch", s.handleLogUnwatch)

	// Supreme Healer
	mux.HandleFunc("/api/healer/issues", s.handleHealerIssues)
	mux.HandleFunc("/api/healer/resolve", s.handleHealerResolve)

	// Projects & System
	mux.HandleFunc("/api/projects/create", s.handleProjectCreate)
	mux.HandleFunc("/api/projects/ghost", s.handleProjectGhost)
	mux.HandleFunc("/api/projects/ghost/discard", s.handleProjectGhostDiscard)
	mux.HandleFunc("/api/projects/templates", s.handleGetTemplates) // New route
	mux.HandleFunc("/api/system/editors", s.handleSystemEditors)
	mux.HandleFunc("/api/system/open-editor", s.handleSystemOpenEditor)
	mux.HandleFunc("/api/system/directories", s.handleSystemDirectories)

	// Env Manager
	mux.HandleFunc("/api/env/files", s.handleEnvFiles)
	mux.HandleFunc("/api/env/read", s.handleEnvRead)
	mux.HandleFunc("/api/env/write", s.handleEnvWrite)
	mux.HandleFunc("/api/env/backups", s.handleEnvBackups)
	mux.HandleFunc("/api/env/restore", s.handleEnvRestore)

	// Artisan Runner
	mux.HandleFunc("/api/artisan/run", s.handleArtisanRun)
	mux.HandleFunc("/api/artisan/commands", s.handleArtisanCommands)

	// Initialize WebSocket Hub
	hub := NewHub()
	go hub.Run()
	SetupEventBridge(hub)
	mux.HandleFunc("/api/ws", s.handleWebSocket(hub))

	// Serve GUI static files
	guiFS, _ := assets.GetGuiFS()
	fileServer := http.FileServer(guiFS)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Skip API requests (addressed by specific handlers, but safe to check)
		if strings.HasPrefix(path, "/api") {
			http.NotFound(w, r)
			return
		}

		f, err := guiFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			defer f.Close()
			stat, _ := f.Stat()
			if !stat.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	fmt.Printf("SLD Daemon listening on port %d...\n", s.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", s.Port), s.corsMiddleware(mux))
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Responses
type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func jsonResponse(w http.ResponseWriter, data interface{}, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // For dev
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	jsonResponse(w, d.State.Data, 200)
}

func (s *Server) handlePark(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.Park(req.Path); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleForget(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.Forget(req.Path); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	// Use absolute path if provided path is relative?
	// The CLI resolves it, but the GUI might send raw strings.
	// Best to assume GUI sends valid paths, but we can verify.
	path, _ := filepath.Abs(req.Path)

	if err := d.Link(req.Name, path); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleUnlink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.Unlink(req.Name); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handlePHP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.SwitchPHP(req.Version); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handlePHPVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}
	d, _ := daemon.GetClient()
	versions, err := d.Adapter.ListPHPVersions()
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, versions, 200)
}

func (s *Server) handleSecure(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	d, _ := daemon.GetClient()
	if err := d.Secure(); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	d, _ := daemon.GetClient()
	if err := d.Restart(); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	sites, err := d.GetSites()
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, sites, 200)
}

func (s *Server) handleSiteUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" && r.Method != "PUT" {
		return
	}

	var req struct {
		Domain   string   `json:"domain"`
		Tags     []string `json:"tags"`
		Category string   `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	conf, ok := d.State.Data.SiteConfigs[req.Domain]
	if !ok {
		conf = state.SiteConfig{}
	}

	conf.Tags = req.Tags
	conf.Category = req.Category
	d.State.SetSiteConfig(req.Domain, conf)

	d.Events.Publish(events.Event{Type: events.SitesUpdated})
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleIgnore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}
	d, _ := daemon.GetClient()
	if err := d.Ignore(req.Path); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleUnignore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}
	d, _ := daemon.GetClient()
	if err := d.Unignore(req.Path); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

// Projects & System

func (s *Server) handleGetTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}
	d, _ := daemon.GetClient()
	jsonResponse(w, d.ProjectManager.GetTemplates(), 200)
}

func (s *Server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Type       string `json:"type"`
		Name       string `json:"name"`
		Directory  string `json:"directory"`
		Repository string `json:"repository"` // New field
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	opts := services.ProjectOptions{
		Type:       req.Type,
		Name:       req.Name,
		Directory:  req.Directory,
		Repository: req.Repository,
	}

	// Run project creation asynchronously to avoid gateway timeout
	// Creating a Laravel project can take 1-2+ minutes
	go func() {
		if err := d.ProjectManager.CreateProject(opts); err != nil {
			fmt.Printf("[ERROR] Project creation failed for %s: %v\n", req.Name, err)
			return
		}

		// Determine project path
		base := d.ProjectManager.BaseDir
		if opts.Directory != "" {
			base = opts.Directory
		}
		projectPath := filepath.Join(base, req.Name)

		// Check if the project is in a parked directory (avoid duplicate listing)
		isInParkedPath := false
		for _, parkedPath := range d.State.Data.Paths {
			if strings.HasPrefix(projectPath, parkedPath) {
				isInParkedPath = true
				break
			}
		}

		// Ensure project is not ignored (e.g. if user previously removed it)
		d.Unignore(projectPath)

		if isInParkedPath {
			// Project is in a parked path, just regenerate certs if secure mode is on
			if d.State.Data.Secure {
				if err := d.Refresh(); err != nil {
					fmt.Printf("[ERROR] Failed to refresh after project creation: %v\n", err)
					return
				}
			}
			fmt.Printf("[INFO] Project %s created in parked path %s\n", req.Name, projectPath)
		} else {
			// Project is NOT in a parked path, link it explicitly
			if err := d.Link(req.Name, projectPath); err != nil {
				fmt.Printf("[ERROR] Failed to link project %s: %v\n", req.Name, err)
				return
			}
			fmt.Printf("[INFO] Project %s created and linked at %s\n", req.Name, projectPath)
		}

		// Emit event to update UI
		d.Events.Publish(events.Event{Type: events.SitesUpdated})
	}()

	jsonResponse(w, SuccessResponse{Success: true, Message: "Project creation started in background"}, 202)
}

// handleProjectGhost creates a "Ghost" clone of a project for experimentation
func (s *Server) handleProjectGhost(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		SourcePath string `json:"source_path"`
		TargetName string `json:"target_name"`
		CloneDB    bool   `json:"clone_db"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.SourcePath == "" {
		jsonResponse(w, ErrorResponse{Error: "source_path required"}, 400)
		return
	}

	d, _ := daemon.GetClient()

	// Run in background since it can take time
	go func() {
		targetPath, err := d.ProjectManager.CloneProject(req.SourcePath, req.TargetName, req.CloneDB, d.DatabaseService)
		if err != nil {
			fmt.Printf("[GHOST MODE] Error: %v\n", err)
			return
		}

		// Link the new ghost project
		ghostName := filepath.Base(targetPath)
		if err := d.Link(ghostName, targetPath); err != nil {
			fmt.Printf("[GHOST MODE] Failed to link %s: %v\n", ghostName, err)
			return
		}

		d.Events.Publish(events.Event{Type: events.SitesUpdated})
		fmt.Printf("[GHOST MODE] Successfully created ghost: %s\n", ghostName)
	}()

	jsonResponse(w, SuccessResponse{Success: true, Message: "Ghost clone started in background"}, 202)
}

func (s *Server) handleProjectGhostDiscard(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Path   string `json:"path"`
		DBName string `json:"db_name"` // Optional, if empty will try convention
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Path == "" {
		jsonResponse(w, ErrorResponse{Error: "path required"}, 400)
		return
	}

	d, _ := daemon.GetClient()

	// 1. Unlink the site first
	name := filepath.Base(req.Path)
	d.Unlink(name)

	// 2. Perform deletion
	if err := d.ProjectManager.DiscardGhost(req.Path, req.DBName, d.DatabaseService); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	d.Events.Publish(events.Event{Type: events.SitesUpdated})
	jsonResponse(w, SuccessResponse{Success: true, Message: "Ghost project discarded"}, 200)
}

func (s *Server) handleSystemEditors(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	editors := d.ProjectManager.DetectEditors()
	jsonResponse(w, editors, 200)
}

func (s *Server) handleSystemOpenEditor(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Path   string `json:"path"`
		Editor string `json:"editor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.ProjectManager.OpenInEditor(req.Path, req.Editor); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleSystemDirectories(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	d, _ := daemon.GetClient()
	dirs, err := d.ProjectManager.ListDirectories(path)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, dirs, 200)
}

// Plugins

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()

	// Convert map to slice for simpler JSON
	plugins := d.PluginManager.GetAll()

	// Create a response struct that maps Plugin interface to JSON fields
	type PluginResponse struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     string `json:"version"`
		Status      string `json:"status"`
		Installed   bool   `json:"installed"`
	}

	var response []PluginResponse
	for _, p := range plugins {
		response = append(response, PluginResponse{
			ID:          p.ID(),
			Name:        p.Name(),
			Description: p.Description(),
			Version:     p.Version(),
			Status:      string(p.Status()),
			Installed:   p.IsInstalled(),
		})
	}

	jsonResponse(w, response, 200)
}

func (s *Server) handlePluginInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	p, ok := d.PluginManager.Get(req.ID)
	if !ok {
		jsonResponse(w, ErrorResponse{Error: "Plugin not found"}, 404)
		return
	}

	if err := p.Install(); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handlePluginToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()

	// Use SetEnabled which handles start/stop and persistence
	if err := d.PluginManager.SetEnabled(req.ID, req.Enabled); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handlePluginLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResponse(w, ErrorResponse{Error: "id parameter required"}, 400)
		return
	}

	linesStr := r.URL.Query().Get("lines")
	lines := 100
	if linesStr != "" {
		if n, err := strconv.Atoi(linesStr); err == nil && n > 0 {
			lines = n
		}
	}

	d, _ := daemon.GetClient()
	p, ok := d.PluginManager.Get(id)
	if !ok {
		jsonResponse(w, ErrorResponse{Error: "Plugin not found"}, 404)
		return
	}

	// Check if plugin implements LogProvider
	if lp, ok := p.(interface{ Logs(int) ([]string, error) }); ok {
		logs, err := lp.Logs(lines)
		if err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}
		jsonResponse(w, map[string]interface{}{"logs": logs}, 200)
		return
	}

	jsonResponse(w, map[string]interface{}{"logs": []string{"Log viewing not supported for this plugin"}}, 200)
}

func (s *Server) handlePluginHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResponse(w, ErrorResponse{Error: "id parameter required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	p, ok := d.PluginManager.Get(id)
	if !ok {
		jsonResponse(w, ErrorResponse{Error: "Plugin not found"}, 404)
		return
	}

	// Check if plugin implements HealthChecker
	if hc, ok := p.(interface{ Health() (bool, string) }); ok {
		ok, msg := hc.Health()
		jsonResponse(w, map[string]interface{}{"healthy": ok, "message": msg}, 200)
		return
	}

	// Default: use Status() as health indicator
	isRunning := p.Status() == "running"
	jsonResponse(w, map[string]interface{}{"healthy": isRunning, "message": string(p.Status())}, 200)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	stats, err := metrics.Collect(d)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, stats, 200)
}

// Tunnel / Share Handlers

func (s *Server) handleShareStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Site string `json:"site"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()

	// Determine target based on Secure mode
	target := "http://localhost:80" // Default
	if d.State.Data.Port != "" {
		target = fmt.Sprintf("http://localhost:%s", d.State.Data.Port)
	}

	if d.State.Data.Secure {
		target = "https://localhost:443"
	}

	url, err := d.TunnelManager.StartTunnel(req.Site, target)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: url}, 200)
}

func (s *Server) handleShareStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Site string `json:"site"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.TunnelManager.StopTunnel(req.Site); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleShareStatus(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	tunnels := d.TunnelManager.GetTunnels()
	jsonResponse(w, tunnels, 200)
}

// Database Manager Handlers

func (s *Server) handleDBStatus(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	// Check connection status
	err := d.DatabaseService.Connect()
	status := map[string]interface{}{
		"connected": err == nil,
		"host":      "localhost",
		"port":      "3306",
		"user":      "root",
	}
	if err != nil {
		status["error"] = err.Error()
	}
	jsonResponse(w, status, 200)
}

func (s *Server) handleDBDatabases(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	databases, err := d.DatabaseService.ListDatabases()
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	// Convert []string to object array for consistency with frontend
	type DBInfo struct {
		Name   string `json:"name"`
		Tables int    `json:"tables"`
	}
	response := make([]DBInfo, 0)
	for _, name := range databases {
		response = append(response, DBInfo{Name: name, Tables: 0})
	}

	jsonResponse(w, response, 200)
}

func (s *Server) handleDBTables(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	if db == "" {
		jsonResponse(w, ErrorResponse{Error: "db parameter required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	tables, err := d.DatabaseService.ListTables(db)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, tables, 200)
}

func (s *Server) handleDBRelationships(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	if db == "" {
		jsonResponse(w, ErrorResponse{Error: "db parameter required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	rels, err := d.DatabaseService.GetTableRelationships(db)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, rels, 200)
}

func (s *Server) handleDBCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, ErrorResponse{Error: "POST method required"}, 405)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Name == "" {
		jsonResponse(w, ErrorResponse{Error: "database name is required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.DatabaseService.CreateDatabase(req.Name); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]string{"message": "Database created successfully"}, 200)
}

func (s *Server) handleDBDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" && r.Method != "POST" {
		jsonResponse(w, ErrorResponse{Error: "DELETE or POST method required"}, 405)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Name == "" {
		jsonResponse(w, ErrorResponse{Error: "database name is required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.DatabaseService.DeleteDatabase(req.Name); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]string{"message": "Database deleted successfully"}, 200)
}

func (s *Server) handleDBTableData(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	table := r.URL.Query().Get("table")
	if db == "" || table == "" {
		jsonResponse(w, ErrorResponse{Error: "db and table parameters required"}, 400)
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		offset, _ = strconv.Atoi(o)
	}

	// New parameters: sort, order, profile
	sortCol := r.URL.Query().Get("sort")
	sortOrder := r.URL.Query().Get("order") // ASC or DESC
	profile := r.URL.Query().Get("profile") == "true"

	// Convert offset/limit to page/perPage
	perPage := limit
	page := (offset / limit) + 1

	d, _ := daemon.GetClient()
	data, err := d.DatabaseService.GetTableDataEx(db, table, page, perPage, sortCol, sortOrder, profile)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	// Return rows as maps to match frontend expectation
	var colNames []string
	for _, col := range data.Columns {
		colNames = append(colNames, col.Name)
	}

	resp := map[string]interface{}{
		"columns":     data.Columns,
		"rows":        data.Rows,
		"total":       data.Total,
		"limit":       data.PerPage,
		"offset":      (data.Page - 1) * data.PerPage,
		"total_pages": data.TotalPages,
	}

	// Include query time if profiling was enabled
	if profile && data.QueryTime > 0 {
		resp["query_time"] = data.QueryTime
	}

	jsonResponse(w, resp, 200)
}

func (s *Server) handleDBSchema(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	table := r.URL.Query().Get("table")
	if db == "" || table == "" {
		jsonResponse(w, ErrorResponse{Error: "db and table parameters required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	schema, err := d.DatabaseService.GetTableColumns(db, table)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, schema, 200)
}

func (s *Server) handleDBSnapshots(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()

	switch r.Method {
	case "GET":
		db := r.URL.Query().Get("db")
		snapshots, err := d.DatabaseService.ListSnapshots()
		if err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}

		// Filter by DB if requested and map to frontend format
		var response []map[string]interface{}
		for _, s := range snapshots {
			if db != "" && s.Database != db {
				continue
			}
			response = append(response, map[string]interface{}{
				"id":         s.Filename, // use filename as ID
				"filename":   s.Filename,
				"database":   s.Database,
				"table":      s.Table,
				"size":       s.Size,
				"created_at": s.CreatedAt,
			})
		}
		jsonResponse(w, response, 200)

	case "POST":
		var req struct {
			Database string `json:"database"`
			Table    string `json:"table"`
			Name     string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
			return
		}

		snapshot, err := d.DatabaseService.CreateSnapshot(req.Database, req.Table)
		if err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}

		resp := map[string]interface{}{
			"id":         snapshot.Filename,
			"filename":   snapshot.Filename,
			"database":   snapshot.Database,
			"table":      snapshot.Table,
			"size":       snapshot.Size,
			"created_at": snapshot.CreatedAt,
		}
		jsonResponse(w, resp, 200)

	case "DELETE":
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
			return
		}

		// ID is filename
		if err := d.DatabaseService.DeleteSnapshot(req.ID); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}
		jsonResponse(w, SuccessResponse{Success: true}, 200)
	}
}

func (s *Server) handleDBRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Database string `json:"database"`
		Path     string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	// RestoreSnapshot takes filename (Path field from frontend)
	if err := d.DatabaseService.RestoreSnapshot(req.Path); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true, Message: "Database restored successfully"}, 200)
}

func (s *Server) handleDBQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Database string `json:"database"`
		Query    string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	result, err := d.DatabaseService.ExecuteQuery(req.Database, req.Query)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, result, 200)
}

func (s *Server) handleDBClone(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Source == "" || req.Target == "" {
		jsonResponse(w, ErrorResponse{Error: "source and target database names required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.DatabaseService.CloneDatabase(req.Source, req.Target); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: fmt.Sprintf("Database '%s' cloned to '%s'", req.Source, req.Target)}, 200)
}

// handleDBRewind performs a Time-Travel restore with auto-backup
func (s *Server) handleDBRewind(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Filename == "" {
		jsonResponse(w, ErrorResponse{Error: "filename required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	backup, err := d.DatabaseService.RewindDatabase(req.Filename)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Rewound to %s. Safety backup: %s", req.Filename, backup.Filename),
		"backup":  backup,
	}, 200)
}

func (s *Server) handleDBDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id parameter required", 400)
		return
	}

	d, _ := daemon.GetClient()
	// Security: Sanitize ID to prevent directory traversal
	// In ListSnapshots we trust filenames in the dir, but here we take user input.
	// Simple check: must not contain slashes
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		http.Error(w, "invalid filename", 400)
		return
	}

	path := filepath.Join(d.DatabaseService.SnapDir, id)

	// Parse filename to extract db/table name for a clean download name
	// Formats: db_timestamp.sql or db__table_timestamp.sql
	cleanName := id
	baseName := strings.TrimSuffix(id, ".sql")
	if strings.Contains(baseName, "__") {
		// Table export: db__table_timestamp.sql -> table.sql
		parts := strings.Split(baseName, "__")
		if len(parts) >= 2 {
			remaining := parts[1]
			remainingParts := strings.Split(remaining, "_")
			if len(remainingParts) >= 2 {
				tableName := strings.Join(remainingParts[:len(remainingParts)-2], "_")
				cleanName = tableName + ".sql"
			}
		}
	} else {
		// Database export: db_timestamp.sql -> db.sql
		parts := strings.Split(baseName, "_")
		if len(parts) >= 2 {
			dbName := strings.Join(parts[:len(parts)-2], "_")
			cleanName = dbName + ".sql"
		}
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, cleanName))
	http.ServeFile(w, r, path)
}

func (s *Server) handleDBImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	// 100MB limit with proper 413 response when exceeded
	const maxUploadSize = 100 << 20 // 100MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		if err.Error() == "http: request body too large" {
			jsonResponse(w, ErrorResponse{Error: "File too large. Maximum size is 100MB"}, 413)
			return
		}
		jsonResponse(w, ErrorResponse{Error: "Error parsing form: " + err.Error()}, 400)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: "Error retrieving file"}, 400)
		return
	}
	defer file.Close()

	d, _ := daemon.GetClient()

	// Create snapshots dir if not exists
	os.MkdirAll(d.DatabaseService.SnapDir, 0755)

	// Save file
	// We preserve the name but might prefix timestamp if collision?
	// For now just overwrite or simple save.
	filename := handler.Filename
	// Sanitize
	filename = filepath.Base(filename)

	destPath := filepath.Join(d.DatabaseService.SnapDir, filename)

	// Write
	dst, err := os.Create(destPath)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	// Check if we should restore
	if r.URL.Query().Get("restore") == "true" {
		// Get target database from form field or query param
		dbName := r.FormValue("database")
		if dbName == "" {
			dbName = r.URL.Query().Get("database")
		}
		if dbName == "" {
			jsonResponse(w, ErrorResponse{Error: "database parameter required for restore"}, 400)
			return
		}

		// Run mysql import directly
		if err := d.DatabaseService.ImportSQL(dbName, destPath); err != nil {
			jsonResponse(w, ErrorResponse{Error: "Upload successful but restore failed: " + err.Error()}, 500)
			return
		}
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: "File uploaded successfully"}, 200)
}

func (s *Server) handleDBForeignValues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dbName := r.URL.Query().Get("database")
	table := r.URL.Query().Get("table")
	column := r.URL.Query().Get("column")

	if dbName == "" || table == "" || column == "" {
		http.Error(w, "Missing database, table, or column parameter", http.StatusBadRequest)
		return
	}

	d, _ := daemon.GetClient()
	values, err := d.DatabaseService.GetForeignValues(dbName, table, column)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(values); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Log Management Handlers

func (s *Server) handleLogSources(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	sources := d.LogWatcher.GetAvailableSources()

	// Convert map to frontend friendly array
	type LogSourceInfo struct {
		ID    string `json:"id"`
		Path  string `json:"path"`
		Label string `json:"label"`
	}

	response := make([]LogSourceInfo, 0)
	for id, path := range sources {
		label := string(id)
		switch id {
		case services.LogSourceNginxError:
			label = "Nginx Error"
		case services.LogSourceNginxAccess:
			label = "Nginx Access"
		case services.LogSourcePHPFPM:
			label = "PHP-FPM"
		}

		response = append(response, LogSourceInfo{
			ID:    string(id),
			Path:  path,
			Label: label,
		})
	}

	jsonResponse(w, response, 200)
}

func (s *Server) handleLogWatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Source      string `json:"source"`
		ProjectPath string `json:"project_path"` // Optional, for Laravel logs
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()

	var err error
	if strings.HasPrefix(req.Source, "laravel") && req.ProjectPath != "" {
		err = d.LogWatcher.WatchLaravelLog(req.ProjectPath)
	} else {
		err = d.LogWatcher.StartWatching(services.LogSource(req.Source))
	}

	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleLogUnwatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	d.LogWatcher.StopWatching(services.LogSource(req.Source))

	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

// Env Manager Handlers

func (s *Server) handleEnvFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		jsonResponse(w, ErrorResponse{Error: "project parameter required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	files, err := d.EnvManager.ListEnvFiles(projectPath)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, files, 200)
}

func (s *Server) handleEnvRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		jsonResponse(w, ErrorResponse{Error: "path parameter required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	envFile, err := d.EnvManager.ReadEnvFile(path)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, envFile, 200)
}

func (s *Server) handleEnvWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" && r.Method != "POST" {
		return
	}

	var req struct {
		Path      string            `json:"path"`
		Variables map[string]string `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.EnvManager.WriteEnvFile(req.Path, req.Variables); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: "Env file saved with backup"}, 200)
}

func (s *Server) handleEnvBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		jsonResponse(w, ErrorResponse{Error: "path parameter required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	backups, err := d.EnvManager.ListBackups(path)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, backups, 200)
}

func (s *Server) handleEnvRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		BackupPath string `json:"backup_path"`
		TargetPath string `json:"target_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.EnvManager.RestoreBackup(req.BackupPath, req.TargetPath); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: "Backup restored"}, 200)
}

// Artisan Runner Handlers

func (s *Server) handleArtisanRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		ProjectPath string `json:"project_path"`
		Command     string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.ProjectPath == "" || req.Command == "" {
		jsonResponse(w, ErrorResponse{Error: "project_path and command required"}, 400)
		return
	}

	d, _ := daemon.GetClient()

	// Run async - output will stream via WebSocket
	go func() {
		if err := d.ArtisanService.RunCommand(req.ProjectPath, req.Command); err != nil {
			fmt.Printf("[ERROR] Artisan command failed: %v\n", err)
		}
	}()

	jsonResponse(w, SuccessResponse{Success: true, Message: "Command started"}, 202)
}

func (s *Server) handleArtisanCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	d, _ := daemon.GetClient()
	commands := d.ArtisanService.GetCommonCommands()
	jsonResponse(w, commands, 200)
}

// Service & Doctor Handlers

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	services, err := d.Adapter.GetServices()
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, services, 200)
}

func (s *Server) handleSystemDoctor(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	checks, err := d.Adapter.GetSystemHealth()
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, checks, 200)
}

func (s *Server) handleServiceControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Service string `json:"service"`
		Action  string `json:"action"` // start, stop, restart
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	var err error

	switch req.Action {
	case "start":
		err = d.Adapter.StartService(req.Service)
	case "stop":
		err = d.Adapter.StopService(req.Service)
	case "restart":
		err = d.Adapter.RestartService(req.Service)
	default:
		jsonResponse(w, ErrorResponse{Error: "Invalid action"}, 400)
		return
	}

	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: fmt.Sprintf("Service %s action %s completed", req.Service, req.Action)}, 200)
}
