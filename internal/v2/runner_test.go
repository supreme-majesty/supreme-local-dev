package v2

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/spf13/cobra"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

type stubEngine struct {
	containers      map[string]*reconcile.ActualContainer
	removedNetworks []string
	removedVolumes  []string
}

func newStubEngine() *stubEngine {
	return &stubEngine{
		containers: make(map[string]*reconcile.ActualContainer),
	}
}

func (s *stubEngine) Ping(ctx context.Context) error { return nil }
func (s *stubEngine) EnsureNetwork(ctx context.Context, name string, labels map[string]string) (string, error) {
	return "net-" + name, nil
}
func (s *stubEngine) RemoveNetwork(ctx context.Context, nameOrID string) error {
	s.removedNetworks = append(s.removedNetworks, nameOrID)
	return nil
}
func (s *stubEngine) EnsureVolume(ctx context.Context, name string, labels map[string]string) (string, error) {
	return name, nil
}
func (s *stubEngine) RemoveVolume(ctx context.Context, name string, force bool) error {
	s.removedVolumes = append(s.removedVolumes, name)
	return nil
}
func (s *stubEngine) ListProjectVolumes(ctx context.Context, projectID string) ([]string, error) {
	return nil, nil
}
func (s *stubEngine) ListProjectContainers(ctx context.Context, projectID string) ([]*reconcile.ActualContainer, error) {
	var out []*reconcile.ActualContainer
	for _, c := range s.containers {
		if c.Labels["com.supreme.sld.project"] == projectID {
			out = append(out, c)
		}
	}
	return out, nil
}
func (s *stubEngine) ListAllContainers(ctx context.Context) ([]*reconcile.ActualContainer, error) {
	var out []*reconcile.ActualContainer
	for _, c := range s.containers {
		out = append(out, c)
	}
	return out, nil
}
func (s *stubEngine) CreateContainer(ctx context.Context, desired *reconcile.DesiredContainer) (string, error) {
	id := "cid-" + desired.Name
	s.containers[desired.Name] = &reconcile.ActualContainer{
		ID:      id,
		Name:    desired.Name,
		Service: desired.Service,
		Image:   desired.Image,
		State:   "created",
		Labels:  desired.Labels,
	}
	return id, nil
}
func (s *stubEngine) StartContainer(ctx context.Context, containerID string) error {
	for _, c := range s.containers {
		if c.ID == containerID {
			c.State = "running"
		}
	}
	return nil
}
func (s *stubEngine) StopContainer(ctx context.Context, containerID string, timeoutSeconds int) error {
	for _, c := range s.containers {
		if c.ID == containerID {
			c.State = "exited"
		}
	}
	return nil
}
func (s *stubEngine) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	for k, c := range s.containers {
		if c.ID == containerID {
			delete(s.containers, k)
		}
	}
	return nil
}
func (s *stubEngine) ConnectNetwork(ctx context.Context, networkID, containerID string) error {
	return nil
}
func (s *stubEngine) InspectContainer(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return types.ContainerJSON{}, nil
}
func (s *stubEngine) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (s *stubEngine) PullImage(ctx context.Context, imageName string) error { return nil }
func (s *stubEngine) Close() error                                          { return nil }

func TestLoadProjectConfig_Success(t *testing.T) {
	tempDir := t.TempDir()

	sldDir := filepath.Join(tempDir, ".sld")
	if err := os.MkdirAll(sldDir, 0755); err != nil {
		t.Fatalf("failed to create .sld dir: %v", err)
	}

	configContent := `version: 1
name: "my-test-app"
runtime:
  php: "8.3"
  web_root: "public"
domain:
  name: "my-test-app.test"
  tls: true
services:
  database:
    type: "mysql"
    version: "8.0"
  redis:
    enabled: true
  mailpit:
    enabled: true
`
	if err := os.WriteFile(filepath.Join(sldDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	cfg, data, err := loadProjectConfig(tempDir)
	if err != nil {
		t.Fatalf("loadProjectConfig failed: %v", err)
	}

	if cfg.Name != "my-test-app" {
		t.Errorf("expected name my-test-app, got %s", cfg.Name)
	}
	if web, ok := cfg.App("web"); !ok || web.Version != "8.3" || cfg.SourceVersion != 1 {
		t.Errorf("expected version 1 file upgraded to a php 8.3 web app, got %s (source v%d)", cfg.Describe(), cfg.SourceVersion)
	}
	if !cfg.Domain.TLS {
		t.Errorf("expected TLS true")
	}
	if len(data) == 0 {
		t.Errorf("expected raw data, got empty")
	}

	if err := ConfigValidate(tempDir); err != nil {
		t.Errorf("ConfigValidate failed: %v", err)
	}
	if err := ConfigShow(tempDir); err != nil {
		t.Errorf("ConfigShow failed: %v", err)
	}
}

func TestLoadProjectConfig_NotFound(t *testing.T) {
	tempDir := t.TempDir()

	_, _, err := loadProjectConfig(tempDir)
	if err == nil {
		t.Errorf("expected error for missing config, got nil")
	}
}

func TestRunner_DestroyAndStatus(t *testing.T) {
	tempDir := t.TempDir()
	sldDir := filepath.Join(tempDir, ".sld")
	_ = os.MkdirAll(sldDir, 0755)
	_ = os.WriteFile(filepath.Join(sldDir, "config.yaml"), []byte("version: 1\nruntime:\n  php: \"8.3\"\n"), 0644)

	eng := newStubEngine()
	runner := NewRunner(eng)
	ctx := context.Background()

	if err := runner.Status(ctx, tempDir, false, true); err != nil {
		t.Fatalf("Status(--json) failed: %v", err)
	}
	if err := runner.Status(ctx, tempDir, true, true); err != nil {
		t.Fatalf("Status(--all --json) failed: %v", err)
	}
	if err := runner.Inspect(ctx, tempDir); err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if err := runner.Destroy(ctx, tempDir, true, true); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}
	if len(eng.removedNetworks) != 1 {
		t.Errorf("expected 1 network removed, got %d", len(eng.removedNetworks))
	}
	if len(eng.removedVolumes) < 2 {
		t.Errorf("expected DB & Redis volumes removed, got %v", eng.removedVolumes)
	}
}

func TestRunner_LegacyMigrateDryRunDoesNotWriteConfig(t *testing.T) {
	tempDir := t.TempDir()
	runner := NewRunner(newStubEngine())

	if err := runner.LegacyMigrate(context.Background(), tempDir, false); err != nil {
		t.Fatalf("LegacyMigrate dry-run failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".sld", "config.yaml")); !os.IsNotExist(err) {
		t.Errorf("dry-run should not create .sld/config.yaml")
	}
}

func TestRegisterCommands_CompleteTree(t *testing.T) {
	root := &cobra.Command{Use: "sld"}
	RegisterCommands(root)

	v2Cmd, _, err := root.Find([]string{"v2"})
	if err != nil || v2Cmd == nil {
		t.Fatalf("v2 command not registered: %v", err)
	}

	expected := []string{
		"init", "start", "stop", "destroy", "restart",
		"status", "inspect", "secure", "config", "doctor",
		"init-project", "logs", "open", "update", "migrate-v1",
		"legacy", "shell", "exec", "php", "composer", "artisan", "npm",
	}

	found := make(map[string]bool)
	for _, sub := range v2Cmd.Commands() {
		found[sub.Name()] = true
	}

	for _, name := range expected {
		if !found[name] {
			t.Errorf("expected 'sld v2 %s' command to be registered", name)
		}
	}
}
