package runtime

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
)

// testContext loads yaml as a project config rooted in a temp dir with files.
func testContext(t *testing.T, yaml string, files map[string]string) *Context {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SLD_TLD", "")
	cfg, err := config.LoadFromBytes([]byte(yaml), "shop")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return &Context{
		Identity:       &project.Identity{ID: "shop-1234abcd", Slug: "shop", Hash: "1234abcd", CanonicalPath: "/p/shop"},
		Config:         cfg,
		ProjectRoot:    root,
		Env:            hostenv.Host(t.TempDir()),
		UID:            "1000",
		GID:            "1000",
		ProjectNetwork: "sld-shop-1234abcd-net",
		GlobalNetwork:  "sld-global",
		ServiceEnv:     map[string]string{"DATABASE_URL": "postgresql://u:p@db:5432/shop"},
		PHPImage:       func(v string) string { return "sld-php:" + v },
		NginxImage:     "sld-nginx:1",
		StaticImage:    "nginx:1.27-alpine",
	}
}

func plan(t *testing.T, ctx *Context, app string) *AppPlan {
	t.Helper()
	a, ok := ctx.Config.App(app)
	if !ok {
		t.Fatalf("no app %s", app)
	}
	p, err := PlanApp(ctx, a)
	if err != nil {
		t.Fatalf("PlanApp(%s): %v", app, err)
	}
	return p
}

func TestPHPDriver(t *testing.T) {
	ctx := testContext(t, `version: 2
domain: {aliases: [www.shop.localhost], tls: true}
apps:
  web: {runtime: php, version: "8.3", xdebug: true}
services: {redis: {enabled: true}}
`, nil)
	p := plan(t, ctx, "web")
	if len(p.Containers) != 2 {
		t.Fatalf("want php + nginx, got %d containers", len(p.Containers))
	}
	php, nginx := p.Containers[0], p.Containers[1]

	if php.Name != "sld-shop-1234abcd-web-php" || php.Image != "sld-php:8.3" || php.Service != docker.ServicePHP {
		t.Errorf("php container: %s %s %s", php.Name, php.Image, php.Service)
	}
	for k, want := range map[string]string{"XDEBUG_MODE": "debug", "WWWUSER": "1000", "CACHE_DRIVER": "redis", "DATABASE_URL": "postgresql://u:p@db:5432/shop"} {
		if php.Environment[k] != want {
			t.Errorf("php env %s = %q, want %q", k, php.Environment[k], want)
		}
	}
	if slices.Contains(php.Networks, "sld-global") {
		t.Error("PHP-FPM must not join the router network (ADR 0004)")
	}

	if nginx.Environment["PHP_HOST"] != php.Name || nginx.Environment["WEB_ROOT"] != "public" {
		t.Errorf("nginx env = %v", nginx.Environment)
	}
	rule := nginx.Labels["traefik.http.routers.shop-1234abcd.rule"]
	if rule != "Host(`shop.localhost`) || Host(`www.shop.localhost`)" {
		t.Errorf("primary app keeps the bare project router name and serves aliases, rule = %q", rule)
	}
	if nginx.Labels["traefik.http.routers.shop-1234abcd-secure.tls"] != "true" {
		t.Error("TLS router missing")
	}
	if nginx.Labels[docker.LabelApp] != "web" || nginx.Labels[docker.LabelRuntime] != "php" {
		t.Errorf("app labels = %v", nginx.Labels)
	}
}

func TestStaticDriver_MountsWebRoot(t *testing.T) {
	ctx := testContext(t, "version: 2\napps:\n  site: {runtime: static, web_root: dist}\n", nil)
	c := plan(t, ctx, "site").Containers[0]
	if c.Image != "nginx:1.27-alpine" || c.Name != "sld-shop-1234abcd-site" {
		t.Errorf("static container: %s %s", c.Name, c.Image)
	}
	m := c.Mounts[0]
	if m.Target != "/usr/share/nginx/html" || !m.ReadOnly || filepath.Base(m.Source) != "dist" {
		t.Errorf("static mount = %+v", m)
	}
	if c.Labels["traefik.http.services.shop-1234abcd.loadbalancer.server.port"] != "80" {
		t.Error("static app should route to port 80")
	}
}

func TestNodeDriver(t *testing.T) {
	yaml := `version: 2
apps:
  web: {runtime: php, version: "8.3"}
  ui: {runtime: node, version: "20", command: "npm run dev -- --host 0.0.0.0", port: 5173, healthcheck: /}
`
	ctx := testContext(t, yaml, map[string]string{"package.json": `{"scripts":{"dev":"vite"}}`, "package-lock.json": `{"v":1}`})
	p := plan(t, ctx, "ui")
	c := p.Containers[0]

	if c.Image != "node:20-bookworm-slim" || c.Name != "sld-shop-1234abcd-ui" || c.User != "0:0" {
		t.Errorf("node container: %s %s user=%s", c.Name, c.Image, c.User)
	}
	if !slices.Equal(p.Volumes, []string{"sld-shop-1234abcd-ui-deps"}) || !slices.Contains(c.Volumes, "sld-shop-1234abcd-ui-deps:/app/node_modules") {
		t.Errorf("deps volume: plan=%v container=%v", p.Volumes, c.Volumes)
	}
	env := c.Environment
	if !strings.Contains(env["SLD_INSTALL"], "npm ci") || env["SLD_COMMAND"] != "npm run dev -- --host 0.0.0.0" || env["PORT"] != "5173" || env["HOST"] != "0.0.0.0" {
		t.Errorf("node env: install=%q command=%q port=%q", env["SLD_INSTALL"], env["SLD_COMMAND"], env["PORT"])
	}
	if env["SLD_UID"] != "1000" || env["DATABASE_URL"] == "" {
		t.Errorf("uid/service env missing: %v", env)
	}
	// The user command must reach the container only through the environment.
	if strings.Contains(strings.Join(c.Cmd, " "), "npm run dev") {
		t.Error("user command was interpolated into the entrypoint script")
	}
	if !c.SkipHealthWait || c.Healthcheck == nil || c.Healthcheck.Test[1] != "node" {
		t.Errorf("node healthcheck/wait: skip=%v hc=%+v", c.SkipHealthWait, c.Healthcheck)
	}
	// Secondary app: own router, <app>.<domain>.
	if rule := c.Labels["traefik.http.routers.shop-1234abcd-ui.rule"]; rule != "Host(`ui.shop.localhost`)" {
		t.Errorf("secondary app rule = %q", rule)
	}
	if c.Labels["traefik.http.services.shop-1234abcd-ui.loadbalancer.server.port"] != "5173" {
		t.Error("node app should route to its port")
	}
}

func TestNodeDriver_DepsHashFollowsLockfile(t *testing.T) {
	yaml := "version: 2\napps:\n  web: {runtime: node}\n"
	a := testContext(t, yaml, map[string]string{"package.json": "{}", "package-lock.json": `{"lock":1}`})
	b := testContext(t, yaml, map[string]string{"package.json": "{}", "package-lock.json": `{"lock":2}`})
	c := testContext(t, yaml, map[string]string{"package.json": "{}", "package-lock.json": `{"lock":1}`})
	ha := plan(t, a, "web").Containers[0].Environment["SLD_DEPS_HASH"]
	hb := plan(t, b, "web").Containers[0].Environment["SLD_DEPS_HASH"]
	hc := plan(t, c, "web").Containers[0].Environment["SLD_DEPS_HASH"]
	if ha == hb {
		t.Error("changing the lockfile must change the dependency hash")
	}
	if ha != hc {
		t.Error("identical lockfiles must give identical hashes")
	}
}

func TestNodeDriver_PackageManagers(t *testing.T) {
	files := map[string]string{"package.json": "{}"}
	for pm, want := range map[string]string{"pnpm": "corepack pnpm install", "yarn": "corepack yarn install", "": "npm install"} {
		ctx := testContext(t, "version: 2\napps:\n  web: {runtime: node, package_manager: "+map[bool]string{true: "npm", false: pm}[pm == ""]+"}\n", files)
		if got := plan(t, ctx, "web").Containers[0].Environment["SLD_INSTALL"]; !strings.Contains(got, want) {
			t.Errorf("package_manager %q: install = %q, want %q", pm, got, want)
		}
	}
	ctx := testContext(t, "version: 2\napps:\n  web: {runtime: node, package_manager: bun}\n", files)
	a, _ := ctx.Config.App("web")
	if _, err := PlanApp(ctx, a); err == nil || !strings.Contains(err.Error(), "bun") {
		t.Errorf("bun should be reported as unsupported, got %v", err)
	}
	// No package.json: nothing to install.
	ctx = testContext(t, "version: 2\napps:\n  web: {runtime: node}\n", nil)
	if got := plan(t, ctx, "web").Containers[0].Environment["SLD_INSTALL"]; got != "" {
		t.Errorf("install without package.json = %q", got)
	}
}

func TestPythonRubyGoDrivers(t *testing.T) {
	ctx := testContext(t, `version: 2
apps:
  api: {runtime: python, command: "uvicorn main:app --host 0.0.0.0 --port 8000", port: 8000}
  admin: {runtime: ruby, command: "bin/rails server -b 0.0.0.0", port: 3000}
  svc: {runtime: go, version: "1.24", command: "go run .", port: 8080}
`, map[string]string{"requirements.txt": "fastapi\n", "Gemfile": "source 'https://rubygems.org'\n"})

	py := plan(t, ctx, "api").Containers[0]
	if py.Image != "python:3.13-slim" || py.Environment["SLD_INSTALL"] != "pip install -r requirements.txt" ||
		!strings.HasPrefix(py.Environment["PATH"], "/opt/sld/venv/bin:") || py.Environment["SLD_PRELUDE"] == "" {
		t.Errorf("python: image=%s install=%q path=%q", py.Image, py.Environment["SLD_INSTALL"], py.Environment["PATH"])
	}
	rb := plan(t, ctx, "admin").Containers[0]
	if rb.Image != "ruby:3.4-slim" || rb.Environment["SLD_INSTALL"] != "bundle install" || !slices.Contains(rb.Volumes, "sld-shop-1234abcd-admin-deps:/usr/local/bundle") {
		t.Errorf("ruby: image=%s install=%q volumes=%v", rb.Image, rb.Environment["SLD_INSTALL"], rb.Volumes)
	}
	gosvc := plan(t, ctx, "svc").Containers[0]
	if gosvc.Image != "golang:1.24" || gosvc.Environment["SLD_INSTALL"] != "" {
		t.Errorf("go: image=%s install=%q", gosvc.Image, gosvc.Environment["SLD_INSTALL"])
	}
}

func TestAppEnvironmentPrecedence(t *testing.T) {
	ctx := testContext(t, `version: 2
environment: {LEVEL: project, DATABASE_URL: overridden-by-project}
apps:
  web:
    runtime: node
    environment: {LEVEL: app, PORT: "9999", SLD_UID: "0"}
`, nil)
	env := plan(t, ctx, "web").Containers[0].Environment
	if env["LEVEL"] != "app" || env["DATABASE_URL"] != "overridden-by-project" || env["PORT"] != "9999" {
		t.Errorf("precedence wrong: %v", env)
	}
	if env["SLD_UID"] != "1000" {
		t.Error("app environment must not be able to override SLD's control variables (privilege drop)")
	}
}

func TestImageAndDockerfileDrivers(t *testing.T) {
	ctx := testContext(t, `version: 2
apps:
  mail: {runtime: image, image: "axllent/mailpit:v1.31.2", port: 8025}
  api: {runtime: dockerfile, port: 8080, context: backend, dockerfile: Dockerfile.dev, target: dev, build_args: {A: "1"}, command: "./run.sh"}
`, nil)

	img := plan(t, ctx, "mail")
	if img.Build != nil || img.Containers[0].Image != "axllent/mailpit:v1.31.2" || img.Containers[0].Cmd != nil {
		t.Errorf("image app: build=%v image=%s cmd=%v", img.Build, img.Containers[0].Image, img.Containers[0].Cmd)
	}

	df := plan(t, ctx, "api")
	if df.Build == nil {
		t.Fatal("dockerfile app needs a build")
	}
	b := df.Build
	if b.Tag != "sld-shop-1234abcd-api:dev" || filepath.Base(b.ContextDir) != "backend" || b.Dockerfile != "Dockerfile.dev" || b.Target != "dev" || b.BuildArgs["A"] != "1" {
		t.Errorf("build = %+v", b)
	}
	c := df.Containers[0]
	if c.Image != b.Tag || !slices.Equal(c.Cmd, []string{"sh", "-c", "./run.sh"}) {
		t.Errorf("dockerfile container image=%s cmd=%v", c.Image, c.Cmd)
	}
}

func TestProxyDriver(t *testing.T) {
	ctx := testContext(t, "version: 2\ndomain: {tls: true}\napps:\n  web: {runtime: proxy, port: 3000}\n", nil)
	p := plan(t, ctx, "web")
	if len(p.Containers) != 0 || p.Route == nil {
		t.Fatalf("proxy should only produce a route: %+v", p)
	}
	out := RenderFileRoute(p.Route)
	for _, want := range []string{
		"shop-1234abcd:", "shop-1234abcd-secure:", "tls: {}",
		"rule: \"Host(`shop.localhost`)\"", `url: "http://host.docker.internal:3000"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("route config missing %q:\n%s", want, out)
		}
	}
}

func TestEveryRuntimeHasADriver(t *testing.T) {
	for _, rt := range config.Runtimes {
		if _, err := For(rt); err != nil {
			t.Errorf("runtime %s: %v", rt, err)
		}
	}
}
