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
	Success bool `json:"success"`
	Message string `json:"message,omitempty"`
}

func jsonResponse(w http.ResponseWriter, data interface{}, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // For dev
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	jsonResponse(w, d.State.Data, 200)
}

func (s *Server) handlePark(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { return }
	var req struct { Path string `json:"path"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400); return
	}

	d, _ := daemon.GetClient()
	if err := d.Park(req.Path); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500); return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleForget(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { return }
	var req struct { Path string `json:"path"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400); return
	}

	d, _ := daemon.GetClient()
	if err := d.Forget(req.Path); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500); return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { return }
	var req struct { Name string `json:"name"`; Path string `json:"path"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400); return
	}

	d, _ := daemon.GetClient()
	// Use absolute path if provided path is relative? 
	// The CLI resolves it, but the GUI might send raw strings.
	// Best to assume GUI sends valid paths, but we can verify.
	path, _ := filepath.Abs(req.Path)
	
	if err := d.Link(req.Name, path); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500); return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleUnlink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { return }
	var req struct { Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400); return
	}

	d, _ := daemon.GetClient()
	if err := d.Unlink(req.Name); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500); return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handlePHP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { return }
	var req struct { Version string `json:"version"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400); return
	}

	d, _ := daemon.GetClient()
	if err := d.SwitchPHP(req.Version); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500); return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleSecure(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { return }
	d, _ := daemon.GetClient()
	if err := d.Secure(); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500); return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}
