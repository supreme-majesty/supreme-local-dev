package secret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateRandomSecret(t *testing.T) {
	s1, err := GenerateRandomSecret(16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s2, err := GenerateRandomSecret(16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(s1) != 32 {
		t.Errorf("expected 32 hex chars, got %d (%s)", len(s1), s1)
	}
	if s1 == s2 {
		t.Errorf("expected two randomly generated secrets to be different, but they were identical")
	}
}

func TestGetOrGenerate_Persistence(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Initial generation
	sec1, err := GetOrGenerate(tempDir, "db_password", 16)
	if err != nil {
		t.Fatalf("initial GetOrGenerate failed: %v", err)
	}

	// 2. Subsequent call must return the exact same secret
	sec2, err := GetOrGenerate(tempDir, "db_password", 16)
	if err != nil {
		t.Fatalf("subsequent GetOrGenerate failed: %v", err)
	}

	if sec1 != sec2 {
		t.Errorf("expected persistent secret, got %s on first call and %s on second", sec1, sec2)
	}

	// 3. Verify file exists on disk
	secretsPath := filepath.Join(tempDir, ".sld", "secrets.json")
	if _, err := os.Stat(secretsPath); err != nil {
		t.Errorf("expected %s to exist: %v", secretsPath, err)
	}
}
