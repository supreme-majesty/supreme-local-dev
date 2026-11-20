package server

// Creating and cloning projects. Templates run their scaffolding tool
// (composer, npx, git) in a throwaway container as the folder's owner, so
// neither the host nor the SLD image needs PHP, Node or Git.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/detect"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
	"github.com/supreme-majesty/supreme-local-dev/pkg/services"
)

const (
	// ghostMarker marks a folder made by Ghost Mode (holds the source path).
	ghostMarker   = ".sld/ghost-of"
	composerImage = "composer:2.8"
	nodeImage     = "node:22-bookworm-slim"
)

func (s *Server) registerCreate(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/templates", s.handleTemplates)
	mux.HandleFunc("POST /api/projects/create", s.handleCreate)
	mux.HandleFunc("POST /api/projects/ghost", s.handleGhost)
	mux.HandleFunc("POST /api/projects/ghost/discard", s.handleGhostDiscard)
	mux.HandleFunc("GET /api/system/directories", s.handleDirectories)
}

type template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

var templates = []template{
	{ID: "laravel", Name: "Laravel", Description: "Modern PHP framework for web artisans", Icon: "laravel"},
	{ID: "wordpress", Name: "WordPress", Description: "The world's most popular CMS", Icon: "wordpress"},
	{ID: "react", Name: "React", Description: "A JavaScript library for building user interfaces", Icon: "react"},
	{ID: "vue", Name: "Vue.js", Description: "The Progressive JavaScript Framework", Icon: "vue"},
	{ID: "nextjs", Name: "Next.js", Description: "The React Framework for the Web", Icon: "nextjs"},
	{ID: "nodejs", Name: "Node.js", Description: "Basic Node.js project", Icon: "nodejs"},
	{ID: "static", Name: "Static HTML", Description: "Simple HTML/CSS/JS project", Icon: "html"},
	{ID: "custom", Name: "Custom (Git)", Description: "Clone from a Git repository", Icon: "git"},
}

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, templates)
}

var projectNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`)

// gitURLRe accepts https://, ssh:// and scp-style (git@host:path) URLs only,
// so a "repository" can never be read as a git option or a local path.
var gitURLRe = regexp.MustCompile(`^(https://|ssh://|git@[A-Za-z0-9.-]+:)[^\s]+$`)

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type       string `json:"type"`
		Name       string `json:"name"`
		Directory  string `json:"directory"`
		Repository string `json:"repository"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !projectNameRe.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "invalid project name: letters, digits, '.', '_' and '-' only")
		return
	}
	if req.Type == "custom" && !gitURLRe.MatchString(req.Repository) {
		writeError(w, http.StatusBadRequest, "repository must be an https://, ssh:// or git@host:path URL")
		return
	}
	known := false
	for _, t := range templates {
		known = known || t.ID == req.Type
	}
	if !known {
		writeError(w, http.StatusBadRequest, "unsupported project type: "+req.Type)
		return
	}
	dir := req.Directory
	if dir == "" && len(s.opts.Workspaces) > 0 {
		dir = s.opts.Workspaces[0]
	}
	base, err := s.inWorkspace(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	target := filepath.Join(base, req.Name)
	if _, err := os.Stat(target); err == nil {
		writeError(w, http.StatusConflict, "folder already exists: "+target)
		return
	}

	job, err := s.jobs.Start("create", "", func(ctx context.Context, job *Job) error {
		defer s.invalidateScan()
		fmt.Fprintf(job, "[SLD] Creating %s project %s in %s\n", req.Type, req.Name, base)
		if err := s.scaffold(ctx, job, req.Type, base, req.Name, req.Repository); err != nil {
			return err
		}
		fmt.Fprintf(job, "[SLD] Created %s. It is listed on the Projects page; set it up and start it there.\n", target)
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, _ := job.Snapshot(0)
	writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "message": "Project creation started", "job_id": view.ID})
}

// scaffold creates base/name from a template.
func (s *Server) scaffold(ctx context.Context, out io.Writer, kind, base, name, repo string) error {
	target := filepath.Join(base, name)
	switch kind {
	case "static":
		return writeFiles(target, map[string]string{"index.html": staticIndex(name)})
	case "nodejs":
		return writeFiles(target, map[string]string{
			"package.json": fmt.Sprintf(`{
  "name": %q,
  "version": "1.0.0",
  "private": true,
  "scripts": { "start": "node server.js", "dev": "node server.js" }
}
`, strings.ToLower(name)),
			"server.js": `const http = require("http");
const port = process.env.PORT || 3000;
http
  .createServer((req, res) => res.end("Hello from Node.js\n"))
  .listen(port, "0.0.0.0", () => console.log("listening on " + port));
`,
		})
	case "wordpress":
		fmt.Fprintln(out, "[SLD] Downloading WordPress")
		return downloadWordPress(ctx, target)
	case "laravel":
		return s.runScaffold(ctx, out, base, composerImage,
			[]string{"composer", "create-project", "laravel/laravel", name, "--prefer-dist", "--no-interaction"})
	case "react", "vue":
		return s.runScaffold(ctx, out, base, nodeImage,
			[]string{"npx", "-y", "create-vite@latest", name, "--template", kind})
	case "nextjs":
		return s.runScaffold(ctx, out, base, nodeImage,
			[]string{"npx", "-y", "create-next-app@latest", name, "--ts", "--tailwind", "--eslint", "--app",
				"--no-src-dir", "--import-alias", "@/*", "--use-npm", "--yes"})
	case "custom":
		return s.runScaffold(ctx, out, base, composerImage, []string{"git", "clone", "--", repo, name})
	}
	return fmt.Errorf("unsupported project type: %s", kind)
}

// runScaffold runs cmd in image with base mounted as the working directory,
// as base's owner so the new files belong to the developer.
func (s *Server) runScaffold(ctx context.Context, out io.Writer, base, image string, cmd []string) error {
	executor, ok := s.opts.Engine.(docker.Executor)
	if !ok {
		return errors.New("docker engine does not support helper containers")
	}
	orch := orchestrate.NewOrchestrator(s.opts.Engine)
	orch.SetEnv(s.opts.Env)
	mount, workDir, err := s.opts.Env.MirrorMount(base, false)
	if err != nil {
		return err
	}
	uid, gid := orch.ProjectOwner(base)
	fmt.Fprintf(out, "[SLD] %s: %s\n", image, strings.Join(cmd, " "))
	if eng, ok := s.opts.Engine.(interface {
		EnsureImage(context.Context, string) error
	}); ok {
		if err := eng.EnsureImage(ctx, image); err != nil {
			return err
		}
	}
	code, err := executor.RunEphemeral(ctx, &reconcile.DesiredContainer{
		Image:      image,
		WorkingDir: workDir,
		User:       uid + ":" + gid,
		Mounts:     []reconcile.Mount{mount},
		Environment: map[string]string{
			"HOME": "/tmp", "COMPOSER_HOME": "/tmp/composer", "npm_config_cache": "/tmp/npm",
			"npm_config_update_notifier": "false", "CI": "1",
		},
	}, docker.ExecOptions{Cmd: cmd, Stdout: out, Stderr: out})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("%s exited with code %d", cmd[0], code)
	}
	return nil
}

func writeFiles(dir string, files map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func staticIndex(name string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%[1]s</title>
  <style>
    body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f0f2f5; }
    .card { background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); text-align: center; }
  </style>
</head>
<body>
  <div class="card"><h1>Welcome to %[1]s</h1><p>Your static site is ready!</p></div>
</body>
</html>
`, name)
}

// downloadWordPress extracts the latest WordPress release into target.
func downloadWordPress(ctx context.Context, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://wordpress.org/latest.tar.gz", nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		// Strip the leading "wordpress/" and refuse anything escaping target.
		rel := strings.TrimPrefix(filepath.ToSlash(h.Name), "wordpress/")
		if rel == "" || rel == h.Name {
			continue
		}
		dest := filepath.Join(target, filepath.FromSlash(rel))
		if !strings.HasPrefix(dest, filepath.Clean(target)+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path in archive: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, io.LimitReader(tr, 256<<20))
			f.Close()
			if err != nil {
				return err
			}
		}
	}
}

// --- ghost clones ---

// ghostSkip are folders not copied into a clone: dependencies and caches are
// rebuilt, and git history is not needed for an experiment.
var ghostSkip = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true, ".sld": true,
	"storage/logs": true, "storage/framework/cache": true, "storage/framework/sessions": true, "storage/framework/views": true,
}

func (s *Server) handleGhost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourcePath string `json:"source_path"`
		TargetName string `json:"target_name"`
		CloneDB    bool   `json:"clone_db"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	source, err := s.inWorkspace(req.SourcePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.TargetName == "" {
		req.TargetName = filepath.Base(source) + "-ghost"
	}
	if !projectNameRe.MatchString(req.TargetName) {
		writeError(w, http.StatusBadRequest, "invalid clone name")
		return
	}
	target := filepath.Join(filepath.Dir(source), req.TargetName)
	if _, err := os.Stat(target); err == nil {
		writeError(w, http.StatusConflict, "folder already exists: "+target)
		return
	}

	job, err := s.jobs.Start("ghost", "", func(ctx context.Context, job *Job) error {
		defer s.invalidateScan()
		fmt.Fprintf(job, "[SLD] Copying %s to %s\n", source, target)
		if err := copyTree(source, target); err != nil {
			return err
		}
		if req.CloneDB {
			cloneGhostDatabase(job, target, req.TargetName)
		}
		// Point the copy at its own domain and give it its own SLD config.
		oldName := filepath.Base(source)
		rewriteEnv(target, func(content string) string {
			for _, tld := range []string{".test", "." + config.DefaultTLD()} {
				content = strings.ReplaceAll(content, oldName+tld, req.TargetName+tld)
			}
			return content
		})
		if err := writeDetectedConfig(target); err != nil {
			fmt.Fprintf(job, "[SLD] ⚠️  could not detect the clone's stack: %v\n", err)
		}
		if err := os.MkdirAll(filepath.Join(target, ".sld"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(target, ghostMarker), []byte(source+"\n"), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(job, "[SLD] Clone ready: %s (Projects page)\n", req.TargetName)
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, _ := job.Snapshot(0)
	writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "message": "Ghost clone started", "job_id": view.ID})
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if ghostSkip[filepath.ToSlash(rel)] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		out := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(out, info.Mode().Perm()|0o700)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, out)
		case d.Type().IsRegular():
			return copyFile(path, out, info.Mode().Perm())
		}
		return nil // sockets, devices: skip
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func rewriteEnv(dir string, fn func(string) string) {
	path := filepath.Join(dir, ".env")
	if data, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path, []byte(fn(string(data))), 0o644)
	}
}

// writeDetectedConfig gives a copied project its own .sld/config.yaml (named
// after its folder), which also makes it show up as a project.
func writeDetectedConfig(root string) error {
	detected, err := detect.Project(root, detect.Options{})
	if err != nil {
		return err
	}
	data, err := config.Marshal(detected.Config)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ".sld"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".sld", "config.yaml"), data, 0o644)
}

// cloneGhostDatabase copies the database named in the clone's .env.
func cloneGhostDatabase(out io.Writer, target, name string) {
	data, err := os.ReadFile(filepath.Join(target, ".env"))
	if err != nil {
		return
	}
	var source string
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "DB_DATABASE="); ok {
			source = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	if source == "" {
		return
	}
	d, err := daemon.GetClient()
	if err != nil {
		fmt.Fprintf(out, "[SLD] ⚠️  database manager unavailable: %v\n", err)
		return
	}
	dbName := strings.ReplaceAll(name, "-", "_") + "_db"
	fmt.Fprintf(out, "[SLD] Cloning database %s → %s\n", source, dbName)
	err = d.DatabaseService.CloneDatabase(source, dbName, services.CloneOptions{
		Mode: "both", CreateDatabase: true, AddAutoIncrement: true, AddConstraints: true,
	})
	if err != nil {
		fmt.Fprintf(out, "[SLD] ⚠️  database clone failed: %v\n", err)
		return
	}
	rewriteEnv(target, func(c string) string {
		return strings.Replace(c, "DB_DATABASE="+source, "DB_DATABASE="+dbName, 1)
	})
}

func (s *Server) handleGhostDiscard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string `json:"path"`
		DBName string `json:"db_name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	path, err := s.inWorkspace(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Only folders created as ghost clones may be deleted from here.
	if _, err := os.Stat(filepath.Join(path, ghostMarker)); err != nil {
		writeError(w, http.StatusBadRequest, path+" is not a ghost clone; only clones made with Ghost Mode can be discarded")
		return
	}
	// Remove its containers first, then the folder and (optionally) its database.
	if err := s.newRunner(io.Discard).Destroy(r.Context(), path, true, true); err != nil {
		fmt.Printf("[SLD] ghost discard: %v\n", err)
	}
	if req.DBName != "" {
		if d, err := daemon.GetClient(); err == nil {
			_ = d.DatabaseService.DeleteDatabase(req.DBName)
		}
	}
	if err := os.RemoveAll(path); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.invalidateScan()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Ghost project discarded"})
}

func (s *Server) handleDirectories(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	if dir == "" && len(s.opts.Workspaces) > 0 {
		dir = s.opts.Workspaces[0]
	}
	path, err := s.inWorkspace(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dirs := []string{}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}
	writeJSON(w, http.StatusOK, dirs)
}
