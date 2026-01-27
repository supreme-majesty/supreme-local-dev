//go:build windows

package services

import (
	"os/exec"
)

// prepareCommand on Windows just sets env (no sudo/credential support in simple way)
func prepareCommand(cmd *exec.Cmd, uid, gid int, env []string) {
	if len(env) > 0 {
		cmd.Env = env
	}
}

// getPathOwner returns 0,0 on Windows (mock)
func getPathOwner(path string) (int, int, error) {
	return 0, 0, nil
}
