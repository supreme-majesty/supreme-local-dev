package server

// Turning HTTPS on or off for one project from the dashboard: sets
// domain.tls in its .sld/config.yaml (keeping the rest of the file, comments
// included) and, when the project is running, applies it right away.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
)

func (s *Server) registerHTTPS(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/projects/{id}/tls", s.handleSetTLS)
}

// setDomainTLS returns data with domain.tls set to on. Off removes the key
// (and an emptied domain block) rather than writing "tls: false".
func setDomainTLS(data []byte, on bool) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid YAML configuration: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("the config is not a YAML mapping")
	}
	root := doc.Content[0]

	domainIdx := -1
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "domain" {
			domainIdx = i
		}
	}
	if domainIdx < 0 {
		if !on {
			return data, nil
		}
		// Add "domain:" right after "name:"/"version:" when present, else at the end.
		at := len(root.Content)
		for i := 0; i+1 < len(root.Content); i += 2 {
			if k := root.Content[i].Value; k == "version" || k == "name" {
				at = i + 2
			}
		}
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "domain"}
		val := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content[:at], append([]*yaml.Node{key, val}, root.Content[at:]...)...)
		domainIdx = at
	}
	domain := root.Content[domainIdx+1]
	if domain.Kind == yaml.ScalarNode && domain.Value == "" { // "domain:" with nothing after it
		domain.Kind, domain.Tag = yaml.MappingNode, "!!map"
	}
	if domain.Kind != yaml.MappingNode {
		return nil, errors.New("domain in the config is not a mapping")
	}

	tlsIdx := -1
	for i := 0; i+1 < len(domain.Content); i += 2 {
		if domain.Content[i].Value == "tls" {
			tlsIdx = i
		}
	}
	switch {
	case on && tlsIdx >= 0:
		domain.Content[tlsIdx+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
	case on:
		domain.Content = append(domain.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tls"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	case tlsIdx >= 0:
		domain.Content = append(domain.Content[:tlsIdx], domain.Content[tlsIdx+2:]...)
		if len(domain.Content) == 0 {
			root.Content = append(root.Content[:domainIdx], root.Content[domainIdx+2:]...)
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

func (s *Server) handleSetTLS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, ok := s.lookup(id)
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	path := filepath.Join(p.Root, ".sld", "config.yaml")
	old, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusConflict, p.Name+" is not set up yet; set it up first")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, err := setDomainTLS(old, req.Enabled)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if _, err := config.LoadFromBytes(updated, filepath.Base(p.Root)); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	running := false
	if v := s.view(r.Context(), p); v.Status == "running" || v.Status == "partial" {
		running = true
	}
	if !bytes.Equal(old, updated) {
		if err := s.writeProjectFile(r.Context(), p.Root, path, updated); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.invalidateScan()
	if !running {
		writeJSON(w, http.StatusOK, map[string]any{"tls": req.Enabled})
		return
	}

	// Running: start it again so the router picks up the HTTPS route (and the
	// certificate is issued). Containers whose spec changed are recreated.
	job, err := s.jobs.Start("https", id, func(ctx context.Context, job *Job) error {
		state := "off"
		if req.Enabled {
			state = "on"
		}
		fmt.Fprintf(job, "[SLD] HTTPS %s for %s; applying…\n", state, p.Name)
		err := s.newRunner(job).Start(ctx, p.Root)
		s.mu.Lock()
		s.scannedAt = time.Time{}
		s.mu.Unlock()
		return err
	})
	var busy ErrBusy
	if errors.As(err, &busy) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "the config is saved, but another operation is running; restart the project when it is done", "job_id": busy.JobID})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, _ := job.Snapshot(0)
	writeJSON(w, http.StatusAccepted, map[string]any{"tls": req.Enabled, "job": view})
}
