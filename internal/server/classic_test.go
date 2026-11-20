package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

func classicServer(t *testing.T, ws string, containers []*reconcile.ActualContainer) func(method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	t.Setenv("SLD_TLD", "")
	t.Setenv("SLD_HTTP_PORT", "8888")
	a, token := newAuth(t)
	mux := http.NewServeMux()
	srv := New(Options{Engine: &fakeEngine{containers: containers}, Env: hostenv.Host(t.TempDir()), Auth: a, Workspaces: []string{ws}, Version: "test"})
	srv.Register(mux)
	h := a.Middleware(mux)
	return func(method, path, body string) *httptest.ResponseRecorder {
		return do(h, method, path, "localhost", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+token)
			if body != "" {
				r.Body = ioNopCloser(body)
				r.Header.Set("Content-Type", "application/json")
			}
		})
	}
}

func TestClassicSitesAndIgnore(t *testing.T) {
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"blog/.git/HEAD":        "x",
		"blog/index.html":       "<h1>blog</h1>",
		"blog/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: static}\n",
		"shop/.git/HEAD":        "x",
		"shop/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: php, version: \"8.3\"}\n",
		"shop/public/index.php": "<?php",
		"shop/composer.json":    "{}",
		"notes/.git/HEAD":       "x", // undetected: not a site
		"notes/readme.txt":      "hi",
	})
	call := classicServer(t, ws, nil)

	rec := call("GET", "/api/sites", "")
	var sites []classicSite
	if err := json.Unmarshal(rec.Body.Bytes(), &sites); err != nil || rec.Code != 200 {
		t.Fatalf("sites: %d %s", rec.Code, rec.Body)
	}
	if len(sites) != 2 || sites[0].Name != "blog" || sites[0].Domain != "blog.localhost" || sites[1].PHPVersion != "8.3" {
		t.Fatalf("sites = %+v", sites)
	}

	// Tags and category are kept per domain.
	if rec := call("POST", "/api/sites/update", `{"domain":"blog.localhost","tags":["cms"],"category":"Personal"}`); rec.Code != 200 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}

	// Removing a project hides it with .sldignore; parking the folder shows it again.
	shop := filepath.Join(ws, "shop")
	if rec := call("POST", "/api/ignore", `{"path":"`+filepath.ToSlash(shop)+`"}`); rec.Code != 200 {
		t.Fatalf("ignore: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(shop, ".sldignore")); err != nil {
		t.Fatal("ignore did not write .sldignore")
	}
	rec = call("GET", "/api/sites?refresh=1", "")
	if strings.Contains(rec.Body.String(), `"shop"`) || !strings.Contains(rec.Body.String(), `"cms"`) {
		t.Errorf("after ignore: %s", rec.Body)
	}
	if rec := call("POST", "/api/park", `{"path":"`+filepath.ToSlash(shop)+`"}`); rec.Code != 200 {
		t.Fatalf("park: %d %s", rec.Code, rec.Body)
	}
	if rec := call("GET", "/api/sites", ""); !strings.Contains(rec.Body.String(), `"shop"`) {
		t.Errorf("after park: %s", rec.Body)
	}

	// Paths outside the workspace are refused.
	outside := filepath.ToSlash(t.TempDir())
	for _, path := range []string{"/api/ignore", "/api/park"} {
		if rec := call("POST", path, `{"path":"`+outside+`"}`); rec.Code != 400 {
			t.Errorf("%s outside workspace: %d", path, rec.Code)
		}
	}
	if rec := call("GET", "/api/system/directories?path="+outside, ""); rec.Code != 400 {
		t.Errorf("directories outside workspace: %d", rec.Code)
	}
	if rec := call("GET", "/api/system/directories", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), "blog") {
		t.Errorf("directories: %d %s", rec.Code, rec.Body)
	}

	// Host-only actions explain themselves instead of failing silently.
	for _, path := range []string{"/api/secure", "/api/php"} {
		if rec := call("POST", path, `{}`); rec.Code != http.StatusConflict {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	if rec := call("GET", "/api/state", ""); !strings.Contains(rec.Body.String(), `"port":"8888"`) {
		t.Errorf("state: %s", rec.Body)
	}
}

func TestClassicServicesAndPlugins(t *testing.T) {
	call := classicServer(t, t.TempDir(), []*reconcile.ActualContainer{
		{ID: "r1", Name: orchestrate.RouterContainer, Image: "ghcr.io/x/sld-router:2", State: "running"},
		{ID: "p1", Name: "sld-plugin-redis", Image: "redis:7-alpine", State: "exited", Labels: map[string]string{pluginLabel: "redis"}},
		{ID: "u1", Name: "someone-elses-app", Image: "nginx:1", State: "running"},
	})

	rec := call("GET", "/api/services", "")
	var services []infraService
	if err := json.Unmarshal(rec.Body.Bytes(), &services); err != nil {
		t.Fatal(err)
	}
	if len(services) != 2 || services[0].Name != "router" || !services[0].Running || services[0].Version != "2" || services[1].Name != "redis" {
		t.Fatalf("services = %+v", services)
	}
	if rec := call("POST", "/api/services/control", `{"service":"someone-elses-app","action":"stop"}`); rec.Code != 404 {
		t.Errorf("controlling a foreign container: %d", rec.Code)
	}
	if rec := call("POST", "/api/services/control", `{"service":"router","action":"restart"}`); rec.Code != 200 {
		t.Errorf("restart router: %d %s", rec.Code, rec.Body)
	}

	rec = call("GET", "/api/plugins", "")
	var plugins []pluginView
	_ = json.Unmarshal(rec.Body.Bytes(), &plugins)
	status := map[string]string{}
	for _, p := range plugins {
		status[p.ID] = p.Status
	}
	if status["redis"] != "stopped" || status["postgres"] != "not_installed" || status["mailpit"] != "not_installed" {
		t.Errorf("plugins = %+v", plugins)
	}

	rec = call("GET", "/api/system/doctor", "")
	if !strings.Contains(rec.Body.String(), `"Router","status":"pass"`) {
		t.Errorf("doctor: %s", rec.Body)
	}
}

func TestCreateValidationAndGhostGuard(t *testing.T) {
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{"app/.git/HEAD": "x", "app/index.html": "hi"})
	call := classicServer(t, ws, nil)

	for _, body := range []string{
		`{"type":"static","name":"../escape"}`,
		`{"type":"static","name":"has space"}`,
		`{"type":"custom","name":"x","repository":"--upload-pack=touch /tmp/pwned"}`,
		`{"type":"custom","name":"x","repository":"/etc"}`,
		`{"type":"rails-from-mars","name":"x"}`,
	} {
		if rec := call("POST", "/api/projects/create", body); rec.Code != 400 {
			t.Errorf("create %s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := call("POST", "/api/projects/create", `{"type":"static","name":"app"}`); rec.Code != http.StatusConflict {
		t.Errorf("create over an existing folder: %d", rec.Code)
	}

	// Only folders made by Ghost Mode can be discarded.
	app := filepath.ToSlash(filepath.Join(ws, "app"))
	if rec := call("POST", "/api/projects/ghost/discard", `{"path":"`+app+`"}`); rec.Code != 400 {
		t.Errorf("discarding a normal project: %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(ws, "app", "index.html")); err != nil {
		t.Fatal("a normal project was deleted")
	}
}

func TestCopyTreeSkipsDependencies(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "copy")
	writeTree(t, src, map[string]string{
		"index.php":                    "<?php",
		"app/Http/Kernel.php":          "<?php",
		"vendor/autoload.php":          "<?php",
		"node_modules/x/index.js":      "",
		".git/HEAD":                    "x",
		".sld/config.yaml":             "version: 2",
		"storage/logs/laravel.log":     "log",
		"storage/app/keep.txt":         "keep",
		"resources/vendor/theme/a.css": "", // only top-level vendor/ is skipped
	})
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"index.php", "app/Http/Kernel.php", "storage/app/keep.txt", "resources/vendor/theme/a.css"} {
		if _, err := os.Stat(filepath.Join(dst, p)); err != nil {
			t.Errorf("%s was not copied", p)
		}
	}
	for _, p := range []string{"vendor", "node_modules", ".git", ".sld", "storage/logs"} {
		if _, err := os.Stat(filepath.Join(dst, p)); err == nil {
			t.Errorf("%s was copied", p)
		}
	}
}

func ioNopCloser(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }
