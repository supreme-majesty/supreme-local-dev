package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/pkg/assets"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon/metrics"
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
	http.HandleFunc("/api/db/snapshots/restore", s.handleDBRestore)

	// Initialize WebSocket Hub
	hub := NewHub()
	go hub.Run()
	SetupXRayBridge(hub)
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

	// Convert offset/limit to page/perPage
	perPage := limit
	page := (offset / limit) + 1

	d, _ := daemon.GetClient()
	data, err := d.DatabaseService.GetTableData(db, table, page, perPage)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	// Transform rows from map to array to match frontend DataTable component
	var rows [][]interface{}
	var colNames []string

	for _, col := range data.Columns {
		colNames = append(colNames, col.Name)
	}

	for _, rowMap := range data.Rows {
		var row []interface{}
		for _, col := range colNames {
			row = append(row, rowMap[col])
		}
		rows = append(rows, row)
	}

	resp := map[string]interface{}{
		"columns": colNames,
		"rows":    rows,
		"total":   data.Total,
		"limit":   data.PerPage,
		"offset":  (data.Page - 1) * data.PerPage,
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
				"name":       s.Filename,
				"database":   s.Database,
				"size":       s.Size,
				"created_at": s.CreatedAt,
				"path":       s.Filename,
			})
		}
		jsonResponse(w, response, 200)

	case "POST":
		var req struct {
			Database string `json:"database"`
			Name     string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
			return
		}

		snapshot, err := d.DatabaseService.CreateSnapshot(req.Database)
		if err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}

		resp := map[string]interface{}{
			"id":         snapshot.Filename,
			"name":       snapshot.Filename,
			"database":   snapshot.Database,
			"size":       snapshot.Size,
			"created_at": snapshot.CreatedAt,
			"path":       snapshot.Filename,
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
