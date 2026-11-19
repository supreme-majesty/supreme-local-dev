//go:build !windows

package hostenv

import (
	"os"
	"strconv"
	"syscall"
)

func ownerOf(p string) (string, string, bool) {
	info, err := os.Stat(p)
	if err != nil {
		return "", "", false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", false
	}
	return strconv.FormatUint(uint64(st.Uid), 10), strconv.FormatUint(uint64(st.Gid), 10), true
}
