package api

import (
	"encoding/json"
	"net/http"

	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
)

func (s *Server) handleHealerIssues(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	issues := d.HealerService.GetActiveIssues()
	jsonResponse(w, issues, 200)
}

func (s *Server) handleHealerResolve(w http.ResponseWriter, r *http.Request) {
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
	if err := d.HealerService.ResolveIssue(req.ID); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}
