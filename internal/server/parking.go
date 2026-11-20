package server

// Which folders SLD detects: only dockerized projects (their own compose file
// or Dockerfile, or containers SLD created for them) and projects the user
// parked or linked. Everything else in a workspace is skipped by the scan, so
// it is not listed and auto setup never writes a config into it.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	v2 "github.com/supreme-majesty/supreme-local-dev/internal/v2"
)

func (s *Server) registerParking(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/projects/{id}/link", s.handleProjectLink)
	mux.HandleFunc("GET /api/v2/parked", s.handleListParked)
	mux.HandleFunc("POST /api/v2/parked/remove", s.handleRemoveParked)
}

// parkedIn returns the parked/linked folder that covers root, or "".
func parkedIn(parked []string, root string) string {
	best := ""
	for _, p := range parked {
		if within(p, root) && len(p) > len(best) {
			best = p
		}
	}
	return best
}

// dockerizedApps reports whether the apps run through the project's own
// Docker files (compose stack or Dockerfile runtime).
func dockerizedApps(apps []v2.AppInfo) bool {
	for _, a := range apps {
		if a.Runtime == config.RuntimeCompose || a.Runtime == config.RuntimeDockerfile {
			return true
		}
	}
	return false
}

// projectsWithContainers returns the ids of projects SLD has containers for.
func (s *Server) projectsWithContainers(ctx context.Context) map[string]bool {
	ids := map[string]bool{}
	all, err := s.opts.Engine.ListAllContainers(ctx)
	if err != nil {
		return ids
	}
	for _, c := range all {
		if id := c.Labels[docker.LabelProject]; id != "" {
			ids[id] = true
		}
	}
	return ids
}

// keepProject is the scan filter: dockerized or parked/linked.
func (s *Server) keepProject() func(root string, apps []v2.AppInfo) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	withContainers := s.projectsWithContainers(ctx)
	parked := s.settings.Get().Parked
	return func(root string, apps []v2.AppInfo) bool {
		if parkedIn(parked, root) != "" || dockerizedApps(apps) || v2.HasDockerFiles(root) {
			return true
		}
		id, err := s.opts.Env.ProjectIdentity(root)
		return err == nil && withContainers[id.ID]
	}
}

// park records a folder (a project or a folder of projects) as parked.
func (s *Server) park(path string) error {
	_, err := s.settings.Update(func(st *Settings) {
		if !slices.Contains(st.Parked, path) {
			st.Parked = append(st.Parked, path)
			slices.Sort(st.Parked)
		}
	})
	return err
}

// unpark removes the parked/linked entry that covers root.
func (s *Server) unpark(root string) (removed string, err error) {
	_, err = s.settings.Update(func(st *Settings) {
		removed = parkedIn(st.Parked, root)
		if removed != "" {
			st.Parked = slices.DeleteFunc(st.Parked, func(p string) bool { return p == removed })
		}
	})
	return removed, err
}

// handleProjectLink links ({"linked": true}) or unlinks a project folder.
// Unlinking a project that was parked through a parent folder unparks that
// folder, which the response names.
func (s *Server) handleProjectLink(w http.ResponseWriter, r *http.Request) {
	p, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var req struct {
		Linked bool `json:"linked"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	resp := map[string]any{"linked": req.Linked}
	if req.Linked {
		if err := s.park(p.Root); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		removed, err := s.unpark(p.Root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if removed == "" {
			writeError(w, http.StatusConflict, p.Name+" is not parked or linked")
			return
		}
		resp["removed"] = s.relFolder(removed)
	}
	s.invalidateScan()
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListParked(w http.ResponseWriter, r *http.Request) {
	out := []map[string]string{}
	for _, p := range s.settings.Get().Parked {
		out = append(out, map[string]string{"path": p, "folder": s.relFolder(p)})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRemoveParked removes one parked/linked folder by its path.
func (s *Server) handleRemoveParked(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	found := false
	if _, err := s.settings.Update(func(st *Settings) {
		st.Parked = slices.DeleteFunc(st.Parked, func(p string) bool {
			if p == req.Path {
				found = true
			}
			return p == req.Path
		})
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not a parked folder: "+req.Path)
		return
	}
	s.invalidateScan()
	writeJSON(w, http.StatusOK, map[string]any{"removed": req.Path})
}
