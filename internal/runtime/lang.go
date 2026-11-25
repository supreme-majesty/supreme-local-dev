package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
	"github.com/supreme-majesty/supreme-local-dev/internal/workspace"
)

// langSpec describes how to run dev servers of one language runtime.
type langSpec struct {
	image     func(version string) string
	depsDir   string            // dependency cache directory, backed by a named volume
	env       map[string]string // extra environment
	prelude   string            // shell run before install (e.g. create a venv)
	lockfiles []string          // files whose content decides when to reinstall
	install   func(projectRoot string, app *config.App) (string, error)
	health    func(url string) []string
}

func fileExists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}

var langSpecs = map[string]langSpec{
	config.RuntimeNode: {
		image:     func(v string) string { return fmt.Sprintf("node:%s-bookworm-slim", v) },
		depsDir:   "/app/node_modules",
		env:       map[string]string{"COREPACK_ENABLE_DOWNLOAD_PROMPT": "0", "NEXT_TELEMETRY_DISABLED": "1"},
		lockfiles: []string{"package.json", "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock"},
		install: func(root string, app *config.App) (string, error) {
			if !fileExists(root, "package.json") {
				return "", nil
			}
			switch app.PackageManager {
			case "", "npm":
				return "if [ -f package-lock.json ] || [ -f npm-shrinkwrap.json ]; then npm ci; else npm install; fi", nil
			case "pnpm":
				return "corepack pnpm install", nil
			case "yarn":
				return "corepack yarn install", nil
			default:
				return "", fmt.Errorf("package_manager %q is not supported yet (use npm, pnpm or yarn)", app.PackageManager)
			}
		},
		health: func(url string) []string {
			return []string{"CMD", "node", "-e",
				fmt.Sprintf("fetch(%q).then(r=>process.exit(r.status<500?0:1)).catch(()=>process.exit(1))", url)}
		},
	},
	config.RuntimePython: {
		image:   func(v string) string { return fmt.Sprintf("python:%s-slim", v) },
		depsDir: "/opt/sld/venv",
		env: map[string]string{
			"VIRTUAL_ENV":                   "/opt/sld/venv",
			"PATH":                          "/opt/sld/venv/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"PYTHONUNBUFFERED":              "1",
			"PIP_DISABLE_PIP_VERSION_CHECK": "1",
		},
		prelude:   `[ -x "$VIRTUAL_ENV/bin/python" ] || python -m venv "$VIRTUAL_ENV"`,
		lockfiles: []string{"requirements.txt", "pyproject.toml", "poetry.lock", "uv.lock", "Pipfile.lock"},
		install: func(root string, _ *config.App) (string, error) {
			switch {
			case fileExists(root, "requirements.txt"):
				return "pip install -r requirements.txt", nil
			case fileExists(root, "pyproject.toml"):
				return "pip install -e .", nil
			}
			return "", nil
		},
		health: func(url string) []string {
			return []string{"CMD", "python", "-c", fmt.Sprintf(
				"import sys,urllib.request,urllib.error\n"+
					"try: urllib.request.urlopen(%q, timeout=3)\n"+
					"except urllib.error.HTTPError as e: sys.exit(0 if e.code < 500 else 1)\n"+
					"except Exception: sys.exit(1)", url)}
		},
	},
	config.RuntimeRuby: {
		// The full image, not -slim: most Ruby apps (every Rails app) need gems
		// with native extensions, which need a compiler and -dev libraries.
		image:     func(v string) string { return fmt.Sprintf("ruby:%s", v) },
		depsDir:   "/usr/local/bundle",
		env:       map[string]string{"BUNDLE_PATH": "/usr/local/bundle"},
		lockfiles: []string{"Gemfile", "Gemfile.lock"},
		install: func(root string, _ *config.App) (string, error) {
			if fileExists(root, "Gemfile") {
				return "bundle install", nil
			}
			return "", nil
		},
		health: func(url string) []string {
			return []string{"CMD", "ruby", "-rnet/http", "-e",
				fmt.Sprintf("exit(Net::HTTP.get_response(URI(%q)).code.to_i < 500 ? 0 : 1) rescue exit(1)", url)}
		},
	},
	config.RuntimeGo: {
		image:     func(v string) string { return fmt.Sprintf("golang:%s", v) },
		depsDir:   "/go/pkg/mod",
		env:       map[string]string{"GOCACHE": "/go/pkg/mod/.sld-build-cache"},
		lockfiles: []string{"go.mod", "go.sum"},
		install:   func(string, *config.App) (string, error) { return "", nil }, // `go run` downloads modules itself
		health: func(url string) []string {
			return []string{"CMD", "curl", "-fsS", "-o", "/dev/null", url}
		},
	},
}

// entrypoint runs as root only to prepare writable directories, then drops to
// the developer's UID/GID. User-provided commands arrive through environment
// variables and are never interpolated into this script.
const entrypoint = `set -e
mkdir -p "$SLD_DEPS_DIR" "$HOME"
if [ "$SLD_UID" != "0" ]; then
  chown "$SLD_UID:$SLD_GID" "$SLD_DEPS_DIR" "$HOME" 2>/dev/null || true
  exec setpriv --reuid="$SLD_UID" --regid="$SLD_GID" --clear-groups -- sh -c "$SLD_RUN"
fi
exec sh -c "$SLD_RUN"`

// workspaceEntrypoint is entrypoint plus the per-package node_modules volumes
// of a JavaScript workspace (SLD_WORKSPACE_DEPS: space-separated paths, which
// the workspace package validates). Only workspace apps use it, so other
// containers keep their spec.
const workspaceEntrypoint = `set -e
mkdir -p "$SLD_DEPS_DIR" "$HOME"
for d in $SLD_WORKSPACE_DEPS; do mkdir -p "$d"; done
if [ "$SLD_UID" != "0" ]; then
  chown "$SLD_UID:$SLD_GID" "$SLD_DEPS_DIR" "$HOME" 2>/dev/null || true
  for d in $SLD_WORKSPACE_DEPS; do chown "$SLD_UID:$SLD_GID" "$d" 2>/dev/null || true; done
  exec setpriv --reuid="$SLD_UID" --regid="$SLD_GID" --clear-groups -- sh -c "$SLD_RUN"
fi
exec sh -c "$SLD_RUN"`

// runScript installs dependencies when their fingerprint changed, then execs the app.
const runScript = `set -e
cd /app
if [ -n "$SLD_PRELUDE" ]; then sh -c "$SLD_PRELUDE"; fi
marker="$SLD_DEPS_DIR/.sld-deps-hash"
if [ -n "$SLD_INSTALL" ] && [ "$(cat "$marker" 2>/dev/null)" != "$SLD_DEPS_HASH" ]; then
  echo "[sld] installing dependencies: $SLD_INSTALL"
  sh -c "$SLD_INSTALL"
  echo "$SLD_DEPS_HASH" > "$marker"
fi
echo "[sld] starting: $SLD_COMMAND"
exec sh -c "$SLD_COMMAND"`

// langDriver runs a long-lived dev server for node, python, ruby or go.
type langDriver struct {
	runtime string
	spec    langSpec
}

func newLangDriver(runtime string) langDriver {
	return langDriver{runtime: runtime, spec: langSpecs[runtime]}
}

func (d langDriver) Plan(ctx *Context, app *config.App) (*AppPlan, error) {
	id := ctx.Identity

	install := app.Install
	if install == "" {
		var err error
		if install, err = d.spec.install(AppRoot(ctx, app), app); err != nil {
			return nil, err
		}
	}

	codeMount, err := appMount(ctx, app, "", "/app", false)
	if err != nil {
		return nil, err
	}
	depsVolume := id.AppDepsVolumeName(app.Name)

	labels, err := appLabels(ctx, app, d.runtime)
	if err != nil {
		return nil, err
	}
	addRouteLabels(ctx, app, labels, app.Port)

	extra := map[string]string{
		"HOST": "0.0.0.0",
		"PORT": fmt.Sprint(app.Port),
		"HOME": "/tmp/sld-home",
	}
	for k, v := range d.spec.env {
		extra[k] = v
	}
	env := appEnv(ctx, app, extra)
	// Control variables come last so app environment cannot break the entrypoint.
	env["SLD_UID"], env["SLD_GID"] = ctx.UID, ctx.GID
	env["SLD_DEPS_DIR"] = d.spec.depsDir
	lockfiles := d.spec.lockfiles
	ws := d.workspaceDeps(ctx, app)
	for _, pkg := range ws.packages {
		lockfiles = append(lockfiles, pkg+"/package.json") // a package's deps changed: reinstall
	}
	env["SLD_DEPS_HASH"] = depsHash(AppRoot(ctx, app), lockfiles, install)
	env["SLD_PRELUDE"] = d.spec.prelude
	env["SLD_INSTALL"] = install
	env["SLD_COMMAND"] = app.Command
	env["SLD_RUN"] = runScript

	c := &reconcile.DesiredContainer{
		Name:        id.AppContainerName(app.Name, ""),
		Service:     d.runtime,
		Image:       d.spec.image(app.Version),
		WorkingDir:  "/app",
		User:        "0:0", // the entrypoint drops privileges after preparing volumes
		Cmd:         []string{"sh", "-c", entrypoint},
		Labels:      labels,
		Environment: env,
		Networks:    []string{ctx.ProjectNetwork, ctx.GlobalNetwork},
		ExtraHosts:  []string{"host.docker.internal:host-gateway"},
		Volumes:     []string{depsVolume + ":" + d.spec.depsDir},
		Mounts:      []reconcile.Mount{codeMount},
		// Dependency installs can take minutes; don't block `sld start` on them.
		SkipHealthWait: true,
	}
	if len(ws.packages) > 0 {
		c.Cmd = []string{"sh", "-c", workspaceEntrypoint}
		c.Environment["SLD_WORKSPACE_DEPS"] = strings.Join(ws.targets, " ")
		for i, vol := range ws.volumes {
			c.Volumes = append(c.Volumes, vol+":"+ws.targets[i])
		}
	}
	if app.ExposePort > 0 {
		c.Ports = []string{ctx.Env.PublishPort(app.ExposePort, fmt.Sprint(app.Port))}
	}
	if app.Healthcheck != "" {
		c.Healthcheck = &reconcile.HealthcheckConfig{
			Test:        d.spec.health(fmt.Sprintf("http://127.0.0.1:%d%s", app.Port, app.Healthcheck)),
			Interval:    10 * time.Second,
			Timeout:     5 * time.Second,
			StartPeriod: 10 * time.Minute,
			Retries:     3,
		}
	}

	return &AppPlan{Containers: []*reconcile.DesiredContainer{c}, Volumes: append([]string{depsVolume}, ws.volumes...)}, nil
}

// workspaceVolumes are the per-package node_modules volumes of a Node app
// whose root is a JavaScript workspace.
type workspaceVolumes struct {
	packages []string // package folders, relative to the app root
	targets  []string // /app/<package>/node_modules
	volumes  []string // named volumes, same order
}

// workspaceDeps gives each workspace package its own node_modules volume, so
// every app container of the workspace installs into its own copy: no two
// containers install into the same folder at once, and nothing is written
// into the project folder (or mixed with a node_modules installed on the host).
func (d langDriver) workspaceDeps(ctx *Context, app *config.App) workspaceVolumes {
	var ws workspaceVolumes
	if d.runtime != config.RuntimeNode {
		return ws
	}
	base := ctx.Identity.AppDepsVolumeName(app.Name)
	for _, pkg := range workspace.Packages(AppRoot(ctx, app)) {
		sum := sha256.Sum256([]byte(pkg))
		ws.packages = append(ws.packages, pkg)
		ws.targets = append(ws.targets, "/app/"+pkg+"/node_modules")
		ws.volumes = append(ws.volumes, base+"-"+hex.EncodeToString(sum[:])[:10])
	}
	return ws
}

// depsHash fingerprints the lockfiles and install command, so dependencies are
// reinstalled (and the container recreated) exactly when they change.
func depsHash(root string, lockfiles []string, install string) string {
	h := sha256.New()
	fmt.Fprintf(h, "install=%s\n", install)
	for _, name := range lockfiles {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		fmt.Fprintf(h, "%s:%d\n", name, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
