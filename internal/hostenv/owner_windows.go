//go:build windows

package hostenv

func ownerOf(string) (string, string, bool) { return "", "", false }
