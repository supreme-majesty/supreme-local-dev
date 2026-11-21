package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
	v2 "github.com/supreme-majesty/supreme-local-dev/internal/v2"
)

// TaskView is a task the dashboard can run: one from the project's
// `tasks:` block, or one SLD suggests from what it finds in the app.
type TaskView struct {
	Name        string `json:"name"`
	Command     string `json:"command"`
	App         string `json:"app,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"` // "config" | "suggested"
}

const maxCommandLen = 4096

// handleTasks lists a project's tasks: GET /api/v2/projects/{id}/tasks.
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	p, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	cfg, _, err := v2.LoadProjectConfig(p.Root)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"tasks": []TaskView{}, "apps": []string{}, "error": err.Error()})
		return
	}
	apps := make([]string, 0, len(cfg.Apps))
	for _, a := range cfg.Apps {
		apps = append(apps, a.Name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": projectTasks(p.Root, cfg), "apps": apps})
}

// projectTasks returns the configured tasks followed by suggestions whose
// command is not configured already.
func projectTasks(root string, cfg *config.Config) []TaskView {
	tasks := []TaskView{}
	have := map[string]bool{}
	for _, t := range cfg.Tasks {
		tasks = append(tasks, TaskView{Name: t.Name, Command: t.Command, App: t.App, Description: t.Description, Source: "config"})
		have[t.Command] = true
	}
	for _, t := range suggestTasks(root, cfg) {
		if !have[t.Command] {
			have[t.Command] = true
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// suggestTasks looks at each app's folder for common one-off commands.
func suggestTasks(root string, cfg *config.Config) []TaskView {
	var out []TaskView
	add := func(app config.App, name, command, desc string) {
		out = append(out, TaskView{Name: name, Command: command, App: app.Name, Description: desc, Source: "suggested"})
	}
	for _, app := range cfg.Apps {
		dir := filepath.Join(root, filepath.FromSlash(app.Path))
		exists := func(rel string) bool { _, err := os.Stat(filepath.Join(dir, rel)); return err == nil }

		switch app.Runtime {
		case config.RuntimePHP:
			if exists("artisan") {
				addLaravel(app, add)
			}
			if exists("composer.json") {
				add(app, "composer install", "composer install", "Install PHP dependencies")
			}
		case config.RuntimeCompose:
			// The command runs in the stack's routed service, which for these
			// frameworks is normally the app container itself.
			switch {
			case exists("artisan"):
				addLaravel(app, add)
			case exists("manage.py"):
				add(app, "migrate", "python manage.py migrate", "Run Django migrations")
				add(app, "test", "python manage.py test", "Run the test suite")
			case exists("bin/rails"):
				add(app, "db:migrate", "bin/rails db:migrate", "Run database migrations")
				add(app, "test", "bin/rails test", "Run the test suite")
			}
		case config.RuntimeNode:
			pm := app.PackageManager
			if pm == "" {
				pm = "npm"
			}
			for _, script := range packageScripts(filepath.Join(dir, "package.json")) {
				add(app, script, pm+" run "+script, "package.json script")
			}
		case config.RuntimePython:
			if exists("manage.py") {
				add(app, "migrate", "python manage.py migrate", "Run Django migrations")
				add(app, "makemigrations", "python manage.py makemigrations", "Create migrations for model changes")
				add(app, "test", "python manage.py test", "Run the test suite")
			}
		case config.RuntimeRuby:
			if exists("bin/rails") {
				add(app, "db:migrate", "bin/rails db:migrate", "Run database migrations")
				add(app, "db:seed", "bin/rails db:seed", "Seed the database")
				add(app, "test", "bin/rails test", "Run the test suite")
			}
		case config.RuntimeGo:
			if exists("go.mod") {
				add(app, "test", "go test ./...", "Run the tests")
			}
		}
	}
	return out
}

// longRunningScripts are dev servers, not one-off tasks (they already run as
// the app's command).
var longRunningScripts = map[string]bool{"dev": true, "start": true, "serve": true, "watch": true, "preview": true}

// packageScripts returns package.json script names that make sense as
// one-off tasks, sorted.
func packageScripts(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	var names []string
	for name := range pkg.Scripts {
		if longRunningScripts[name] || strings.HasPrefix(name, "pre") || strings.HasPrefix(name, "post") || strings.HasSuffix(name, ":watch") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// handleExec runs a command in one of the project's running app containers
// as a background job: POST /api/v2/projects/{id}/exec {command, app} or
// {task}. It does not block start/stop of the project.
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, ok := s.lookup(id)
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var body struct {
		Command string `json:"command"`
		App     string `json:"app"`
		Task    string `json:"task"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	cfg, _, _ := v2.LoadProjectConfig(p.Root)
	if body.Task != "" {
		if cfg == nil {
			writeError(w, http.StatusBadRequest, "the project has no valid .sld/config.yaml")
			return
		}
		found := false
		for _, t := range projectTasks(p.Root, cfg) {
			if t.Name == body.Task && (body.App == "" || body.App == t.App) {
				body.Command, body.App, found = t.Command, t.App, true
				break
			}
		}
		if !found {
			writeError(w, http.StatusNotFound, "no task named "+body.Task)
			return
		}
	}
	body.Command = strings.TrimSpace(body.Command)
	if body.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	if len(body.Command) > maxCommandLen {
		writeError(w, http.StatusBadRequest, "command is too long")
		return
	}

	kind := "exec"
	if body.Task != "" {
		kind = "task:" + body.Task
	}
	job, err := s.jobs.Start(kind, "", func(ctx context.Context, job *Job) error {
		return s.runInApp(ctx, job, p.Root, cfg, body.App, body.Command)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job.mu.Lock()
	job.ProjectID = id
	job.mu.Unlock()
	view, _ := job.Snapshot(0)
	writeJSON(w, http.StatusAccepted, view)
}

// runInApp executes command with sh -c in the app's running container. App
// code containers run it as the project folder's owner, so files it creates
// (caches, migrations, build output) are not root-owned.
func (s *Server) runInApp(ctx context.Context, out io.Writer, root string, cfg *config.Config, app, command string) error {
	identity, err := s.opts.Env.ProjectIdentity(root)
	if err != nil {
		return err
	}
	containers, err := s.opts.Engine.ListProjectContainers(ctx, identity.ID)
	if err != nil {
		return fmt.Errorf("listing containers: %w", err)
	}
	c, err := v2.ResolveContainer(containers, cfg, app, false)
	if err != nil {
		return err
	}
	executor, ok := s.opts.Engine.(docker.Executor)
	if !ok {
		return errors.New("docker engine cannot run commands in containers")
	}
	opts := docker.ExecOptions{Cmd: []string{"sh", "-c", command}, Stdout: out, Stderr: out}
	if runsAsOwner(c) {
		opts.User = projectOwner(root)
		// An arbitrary UID may have no home directory in the image.
		opts.Env = []string{"HOME=/tmp", "COMPOSER_HOME=/tmp/.composer", "npm_config_cache=/tmp/.npm"}
	}
	fmt.Fprintf(out, "[SLD] %s $ %s\n", c.Name, command)
	code, err := executor.Exec(ctx, c.Name, opts)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("command exited with code %d", code)
	}
	return nil
}

// runsAsOwner reports whether a container runs the project's code with an
// SLD runtime (php, node, ...), where commands should run as the developer.
// Compose, image and Dockerfile apps keep their image's user.
func runsAsOwner(c *reconcile.ActualContainer) bool {
	switch c.Service {
	case docker.ServicePHP, config.RuntimeNode, config.RuntimePython, config.RuntimeRuby, config.RuntimeGo:
		return true
	}
	return false
}

// projectOwner is the UID:GID that owns the project folder (1000:1000 when
// unknown or root, as on Docker Desktop).
func projectOwner(root string) string {
	if uid, gid, ok := hostenv.OwnerOf(root); ok && uid != "0" {
		return uid + ":" + gid
	}
	return "1000:1000"
}

// handleContainerAction starts, stops or restarts one container of a project:
// POST /api/v2/projects/{id}/containers/{name}/{action}.
func (s *Server) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	p, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	name, action := r.PathValue("name"), r.PathValue("action")
	identity, err := s.opts.Env.ProjectIdentity(p.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	containers, err := s.opts.Engine.ListProjectContainers(r.Context(), identity.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var target *reconcile.ActualContainer
	for _, c := range containers {
		if c.Name == name {
			target = c
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "no container "+name+" in this project")
		return
	}
	switch action {
	case "start":
		err = s.opts.Engine.StartContainer(r.Context(), target.ID)
	case "stop":
		err = s.opts.Engine.StopContainer(r.Context(), target.ID, 10)
	case "restart":
		if err = s.opts.Engine.StopContainer(r.Context(), target.ID, 10); err == nil {
			err = s.opts.Engine.StartContainer(r.Context(), target.ID)
		}
	default:
		writeError(w, http.StatusNotFound, "unknown action "+action)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mu.Lock()
	s.scannedAt = time.Time{}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "container": name, "action": action})
}

// handleArtisanRun runs `php artisan <command>` in the project's PHP container
// for the Artisan console: POST /api/artisan/run {project_path, command}.
// (SLD's image has no PHP; the old handler ran it there.) The output is a job.
func (s *Server) handleArtisanRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectPath string `json:"project_path"`
		Command     string `json:"command"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Command = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(body.Command), "php artisan "))
	if body.Command == "" || len(body.Command) > maxCommandLen {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	results, err := s.scan(false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	root := filepath.Clean(body.ProjectPath)
	var project *v2.ScanResult
	for i := range results {
		if filepath.Clean(results[i].Root) == root {
			project = &results[i]
			break
		}
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "this folder is not a project SLD manages")
		return
	}
	cfg, _, err := v2.LoadProjectConfig(project.Root)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the project has no valid .sld/config.yaml: "+err.Error())
		return
	}
	app := ""
	for _, a := range cfg.Apps {
		if a.Runtime == config.RuntimePHP {
			if _, err := os.Stat(filepath.Join(project.Root, filepath.FromSlash(a.Path), "artisan")); err == nil {
				app = a.Name
				break
			}
		}
	}
	if app == "" {
		writeError(w, http.StatusBadRequest, "no Laravel app (a PHP app with an artisan file) in this project")
		return
	}
	id, _ := s.opts.Env.ProjectIdentity(project.Root)
	job, err := s.jobs.Start("artisan", "", func(ctx context.Context, job *Job) error {
		return s.runInApp(ctx, job, project.Root, cfg, app, "php artisan "+body.Command)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job.mu.Lock()
	job.ProjectID = id.ID
	job.mu.Unlock()
	writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "message": "started", "job_id": job.ID})
}

func addLaravel(app config.App, add func(config.App, string, string, string)) {
	add(app, "migrate", "php artisan migrate", "Run database migrations")
	add(app, "db:seed", "php artisan db:seed", "Seed the database")
	add(app, "optimize:clear", "php artisan optimize:clear", "Clear config, route, view and cache")
	add(app, "test", "php artisan test", "Run the test suite")
	add(app, "route:list", "php artisan route:list", "List routes")
}
