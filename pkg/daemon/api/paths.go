package api

import (
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

// WithRoots limits every path-taking route (files, env, editor, artisan) to
// these folders. Without roots those routes refuse every path: inside the SLD
// container the process can reach /data, the Docker socket and more, so an
// unrestricted path is never acceptable.
func (s *Server) WithRoots(roots []string) *Server {
	s.roots = nil
	for _, r := range roots {
		if real, err := filepath.EvalSymlinks(r); err == nil {
			s.roots = append(s.roots, real)
		}
	}
	return s
}

var errOutsideRoots = errors.New("path is outside your workspace folders")

// resolveAllowed returns p with symlinks resolved, or an error when it is not
// inside one of the roots. A missing file is allowed when its folder exists
// inside a root (for saves that create files).
func (s *Server) resolveAllowed(p string) (string, error) {
	if p == "" || strings.ContainsRune(p, 0) || !filepath.IsAbs(p) {
		return "", errors.New("an absolute path is required")
	}
	p = filepath.Clean(p)
	real, err := filepath.EvalSymlinks(p)
	if errors.Is(err, fs.ErrNotExist) {
		parent, perr := filepath.EvalSymlinks(filepath.Dir(p))
		if perr != nil {
			return "", perr
		}
		real, err = filepath.Join(parent, filepath.Base(p)), nil
	}
	if err != nil {
		return "", err
	}
	for _, root := range s.roots {
		if rel, err := filepath.Rel(root, real); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return real, nil
		}
	}
	return "", errOutsideRoots
}

// allowPath writes a 403 and returns false when p is not inside a root.
func (s *Server) allowPath(w http.ResponseWriter, p string) bool {
	if _, err := s.resolveAllowed(p); err != nil {
		code := http.StatusForbidden
		if !errors.Is(err, errOutsideRoots) {
			code = http.StatusBadRequest
		}
		jsonResponse(w, ErrorResponse{Error: err.Error()}, code)
		return false
	}
	return true
}

// allowEnvFile also requires the file to be an .env file.
func (s *Server) allowEnvFile(w http.ResponseWriter, p string) bool {
	if !strings.HasPrefix(filepath.Base(p), ".env") {
		jsonResponse(w, ErrorResponse{Error: "only .env files can be edited here"}, http.StatusBadRequest)
		return false
	}
	return s.allowPath(w, p)
}

// allowEnvBackup checks that backup is one of target's backups, which the env
// manager keeps in <target folder>/.env-backups/<name>.<stamp>.bak.
func (s *Server) allowEnvBackup(w http.ResponseWriter, backup, target string) bool {
	if !s.allowEnvFile(w, target) || !s.allowPath(w, backup) {
		return false
	}
	wantDir := filepath.Join(filepath.Dir(filepath.Clean(target)), ".env-backups")
	name := filepath.Base(backup)
	if filepath.Dir(filepath.Clean(backup)) != wantDir ||
		!strings.HasPrefix(name, filepath.Base(target)+".") || !strings.HasSuffix(name, ".bak") {
		jsonResponse(w, ErrorResponse{Error: "not a backup of " + filepath.Base(target)}, http.StatusBadRequest)
		return false
	}
	return true
}
