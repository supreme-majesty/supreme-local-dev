package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectIdentity_Deterministic(t *testing.T) {
	tempDir := t.TempDir()

	id1, err := ComputeIdentity(tempDir)
	if err != nil {
		t.Fatalf("ComputeIdentity failed: %v", err)
	}

	id2, err := ComputeIdentity(tempDir)
	if err != nil {
		t.Fatalf("ComputeIdentity failed: %v", err)
	}

	if id1.ID != id2.ID {
		t.Errorf("expected deterministic ID, got %s and %s", id1.ID, id2.ID)
	}

	if id1.Hash != id2.Hash {
		t.Errorf("expected matching hash, got %s and %s", id1.Hash, id2.Hash)
	}
}

func TestProjectIdentity_CollisionAvoidance_SameBasename(t *testing.T) {
	tempRoot := t.TempDir()

	pathA := filepath.Join(tempRoot, "client-a", "api")
	pathB := filepath.Join(tempRoot, "client-b", "api")

	if err := os.MkdirAll(pathA, 0755); err != nil {
		t.Fatalf("failed to create pathA: %v", err)
	}
	if err := os.MkdirAll(pathB, 0755); err != nil {
		t.Fatalf("failed to create pathB: %v", err)
	}

	idA, err := ComputeIdentity(pathA)
	if err != nil {
		t.Fatalf("ComputeIdentity pathA failed: %v", err)
	}

	idB, err := ComputeIdentity(pathB)
	if err != nil {
		t.Fatalf("ComputeIdentity pathB failed: %v", err)
	}

	// Slugs should be identical ("api")
	if idA.Slug != "api" || idB.Slug != "api" {
		t.Errorf("expected slug 'api', got A=%s, B=%s", idA.Slug, idB.Slug)
	}

	// But IDs must be distinct due to hash of canonical path!
	if idA.ID == idB.ID {
		t.Fatalf("collision detected! Identical ID for different paths: %s", idA.ID)
	}

	if idA.Hash == idB.Hash {
		t.Fatalf("collision detected! Identical hash for different paths: %s", idA.Hash)
	}
}

func TestProjectIdentity_SlugSanitization(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"My App", "my-app"},
		{"Super_Store.E-Commerce", "super-store-e-commer"}, // truncated to 20
		{"---Leading-Trailing---", "leading-trailing"},
		{"!!!Special$$$Characters@@@", "special-characters"},
		{"", "project"},
		{"...", "project"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := Slugify(c.input)
			if got != c.expected {
				t.Errorf("Slugify(%q) = %q; want %q", c.input, got, c.expected)
			}
			if len(got) > 20 {
				t.Errorf("Slugify(%q) produced slug exceeding 20 chars: %d", c.input, len(got))
			}
		})
	}
}

func TestProjectIdentity_ResourceNaming(t *testing.T) {
	tempDir := t.TempDir()
	id, err := ComputeIdentity(tempDir)
	if err != nil {
		t.Fatalf("ComputeIdentity failed: %v", err)
	}

	expectedPrefix := "sld-" + id.ID

	if !strings.HasPrefix(id.NetworkName(), expectedPrefix) || !strings.HasSuffix(id.NetworkName(), "-net") {
		t.Errorf("unexpected NetworkName: %s", id.NetworkName())
	}
	if id.NginxContainerName() != expectedPrefix+"-nginx" {
		t.Errorf("unexpected NginxContainerName: %s", id.NginxContainerName())
	}
	if id.PHPContainerName() != expectedPrefix+"-php" {
		t.Errorf("unexpected PHPContainerName: %s", id.PHPContainerName())
	}
	if id.DBContainerName() != expectedPrefix+"-db" {
		t.Errorf("unexpected DBContainerName: %s", id.DBContainerName())
	}
	if id.RedisContainerName() != expectedPrefix+"-redis" {
		t.Errorf("unexpected RedisContainerName: %s", id.RedisContainerName())
	}
	if id.NodeContainerName() != expectedPrefix+"-node" {
		t.Errorf("unexpected NodeContainerName: %s", id.NodeContainerName())
	}
	if id.DBVolumeName() != expectedPrefix+"-db-data" {
		t.Errorf("unexpected DBVolumeName: %s", id.DBVolumeName())
	}
	if id.RedisVolumeName() != expectedPrefix+"-redis-data" {
		t.Errorf("unexpected RedisVolumeName: %s", id.RedisVolumeName())
	}
}

func TestNormalizeHostPath(t *testing.T) {
	tests := map[string]string{
		"/home/dev/Projects/shop/":                 "/home/dev/Projects/shop",
		"/home/dev/../dev/shop":                    "/home/dev/shop",
		`C:\Users\Dev\Code\shop`:                   "c:/Users/Dev/Code/shop",
		"C:/Users/Dev/Code/shop":                   "c:/Users/Dev/Code/shop",
		"/run/desktop/mnt/host/c/Users/Dev/shop":   "c:/Users/Dev/shop",
		"/host_mnt/d/work/api":                     "d:/work/api",
		"/run/desktop/mnt/host/wsl/Ubuntu/home/me": "/run/desktop/mnt/host/wsl/Ubuntu/home/me",
		"": "",
	}
	for in, want := range tests {
		if got := NormalizeHostPath(in); got != want {
			t.Errorf("NormalizeHostPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestComputeIdentityFromHostPath_MatchesAcrossDockerDesktopForms(t *testing.T) {
	a, err := ComputeIdentityFromHostPath(`C:\Users\Dev\shop`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ComputeIdentityFromHostPath("/run/desktop/mnt/host/c/Users/Dev/shop")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID || a.Slug != "shop" {
		t.Errorf("IDs differ or bad slug: %s vs %s", a.ID, b.ID)
	}
	if _, err := ComputeIdentityFromHostPath("  "); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestComputeIdentityFromHostPath_MatchesComputeIdentity(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	local, err := ComputeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	fromHost, err := ComputeIdentityFromHostPath(local.CanonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if local.ID != fromHost.ID {
		t.Errorf("ComputeIdentity %s != ComputeIdentityFromHostPath %s", local.ID, fromHost.ID)
	}
}
