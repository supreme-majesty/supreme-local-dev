package server

// System Logs: the logs of every container SLD runs, SLD's own services
// (control plane, router, database server, plugins, share tunnels) and each
// project's containers, streamed from Docker. Only containers in the source
// list can be read.

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
)

func (s *Server) registerSystemLogs(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/logs/sources", s.handleLogSources)
	mux.HandleFunc("GET /api/v2/logs/stream", s.handleLogStream)
}

// LogSource is one container whose logs can be read.
type LogSource struct {
	Container string `json:"container"`
	Label     string `json:"label"`
	Group     string `json:"group"` // "SLD", "Shares" or the project name
	State     string `json:"state"`
}

const sldGroup = "SLD"

func (s *Server) logSources(ctx context.Context) ([]LogSource, error) {
	containers, err := s.opts.Engine.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}
	stack := s.selfComposeProject(ctx)
	selfID := s.opts.Env.ContainerID()
	names := s.projectNames()

	out := []LogSource{}
	for _, c := range containers {
		src := LogSource{Container: c.Name, State: c.State, Label: c.Name}
		switch {
		case selfID != "" && strings.HasPrefix(c.ID, selfID):
			src.Group, src.Label = sldGroup, "Control plane (this dashboard)"
		case c.Name == orchestrate.RouterContainer:
			src.Group, src.Label = sldGroup, "Router (Traefik)"
		case c.Labels[pluginLabel] != "":
			src.Group, src.Label = sldGroup, c.Labels[pluginLabel]
		case stack != "" && c.Labels[composeProjectLabel] == stack:
			src.Group, src.Label = sldGroup, c.Labels[composeServiceLabel]
		case c.Labels[shareLabel] != "":
			src.Group, src.Label = "Shares", c.Labels[shareLabel]+" tunnel"
		case c.Labels[shareProxyLabel] != "":
			src.Group, src.Label = "Shares", c.Labels[shareProxyLabel]+" proxy"
		case c.Labels[docker.LabelProject] != "":
			id := c.Labels[docker.LabelProject]
			src.Group = names[id]
			if src.Group == "" {
				src.Group = filepath.Base(c.Labels[docker.LabelProjectPath])
			}
			if svc := firstNonEmptyLabel(c.Labels[composeServiceLabel], c.Labels[docker.LabelService]); svc != "" {
				src.Label = svc
			}
		default:
			continue
		}
		out = append(out, src)
	}
	sort.SliceStable(out, func(i, j int) bool {
		gi, gj := groupRank(out[i].Group), groupRank(out[j].Group)
		if gi != gj {
			return gi < gj
		}
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Label < out[j].Label
	})
	return out, nil
}

func groupRank(g string) int {
	switch g {
	case sldGroup:
		return 0
	case "Shares":
		return 2
	}
	return 1
}

func firstNonEmptyLabel(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// projectNames maps project ids to names from the last scan.
func (s *Server) projectNames() map[string]string {
	_, _ = s.scan(false)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.projects))
	for id, p := range s.projects {
		out[id] = p.Name
	}
	return out
}

func (s *Server) handleLogSources(w http.ResponseWriter, r *http.Request) {
	list, err := s.logSources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleLogStream streams one container's logs as text, one line per entry,
// each prefixed with its RFC 3339 timestamp. ?tail=N (default 200),
// ?follow=1 keeps the stream open.
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("container")
	list, err := s.logSources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	known := false
	for _, src := range list {
		known = known || src.Container == name
	}
	if !known {
		writeError(w, http.StatusNotFound, "no log source named "+name)
		return
	}
	tail := r.URL.Query().Get("tail")
	if n, err := strconv.Atoi(tail); err != nil || n <= 0 || n > 5000 {
		tail = "200"
	}
	follow := r.URL.Query().Get("follow") == "1"
	reader, err := s.opts.Engine.ContainerLogs(r.Context(), name, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: follow, Tail: tail, Timestamps: true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	out := io.Writer(w)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
		if follow {
			out = flushWriter{w: w, f: f}
		}
	}
	_, _ = stdcopy.StdCopy(out, out, reader)
}
