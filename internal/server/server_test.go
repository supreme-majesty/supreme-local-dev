package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

func newAuth(t *testing.T) (*Auth, string) {
	t.Helper()
	t.Setenv(envAdminToken, "")
	t.Setenv(envAllowedHost, "")
	var out bytes.Buffer
	a, err := LoadAuth(t.TempDir(), false, &out)
	if err != nil {
		t.Fatal(err)
	}
	// The generated token is printed once; extract it from the banner.
	for _, line := range strings.Split(out.String(), "\n") {
		if l := strings.TrimSpace(line); len(l) > 20 && !strings.ContainsAny(l, " ═:") {
			return a, l
		}
	}
	t.Fatalf("token not printed:\n%s", out.String())
	return nil, ""
}

func protected() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
}

func do(h http.Handler, method, path, host string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Host = host
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthMiddleware(t *testing.T) {
	a, token := newAuth(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", a.handleLogin)
	mux.Handle("/", protected())
	h := a.Middleware(mux)

	// Static dashboard files and health are public; API is not.
	if rec := do(h, "GET", "/index.html", "localhost:2025", nil); rec.Code != 200 {
		t.Errorf("static file: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/health", "localhost:2025", nil); rec.Code != 200 {
		t.Errorf("health: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v2/projects", "localhost:2025", nil); rec.Code != 401 {
		t.Errorf("unauthenticated API should be 401, got %d", rec.Code)
	}

	// DNS rebinding: unknown Host names are refused even for public paths.
	if rec := do(h, "GET", "/", "evil.example.com", nil); rec.Code != 403 {
		t.Errorf("foreign Host should be 403, got %d", rec.Code)
	}
	for _, host := range []string{"127.0.0.1:2025", "sld.localhost:8888", "[::1]:2025"} {
		if rec := do(h, "GET", "/", host, nil); rec.Code != 200 {
			t.Errorf("Host %s should be allowed, got %d", host, rec.Code)
		}
	}

	// Bearer token for scripts.
	bearer := func(tok string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
	}
	if rec := do(h, "POST", "/api/v2/projects/x/start", "localhost", bearer(token)); rec.Code != 200 {
		t.Errorf("valid bearer: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v2/projects", "localhost", bearer("wrong")); rec.Code != 401 {
		t.Errorf("wrong bearer: %d", rec.Code)
	}

	// Browser sign-in → session cookie.
	login := func(tok string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v2/auth/login", strings.NewReader(`{"token":"`+tok+`"}`))
		req.Host = "localhost"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := login("nope"); rec.Code != 401 {
		t.Errorf("bad login: %d", rec.Code)
	}
	rec := login(token)
	cookies := rec.Result().Cookies()
	if rec.Code != 200 || len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("login: code=%d cookies=%+v", rec.Code, cookies)
	}
	withCookie := func(extra func(*http.Request)) func(*http.Request) {
		return func(r *http.Request) {
			r.AddCookie(cookies[0])
			if extra != nil {
				extra(r)
			}
		}
	}
	if rec := do(h, "GET", "/api/v2/projects", "localhost", withCookie(nil)); rec.Code != 200 {
		t.Errorf("cookie GET: %d", rec.Code)
	}
	// Writes with a cookie need the CSRF header and a local Origin.
	if rec := do(h, "POST", "/api/v2/projects/x/start", "localhost", withCookie(nil)); rec.Code != 403 {
		t.Errorf("cookie POST without CSRF header should be 403, got %d", rec.Code)
	}
	csrf := func(r *http.Request) { r.Header.Set(csrfHeader, "1") }
	if rec := do(h, "POST", "/api/v2/projects/x/start", "localhost", withCookie(csrf)); rec.Code != 200 {
		t.Errorf("cookie POST with CSRF header: %d", rec.Code)
	}
	evilOrigin := func(r *http.Request) { csrf(r); r.Header.Set("Origin", "https://evil.example.com") }
	if rec := do(h, "POST", "/api/v2/projects/x/start", "localhost", withCookie(evilOrigin)); rec.Code != 403 {
		t.Errorf("cross-origin POST should be 403, got %d", rec.Code)
	}
}

func TestLoadAuth_PersistsOnlyHash(t *testing.T) {
	t.Setenv(envAdminToken, "")
	dir := t.TempDir()
	var first, second bytes.Buffer
	a1, err := LoadAuth(dir, false, &first)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, authFileName))
	for _, line := range strings.Split(first.String(), "\n") {
		if tok := strings.TrimSpace(line); len(tok) > 20 && !strings.ContainsAny(tok, " ═:") && strings.Contains(string(data), tok) {
			t.Fatal("token stored in plain text")
		}
	}
	a2, err := LoadAuth(dir, false, &second)
	if err != nil {
		t.Fatal(err)
	}
	if a1.tokenHash != a2.tokenHash || strings.Contains(second.String(), "shown only once") {
		t.Error("second start must reuse the stored token without printing a new one")
	}
	a3, _ := LoadAuth(dir, true, io.Discard)
	if a3.tokenHash == a1.tokenHash {
		t.Error("--reset-token must issue a new token")
	}

	t.Setenv(envAdminToken, "from-env")
	a4, _ := LoadAuth(dir, false, io.Discard)
	if !a4.checkToken("from-env") {
		t.Error("SLD_ADMIN_TOKEN should be accepted")
	}
}

func TestJobs(t *testing.T) {
	js := NewJobs()
	release := make(chan struct{})
	job, err := js.Start("start", "p1", func(ctx context.Context, j *Job) error {
		io.WriteString(j, "step 1\n")
		<-release
		io.WriteString(j, "step 2\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.Start("stop", "p1", func(context.Context, *Job) error { return nil }); !errors.As(err, &ErrBusy{}) {
		t.Errorf("second job for the same project should be refused, got %v", err)
	}
	if js.Running("p1") != job.ID {
		t.Error("Running should report the job")
	}
	close(release)
	waitDone(t, job)
	view, _ := job.Snapshot(0)
	if view.Status != JobSucceeded || view.Output != "step 1\nstep 2\n" {
		t.Errorf("job view = %+v", view)
	}
	if again, _ := job.Snapshot(view.Offset); again.Output != "" {
		t.Errorf("offset should skip seen output, got %q", again.Output)
	}

	failed, _ := js.Start("start", "p2", func(context.Context, *Job) error { return errors.New("boom") })
	waitDone(t, failed)
	if v, _ := failed.Snapshot(0); v.Status != JobFailed || v.Error != "boom" {
		t.Errorf("failed job = %+v", v)
	}
	panicky, _ := js.Start("start", "p3", func(context.Context, *Job) error { panic("bad") })
	waitDone(t, panicky)
	if v, _ := panicky.Snapshot(0); v.Status != JobFailed || !strings.Contains(v.Error, "bad") {
		t.Errorf("panicking job = %+v", v)
	}
}

func waitDone(t *testing.T, j *Job) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		v, updated := j.Snapshot(0)
		if v.Status != JobRunning {
			return
		}
		select {
		case <-updated:
		case <-deadline:
			t.Fatal("job did not finish")
		}
	}
}

// fakeEngine satisfies docker.EngineClient for API tests.
type fakeEngine struct{ containers []*reconcile.ActualContainer }

func (f *fakeEngine) Ping(context.Context) error { return nil }
func (f *fakeEngine) EnsureNetwork(context.Context, string, map[string]string) (string, error) {
	return "", nil
}
func (f *fakeEngine) RemoveNetwork(context.Context, string) error { return nil }
func (f *fakeEngine) EnsureVolume(context.Context, string, map[string]string) (string, error) {
	return "", nil
}
func (f *fakeEngine) RemoveVolume(context.Context, string, bool) error             { return nil }
func (f *fakeEngine) ListProjectVolumes(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeEngine) ListProjectContainers(_ context.Context, id string) ([]*reconcile.ActualContainer, error) {
	var out []*reconcile.ActualContainer
	for _, c := range f.containers {
		if c.Labels["com.supreme.sld.project"] == id {
			out = append(out, c)
		}
	}
	return out, nil
}
func (f *fakeEngine) ListAllContainers(context.Context) ([]*reconcile.ActualContainer, error) {
	return f.containers, nil
}
func (f *fakeEngine) CreateContainer(context.Context, *reconcile.DesiredContainer) (string, error) {
	return "", nil
}
func (f *fakeEngine) StartContainer(context.Context, string) error         { return nil }
func (f *fakeEngine) StopContainer(context.Context, string, int) error     { return nil }
func (f *fakeEngine) RemoveContainer(context.Context, string, bool) error  { return nil }
func (f *fakeEngine) ConnectNetwork(context.Context, string, string) error { return nil }
func (f *fakeEngine) InspectContainer(context.Context, string) (types.ContainerJSON, error) {
	return types.ContainerJSON{}, nil
}
func (f *fakeEngine) ContainerLogs(context.Context, string, container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (f *fakeEngine) PullImage(context.Context, string) error { return nil }
func (f *fakeEngine) Close() error                            { return nil }

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, c := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProjectsAPI(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	t.Setenv("SLD_HTTP_PORT", "8888")
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"laravel-builds/swiftsms/.git/HEAD":    "x",
		"laravel-builds/swiftsms/compose.yaml": "services:\n  app:\n    image: swiftsms\n    ports: [\"${APP_PORT:-8080}:8080\"]\n  db:\n    image: mysql:8.4\n",
		"sites/blog/.git/HEAD":                 "x",
		"sites/blog/index.html":                "<h1>blog</h1>",
		"sites/blog/.sld/config.yaml":          "version: 2\napps:\n  web: {runtime: static}\n",
	})
	env := hostenv.Host(t.TempDir())
	blogID, err := env.ProjectIdentity(filepath.Join(ws, "sites", "blog"))
	if err != nil {
		t.Fatal(err)
	}
	engine := &fakeEngine{containers: []*reconcile.ActualContainer{{
		ID: "c1", Name: "sld-" + blogID.ID + "-web", Service: "web", State: "running",
		Labels: map[string]string{"com.supreme.sld.project": blogID.ID, "com.supreme.sld.app": "web"},
	}}}

	a, token := newAuth(t)
	mux := http.NewServeMux()
	srv := New(Options{Engine: engine, Env: env, Auth: a, Workspaces: []string{ws}, Version: "test"})
	srv.Register(mux)
	h := a.Middleware(mux)
	call := func(method, path string) *httptest.ResponseRecorder {
		return do(h, method, path, "localhost", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) })
	}

	rec := call("GET", "/api/v2/projects")
	if rec.Code != 200 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	var projects []ProjectView
	if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %+v", projects)
	}
	byName := map[string]ProjectView{}
	for _, p := range projects {
		byName[p.Name] = p
	}
	blog, swift := byName["blog"], byName["swiftsms"]
	if blog.Status != "running" || !blog.Configured || blog.Running != 1 || blog.Apps[0].URL != "http://blog.localhost:8888" {
		t.Errorf("blog = %+v", blog)
	}
	if swift.Status != "not-configured" || swift.Configured || swift.Folder != "laravel-builds/swiftsms" || swift.Apps[0].Runtime != "compose" {
		t.Errorf("swiftsms = %+v", swift)
	}

	// Set up swiftsms through the API (writes its config) and follow the job.
	rec = call("POST", "/api/v2/projects/"+swift.ID+"/setup")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}
	var job JobView
	_ = json.Unmarshal(rec.Body.Bytes(), &job)
	j, ok := srv.jobs.Get(job.ID)
	if !ok {
		t.Fatal("job not registered")
	}
	waitDone(t, j)
	if v, _ := j.Snapshot(0); v.Status != JobSucceeded || !strings.Contains(v.Output, "written") {
		t.Errorf("setup job = %+v", v)
	}
	if _, err := os.Stat(filepath.Join(ws, "laravel-builds", "swiftsms", ".sld", "config.yaml")); err != nil {
		t.Error("setup did not write the config")
	}

	rec = call("GET", "/api/v2/projects/"+swift.ID)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "runtime: compose") || !strings.Contains(rec.Body.String(), `"configured":true`) {
		t.Errorf("project detail after setup: %d %s", rec.Code, rec.Body)
	}

	if rec := call("POST", "/api/v2/projects/"+swift.ID+"/explode"); rec.Code != 404 {
		t.Errorf("unknown action: %d", rec.Code)
	}
	if rec := call("GET", "/api/v2/projects/nope"); rec.Code != 404 {
		t.Errorf("unknown project: %d", rec.Code)
	}
	if rec := call("GET", "/api/v2/system"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"http_port":8888`) {
		t.Errorf("system: %d %s", rec.Code, rec.Body)
	}
}

func TestSettingsStore(t *testing.T) {
	dir := t.TempDir()
	st, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Get(); !got.RestoreRunning || got.AutoSetup || got.AutostartAll {
		t.Errorf("defaults = %+v, want only restore_running on", got)
	}
	if _, err := st.Update(func(s *Settings) {
		s.AutoSetup = true
		s.Autostart["a"] = true
		s.Autostart["b"] = false // dropped on save
		s.Running["c"] = true
	}); err != nil {
		t.Fatal(err)
	}
	again, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := again.Get()
	if !got.AutoSetup || !got.Autostart["a"] || len(got.Autostart) != 1 || !got.Running["c"] {
		t.Errorf("reloaded = %+v", got)
	}
	// Get returns a copy: mutating it must not change the store.
	got.Autostart["x"] = true
	if again.Get().Autostart["x"] {
		t.Error("Get must return a copy")
	}
}

func TestStartupTargets(t *testing.T) {
	projects := []ProjectView{
		{ID: "z-run", Folder: "z", Configured: true, Status: "stopped"},
		{ID: "a-pin", Folder: "a", Configured: true, Status: "stopped"},
		{ID: "m-up", Folder: "m", Configured: true, Status: "running"},
		{ID: "n-new", Folder: "n", Configured: false, Status: "not-configured"},
		{ID: "b-other", Folder: "b", Configured: true, Status: "stopped"},
	}
	ids := func(ps []ProjectView) string {
		var out []string
		for _, p := range ps {
			out = append(out, p.ID)
		}
		return strings.Join(out, ",")
	}
	s := defaultSettings()
	s.Running["z-run"] = true
	s.Running["m-up"] = true
	s.Autostart["a-pin"] = true
	if got := ids(startupTargets(projects, s)); got != "a-pin,z-run" {
		t.Errorf("restore + pinned = %s, want a-pin,z-run (sorted, running ones skipped)", got)
	}
	s.RestoreRunning = false
	if got := ids(startupTargets(projects, s)); got != "a-pin" {
		t.Errorf("without restore = %s", got)
	}
	s.AutostartAll = true
	if got := ids(startupTargets(projects, s)); got != "a-pin,b-other,z-run" {
		t.Errorf("autostart all = %s (must skip not set up and already running)", got)
	}
}

func TestAutomationAPI(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"blog/.git/HEAD":        "x",
		"blog/index.html":       "<h1>blog</h1>",
		"shop/.git/HEAD":        "x",
		"shop/index.html":       "<h1>shop</h1>",
		"blog/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: static}\n",
	})
	env := hostenv.Host(t.TempDir())
	a, token := newAuth(t)
	mux := http.NewServeMux()
	srv := New(Options{Engine: &fakeEngine{}, Env: env, Auth: a, Workspaces: []string{ws}})
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

	if rec := call("GET", "/api/v2/settings", ""); !strings.Contains(rec.Body.String(), `"restore_running":true`) {
		t.Errorf("default settings: %s", rec.Body)
	}
	// Turning auto setup on writes the config of the new project on the next listing.
	if rec := call("PUT", "/api/v2/settings", `{"auto_setup":true}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"auto_setup":true`) {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}
	rec := call("GET", "/api/v2/projects", "")
	if _, err := os.Stat(filepath.Join(ws, "shop", ".sld", "config.yaml")); err != nil {
		t.Errorf("auto setup did not write shop's config: %v", err)
	}
	var projects []ProjectView
	_ = json.Unmarshal(rec.Body.Bytes(), &projects)
	var shop ProjectView
	for _, p := range projects {
		if p.Name == "shop" {
			shop = p
		}
	}
	if !shop.Configured {
		t.Fatalf("shop should be set up: %+v", shop)
	}

	// Per-project auto start toggle is reflected in the listing.
	if rec := call("POST", "/api/v2/projects/"+shop.ID+"/autostart", `{"enabled":true}`); rec.Code != 200 {
		t.Fatalf("autostart: %d %s", rec.Code, rec.Body)
	}
	srv.invalidateScan()
	rec = call("GET", "/api/v2/projects", "")
	if !strings.Contains(rec.Body.String(), `"autostart":true`) {
		t.Errorf("autostart not shown: %s", rec.Body)
	}
	if !srv.settings.Get().Autostart[shop.ID] {
		t.Error("autostart not persisted")
	}

	// Stop/start bookkeeping for "restart what was running".
	srv.recordRunning(shop.ID, "start")
	if !srv.settings.Get().Running[shop.ID] {
		t.Error("start should be remembered")
	}
	srv.recordRunning(shop.ID, "stop")
	if srv.settings.Get().Running[shop.ID] {
		t.Error("stop should be remembered")
	}
}
