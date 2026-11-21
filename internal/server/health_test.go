package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

func TestDiagnose(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name       string
		h          ContainerHealth
		restarting bool
		loop       bool
		problem    string
	}{
		{"healthy", ContainerHealth{State: "running", StartedAt: now.Add(-time.Hour)}, false, false, ""},
		{"restarting", ContainerHealth{State: "restarting", ExitCode: 1}, true, true, "keeps restarting (crash loop), last exit code 1"},
		{"many restarts, just started", ContainerHealth{State: "running", RestartCount: 5, StartedAt: now.Add(-30 * time.Second)}, false, true, "keeps restarting (crash loop)"},
		{"old restarts, stable now", ContainerHealth{State: "running", RestartCount: 5, StartedAt: now.Add(-time.Hour)}, false, false, ""},
		{"oom", ContainerHealth{State: "exited", ExitCode: 137, OOMKilled: true}, false, false, "killed for running out of memory"},
		{"unhealthy", ContainerHealth{State: "running", Health: "unhealthy", StartedAt: now.Add(-time.Hour)}, false, false, "healthcheck failing"},
		{"crashed", ContainerHealth{State: "exited", ExitCode: 1}, false, false, "exited with code 1"},
		{"stopped normally", ContainerHealth{State: "exited", ExitCode: 143}, false, false, ""},
		{"killed by docker stop", ContainerHealth{State: "exited", ExitCode: 137}, false, false, ""},
		{"php-fpm stopped with SIGQUIT", ContainerHealth{State: "exited", ExitCode: 131}, false, false, ""},
		{"stale unhealthy after stop", ContainerHealth{State: "exited", Health: "unhealthy"}, false, false, ""},
	}
	for _, c := range cases {
		loop, problem := diagnose(c.h, c.restarting, now)
		if loop != c.loop || problem != c.problem {
			t.Errorf("%s: got (%v, %q), want (%v, %q)", c.name, loop, problem, c.loop, c.problem)
		}
	}
}

func TestFindLastError(t *testing.T) {
	logs := strings.Join([]string{
		"2026-11-21T10:00:00.000000000Z [info] server started",
		"2026-11-21T10:00:01.000000000Z PHP Fatal error:  Uncaught Error: Class \"Foo\" not found",
		"2026-11-21T10:00:02.000000000Z display_errors = Off",
		"2026-11-21T10:00:03.000000000Z SQLSTATE[HY000] [2002] Connection refused",
		"2026-11-21T10:00:04.000000000Z build finished with 0 errors",
		"2026-11-21T10:00:05.000000000Z GET /health 200",
	}, "\n")
	got := findLastError([]byte(logs))
	if got == nil || !strings.HasPrefix(got.Line, "SQLSTATE[HY000]") || got.Time.Second() != 3 {
		t.Fatalf("last error = %+v", got)
	}
	if findLastError([]byte("2026-11-21T10:00:00Z all good\n")) != nil {
		t.Error("no error line expected")
	}
}

func TestUsageFromStats(t *testing.T) {
	var s container.StatsResponse
	s.CPUStats.CPUUsage.TotalUsage = 3_000_000
	s.PreCPUStats.CPUUsage.TotalUsage = 1_000_000
	s.CPUStats.SystemUsage = 20_000_000
	s.PreCPUStats.SystemUsage = 10_000_000
	s.CPUStats.OnlineCPUs = 4
	s.MemoryStats.Usage = 300 << 20
	s.MemoryStats.Limit = 2 << 30
	s.MemoryStats.Stats = map[string]uint64{"inactive_file": 100 << 20}
	u := docker.UsageFromStats(&s)
	if u.CPUPercent < 79.9 || u.CPUPercent > 80.1 { // 2/10 of the system * 4 CPUs
		t.Errorf("cpu = %v, want 80", u.CPUPercent)
	}
	if u.MemBytes != 200<<20 || u.MemLimit != 2<<30 {
		t.Errorf("mem = %d / %d", u.MemBytes, u.MemLimit)
	}
}

// healthEngine reports fixed stats, states and logs.
type healthEngine struct {
	fakeEngine
	states map[string]*types.ContainerState
	logs   map[string]string
}

func (e *healthEngine) ContainerUsage(_ context.Context, id string) (docker.Usage, error) {
	return docker.Usage{CPUPercent: 12.5, MemBytes: 100 << 20}, nil
}

func (e *healthEngine) InspectContainer(_ context.Context, id string) (types.ContainerJSON, error) {
	st := e.states[id]
	if st == nil {
		st = &types.ContainerState{Status: "running", Running: true, StartedAt: time.Now().Add(-time.Hour).Format(time.RFC3339Nano)}
	}
	restarts := 0
	if st.Restarting {
		restarts = 7
	}
	return types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{State: st, RestartCount: restarts}}, nil
}

func (e *healthEngine) ContainerLogs(_ context.Context, id string, _ container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(e.logs[id])), nil
}

func TestHealthAndUsageAPI(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"shop/.git/HEAD":        "x",
		"shop/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: static}\n  api: {runtime: node, command: \"node server.js\", port: 3000}\n",
	})
	env := hostenv.Host(t.TempDir())
	id, err := env.ProjectIdentity(filepath.Join(ws, "shop"))
	if err != nil {
		t.Fatal(err)
	}
	lbl := func(app string) map[string]string {
		return map[string]string{"com.supreme.sld.project": id.ID, "com.supreme.sld.app": app}
	}
	engine := &healthEngine{
		fakeEngine: fakeEngine{containers: []*reconcile.ActualContainer{
			{ID: "c-web", Name: "sld-" + id.ID + "-web", Service: "static", State: "running", Labels: lbl("web")},
			{ID: "c-api", Name: "sld-" + id.ID + "-api", Service: "node", State: "restarting", Labels: lbl("api")},
			{ID: "c-router", Name: "sld-router", Service: "router", State: "running", Labels: map[string]string{}},
		}},
		states: map[string]*types.ContainerState{
			"c-api": {Status: "restarting", Restarting: true, ExitCode: 1, StartedAt: time.Now().Format(time.RFC3339Nano)},
		},
		logs: map[string]string{"c-api": "2026-11-21T10:00:00Z Error: Cannot find module 'express'\n"},
	}

	a, token := newAuth(t)
	mux := http.NewServeMux()
	srv := New(Options{Engine: engine, Env: env, Auth: a, Workspaces: []string{ws}, Version: "test"})
	srv.Register(mux)
	h := a.Middleware(mux)
	call := func(path string) *httptest.ResponseRecorder {
		return do(h, "GET", path, "localhost", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) })
	}

	rec := call("/api/v2/projects/" + id.ID + "/health")
	if rec.Code != 200 {
		t.Fatalf("health: %d %s", rec.Code, rec.Body)
	}
	var health struct {
		Containers []ContainerHealth `json:"containers"`
		Total      ProjectUsage      `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &health)
	if len(health.Containers) != 2 {
		t.Fatalf("containers: %+v", health.Containers)
	}
	api := health.Containers[0] // sorted by name: ...-api before ...-web
	if !api.CrashLoop || api.RestartCount != 7 || api.LastError == nil || !strings.Contains(api.LastError.Line, "Cannot find module") {
		t.Errorf("api container = %+v", api)
	}
	if web := health.Containers[1]; web.Problem != "" || web.Usage == nil || web.Usage.MemBytes != 100<<20 {
		t.Errorf("web container = %+v", web)
	}
	if health.Total.Problems != 1 || health.Total.Running != 1 {
		t.Errorf("total = %+v", health.Total)
	}

	rec = call("/api/v2/usage")
	if rec.Code != 200 {
		t.Fatalf("usage: %d %s", rec.Code, rec.Body)
	}
	var usage struct {
		Projects map[string]ProjectUsage `json:"projects"`
		Total    ProjectUsage            `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &usage)
	if p := usage.Projects[id.ID]; p.Running != 1 || p.Problems != 1 || p.MemBytes != 100<<20 {
		t.Errorf("project usage = %+v", p)
	}
	if sld := usage.Projects["sld"]; sld.Running != 1 || sld.CPUPercent != 12.5 {
		t.Errorf("SLD's own containers = %+v", sld)
	}
	if usage.Total.Running != 2 || usage.Total.MemBytes != 200<<20 {
		t.Errorf("total = %+v", usage.Total)
	}
}
