package orchestrate

import (
	"context"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
	appruntime "github.com/supreme-majesty/supreme-local-dev/internal/runtime"
)

func TestBuildComposeOverride(t *testing.T) {
	spec := &ProjectSpec{Identity: &project.Identity{ID: "tailorly-1234abcd", CanonicalPath: "/p/tailorly"}}
	stack := &appruntime.ComposeStack{
		Dir:   "/p/tailorly",
		Files: []string{"docker-compose.yml"},
		Routes: []appruntime.ComposeRoute{
			{App: "web", Service: "web", Port: 5173, RouterName: "tailorly-1234abcd", Hosts: []string{"tailorly.localhost"}, TLS: true},
			{App: "api", Service: "api", Port: 8000, RouterName: "tailorly-1234abcd-api", Hosts: []string{"api.tailorly.localhost"}},
		},
	}
	resolved := &composeConfig{Services: map[string]struct {
		Networks map[string]any `json:"networks"`
	}{
		"web":   {Networks: map[string]any{"default": nil}},
		"api":   {Networks: map[string]any{"backend": nil, "default": nil}},
		"mysql": {Networks: map[string]any{"backend": nil}},
	}}

	out, err := buildComposeOverride(spec, stack, resolved)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)

	// Host ports are reset on every service so stacks don't collide.
	if strings.Count(text, "ports: !reset []") != 3 {
		t.Errorf("expected 3 port resets:\n%s", text)
	}
	var doc struct {
		Services map[string]struct {
			Labels   map[string]string         `yaml:"labels"`
			Networks map[string]map[string]any `yaml:"networks"`
		} `yaml:"services"`
		Networks map[string]struct {
			Name     string `yaml:"name"`
			External bool   `yaml:"external"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("override is not valid YAML: %v\n%s", err, text)
	}

	web := doc.Services["web"]
	if web.Labels["traefik.http.routers.tailorly-1234abcd.rule"] != "Host(`tailorly.localhost`)" ||
		web.Labels["traefik.http.services.tailorly-1234abcd.loadbalancer.server.port"] != "5173" ||
		web.Labels["traefik.http.routers.tailorly-1234abcd-secure.tls"] != "true" {
		t.Errorf("web traefik labels: %v", web.Labels)
	}
	if _, ok := web.Networks["default"]; !ok || web.Networks["sld-global"] == nil && len(web.Networks) != 2 {
		t.Errorf("web must keep its networks and join sld-global: %v", web.Networks)
	}
	api := doc.Services["api"]
	if _, ok := api.Networks["backend"]; !ok {
		t.Errorf("api lost its backend network: %v", api.Networks)
	}
	if _, ok := api.Networks["sld-global"]; !ok {
		t.Errorf("api not on sld-global: %v", api.Networks)
	}
	mysql := doc.Services["mysql"]
	if mysql.Labels["com.supreme.sld.project"] != "tailorly-1234abcd" || mysql.Labels["traefik.enable"] != "" || mysql.Networks != nil {
		t.Errorf("mysql should be labeled but not routed: %+v", mysql)
	}
	if n := doc.Networks["sld-global"]; !n.External || n.Name != "sld-global" {
		t.Errorf("sld-global must be declared external: %+v", n)
	}

	// keep_ports leaves published ports alone.
	stack.KeepPorts = true
	out, _ = buildComposeOverride(spec, stack, resolved)
	if strings.Contains(string(out), "!reset") {
		t.Error("keep_ports should not reset ports")
	}
}

func TestMergeComposeStacks(t *testing.T) {
	a := &appruntime.ComposeStack{Dir: "/p", Files: []string{"compose.yaml"}, Routes: []appruntime.ComposeRoute{{App: "web"}}}
	b := &appruntime.ComposeStack{Dir: "/p", Files: []string{"compose.yaml"}, Routes: []appruntime.ComposeRoute{{App: "api"}}, KeepPorts: true}
	c := &appruntime.ComposeStack{Dir: "/p/other", Files: []string{"compose.yaml"}, Routes: []appruntime.ComposeRoute{{App: "x"}}}
	merged := mergeComposeStacks([]*appruntime.ComposeStack{a, b, c})
	if len(merged) != 2 || len(merged[0].Routes) != 2 || !merged[0].KeepPorts {
		t.Errorf("merge result: %+v", merged)
	}
}

// TestStartProject_LeavesComposeContainersAlone guards against SLD removing
// containers owned by a project's compose stack as "obsolete".
func TestStartProject_LeavesComposeContainersAlone(t *testing.T) {
	engine := newMockEngine()
	orch, projectRoot := containerOrchestrator(t, engine)
	id, err := orch.Env().ProjectIdentity(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	engine.containers["swiftsms-app-1"] = &reconcile.ActualContainer{
		ID: "compose-app", Name: "swiftsms-app-1", State: "running",
		Labels: map[string]string{"com.supreme.sld.project": id.ID, "com.supreme.sld.runtime": "compose"},
	}
	engine.containers["sld-old-leftover"] = &reconcile.ActualContainer{
		ID: "leftover", Name: "sld-old-leftover", State: "exited",
		Labels: map[string]string{"com.supreme.sld.project": id.ID},
	}
	spec := &ProjectSpec{
		Identity:    id,
		Config:      &config.Config{Version: 2, Name: "shop", Apps: config.Apps{{Name: "web", Runtime: config.RuntimeStatic, WebRoot: "."}}, Domain: config.DomainConfig{Name: "shop.localhost"}},
		ProjectRoot: projectRoot,
	}
	if err := orch.StartProject(context.Background(), spec); err != nil {
		t.Fatalf("StartProject: %v", err)
	}
	for _, removed := range engine.removedIDs {
		if removed == "compose-app" {
			t.Fatal("StartProject removed a compose-managed container")
		}
	}
	if !slices.Contains(engine.removedIDs, "leftover") {
		t.Error("obsolete SLD containers should still be removed")
	}
}
