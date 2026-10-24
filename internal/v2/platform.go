package v2

import (
	"os/exec"
)

// newCommand creates a platform-appropriate exec.Cmd.
func newCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
