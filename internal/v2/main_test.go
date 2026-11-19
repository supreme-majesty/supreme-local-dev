package v2

import (
	"os"
	"testing"
)

// TestMain isolates SLD's data directory so tests never touch the real ~/.sld,
// and forces host mode even when the test binary runs inside a container.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sld-v2-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("SLD_STATE_DIR", dir)
	os.Setenv("SLD_IN_CONTAINER", "0")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
