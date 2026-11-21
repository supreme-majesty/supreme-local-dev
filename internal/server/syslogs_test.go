package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

func TestLogSources(t *testing.T) {
	engine := &fakeEngine{containers: []*reconcile.ActualContainer{
		{ID: "p1", Name: "shop-app-1", State: "running", Labels: map[string]string{docker.LabelProject: "shop-1234", docker.LabelProjectPath: "/ws/shop", composeServiceLabel: "app"}},
		{ID: "r1", Name: orchestrate.RouterContainer, State: "running", Labels: map[string]string{}},
		{ID: "s1", Name: "sld-share-shop", State: "running", Labels: map[string]string{shareLabel: "shop"}},
		{ID: "x1", Name: "unrelated", State: "running", Labels: map[string]string{}},
	}}
	srv := New(Options{Engine: engine, Env: hostenv.Host(t.TempDir()), Workspaces: []string{t.TempDir()}})
	list, err := srv.logSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []LogSource{
		{Container: orchestrate.RouterContainer, Label: "Router (Traefik)", Group: "SLD", State: "running"},
		{Container: "shop-app-1", Label: "app", Group: "shop", State: "running"},
		{Container: "sld-share-shop", Label: "shop tunnel", Group: "Shares", State: "running"},
	}
	if len(list) != len(want) {
		t.Fatalf("sources = %+v", list)
	}
	for i := range want {
		if list[i] != want[i] {
			t.Errorf("source %d = %+v, want %+v", i, list[i], want[i])
		}
	}

	// Containers outside the list cannot be read.
	rec := httptest.NewRecorder()
	srv.handleLogStream(rec, httptest.NewRequest("GET", "/api/v2/logs/stream?container=unrelated", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unlisted container: %d", rec.Code)
	}
}
