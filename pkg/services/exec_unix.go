//go:build !windows

package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// prepareCommand sets the credential for the command to run as a specific user
func prepareCommand(cmd *exec.Cmd, uid, gid int, env []string) {
	if uid != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
		cmd.Env = env
	}
}

// getPathOwner returns the UID and GID of the directory owner
func getPathOwner(path string) (int, int, error) {
	for {
		if info, err := os.Stat(path); err == nil {
			stat := info.Sys().(*syscall.Stat_t)
			return int(stat.Uid), int(stat.Gid), nil
		}
		parent := filepath.Dir(path)
		if parent == path || parent == "." || parent == "/" {
			if parent == "/" {
				if info, err := os.Stat("/"); err == nil {
					stat := info.Sys().(*syscall.Stat_t)
					return int(stat.Uid), int(stat.Gid), nil
				}
				return 0, 0, fmt.Errorf("root not accessible")
			}
		}
		path = parent
	}
}
