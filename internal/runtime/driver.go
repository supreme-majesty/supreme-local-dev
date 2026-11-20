// Package runtime turns the apps of a project config into concrete Docker
// containers, image builds and router entries. Each runtime (php, node,
// python, ...) has a driver; the orchestrator runs whatever the drivers plan.
package runtime

import (
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// Context is everything a driver needs to plan one project.
type Context struct {
	Identity    *project.Identity
	Config      *config.Config
	ProjectRoot string // absolute path SLD can read
	Env         *hostenv.Env

	// UID/GID that app processes run as, so files they write belong to the developer.
	UID, GID string

	ProjectNetwork string
	GlobalNetwork  string

	// ServiceEnv holds connection variables for the project's database,
	// Redis and Mailpit (DB_HOST, DATABASE_URL, REDIS_URL, MAIL_HOST, ...).
	ServiceEnv map[string]string

	// Images used by built-in drivers.
	PHPImage    func(version string) string
	NginxImage  string
	StaticImage string
}

// Build describes an image to build from the project before containers start.
type Build struct {
	App        string
	ContextDir string // absolute local path of the build context
	Dockerfile string // relative to ContextDir
	Target     string
	BuildArgs  map[string]string
	Tag        string
}

// FileRoute is a router entry for something that is not a container
// (the proxy runtime). It is written as a Traefik dynamic config file.
type FileRoute struct {
	Name  string   // router/service name, unique per project app
	Hosts []string // host names to match
	URL   string   // backend, e.g. http://host.docker.internal:3000
	TLS   bool
}

// AppPlan is a driver's plan for one app.
type AppPlan struct {
	Containers []*reconcile.DesiredContainer
	Volumes    []string // named volumes that must exist (dependency caches)
	Build      *Build
	Route      *FileRoute
}

// Driver plans the containers for apps of one runtime.
type Driver interface {
	Plan(ctx *Context, app *config.App) (*AppPlan, error)
}

var drivers = map[string]Driver{
	config.RuntimePHP:        phpDriver{},
	config.RuntimeStatic:     staticDriver{},
	config.RuntimeNode:       newLangDriver(config.RuntimeNode),
	config.RuntimePython:     newLangDriver(config.RuntimePython),
	config.RuntimeRuby:       newLangDriver(config.RuntimeRuby),
	config.RuntimeGo:         newLangDriver(config.RuntimeGo),
	config.RuntimeImage:      imageDriver{},
	config.RuntimeDockerfile: imageDriver{build: true},
	config.RuntimeProxy:      proxyDriver{},
}

// For returns the driver for a runtime.
func For(runtime string) (Driver, error) {
	d, ok := drivers[runtime]
	if !ok {
		return nil, fmt.Errorf("no driver for runtime %q", runtime)
	}
	return d, nil
}

// PlanApp plans one app with its runtime's driver.
func PlanApp(ctx *Context, app *config.App) (*AppPlan, error) {
	d, err := For(app.Runtime)
	if err != nil {
		return nil, err
	}
	plan, err := d.Plan(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("app %q: %w", app.Name, err)
	}
	return plan, nil
}

// --- shared helpers for drivers ---

// appLabels returns the standard SLD labels for one of an app's containers.
// service is the container's role, used by `sld logs/shell --service`.
func appLabels(ctx *Context, app *config.App, service string) (map[string]string, error) {
	labels, err := docker.BuildContainerLabels(docker.LabelParams{
		ProjectID:     ctx.Identity.ID,
		CanonicalPath: ctx.Identity.CanonicalPath,
		Service:       service,
	})
	if err != nil {
		return nil, err
	}
	labels[docker.LabelApp] = app.Name
	labels[docker.LabelRuntime] = app.Runtime
	return labels, nil
}

// RouterName is the Traefik router/service name for an app. The primary app
// keeps the bare project ID so routers created by earlier SLD versions match.
func RouterName(ctx *Context, app *config.App) string {
	if app == ctx.Config.PrimaryApp() {
		return ctx.Identity.ID
	}
	return ctx.Identity.ID + "-" + app.Name
}

// AppHosts lists the host names an app is served on: its domain, plus the
// root-domain aliases for the primary app.
func AppHosts(cfg *config.Config, app *config.App) []string {
	hosts := []string{app.Domain}
	if app == cfg.PrimaryApp() {
		hosts = append(hosts, cfg.Domain.Aliases...)
	}
	return hosts
}

// HostRule builds a Traefik rule matching any of hosts.
func HostRule(hosts ...string) string {
	parts := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if h != "" {
			parts = append(parts, fmt.Sprintf("Host(`%s`)", h))
		}
	}
	return strings.Join(parts, " || ")
}

// addRouteLabels makes Traefik route the app's hosts to port of this container.
func addRouteLabels(ctx *Context, app *config.App, labels map[string]string, port int) {
	name := RouterName(ctx, app)
	rule := HostRule(AppHosts(ctx.Config, app)...)
	labels["traefik.enable"] = "true"
	labels["traefik.docker.network"] = ctx.GlobalNetwork
	labels["traefik.http.routers."+name+".rule"] = rule
	labels["traefik.http.routers."+name+".entrypoints"] = "web"
	labels["traefik.http.services."+name+".loadbalancer.server.port"] = fmt.Sprint(port)
	if ctx.Config.Domain.TLS {
		labels["traefik.http.routers."+name+"-secure.rule"] = rule
		labels["traefik.http.routers."+name+"-secure.entrypoints"] = "websecure"
		labels["traefik.http.routers."+name+"-secure.tls"] = "true"
	}
}

// appEnv merges, in increasing priority: service connection variables, the
// project environment, extra driver variables and the app's own environment.
func appEnv(ctx *Context, app *config.App, extra map[string]string) map[string]string {
	env := map[string]string{}
	maps.Copy(env, ctx.ServiceEnv)
	maps.Copy(env, ctx.Config.Environment)
	maps.Copy(env, extra)
	maps.Copy(env, app.Environment)
	return env
}

// projectMount mounts the project (or a sub-directory of it) into a container.
func projectMount(ctx *Context, sub, target string, readOnly bool) (reconcile.Mount, error) {
	local := ctx.ProjectRoot
	if sub != "" && sub != "." {
		local = filepath.Join(ctx.ProjectRoot, filepath.FromSlash(sub))
	}
	return ctx.Env.MountFor(local, target, readOnly)
}

// sortedKeys returns map keys in order (stable output for hashing/tests).
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
