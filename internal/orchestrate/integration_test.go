//go:build integration

package orchestrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// TestDockerEngine_LifecycleAndIdempotency exercises real Docker Engine network,
// volume, container create/start, idempotent diff, recreate, and teardown.
// Run with: go test -tags=integration -v ./internal/orchestrate/...
func TestDockerEngine_LifecycleAndIdempotency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	engine, err := docker.NewDockerEngine()
	if err != nil {
		t.Fatalf("failed to connect to Docker Engine: %v", err)
	}
	defer engine.Close()

	if err := engine.Ping(ctx); err != nil {
		t.Skipf("Docker Engine unreachable: %v", err)
	}

	tmpDir := t.TempDir()
	id, err := project.ComputeIdentity(filepath.Join(tmpDir, "sld-it"))
	if err != nil {
		t.Fatalf("ComputeIdentity failed: %v", err)
	}

	orch := NewOrchestrator(engine)
	defer func() {
		_ = orch.DestroyProject(context.Background(), id, true)
	}()

	// 1. Network + volume creation
	netLabels, err := docker.BuildNetworkLabels(docker.LabelParams{
		ProjectID:     id.ID,
		CanonicalPath: id.CanonicalPath,
		ConfigHash:    "hash-v1",
	})
	if err != nil {
		t.Fatalf("BuildNetworkLabels: %v", err)
	}
	if _, err := engine.EnsureNetwork(ctx, id.NetworkName(), netLabels); err != nil {
		t.Fatalf("EnsureNetwork: %v", err)
	}

	volLabels, err := docker.BuildVolumeLabels(docker.LabelParams{
		ProjectID:     id.ID,
		CanonicalPath: id.CanonicalPath,
		ConfigHash:    "hash-v1",
	})
	if err != nil {
		t.Fatalf("BuildVolumeLabels: %v", err)
	}
	if _, err := engine.EnsureVolume(ctx, id.RedisVolumeName(), volLabels); err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}

	// 2. Create and start a real redis:7-alpine container with healthcheck
	containerLabels, err := docker.BuildContainerLabels(docker.LabelParams{
		ProjectID:     id.ID,
		CanonicalPath: id.CanonicalPath,
		ConfigHash:    "hash-v1",
		Service:       docker.ServiceRedis,
	})
	if err != nil {
		t.Fatalf("BuildContainerLabels: %v", err)
	}

	desired := &reconcile.DesiredContainer{
		Name:       id.RedisContainerName(),
		Service:    docker.ServiceRedis,
		Image:      "redis:7-alpine",
		ConfigHash: "hash-v1",
		Labels:     containerLabels,
		Networks:   []string{id.NetworkName()},
		Volumes:    []string{id.RedisVolumeName() + ":/data"},
		Healthcheck: &reconcile.HealthcheckConfig{
			Test:        []string{"CMD", "redis-cli", "ping"},
			Interval:    2 * time.Second,
			Timeout:     2 * time.Second,
			StartPeriod: 1 * time.Second,
			Retries:     5,
		},
	}

	cid, err := orch.applyPlan(ctx, reconcile.DiffContainer(desired, nil))
	if err != nil {
		t.Fatalf("applyPlan(CREATE) failed: %v", err)
	}
	if err := orch.waitForHealthy(ctx, cid, desired.Name); err != nil {
		t.Fatalf("waitForHealthy failed: %v", err)
	}

	// 3. Idempotency check: second diff should be NOOP
	actualList, err := engine.ListProjectContainers(ctx, id.ID)
	if err != nil || len(actualList) != 1 {
		t.Fatalf("ListProjectContainers = (%d, %v), want 1", len(actualList), err)
	}
	if plan := reconcile.DiffContainer(desired, actualList[0]); plan.Action != reconcile.ActionNoOp {
		t.Fatalf("expected ActionNoOp on unchanged config, got %s (%s)", plan.Action, plan.Reason)
	}

	// 4. Config hash change should trigger RECREATE
	desired.ConfigHash = "hash-v2"
	desired.Labels[docker.LabelConfigHash] = "hash-v2"
	recreatePlan := reconcile.DiffContainer(desired, actualList[0])
	if recreatePlan.Action != reconcile.ActionRecreate {
		t.Fatalf("expected ActionRecreate on config hash change, got %s", recreatePlan.Action)
	}
	cid2, err := orch.applyPlan(ctx, recreatePlan)
	if err != nil {
		t.Fatalf("applyPlan(RECREATE) failed: %v", err)
	}
	if cid2 == cid {
		t.Errorf("expected new container ID after recreate, got same ID %s", cid2)
	}

	// 5. Destroy with volumes removes all containers, network, and volumes
	if err := orch.DestroyProject(ctx, id, true); err != nil {
		t.Fatalf("DestroyProject(true) failed: %v", err)
	}

	remaining, err := engine.ListProjectContainers(ctx, id.ID)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("expected 0 remaining containers after destroy, got %d (err=%v)", len(remaining), err)
	}

	vols, err := engine.ListProjectVolumes(ctx, id.ID)
	if err != nil || len(vols) != 0 {
		t.Fatalf("expected 0 remaining volumes after destroy --volumes, got %v (err=%v)", vols, err)
	}

	_ = os.RemoveAll(tmpDir)
}
