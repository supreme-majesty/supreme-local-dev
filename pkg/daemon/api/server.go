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
	http.HandleFunc("/api/state", s.handleState)
	http.HandleFunc("/api/park", s.handlePark)
	http.HandleFunc("/api/forget", s.handleForget)
	http.HandleFunc("/api/link", s.handleLink)
	http.HandleFunc("/api/unlink", s.handleUnlink)
	http.HandleFunc("/api/php", s.handlePHP)
	http.HandleFunc("/api/secure", s.handleSecure)
	http.HandleFunc("/api/restart", s.handleRestart)
	http.HandleFunc("/api/sites", s.handleSites)
	http.HandleFunc("/api/ignore", s.handleIgnore)
	http.HandleFunc("/api/unignore", s.handleUnignore)
	http.HandleFunc("/api/plugins", s.handlePlugins)
	http.HandleFunc("/api/plugins/install", s.handlePluginInstall)
	http.HandleFunc("/api/plugins/toggle", s.handlePluginToggle)
	http.HandleFunc("/api/metrics", s.handleMetrics)
	http.HandleFunc("/api/share/start", s.handleShareStart)
	http.HandleFunc("/api/share/stop", s.handleShareStop)
	http.HandleFunc("/api/share/status", s.handleShareStatus)

	// Database Manager
	http.HandleFunc("/api/db/status", s.handleDBStatus)
	http.HandleFunc("/api/db/databases", s.handleDBDatabases)
	http.HandleFunc("/api/db/tables", s.handleDBTables)
	http.HandleFunc("/api/db/table", s.handleDBTableData)
	http.HandleFunc("/api/db/schema", s.handleDBSchema)
	http.HandleFunc("/api/db/snapshots", s.handleDBSnapshots)
	http.HandleFunc("/api/db/snapshots/download", s.handleDBDownload)
	http.HandleFunc("/api/db/snapshots/restore", s.handleDBRestore)
	http.HandleFunc("/api/db/import", s.handleDBImport)
	http.HandleFunc("/api/db/query", s.handleDBQuery)
	http.HandleFunc("/api/db/foreign-values", s.handleDBForeignValues)

	// Projects & System
	http.HandleFunc("/api/projects/create", s.handleProjectCreate)
	http.HandleFunc("/api/system/editors", s.handleSystemEditors)
	http.HandleFunc("/api/system/open-editor", s.handleSystemOpenEditor)
	http.HandleFunc("/api/system/directories", s.handleSystemDirectories)

	// Initialize WebSocket Hub
	hub := NewHub()
	go hub.Run()
	SetupEventBridge(hub)
	http.HandleFunc("/api/ws", s.handleWebSocket(hub))

	// Serve GUI static files
	guiFS, _ := assets.GetGuiFS()
	fileServer := http.FileServer(guiFS)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Skip API requests (addressed by specific handlers, but safe to check)
		if strings.HasPrefix(path, "/api") {
			http.NotFound(w, r)
			return
		}

		// Try to open the file relative to the FS
		// http.FS expects paths without leading slash for Open usually, but let's check.
		// Actually http.Dir implementation handles it.
		// Let's use simple logic:
		f, err := guiFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			defer f.Close()
			stat, _ := f.Stat()
			if !stat.IsDir() {
				// File exists, serve it
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// Fallback to index.html for SPA
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	fmt.Printf("SLD Daemon listening on port %d...\n", s.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", s.Port), nil)
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

func (s *Server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Directory string `json:"directory"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	opts := services.ProjectOptions{
		Type:      req.Type,
		Name:      req.Name,
		Directory: req.Directory,
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
	url, err := d.TunnelManager.StartTunnel(req.Site)
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
	var response []DBInfo
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
