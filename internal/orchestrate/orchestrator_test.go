package orchestrate

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// mockEngine is a minimal test double for the Docker engine.
type mockEngine struct {
	pinged          bool
	networks        map[string]string
	removedNetworks []string
	volumes         map[string]string
	volumeLabels    map[string]map[string]string
	removedVolumes  []string
	containers      map[string]*reconcile.ActualContainer
	createdSpecs    []*reconcile.DesiredContainer
	createdNames    []string
	startedIDs      []string
	stoppedIDs      []string
	removedIDs      []string
	connectedNets   []string
	inspectFn       func(containerID string) types.ContainerJSON
	logsText        string
	failCreate      bool
}

func newMockEngine() *mockEngine {
	return &mockEngine{
		networks:     make(map[string]string),
		volumes:      make(map[string]string),
		volumeLabels: make(map[string]map[string]string),
		containers:   make(map[string]*reconcile.ActualContainer),
	}
}

func (m *mockEngine) Ping(ctx context.Context) error {
	m.pinged = true
	return nil
}

func (m *mockEngine) EnsureNetwork(ctx context.Context, name string, labels map[string]string) (string, error) {
	id := "net-" + name
	m.networks[name] = id
	return id, nil
}

func (m *mockEngine) RemoveNetwork(ctx context.Context, nameOrID string) error {
	m.removedNetworks = append(m.removedNetworks, nameOrID)
	delete(m.networks, nameOrID)
	return nil
}

func (m *mockEngine) EnsureVolume(ctx context.Context, name string, labels map[string]string) (string, error) {
	m.volumes[name] = name
	m.volumeLabels[name] = labels
	return name, nil
}

func (m *mockEngine) RemoveVolume(ctx context.Context, name string, force bool) error {
	m.removedVolumes = append(m.removedVolumes, name)
	delete(m.volumes, name)
	return nil
}

func (m *mockEngine) ListProjectVolumes(ctx context.Context, projectID string) ([]string, error) {
	var res []string
	for name, labels := range m.volumeLabels {
		if labels["com.supreme.sld.project"] == projectID {
			res = append(res, name)
		}
	}
	return res, nil
}

func (m *mockEngine) ListProjectContainers(ctx context.Context, projectID string) ([]*reconcile.ActualContainer, error) {
	var result []*reconcile.ActualContainer
	for _, c := range m.containers {
		if c.Labels["com.supreme.sld.project"] == projectID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockEngine) ListAllContainers(ctx context.Context) ([]*reconcile.ActualContainer, error) {
	var result []*reconcile.ActualContainer
	for _, c := range m.containers {
		result = append(result, c)
	}
	return result, nil
}

func (m *mockEngine) CreateContainer(ctx context.Context, desired *reconcile.DesiredContainer) (string, error) {
	id := "cid-" + desired.Name
	m.createdSpecs = append(m.createdSpecs, desired)
	m.createdNames = append(m.createdNames, desired.Name)
	m.containers[desired.Name] = &reconcile.ActualContainer{
		ID:      id,
		Name:    desired.Name,
		Service: desired.Service,
		Image:   desired.Image,
		State:   "created",
		Labels:  desired.Labels,
	}
	return id, nil
}

func (m *mockEngine) StartContainer(ctx context.Context, containerID string) error {
	m.startedIDs = append(m.startedIDs, containerID)
	for _, c := range m.containers {
		if c.ID == containerID {
			c.State = "running"
		}
	}
	return nil
}

func (m *mockEngine) StopContainer(ctx context.Context, containerID string, timeoutSeconds int) error {
	m.stoppedIDs = append(m.stoppedIDs, containerID)
	for _, c := range m.containers {
		if c.ID == containerID {
			c.State = "exited"
		}
	}
	return nil
}

func (m *mockEngine) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	m.removedIDs = append(m.removedIDs, containerID)
	for name, c := range m.containers {
		if c.ID == containerID {
			delete(m.containers, name)
		}
	}
	return nil
}

func (m *mockEngine) ConnectNetwork(ctx context.Context, networkID, containerID string) error {
	m.connectedNets = append(m.connectedNets, networkID)
	return nil
}

func (m *mockEngine) InspectContainer(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	if m.inspectFn != nil {
		return m.inspectFn(containerID), nil
	}
	return types.ContainerJSON{}, nil
}

func (m *mockEngine) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(m.logsText)), nil
}

func (m *mockEngine) PullImage(ctx context.Context, imageName string) error {
	return nil
}

func (m *mockEngine) Close() error { return nil }

func TestEnsureGlobalInfra(t *testing.T) {
	engine := newMockEngine()
	orch := NewOrchestrator(engine)

	err := orch.EnsureGlobalInfra(context.Background())
	if err != nil {
		t.Fatalf("EnsureGlobalInfra failed: %v", err)
	}

	// Global network should be created
	if _, ok := engine.networks[GlobalNetwork]; !ok {
		t.Error("expected sld-global network to be created")
	}

	// Router container should be created and started
	if len(engine.createdNames) != 1 || engine.createdNames[0] != RouterContainer {
		t.Errorf("expected router container to be created, got: %v", engine.createdNames)
	}
	if len(engine.startedIDs) != 1 {
		t.Errorf("expected 1 container started, got %d", len(engine.startedIDs))
	}
}

func TestBuildDesiredContainers_MinimalConfig(t *testing.T) {
	engine := newMockEngine()
	orch := NewOrchestrator(engine)

	identity := &project.Identity{
		ID:            "my-app-abcd1234",
		Slug:          "my-app",
		Hash:          "abcd1234",
		CanonicalPath: "/home/dev/my-app",
	}

	cfg := &config.Config{
		Version: 2,
		Name:    "my-app",
		Apps:    phpApps("8.3"),
		Domain: config.DomainConfig{
			Name: "my-app.test",
		},
	}

	spec := &ProjectSpec{
		Identity:    identity,
		Config:      cfg,
		ConfigHash:  "hash123",
		ProjectRoot: "/home/dev/my-app",
	}

	containers, err := orch.buildDesiredContainers(spec)
	if err != nil {
		t.Fatalf("buildDesiredContainers failed: %v", err)
	}

	// Should have exactly PHP + Nginx = 2 containers
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers (PHP + Nginx), got %d", len(containers))
	}

	// Verify PHP container
	php := containers[0]
	if php.Name != identity.PHPContainerName() {
		t.Errorf("expected PHP name %s, got %s", identity.PHPContainerName(), php.Name)
	}
	if php.Image != "ghcr.io/supreme-majesty/sld-php:8.3" {
		t.Errorf("unexpected PHP image: %s", php.Image)
	}

	// Verify Nginx container
	nginx := containers[1]
	if nginx.Name != identity.NginxContainerName() {
		t.Errorf("expected Nginx name %s, got %s", identity.NginxContainerName(), nginx.Name)
	}
}

func TestBuildDesiredContainers_FullStack(t *testing.T) {
	engine := newMockEngine()
	orch := NewOrchestrator(engine)

	identity := &project.Identity{
		ID:            "laravel-app-ef567890",
		Slug:          "laravel-app",
		Hash:          "ef567890",
		CanonicalPath: "/home/dev/laravel-app",
	}

	cfg := &config.Config{
		Version: 2,
		Name:    "laravel-app",
		Apps:    phpApps("8.4"),
		Domain: config.DomainConfig{
			Name: "laravel-app.test",
		},
		Services: config.ServicesConfig{
			Database: config.DatabaseConfig{
				Type:    "mysql",
				Version: "8.0",
			},
			Redis: config.RedisConfig{
				Enabled: true,
				Version: "7-alpine",
			},
			Mailpit: config.MailpitConfig{
				Enabled: true,
			},
		},
	}

	spec := &ProjectSpec{
		Identity:    identity,
		Config:      cfg,
		ConfigHash:  "fullhash",
		ProjectRoot: "/home/dev/laravel-app",
	}

	containers, err := orch.buildDesiredContainers(spec)
	if err != nil {
		t.Fatalf("buildDesiredContainers failed: %v", err)
	}

	// PHP + Nginx + DB + Redis + Mailpit = 5
	if len(containers) != 5 {
		t.Fatalf("expected 5 containers, got %d", len(containers))
	}

	serviceNames := make(map[string]bool)
	for _, c := range containers {
		serviceNames[c.Service] = true
	}

	expected := []string{"php", "nginx", "db", "redis", "mailpit"}
	for _, s := range expected {
		if !serviceNames[s] {
			t.Errorf("missing expected service: %s", s)
		}
	}
}

func TestResolveDBHelpers(t *testing.T) {
	tests := []struct {
		dbType   string
		wantPort string
		wantConn string
		wantDir  string
	}{
		{"mysql", "3306", "mysql", "/var/lib/mysql"},
		{"mariadb", "3306", "mysql", "/var/lib/mysql"},
		{"postgres", "5432", "pgsql", "/var/lib/postgresql/data"},
		{"postgresql", "5432", "pgsql", "/var/lib/postgresql/data"},
	}

	for _, tt := range tests {
		t.Run(tt.dbType, func(t *testing.T) {
			if got := resolveDBPort(tt.dbType); got != tt.wantPort {
				t.Errorf("resolveDBPort(%s) = %s, want %s", tt.dbType, got, tt.wantPort)
			}
			if got := resolveDBConnection(tt.dbType); got != tt.wantConn {
				t.Errorf("resolveDBConnection(%s) = %s, want %s", tt.dbType, got, tt.wantConn)
			}
			if got := resolveDBDataDir(tt.dbType); got != tt.wantDir {
				t.Errorf("resolveDBDataDir(%s) = %s, want %s", tt.dbType, got, tt.wantDir)
			}
		})
	}
}

func TestBuildDesiredContainers_DependencyOrderAndSpecs(t *testing.T) {
	engine := newMockEngine()
	orch := NewOrchestrator(engine)
	tmpDir := t.TempDir()

	identity := &project.Identity{
		ID:            "full-12345678",
		Slug:          "full",
		Hash:          "12345678",
		CanonicalPath: tmpDir,
	}
	cfg := &config.Config{
		Version: 2,
		Name:    "full",
		Apps:    withVite(phpApps("8.3"), "22", "npm run dev -- --host"),
		Domain:  config.DomainConfig{Name: "full.test", TLS: true},
		Services: config.ServicesConfig{
			Database: config.DatabaseConfig{Type: "postgres", Version: "16"},
			Redis:    config.RedisConfig{Enabled: true, Version: "7-alpine"},
			Mailpit:  config.MailpitConfig{Enabled: true},
		},
	}

	desired, err := orch.buildDesiredContainers(&ProjectSpec{
		Identity:    identity,
		Config:      cfg,
		ConfigHash:  "abc123",
		ProjectRoot: tmpDir,
	})
	if err != nil {
		t.Fatalf("buildDesiredContainers failed: %v", err)
	}

	wantOrder := []string{"db", "redis", "mailpit", "php", "nginx", "node"}
	if len(desired) != len(wantOrder) {
		t.Fatalf("got %d containers, want %d", len(desired), len(wantOrder))
	}
	for i, want := range wantOrder {
		if desired[i].Service != want {
			t.Errorf("container[%d].Service = %q, want %q", i, desired[i].Service, want)
		}
	}

	// Verify DB healthcheck
	if desired[0].Healthcheck == nil || len(desired[0].Healthcheck.Test) == 0 {
		t.Error("expected database container to define a healthcheck")
	}
	// Verify Redis healthcheck
	if desired[1].Healthcheck == nil || len(desired[1].Healthcheck.Test) == 0 {
		t.Error("expected redis container to define a healthcheck")
	}
	// Verify PHP ExtraHosts & WorkingDir
	php := desired[3]
	if php.WorkingDir != "/app" {
		t.Errorf("PHP WorkingDir = %q, want /app", php.WorkingDir)
	}
	if len(php.ExtraHosts) == 0 || php.ExtraHosts[0] != "host.docker.internal:host-gateway" {
		t.Errorf("PHP ExtraHosts = %v, want host.docker.internal:host-gateway", php.ExtraHosts)
	}
	// Verify Nginx traefik.docker.network label
	nginx := desired[4]
	if nginx.Labels["traefik.docker.network"] != GlobalNetwork {
		t.Errorf("Nginx traefik.docker.network = %q, want %q", nginx.Labels["traefik.docker.network"], GlobalNetwork)
	}
	// Verify Node managed dev server Cmd & WorkingDir
	node := desired[5]
	if node.WorkingDir != "/app" {
		t.Errorf("Node WorkingDir = %q, want /app", node.WorkingDir)
	}
	if len(node.Cmd) != 3 || node.Cmd[0] != "sh" || node.Cmd[1] != "-c" || node.Cmd[2] != "npm run dev -- --host" {
		t.Errorf("Node Cmd = %v, want [sh -c 'npm run dev -- --host']", node.Cmd)
	}
}

func TestWaitForHealthy_States(t *testing.T) {
	t.Run("Healthy immediately", func(t *testing.T) {
		engine := newMockEngine()
		engine.inspectFn = func(id string) types.ContainerJSON {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					State: &types.ContainerState{
						Status: "running",
						Health: &types.Health{Status: "healthy"},
					},
				},
			}
		}
		orch := NewOrchestrator(engine)
		if err := orch.waitForHealthy(context.Background(), "cid-1", "test-db"); err != nil {
			t.Fatalf("expected healthy container to pass, got %v", err)
		}
	})

	t.Run("Unhealthy surfaces stderr", func(t *testing.T) {
		engine := newMockEngine()
		engine.logsText = "FATAL: password authentication failed"
		engine.inspectFn = func(id string) types.ContainerJSON {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					State: &types.ContainerState{
						Status: "running",
						Health: &types.Health{Status: "unhealthy"},
					},
				},
			}
		}
		orch := NewOrchestrator(engine)
		orch.healthTimeout = 50 * time.Millisecond
		err := orch.waitForHealthy(context.Background(), "cid-1", "test-db")
		if err == nil || !strings.Contains(err.Error(), "password authentication failed") {
			t.Fatalf("expected unhealthy error with stderr, got: %v", err)
		}
	})

	t.Run("Exited unexpectedly surfaces error", func(t *testing.T) {
		engine := newMockEngine()
		engine.logsText = "syntax error in nginx.conf"
		engine.inspectFn = func(id string) types.ContainerJSON {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					State: &types.ContainerState{
						Status:   "exited",
						ExitCode: 1,
					},
				},
			}
		}
		orch := NewOrchestrator(engine)
		err := orch.waitForHealthy(context.Background(), "cid-1", "test-nginx")
		if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
			t.Fatalf("expected exit error, got: %v", err)
		}
	})
}

func TestDestroyProject(t *testing.T) {
	engine := newMockEngine()
	orch := NewOrchestrator(engine)
	tmpDir := t.TempDir()

	identity := &project.Identity{
		ID:            "demo-a1b2c3d4",
		Slug:          "demo",
		Hash:          "a1b2c3d4",
		CanonicalPath: tmpDir,
	}
	spec := &ProjectSpec{
		Identity: identity,
		Config: &config.Config{
			Version: 2,
			Name:    "demo",
			Apps:    phpApps("8.3"),
			Domain:  config.DomainConfig{Name: "demo.test"},
			Services: config.ServicesConfig{
				Database: config.DatabaseConfig{Type: "mysql", Version: "8.0"},
				Redis:    config.RedisConfig{Enabled: true},
			},
		},
		ConfigHash:  "hash-demo",
		ProjectRoot: tmpDir,
	}

	if err := orch.StartProject(context.Background(), spec); err != nil {
		t.Fatalf("StartProject failed: %v", err)
	}

	// Destroy without removing volumes
	if err := orch.DestroyProject(context.Background(), identity, false); err != nil {
		t.Fatalf("DestroyProject(false) failed: %v", err)
	}
	if len(engine.removedNetworks) != 1 || engine.removedNetworks[0] != identity.NetworkName() {
		t.Errorf("expected project network %s to be removed, got %v", identity.NetworkName(), engine.removedNetworks)
	}
	if len(engine.removedVolumes) != 0 {
		t.Errorf("expected volumes to be preserved when removeVolumes=false, got removed: %v", engine.removedVolumes)
	}

	// Re-start and destroy WITH volumes
	if err := orch.StartProject(context.Background(), spec); err != nil {
		t.Fatalf("second StartProject failed: %v", err)
	}
	if err := orch.DestroyProject(context.Background(), identity, true); err != nil {
		t.Fatalf("DestroyProject(true) failed: %v", err)
	}
	if len(engine.removedVolumes) < 2 {
		t.Errorf("expected DB and Redis volumes to be removed, got %v", engine.removedVolumes)
	}
}
