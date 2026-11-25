package orchestrate

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

func TestServed(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{200, "<html>", true},
		{302, "", true}, // e.g. a redirect to /login
		{500, "Whoops", true},
		{404, "<h1>Not Found</h1>", true},      // the app's own 404 page
		{404, "404 page not found\n", false},   // Traefik: no route yet
		{502, "<center>nginx</center>", false}, // nginx before PHP-FPM is up
		{503, "", false},
		{504, "", false},
	}
	for _, tc := range cases {
		if got := served(tc.status, tc.body); got != tc.want {
			t.Errorf("served(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
		}
	}
}

// A stopped container whose network was removed (docker network prune) cannot
// be started; SLD recreates it from the same spec instead of failing.
func TestApplyPlan_RecreatesContainerWithMissingNetwork(t *testing.T) {
	engine := newMockEngine()
	engine.startErrs = map[string]error{
		"old": errors.New("Error response from daemon: failed to set up container networking: network 9f414a4c8c22 not found"),
	}
	orch := NewOrchestrator(engine)
	plan := reconcile.DesiredResourcePlan{
		Action:  reconcile.ActionStart,
		Actual:  &reconcile.ActualContainer{ID: "old", Name: "sld-rails-web", State: "exited"},
		Desired: &reconcile.DesiredContainer{Name: "sld-rails-web", Image: "ruby:3.4"},
	}
	id, err := orch.applyPlan(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if id != "cid-sld-rails-web" || !slices.Equal(engine.removedIDs, []string{"old"}) || !slices.Contains(engine.startedIDs, id) {
		t.Errorf("want old removed and a new container started: id=%s removed=%v started=%v", id, engine.removedIDs, engine.startedIDs)
	}

	// Any other start error is returned as is.
	engine = newMockEngine()
	engine.startErrs = map[string]error{"old": errors.New("port is already allocated")}
	if _, err := NewOrchestrator(engine).applyPlan(context.Background(), plan); err == nil || len(engine.removedIDs) != 0 {
		t.Errorf("other errors must not recreate: err=%v removed=%v", err, engine.removedIDs)
	}
}

func TestIsMissingNetwork(t *testing.T) {
	for msg, want := range map[string]bool{
		"failed to set up container networking: network 9f414a4c8c22 not found": true,
		"Error response from daemon: network sld-shop-net not found":            true,
		"port is already allocated":                                             false,
		"No such image: ruby:3.4":                                               false,
	} {
		if got := isMissingNetwork(errors.New(msg)); got != want {
			t.Errorf("isMissingNetwork(%q) = %v, want %v", msg, got, want)
		}
	}
}
