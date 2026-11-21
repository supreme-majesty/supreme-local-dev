package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
)

// Per-project detail view: config editor, file browser and env editor.
// Every path is relative to the project folder and may not leave it, also
// not through symlinks.

const (
	maxEditableFile = 2 << 20 // 2 MiB: larger files are not opened in the browser editor
	maxEnvBackups   = 20
)

var (
	envFileNames = []string{".env", ".env.local", ".env.development", ".env.example", ".env.testing", ".env.production"}
	envKeyRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
)

func (s *Server) registerProjectFiles(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/v2/projects/{id}/config", s.handlePutConfig)
	mux.HandleFunc("GET /api/v2/projects/{id}/files", s.handleListFiles)
	mux.HandleFunc("GET /api/v2/projects/{id}/file", s.handleReadFile)
	mux.HandleFunc("PUT /api/v2/projects/{id}/file", s.handleWriteFile)
	mux.HandleFunc("GET /api/v2/projects/{id}/env", s.handleListEnv)
	mux.HandleFunc("PUT /api/v2/projects/{id}/env", s.handleWriteEnv)
	mux.HandleFunc("GET /api/v2/projects/{id}/env/backups", s.handleEnvBackups)
	mux.HandleFunc("POST /api/v2/projects/{id}/env/restore", s.handleEnvRestore)
}

// projectRoot looks up the project and returns its folder with symlinks resolved.
func (s *Server) projectRoot(w http.ResponseWriter, r *http.Request) (string, bool) {
	p, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return "", false
	}
	root, err := filepath.EvalSymlinks(p.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return "", false
	}
	return root, true
}

var errOutsideProject = errors.New("path is outside the project folder")

// resolveInProject turns rel (slash-separated, relative to root) into an
// absolute path inside root. With mustExist=false the file may be missing,
// but its parent folder must exist and be inside root.
func resolveInProject(root, rel string, mustExist bool) (string, error) {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	if strings.ContainsRune(rel, 0) {
		return "", errOutsideProject
	}
	p := filepath.Join(root, filepath.FromSlash(rel))
	if !within(root, p) {
		return "", errOutsideProject
	}
	real, err := filepath.EvalSymlinks(p)
	switch {
	case err == nil:
		if !within(root, real) {
			return "", errOutsideProject
		}
		return real, nil
	case errors.Is(err, fs.ErrNotExist) && !mustExist:
		parent, perr := filepath.EvalSymlinks(filepath.Dir(p))
		if perr != nil {
			return "", fmt.Errorf("folder %s does not exist", filepath.ToSlash(filepath.Dir(rel)))
		}
		if !within(root, parent) {
			return "", errOutsideProject
		}
		return filepath.Join(parent, filepath.Base(p)), nil
	default:
		return "", err
	}
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func relTo(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

// writeProjectFile writes data atomically enough for an editor save and keeps
// the existing file's mode. New files get the project folder's owner, so they
// are not left owned by root on Linux.
func writeProjectFile(root, path string, data []byte) error {
	mode := os.FileMode(0o644)
	info, err := os.Stat(path)
	existed := err == nil
	if existed {
		if info.IsDir() {
			return errors.New("path is a folder")
		}
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	if !existed {
		chownLike(root, path)
	}
	return nil
}

func chownLike(root, path string) {
	uid, gid, ok := hostenv.OwnerOf(root)
	if !ok {
		return
	}
	u, err1 := strconv.Atoi(uid)
	g, err2 := strconv.Atoi(gid)
	if err1 == nil && err2 == nil {
		_ = os.Lchown(path, u, g)
	}
}

func decodeBody(r *http.Request, limit int64, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, limit))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

// ---- config ----

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	var body struct {
		Config string `json:"config"`
	}
	if err := decodeBody(r, maxEditableFile, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := config.LoadFromBytes([]byte(body.Config), filepath.Base(root)); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	dir := filepath.Join(root, ".sld")
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(dir, 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		chownLike(root, dir)
	}
	if err := s.writeProjectFile(r.Context(), root, filepath.Join(dir, "config.yaml"), []byte(body.Config)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = s.scan(true) // name, domains and status may have changed
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

// ---- files ----

type fileEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"` // relative to the project folder
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	dir, err := resolveInProject(root, r.URL.Query().Get("path"), true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	list := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		isDir := e.IsDir()
		if info.Mode()&fs.ModeSymlink != 0 {
			// Show links that stay inside the project as what they point to; hide the rest.
			target, err := resolveInProject(root, relTo(root, filepath.Join(dir, e.Name())), true)
			if err != nil {
				continue
			}
			if ti, err := os.Stat(target); err == nil {
				isDir, info = ti.IsDir(), ti
			}
		}
		list = append(list, fileEntry{
			Name: e.Name(), Path: relTo(root, filepath.Join(dir, e.Name())),
			IsDir: isDir, Size: info.Size(), ModTime: info.ModTime(),
		})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].IsDir != list[j].IsDir {
			return list[i].IsDir
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	writeJSON(w, http.StatusOK, map[string]any{"path": relTo(root, dir), "entries": list})
}

func (s *Server) handleReadFile(w http.ResponseWriter, r *http.Request) {
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	p, err := resolveInProject(root, r.URL.Query().Get("path"), true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	info, err := os.Stat(p)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a folder")
		return
	}
	if info.Size() > maxEditableFile {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file is too large to edit here (%d bytes, limit %d)", info.Size(), maxEditableFile))
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if bytes.IndexByte(data, 0) >= 0 {
		writeError(w, http.StatusUnsupportedMediaType, "binary file; it cannot be edited here")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": relTo(root, p), "content": string(data), "size": info.Size(), "mod_time": info.ModTime(),
	})
}

func (s *Server) handleWriteFile(w http.ResponseWriter, r *http.Request) {
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	var body struct {
		Path    string     `json:"path"`
		Content string     `json:"content"`
		ModTime *time.Time `json:"mod_time,omitempty"` // when set, refuse to overwrite a file changed since
	}
	if err := decodeBody(r, maxEditableFile+4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	p, err := resolveInProject(root, body.Path, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if rel := relTo(root, p); rel == ".git" || strings.HasPrefix(rel, ".git/") {
		writeError(w, http.StatusForbidden, "files inside .git cannot be edited here")
		return
	}
	if body.ModTime != nil {
		if info, err := os.Stat(p); err == nil && !info.ModTime().Equal(*body.ModTime) {
			writeError(w, http.StatusConflict, "the file was changed on disk since you opened it; reload it first")
			return
		}
	}
	if err := s.writeProjectFile(r.Context(), root, p, []byte(body.Content)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	info, _ := os.Stat(p)
	resp := map[string]any{"path": relTo(root, p)}
	if info != nil {
		resp["mod_time"] = info.ModTime()
	}
	if relTo(root, p) == ".sld/config.yaml" {
		_, _ = s.scan(true)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- env ----

type envVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type envFileView struct {
	Name      string    `json:"name"`
	Variables []envVar  `json:"variables"` // in file order
	ModTime   time.Time `json:"mod_time"`
}

func (s *Server) handleListEnv(w http.ResponseWriter, r *http.Request) {
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	files := []envFileView{}
	for _, name := range envFileNames {
		p, err := resolveInProject(root, name, true)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		info, _ := os.Stat(p)
		files = append(files, envFileView{Name: name, Variables: parseEnv(data), ModTime: info.ModTime()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "known": envFileNames})
}

func (s *Server) handleWriteEnv(w http.ResponseWriter, r *http.Request) {
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	var body struct {
		Name      string   `json:"name"`
		Variables []envVar `json:"variables"`
	}
	if err := decodeBody(r, maxEditableFile, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !slices.Contains(envFileNames, body.Name) {
		writeError(w, http.StatusBadRequest, "unknown env file "+body.Name)
		return
	}
	seen := map[string]bool{}
	for _, v := range body.Variables {
		if !envKeyRe.MatchString(v.Key) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid variable name %q", v.Key))
			return
		}
		if seen[v.Key] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("variable %s appears twice", v.Key))
			return
		}
		if strings.ContainsAny(v.Value, "\r\n") {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s: values cannot span several lines here; edit the file in Files instead", v.Key))
			return
		}
		seen[v.Key] = true
	}
	p, err := resolveInProject(root, body.Name, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	old, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err == nil {
		if err := s.backupEnv(r.PathValue("id"), body.Name, old); err != nil {
			writeError(w, http.StatusInternalServerError, "backup failed: "+err.Error())
			return
		}
	}
	if err := s.writeProjectFile(r.Context(), root, p, updateEnv(old, body.Variables)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

// Backups live in SLD's data volume, not in the project, so they never end
// up in the project's git history.
func (s *Server) envBackupDir(id string) string {
	return filepath.Join(s.opts.Env.DataDir(), "env-backups", id)
}

func (s *Server) backupEnv(id, name string, data []byte) error {
	dir := s.envBackupDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file := fmt.Sprintf("%s.%s.bak", name, time.Now().UTC().Format("20060102-150405.000"))
	if err := os.WriteFile(filepath.Join(dir, file), data, 0o600); err != nil {
		return err
	}
	// Keep the newest maxEnvBackups per file.
	backups := listEnvBackups(dir, name)
	for _, b := range backups[min(len(backups), maxEnvBackups):] {
		_ = os.Remove(filepath.Join(dir, b.ID))
	}
	return nil
}

type envBackup struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
}

func listEnvBackups(dir, name string) []envBackup {
	entries, _ := os.ReadDir(dir)
	var out []envBackup
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".bak") || (name != "" && !strings.HasPrefix(n, name+".")) {
			continue
		}
		// Only exact names: ".env.<stamp>.bak" must not match ".env.local.<stamp>.bak".
		stamp := strings.TrimSuffix(strings.TrimPrefix(n, name+"."), ".bak")
		if strings.Contains(stamp, ".env") || strings.Count(stamp, ".") != 1 {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, envBackup{ID: n, Name: name, CreatedAt: info.ModTime(), Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *Server) handleEnvBackups(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.projectRoot(w, r); !ok {
		return
	}
	name := r.URL.Query().Get("name")
	if !slices.Contains(envFileNames, name) {
		writeError(w, http.StatusBadRequest, "unknown env file "+name)
		return
	}
	backups := listEnvBackups(s.envBackupDir(r.PathValue("id")), name)
	if backups == nil {
		backups = []envBackup{}
	}
	writeJSON(w, http.StatusOK, backups)
}

func (s *Server) handleEnvRestore(w http.ResponseWriter, r *http.Request) {
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := decodeBody(r, 4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	var found *envBackup
	for _, name := range envFileNames {
		for _, b := range listEnvBackups(s.envBackupDir(id), name) {
			if b.ID == body.ID {
				found = &b
			}
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	data, err := os.ReadFile(filepath.Join(s.envBackupDir(id), found.ID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, err := resolveInProject(root, found.Name, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if cur, err := os.ReadFile(p); err == nil {
		if err := s.backupEnv(id, found.Name, cur); err != nil {
			writeError(w, http.StatusInternalServerError, "backup failed: "+err.Error())
			return
		}
	}
	if err := s.writeProjectFile(r.Context(), root, p, data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restored": found.Name})
}

// parseEnv reads KEY=VALUE lines in order. "export " prefixes and matching
// quotes are removed; comments and blank lines are skipped.
func parseEnv(data []byte) []envVar {
	vars := []envVar{}
	index := map[string]int{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := parseEnvLine(line)
		if !ok {
			continue
		}
		if i, dup := index[key]; dup {
			vars[i].Value = value // the last assignment wins, as in dotenv
			continue
		}
		index[key] = len(vars)
		vars = append(vars, envVar{Key: key, Value: value})
	}
	return vars
}

func parseEnvLine(line string) (key, value string, ok bool) {
	t := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if t == "" || strings.HasPrefix(t, "#") {
		return "", "", false
	}
	t = strings.TrimPrefix(t, "export ")
	k, v, found := strings.Cut(t, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(k)
	if !envKeyRe.MatchString(key) {
		return "", "", false
	}
	v = strings.TrimSpace(v)
	switch {
	case len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"':
		v = strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(v[1 : len(v)-1])
	case len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'':
		v = v[1 : len(v)-1]
	}
	return key, v, true
}

// updateEnv rewrites old so it holds exactly vars: lines of kept variables are
// updated in place only when their value changed, removed variables are
// dropped, new ones are appended. Comments, blank lines and order survive.
func updateEnv(old []byte, vars []envVar) []byte {
	want := map[string]string{}
	for _, v := range vars {
		want[v.Key] = v.Value
	}
	written := map[string]bool{}
	var out []string
	lines := strings.Split(string(old), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		key, value, ok := parseEnvLine(line)
		if !ok {
			out = append(out, line)
			continue
		}
		nv, keep := want[key]
		if !keep || written[key] {
			continue // removed, or a duplicate assignment
		}
		written[key] = true
		if nv == value {
			out = append(out, line)
		} else {
			out = append(out, key+"="+quoteEnv(nv))
		}
	}
	for _, v := range vars {
		if !written[v.Key] {
			out = append(out, v.Key+"="+quoteEnv(v.Value))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return []byte(strings.Join(out, "\n") + "\n")
}

func quoteEnv(v string) string {
	if v == "" || !strings.ContainsAny(v, " \t#\"'$`\\") {
		return v
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}
