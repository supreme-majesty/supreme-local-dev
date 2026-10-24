package orchestrate

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// mockEngine is a minimal test double for the Docker engine.
type mockEngine struct {
	pinged         bool
	networks       map[string]string
	volumes        map[string]string
	containers     map[string]*reconcile.ActualContainer
	createdNames   []string
	startedIDs     []string
	stoppedIDs     []string
	removedIDs     []string
	connectedNets  []string
	failCreate     bool
}

func newMockEngine() *mockEngine {
	return &mockEngine{
		networks:   make(map[string]string),
		volumes:    make(map[string]string),
		containers: make(map[string]*reconcile.ActualContainer),
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

func (m *mockEngine) EnsureVolume(ctx context.Context, name string, labels map[string]string) (string, error) {
	m.volumes[name] = name
	return name, nil
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
	m.createdNames = append(m.createdNames, desired.Name)
	m.containers[desired.Name] = &reconcile.ActualContainer{
		ID:    id,
		Name:  desired.Name,
		Image: desired.Image,
		State: "created",
	}
	return id, nil
}

func (m *mockEngine) StartContainer(ctx context.Context, containerID string) error {
	m.startedIDs = append(m.startedIDs, containerID)
	return nil
}

func (m *mockEngine) StopContainer(ctx context.Context, containerID string, timeoutSeconds int) error {
	m.stoppedIDs = append(m.stoppedIDs, containerID)
	return nil
}

func (m *mockEngine) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	m.removedIDs = append(m.removedIDs, containerID)
	return nil
}

func (m *mockEngine) ConnectNetwork(ctx context.Context, networkID, containerID string) error {
	m.connectedNets = append(m.connectedNets, networkID)
	return nil
}

func (m *mockEngine) InspectContainer(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return types.ContainerJSON{}, nil
}

func (m *mockEngine) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
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
		Version: 1,
		Name:    "my-app",
		Runtime: config.RuntimeConfig{
			PHP:     "8.3",
			WebRoot: "public",
		},
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
		Version: 1,
		Name:    "laravel-app",
		Runtime: config.RuntimeConfig{
			PHP:     "8.4",
			WebRoot: "public",
		},
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
		dbType     string
		wantPort   string
		wantConn   string
		wantDir    string
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
