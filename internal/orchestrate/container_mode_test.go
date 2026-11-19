package orchestrate

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// TestMain keeps tests that use the default host environment from writing
// certificates into the developer's real ~/.sld directory.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sld-orchestrate-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("SLD_STATE_DIR", dir)
	os.Setenv("SLD_BIND_HOST", "")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// containerOrchestrator simulates SLD running in a container with a project
// workspace bind-mounted from the host and its data dir on a named volume.
func containerOrchestrator(t *testing.T, engine *mockEngine) (*Orchestrator, string) {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(filepath.Join(workspace, "shop"), 0o755); err != nil {
		t.Fatal(err)
	}
	orch := NewOrchestrator(engine)
	orch.SetEnv(hostenv.Container("sld-self", []hostenv.Mount{
		{Type: "bind", Source: "/home/dev/Projects", Destination: workspace},
		{Type: "volume", Name: "sld-data", Destination: data},
	}, data))
	return orch, filepath.Join(workspace, "shop")
}

func findMount(t *testing.T, d *reconcile.DesiredContainer, target string) reconcile.Mount {
	t.Helper()
	for _, m := range d.Mounts {
		if m.Target == target {
			return m
		}
	}
	t.Fatalf("%s has no mount at %s (mounts: %+v)", d.Name, target, d.Mounts)
	return reconcile.Mount{}
}

func TestEnsureGlobalInfra_ContainerMode(t *testing.T) {
	engine := newMockEngine()
	orch, _ := containerOrchestrator(t, engine)

	if err := orch.EnsureGlobalInfra(context.Background()); err != nil {
		t.Fatalf("EnsureGlobalInfra: %v", err)
	}
	if !slices.Contains(engine.connectedNets, GlobalNetwork) {
		t.Errorf("SLD container was not attached to %s (connected: %v)", GlobalNetwork, engine.connectedNets)
	}

	router := engine.createdSpecs[0]
	if certs := findMount(t, router, "/etc/traefik/certs"); certs.Type != reconcile.MountTypeVolume || certs.Source != "sld-data" || certs.Subpath != "certs" || !certs.ReadOnly {
		t.Errorf("cert mount = %+v, want read-only sld-data volume subpath certs", certs)
	}
	if dyn := findMount(t, router, "/etc/traefik/dynamic"); dyn.Type != reconcile.MountTypeVolume || dyn.Subpath != "traefik" {
		t.Errorf("dynamic config mount = %+v, want sld-data volume subpath traefik", dyn)
	}
	if want := []string{"127.0.0.1:80:80", "127.0.0.1:443:443"}; !slices.Equal(router.Ports, want) {
		t.Errorf("router ports = %v, want loopback-only %v", router.Ports, want)
	}
	if router.ConfigHash == "" || router.Labels["com.supreme.sld.config-hash"] != router.ConfigHash {
		t.Errorf("router must carry a spec hash label so changes recreate it: %q / %q", router.ConfigHash, router.Labels["com.supreme.sld.config-hash"])
	}
	// The CA must exist so projects can get certificates; its key must not be mounted.
	if _, err := os.Stat(filepath.Join(orch.Env().DataDir(), "ca", "ca.crt")); err != nil {
		t.Errorf("CA not created: %v", err)
	}
}

func TestEnsureGlobalInfra_HostModeDoesNotJoinNetwork(t *testing.T) {
	engine := newMockEngine()
	orch := NewOrchestrator(engine)
	if err := orch.EnsureGlobalInfra(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(engine.connectedNets) != 0 {
		t.Errorf("host-mode SLD must not try to join networks, got %v", engine.connectedNets)
	}
	if got := engine.createdSpecs[0].Ports; !slices.Equal(got, []string{"127.0.0.1:80:80", "127.0.0.1:443:443"}) {
		t.Errorf("router ports = %v", got)
	}
}

func TestBuildDesiredContainers_ContainerModeTranslatesProjectMounts(t *testing.T) {
	engine := newMockEngine()
	orch, projectRoot := containerOrchestrator(t, engine)

	id, err := orch.Env().ProjectIdentity(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	spec := &ProjectSpec{
		Identity: id,
		Config: &config.Config{
			Version: 2,
			Name:    "shop",
			Apps:    withVite(phpApps("8.3"), "", "npm run dev"),
			Domain:  config.DomainConfig{Name: "shop.localhost"},
			Services: config.ServicesConfig{
				Database: config.DatabaseConfig{Type: "mysql", Version: "8.0", ExposePort: 3307},
			},
		},
		ConfigHash:  "h",
		ProjectRoot: projectRoot,
	}

	containers, err := orch.buildDesiredContainers(spec)
	if err != nil {
		t.Fatalf("buildDesiredContainers: %v", err)
	}

	byService := map[string]*reconcile.DesiredContainer{}
	for _, c := range containers {
		byService[c.Service] = c
	}

	for _, svc := range []string{"php", "nginx", "node"} {
		m := findMount(t, byService[svc], "/app")
		if m.Type != reconcile.MountTypeBind || m.Source != "/home/dev/Projects/shop" {
			t.Errorf("%s /app mount = %+v, want host path /home/dev/Projects/shop", svc, m)
		}
	}
	if !findMount(t, byService["nginx"], "/app").ReadOnly {
		t.Error("nginx must mount the project read-only")
	}
	if got := byService["db"].Ports; !slices.Equal(got, []string{"127.0.0.1:3307:3306"}) {
		t.Errorf("db ports = %v, want loopback binding", got)
	}
	if got := byService["node"].Ports; !slices.Equal(got, []string{"127.0.0.1:5173:5173"}) {
		t.Errorf("node ports = %v, want loopback binding", got)
	}

	hostID, _ := project.ComputeIdentityFromHostPath("/home/dev/Projects/shop")
	if id.ID != hostID.ID {
		t.Errorf("project identity %s should match host identity %s", id.ID, hostID.ID)
	}
}

func TestBuildDesiredContainers_RejectsProjectOutsideMounts(t *testing.T) {
	engine := newMockEngine()
	orch, _ := containerOrchestrator(t, engine)
	outside := t.TempDir()

	spec := &ProjectSpec{
		Identity:    &project.Identity{ID: "x-12345678", Slug: "x"},
		Config:      &config.Config{Version: 2, Name: "x", Apps: phpApps("8.3"), Domain: config.DomainConfig{Name: "x.localhost"}},
		ProjectRoot: outside,
	}
	if _, err := orch.buildDesiredContainers(spec); err == nil {
		t.Fatal("expected an error for a project that is not inside a mounted workspace")
	}
}

func TestStartAndDestroyProject_ManageProjectCertificate(t *testing.T) {
	engine := newMockEngine()
	orch, projectRoot := containerOrchestrator(t, engine)
	if err := orch.EnsureGlobalInfra(context.Background()); err != nil {
		t.Fatal(err)
	}
	id, err := orch.Env().ProjectIdentity(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	spec := &ProjectSpec{
		Identity: id,
		Config: &config.Config{
			Version:  1,
			Name:     "shop",
			Apps:     phpApps("8.3"),
			Domain:   config.DomainConfig{Name: "shop.localhost", TLS: true},
			Services: config.ServicesConfig{Mailpit: config.MailpitConfig{Enabled: true}},
		},
		ConfigHash:  "h",
		ProjectRoot: projectRoot,
	}
	if err := orch.StartProject(context.Background(), spec); err != nil {
		t.Fatalf("StartProject: %v", err)
	}

	certMgr, err := orch.CertManager()
	if err != nil {
		t.Fatal(err)
	}
	hosts, err := certMgr.CertHosts(id.ID)
	if err != nil {
		t.Fatalf("project certificate not issued: %v", err)
	}
	if want := []string{"mail.shop.localhost", "shop.localhost"}; !slices.Equal(hosts, want) {
		t.Errorf("certificate hosts = %v, want %v", hosts, want)
	}

	if err := orch.DestroyProject(context.Background(), id, false); err != nil {
		t.Fatalf("DestroyProject: %v", err)
	}
	if _, err := os.Stat(certMgr.CertPath(id.ID)); !os.IsNotExist(err) {
		t.Errorf("project certificate should be removed on destroy, stat err = %v", err)
	}
}

func TestProjectOwner_ContainerModeUsesWorkspaceOwner(t *testing.T) {
	orch, projectRoot := containerOrchestrator(t, newMockEngine())
	uid, gid := orch.ProjectOwner(projectRoot)
	if uid == "" || gid == "" {
		t.Fatalf("empty owner %q:%q", uid, gid)
	}
	if uid == "0" {
		t.Error("root-owned workspaces must fall back to a non-root UID")
	}
	if wantUID, _, ok := hostenv.OwnerOf(projectRoot); ok && wantUID != "0" && uid != wantUID {
		t.Errorf("uid = %s, want workspace owner %s", uid, wantUID)
	}
}

// phpApps is the app list of a typical PHP project (what a version 1 config upgrades to).
func phpApps(version string) config.Apps {
	return config.Apps{{Name: "web", Runtime: config.RuntimePHP, Version: version, WebRoot: "public"}}
}

// withVite adds a Node dev server app like a version 1 `node.dev_server`.
func withVite(apps config.Apps, nodeVersion, command string) config.Apps {
	return append(apps, config.App{
		Name: "vite", Runtime: config.RuntimeNode, Version: nodeVersion,
		Command: command, Port: 5173, ExposePort: 5173,
	})
}

// TestMigratedConfigProducesIdenticalContainers guards the upgrade path: a
// version 1 file and its `config migrate` output must yield the same container
// specs, so migrating never renames containers or changes images/volumes.
func TestMigratedConfigProducesIdenticalContainers(t *testing.T) {
	v1 := []byte(`version: 1
name: shop
runtime:
  php: "8.3"
  web_root: public
domain:
  name: shop.test
  tls: true
services:
  database: {type: postgres}
  redis: {enabled: true}
  mailpit: {enabled: true}
node:
  version: "20"
  dev_server: {enabled: true, command: "npm run dev -- --host"}
xdebug: {enabled: true}
environment:
  APP_ENV: local
`)
	v2, err := config.MigrateV1(v1)
	if err != nil {
		t.Fatalf("MigrateV1: %v", err)
	}

	build := func(data []byte) []*reconcile.DesiredContainer {
		cfg, err := config.LoadFromBytes(data, "shop")
		if err != nil {
			t.Fatalf("LoadFromBytes: %v\n%s", err, data)
		}
		root := t.TempDir()
		orch := NewOrchestrator(newMockEngine())
		orch.SetEnv(hostenv.Host(t.TempDir()))
		containers, err := orch.buildDesiredContainers(&ProjectSpec{
			Identity:    &project.Identity{ID: "shop-1234abcd", Slug: "shop", Hash: "1234abcd", CanonicalPath: "/p/shop"},
			Config:      cfg,
			ConfigHash:  "same", // the file hash differs by design; compare everything else
			ProjectRoot: root,
		})
		if err != nil {
			t.Fatalf("buildDesiredContainers: %v", err)
		}
		// Project root (temp dir) and generated DB password differ per call.
		for _, c := range containers {
			for i := range c.Mounts {
				c.Mounts[i].Source = "<root>"
			}
			for k := range c.Environment {
				if strings.Contains(k, "PASSWORD") {
					c.Environment[k] = "<secret>"
				}
			}
		}
		return containers
	}

	before, after := build(v1), build(v2)
	if len(before) != len(after) {
		t.Fatalf("container count changed: %d -> %d", len(before), len(after))
	}
	for i := range before {
		if !reflect.DeepEqual(before[i], after[i]) {
			t.Errorf("container %s differs after migration:\nv1: %+v\nv2: %+v", before[i].Name, before[i], after[i])
		}
	}
	names := make([]string, len(before))
	for i, c := range before {
		names[i] = c.Service
	}
	if got := strings.Join(names, ","); got != "db,redis,mailpit,php,nginx,node" {
		t.Errorf("service order = %s", got)
	}
}
