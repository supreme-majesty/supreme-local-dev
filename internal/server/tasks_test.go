package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// execEngine is a fakeEngine that also runs commands (docker.Executor).
type execEngine struct {
	fakeEngine
	mu      sync.Mutex
	execs   []string // "container|user|cmd"
	exit    int
	stopped []string
	started []string
}

func (e *execEngine) Exec(_ context.Context, name string, opts docker.ExecOptions) (int, error) {
	e.mu.Lock()
	e.execs = append(e.execs, name+"|"+opts.User+"|"+strings.Join(opts.Cmd, " "))
	e.mu.Unlock()
	fmt.Fprintln(opts.Stdout, "output from", name)
	return e.exit, nil
}

func (e *execEngine) RunEphemeral(context.Context, *reconcile.DesiredContainer, docker.ExecOptions) (int, error) {
	return 0, nil
}

func (e *execEngine) StopContainer(_ context.Context, id string, _ int) error {
	e.stopped = append(e.stopped, id)
	return nil
}

func (e *execEngine) StartContainer(_ context.Context, id string) error {
	e.started = append(e.started, id)
	return nil
}

func TestTasksAndExec(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"shop/.git/HEAD": "x",
		"shop/artisan":   "<?php",
		"shop/.sld/config.yaml": `version: 2
apps:
  web:  { runtime: php, version: "8.3" }
  vite: { runtime: node, command: "npm run dev", port: 5173, path: frontend }
tasks:
  fresh: { command: "php artisan migrate:fresh --seed", description: Rebuild the database }
`,
		"shop/frontend/package.json": `{"scripts":{"dev":"vite","build":"vite build","lint":"eslint .","prebuild":"x"}}`,
	})
	env := hostenv.Host(t.TempDir())
	id, err := env.ProjectIdentity(filepath.Join(ws, "shop"))
	if err != nil {
		t.Fatal(err)
	}
	labels := func(app string) map[string]string {
		return map[string]string{"com.supreme.sld.project": id.ID, "com.supreme.sld.app": app}
	}
	engine := &execEngine{fakeEngine: fakeEngine{containers: []*reconcile.ActualContainer{
		{ID: "c-php", Name: "sld-" + id.ID + "-web-php", Service: "php", State: "running", Labels: labels("web")},
		{ID: "c-nginx", Name: "sld-" + id.ID + "-web-nginx", Service: "nginx", State: "running", Labels: labels("web")},
		{ID: "c-vite", Name: "sld-" + id.ID + "-vite", Service: "node", State: "running", Labels: labels("vite")},
	}}}

	a, token := newAuth(t)
	mux := http.NewServeMux()
	// The legacy routes share the mux in `sld serve`; the method-specific
	// Artisan route must not conflict with them.
	mux.HandleFunc("/api/artisan/run", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	srv := New(Options{Engine: engine, Env: env, Auth: a, Workspaces: []string{ws}, Version: "test"})
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

	// Tasks: configured first, then suggestions (artisan, package.json
	// scripts except dev servers and pre/post hooks).
	rec := call("GET", "/api/v2/projects/"+id.ID+"/tasks", "")
	if rec.Code != 200 {
		t.Fatalf("tasks: %d %s", rec.Code, rec.Body)
	}
	var resp struct {
		Tasks []TaskView `json:"tasks"`
		Apps  []string   `json:"apps"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	byName := map[string]TaskView{}
	for _, tv := range resp.Tasks {
		byName[tv.App+"/"+tv.Name] = tv
	}
	if len(resp.Tasks) == 0 || resp.Tasks[0].Name != "fresh" || resp.Tasks[0].Source != "config" {
		t.Errorf("first task should be the configured one: %+v", resp.Tasks)
	}
	if byName["web/migrate"].Command != "php artisan migrate" {
		t.Errorf("missing artisan suggestion: %+v", resp.Tasks)
	}
	if byName["vite/build"].Command != "npm run build" || byName["vite/lint"].Command == "" {
		t.Errorf("missing package.json scripts: %+v", resp.Tasks)
	}
	if _, ok := byName["vite/dev"]; ok {
		t.Error("the dev server script is not a task")
	}
	if _, ok := byName["vite/prebuild"]; ok {
		t.Error("pre/post hooks are not tasks")
	}
	if strings.Join(resp.Apps, ",") != "web,vite" {
		t.Errorf("apps = %v", resp.Apps)
	}

	runJob := func(body string) JobView {
		t.Helper()
		rec := call("POST", "/api/v2/projects/"+id.ID+"/exec", body)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("exec %s: %d %s", body, rec.Code, rec.Body)
		}
		var v JobView
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		j, _ := srv.jobs.Get(v.ID)
		waitDone(t, j)
		v, _ = j.Snapshot(0)
		return v
	}

	// A task runs in the PHP container (not its nginx sidecar), as the
	// project owner.
	v := runJob(`{"task":"migrate","app":"web"}`)
	if v.Status != JobSucceeded || v.ProjectID != id.ID || !strings.Contains(v.Output, "php artisan migrate") {
		t.Errorf("task job = %+v", v)
	}
	last := engine.execs[len(engine.execs)-1]
	if !strings.HasPrefix(last, "sld-"+id.ID+"-web-php|") || !strings.HasSuffix(last, "|sh -c php artisan migrate") || strings.Split(last, "|")[1] == "" {
		t.Errorf("exec = %q", last)
	}

	// A free-form command in another app.
	v = runJob(`{"command":"node -v","app":"vite"}`)
	if v.Status != JobSucceeded || !strings.HasPrefix(engine.execs[len(engine.execs)-1], "sld-"+id.ID+"-vite|") {
		t.Errorf("command job = %+v, execs=%v", v, engine.execs)
	}

	// A failing command fails the job with its exit code.
	engine.exit = 3
	if v := runJob(`{"command":"false"}`); v.Status != JobFailed || !strings.Contains(v.Error, "code 3") {
		t.Errorf("failing job = %+v", v)
	}

	if rec := call("POST", "/api/v2/projects/"+id.ID+"/exec", `{"task":"nope"}`); rec.Code != 404 {
		t.Errorf("unknown task: %d", rec.Code)
	}
	if rec := call("POST", "/api/v2/projects/"+id.ID+"/exec", `{"command":"  "}`); rec.Code != 400 {
		t.Errorf("empty command: %d", rec.Code)
	}

	// Container actions only touch this project's containers.
	if rec := call("POST", "/api/v2/projects/"+id.ID+"/containers/sld-"+id.ID+"-vite/restart", ""); rec.Code != 200 {
		t.Fatalf("restart: %d %s", rec.Code, rec.Body)
	}
	if strings.Join(engine.stopped, ",") != "c-vite" || strings.Join(engine.started, ",") != "c-vite" {
		t.Errorf("restart stopped=%v started=%v", engine.stopped, engine.started)
	}
	if rec := call("POST", "/api/v2/projects/"+id.ID+"/containers/someone-else/stop", ""); rec.Code != 404 {
		t.Errorf("foreign container: %d", rec.Code)
	}

	// The Artisan console runs in the Laravel app's PHP container.
	engine.exit = 0
	rec = call("POST", "/api/artisan/run", `{"project_path":"`+filepath.ToSlash(filepath.Join(ws, "shop"))+`","command":"php artisan route:list"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("artisan: %d %s", rec.Code, rec.Body)
	}
	var started struct {
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &started)
	j, _ := srv.jobs.Get(started.JobID)
	waitDone(t, j)
	if last := engine.execs[len(engine.execs)-1]; !strings.HasPrefix(last, "sld-"+id.ID+"-web-php|") || !strings.HasSuffix(last, "sh -c php artisan route:list") {
		t.Errorf("artisan exec = %q", last)
	}
	if rec := call("POST", "/api/artisan/run", `{"project_path":"/elsewhere","command":"list"}`); rec.Code != 404 {
		t.Errorf("unknown project path: %d", rec.Code)
	}
}
