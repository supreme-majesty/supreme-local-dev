package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	nonAlphaNumericRegex = regexp.MustCompile(`[^a-z0-9]+`)
	multipleHyphensRegex = regexp.MustCompile(`-+`)
)

// Identity represents the deterministic identity of an SLD V2 project.
type Identity struct {
	ID            string // <slug>-<hash8> e.g. "my-app-3f7b49a1"
	Slug          string // e.g. "my-app"
	Hash          string // 8-char hex sha256 e.g. "3f7b49a1"
	CanonicalPath string // normalized absolute path
}

// CanonicalizePath normalizes a filesystem path:
// 1. Resolves symlinks if they exist
// 2. Converts to clean absolute path
// 3. Normalizes path separators to '/' and lowercase drive letters on Windows
func CanonicalizePath(rawPath string) (string, error) {
	if rawPath == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Try to resolve symlinks; if target doesn't exist yet, clean the abs path
	if evalPath, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = evalPath
	}

	cleaned := filepath.Clean(absPath)

	// Normalize separators to forward slash for cross-platform consistency
	normalized := filepath.ToSlash(cleaned)

	// On Windows, drive letter format like "C:/" -> "c:/"
	if len(normalized) >= 2 && normalized[1] == ':' {
		normalized = strings.ToLower(string(normalized[0])) + normalized[1:]
	}

	return normalized, nil
}

// Slugify generates a URL-safe, Docker-safe slug from a directory name.
func Slugify(name string) string {
	lower := strings.ToLower(name)
	replaced := nonAlphaNumericRegex.ReplaceAllString(lower, "-")
	collapsed := multipleHyphensRegex.ReplaceAllString(replaced, "-")
	trimmed := strings.Trim(collapsed, "-")

	if trimmed == "" {
		trimmed = "project"
	}

	// Truncate to maximum 20 characters to keep Docker/DNS names short
	if len(trimmed) > 20 {
		trimmed = strings.Trim(trimmed[:20], "-")
		if trimmed == "" {
			trimmed = "project"
		}
	}

	return trimmed
}

// ComputeIdentity calculates the deterministic Identity from a path on the
// filesystem this process can see.
func ComputeIdentity(path string) (*Identity, error) {
	canonical, err := CanonicalizePath(path)
	if err != nil {
		return nil, err
	}
	return identityFromCanonical(canonical), nil
}

// ComputeIdentityFromHostPath calculates the Identity from a path as the Docker
// host reports it (e.g. a bind-mount source seen while SLD runs in a container).
// The path is normalised as text only; it does not need to exist locally. For a
// Linux or macOS host this yields the same ID as ComputeIdentity run on the host.
func ComputeIdentityFromHostPath(hostPath string) (*Identity, error) {
	canonical := NormalizeHostPath(hostPath)
	if canonical == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}
	return identityFromCanonical(canonical), nil
}

// NormalizeHostPath canonicalises a host path string without touching the
// local filesystem: forward slashes, cleaned, lower-case Windows drive letter,
// and Docker Desktop VM mount prefixes mapped back to their Windows drive
// ("/run/desktop/mnt/host/c/Users/x" -> "c:/Users/x").
func NormalizeHostPath(hostPath string) string {
	p := strings.TrimSpace(strings.ReplaceAll(hostPath, `\`, "/"))
	if p == "" {
		return ""
	}
	for _, prefix := range []string{"/run/desktop/mnt/host/", "/host_mnt/", "/mnt/host/"} {
		if rest, ok := strings.CutPrefix(p, prefix); ok && len(rest) >= 1 && isDriveLetter(rest[0]) && (len(rest) == 1 || rest[1] == '/') {
			p = rest[:1] + ":/" + strings.TrimPrefix(rest[1:], "/")
			break
		}
	}
	if len(p) >= 2 && p[1] == ':' && isDriveLetter(p[0]) {
		drive := strings.ToLower(p[:1])
		return drive + ":" + path.Clean("/"+p[2:])
	}
	return path.Clean(p)
}

func isDriveLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func identityFromCanonical(canonical string) *Identity {
	baseName := path.Base(canonical)
	// Handle root directories or empty basenames
	if baseName == "" || baseName == "." || baseName == "/" || (len(baseName) >= 2 && baseName[1] == ':') {
		baseName = "project"
	}

	slug := Slugify(baseName)

	hasher := sha256.New()
	hasher.Write([]byte(canonical))
	fullHash := hex.EncodeToString(hasher.Sum(nil))
	hash8 := fullHash[:8]

	projectID := fmt.Sprintf("%s-%s", slug, hash8)

	return &Identity{
		ID:            projectID,
		Slug:          slug,
		Hash:          hash8,
		CanonicalPath: canonical,
	}
}

// Resource Naming Helpers

func (i *Identity) NetworkName() string {
	return fmt.Sprintf("sld-%s-net", i.ID)
}

func (i *Identity) NginxContainerName() string {
	return fmt.Sprintf("sld-%s-nginx", i.ID)
}

func (i *Identity) PHPContainerName() string {
	return fmt.Sprintf("sld-%s-php", i.ID)
}

func (i *Identity) DBContainerName() string {
	return fmt.Sprintf("sld-%s-db", i.ID)
}

func (i *Identity) RedisContainerName() string {
	return fmt.Sprintf("sld-%s-redis", i.ID)
}

func (i *Identity) NodeContainerName() string {
	return fmt.Sprintf("sld-%s-node", i.ID)
}

func (i *Identity) DBVolumeName() string {
	return fmt.Sprintf("sld-%s-db-data", i.ID)
}

func (i *Identity) RedisVolumeName() string {
	return fmt.Sprintf("sld-%s-redis-data", i.ID)
}
