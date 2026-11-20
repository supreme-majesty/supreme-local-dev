package server

// Sharing a project publicly: a cloudflared quick tunnel runs as a container
// on the router network and forwards a random trycloudflare.com URL to the
// router with the project's Host header.

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
	v2 "github.com/supreme-majesty/supreme-local-dev/internal/v2"
)

const (
	shareLabel       = "com.supreme.sld.share"
	shareDomainLabel = "com.supreme.sld.share-domain"
	cloudflaredImage = "cloudflare/cloudflared:2024.12.2"
)

var tunnelURL = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

func (s *Server) registerShare(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/share/start", s.handleShareStart)
	mux.HandleFunc("POST /api/share/stop", s.handleShareStop)
	mux.HandleFunc("GET /api/share/status", s.handleShareStatus)
}

type tunnelView struct {
	SiteName  string    `json:"site_name"`
	PublicURL string    `json:"public_url"`
	StartedAt time.Time `json:"started_at"`
}

var shareNameRe = regexp.MustCompile(`[^a-z0-9-]+`)

// shareTarget resolves a site name, domain or project id to the project and
// the domain to share (the project's primary app unless a domain is given).
func (s *Server) shareTarget(site string) (v2.ScanResult, string, bool) {
	results, err := s.scan(false)
	if err != nil {
		return v2.ScanResult{}, "", false
	}
	for _, res := range results {
		id, _ := s.opts.Env.ProjectIdentity(res.Root)
		for _, app := range res.AppList {
			if res.Name == site || app.Domain == site || (id != nil && id.ID == site) {
				return res, app.Domain, true
			}
		}
	}
	return v2.ScanResult{}, "", false
}

func (s *Server) shareContainers(ctx context.Context) ([]*reconcile.ActualContainer, error) {
	all, err := s.opts.Engine.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}
	var out []*reconcile.ActualContainer
	for _, c := range all {
		if c.Labels[shareLabel] != "" {
			out = append(out, c)
		}
	}
	return out, nil
}

func decodeSite(r *http.Request) string {
	var req struct {
		Site string `json:"site"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
	return req.Site
}

func (s *Server) handleShareStart(w http.ResponseWriter, r *http.Request) {
	site := decodeSite(r)
	project, domain, ok := s.shareTarget(site)
	if !ok {
		writeError(w, http.StatusNotFound, "no project named "+site)
		return
	}
	ctx := r.Context()
	// A tunnel to a stopped project only ever answers 404, publicly.
	if v := s.view(ctx, project); v.Status != "running" && v.Status != "partial" {
		writeError(w, http.StatusConflict, project.Name+" is not running; start it before sharing it")
		return
	}
	site = project.Name // tunnels are labelled with the project name, whatever was passed
	existing, err := s.shareContainers(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, c := range existing {
		if c.Labels[shareLabel] == site && c.State == "running" {
			if url := s.tunnelURLOf(ctx, c.ID, 15*time.Second); url != "" {
				writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": url})
				return
			}
		}
		if c.Labels[shareLabel] == site {
			_ = s.opts.Engine.RemoveContainer(ctx, c.ID, true)
		}
	}

	id, err := s.opts.Engine.CreateContainer(ctx, &reconcile.DesiredContainer{
		Name:     "sld-share-" + strings.Trim(shareNameRe.ReplaceAllString(strings.ToLower(site), "-"), "-"),
		Image:    cloudflaredImage,
		Cmd:      []string{"tunnel", "--no-autoupdate", "--url", "http://" + orchestrate.RouterContainer + ":80", "--http-host-header", domain},
		Networks: []string{orchestrate.GlobalNetwork},
		Labels:   map[string]string{shareLabel: site, shareDomainLabel: domain},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.opts.Engine.StartContainer(ctx, id); err != nil {
		_ = s.opts.Engine.RemoveContainer(ctx, id, true)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	url := s.tunnelURLOf(ctx, id, 45*time.Second)
	if url == "" {
		_ = s.opts.Engine.RemoveContainer(context.Background(), id, true)
		writeError(w, http.StatusGatewayTimeout, "the tunnel did not report a public URL (is the internet reachable from Docker?)")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": url})
}

// tunnelURLOf reads the public URL cloudflared prints when the tunnel is up.
func (s *Server) tunnelURLOf(ctx context.Context, id string, wait time.Duration) string {
	deadline := time.Now().Add(wait)
	for {
		if url := s.scanLogsFor(ctx, id); url != "" {
			return url
		}
		if time.Now().After(deadline) {
			return ""
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(time.Second):
		}
	}
}

func (s *Server) scanLogsFor(ctx context.Context, id string) string {
	rc, err := s.opts.Engine.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: "200"})
	if err != nil {
		return ""
	}
	defer rc.Close()
	pr, pw := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, rc)
		pw.CloseWithError(err)
	}()
	defer pr.Close()
	sc := bufio.NewScanner(pr)
	for sc.Scan() {
		if url := tunnelURL.FindString(sc.Text()); url != "" {
			return url
		}
	}
	return ""
}

func (s *Server) handleShareStop(w http.ResponseWriter, r *http.Request) {
	site := decodeSite(r)
	containers, err := s.shareContainers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, c := range containers {
		if c.Labels[shareLabel] == site {
			if err := s.opts.Engine.RemoveContainer(r.Context(), c.ID, true); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleShareStatus(w http.ResponseWriter, r *http.Request) {
	containers, err := s.shareContainers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tunnels := []tunnelView{}
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		v := tunnelView{SiteName: c.Labels[shareLabel], PublicURL: s.scanLogsFor(r.Context(), c.ID)}
		if info, err := s.opts.Engine.InspectContainer(r.Context(), c.ID); err == nil && info.State != nil {
			v.StartedAt, _ = time.Parse(time.RFC3339Nano, info.State.StartedAt)
		}
		tunnels = append(tunnels, v)
	}
	sort.Slice(tunnels, func(i, j int) bool { return tunnels[i].SiteName < tunnels[j].SiteName })
	writeJSON(w, http.StatusOK, tunnels)
}
