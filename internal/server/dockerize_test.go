package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
)

func TestDockerizeCandidates(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"blog/.git/HEAD":   "x",
		"blog/index.html":  "<h1>blog</h1>",
		"shop/.git/HEAD":   "x",
		"shop/index.html":  "<h1>shop</h1>",
		"shop/Dockerfile":  "FROM nginx\n",
		"notes/.git/HEAD":  "x",
		"notes/readme.txt": "no stack here",
		// No .git: found through their manifests.
		"group/wp/wp-config-sample.php":     "<?php",
		"group/wp/index.php":                "<?php",
		"group/wp/wp-content/plugins/x.php": "<?php /* Plugin Name: Inner */",
		"group/sms-plugin/index.php":        "<?php\n/*\n * Plugin Name: SMS Notifications\n */",
		"group/sms-plugin/composer.json":    `{"require":{}}`,
		// A repository with sub-folder apps.
		"mono/.git/HEAD":           "x",
		"mono/backend/index.html":  "<h1>api</h1>",
		"mono/frontend/index.html": "<h1>ui</h1>",
	})
	a, token := newAuth(t)
	mux := http.NewServeMux()
	srv := New(Options{Engine: &fakeEngine{}, Env: hostenv.Host(t.TempDir()), Auth: a, Workspaces: []string{ws}})
	srv.Register(mux)
	h := a.Middleware(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Host = "localhost"
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// shop has a Dockerfile, so it is already detected; blog and notes are candidates.
	rec := call("GET", "/api/v2/candidates", "")
	var list []Candidate
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	got := map[string]Candidate{}
	for _, c := range list {
		got[c.Folder] = c
	}
	if _, ok := got["shop"]; ok || len(got) == 0 {
		t.Fatalf("candidates = %+v, want blog (and notes) but not shop", list)
	}
	blog, ok := got["blog"]
	if !ok || !blog.Detected || len(blog.Apps) == 0 {
		t.Fatalf("blog should be a detected candidate: %+v", list)
	}

	if wp, ok := got["group/wp"]; !ok || !wp.Detected {
		t.Errorf("a WordPress site without .git should be a detected candidate: %+v", list)
	}
	plugin, ok := got["group/sms-plugin"]
	if !ok || plugin.Detected || !strings.Contains(plugin.Reason, "WordPress plugin") {
		t.Errorf("a WordPress plugin must be listed as not dockerizable: %+v", plugin)
	}
	if mono := got["mono"]; len(mono.Apps) != 2 {
		t.Fatalf("mono should list its two sub-folder apps: %+v", mono)
	}

	// Only the chosen sub-folder app is dockerized.
	body, _ := json.Marshal(map[string]any{"projects": []map[string]any{{"path": got["mono"].Path, "apps": []string{got["mono"].Apps[1].Name}}}})
	rec = call("POST", "/api/v2/dockerize", string(body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("dockerize mono: %d %s", rec.Code, rec.Body)
	}
	var monoJob JobView
	_ = json.Unmarshal(rec.Body.Bytes(), &monoJob)
	if j, ok := srv.jobs.Get(monoJob.ID); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if done := waitJob(ctx, j); done.Status != JobSucceeded {
			t.Fatalf("mono job %s: %s\n%s", done.Status, done.Error, done.Output)
		}
		cancel()
	}
	compose, _ := os.ReadFile(filepath.Join(ws, "mono", "compose.yaml"))
	if n := strings.Count(string(compose), "\n  web:\n"); n != 1 || strings.Contains(string(compose), "./"+got["mono"].Apps[0].Path) {
		t.Errorf("only %s should be in compose.yaml:\n%s", got["mono"].Apps[1].Name, compose)
	}
	if rec := call("POST", "/api/v2/dockerize", `{"paths":["`+plugin.Path+`"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("plugin must be refused: %d %s", rec.Code, rec.Body)
	}

	// An arbitrary path is refused.
	if rec := call("POST", "/api/v2/dockerize", `{"paths":["/etc"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("arbitrary path: %d %s", rec.Code, rec.Body)
	}
	if n, ok := got["notes"]; ok && !n.Detected {
		body, _ := json.Marshal(map[string]any{"paths": []string{n.Path}})
		if rec := call("POST", "/api/v2/dockerize", string(body)); rec.Code != http.StatusBadRequest {
			t.Errorf("undetected stack: %d %s", rec.Code, rec.Body)
		}
	}

	body, _ = json.Marshal(map[string]any{"paths": []string{blog.Path}})
	rec = call("POST", "/api/v2/dockerize", string(body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("dockerize: %d %s", rec.Code, rec.Body)
	}
	var started JobView
	_ = json.Unmarshal(rec.Body.Bytes(), &started)
	job, ok := srv.jobs.Get(started.ID)
	if !ok {
		t.Fatal("job not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if done := waitJob(ctx, job); done.Status != JobSucceeded {
		t.Fatalf("job %s: %s\n%s", done.Status, done.Error, done.Output)
	}

	for _, f := range []string{"compose.yaml", "Dockerfile", ".dockerignore", "DOCKER.md", ".sld/config.yaml"} {
		if _, err := os.Stat(filepath.Join(ws, "blog", f)); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
	}
	if cfg, _ := os.ReadFile(filepath.Join(ws, "blog", ".sld", "config.yaml")); !strings.Contains(string(cfg), "runtime: compose") {
		t.Errorf("SLD config should use the compose stack:\n%s", cfg)
	}
	if !strings.Contains(call("GET", "/api/v2/projects", "").Body.String(), `"folder":"blog"`) {
		t.Error("blog should now be listed with the projects")
	}
	rec = call("GET", "/api/v2/candidates", "")
	if strings.Contains(rec.Body.String(), `"folder":"blog"`) {
		t.Errorf("blog should no longer be a candidate: %s", rec.Body)
	}
}
