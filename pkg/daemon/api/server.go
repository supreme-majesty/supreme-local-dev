package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/supreme-majesty/supreme-local-dev/pkg/assets"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
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

	// Serve GUI static files
	guiFS, _ := assets.GetGuiFS()
	http.Handle("/", http.FileServer(guiFS))

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
