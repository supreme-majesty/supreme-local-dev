package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectManager_DiscardGhost(t *testing.T) {
	pm := NewProjectManager("/tmp")

	// Create a temporary ghost directory
	ghostPath := filepath.Join(os.TempDir(), "test-ghost-project")
	os.MkdirAll(ghostPath, 0755)

	// Create a dummy file
	os.WriteFile(filepath.Join(ghostPath, "test.txt"), []byte("test"), 0644)

	// Verify it exists
	if _, err := os.Stat(ghostPath); os.IsNotExist(err) {
		t.Fatal("Ghost path should exist before discard")
	}

	// Mock DB service (nil for this test - won't delete DB)
	err := pm.DiscardGhost(ghostPath, "", nil)
	if err != nil {
		t.Errorf("DiscardGhost should not return error, got: %v", err)
	}

	// Verify it was deleted
	if _, err := os.Stat(ghostPath); !os.IsNotExist(err) {
		t.Error("Ghost path should be deleted after DiscardGhost")
	}
}

func TestProjectManager_CloneProject_PathValidation(t *testing.T) {
	pm := NewProjectManager("/tmp")

	// Test with non-existent source
	_, err := pm.CloneProject("/nonexistent/path", "test", false, nil)
	if err == nil {
		t.Error("CloneProject should return error for non-existent source")
	}
}
