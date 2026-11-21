// Package dockerize generates Docker infrastructure (Dockerfile, compose.yaml,
// .dockerignore, a report) for a project that has none, from its detected
// stack.
//
// Rules it follows:
//   - Inspect first: runtimes, versions, package managers, commands and ports
//     come from the project's own manifests and lockfiles (via package detect).
//   - Add only services the project shows it needs: a database only when
//     .env/.env.example or a driver dependency names one, Redis only when it is
//     actually used, a queue worker or scheduler only when the code has them.
//   - Pinned major versions, never "latest"; non-root runtime users; lockfile
//     installs; development and production targets in one multi-stage
//     Dockerfile; databases never published to the host; ports on loopback.
//   - Never overwrite: a project that already has any of the files is refused.
//   - Every decision and assumption goes into DOCKER.md, so the result can be
//     reviewed and maintained by someone else.
package dockerize

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/detect"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
)

// Image versions used for supporting services and helpers. Major (or
// major.minor) tags: patch updates arrive on rebuild, breaking upgrades don't.
const (
	nginxImage      = "nginx:1.27-alpine"
	nginxUnpriv     = "nginxinc/nginx-unprivileged:1.27-alpine"
	composerImage   = "composer:2"
	phpExtInstaller = "mlocati/php-extension-installer:2"
	redisImage      = "redis:7.4-alpine"
	distrolessImage = "gcr.io/distroless/static-debian12:nonroot"
)

var dbImages = map[string]string{"mysql": "mysql", "mariadb": "mariadb", "postgres": "postgres"}
var dbDefaultVersions = map[string]string{"mysql": "8.4", "mariadb": "11.4", "postgres": "16"}

// File is one generated file, relative to the project root.
type File struct {
	Path    string
	Content string
}

// Row is one line of the effective configuration matrix.
type Row struct {
	Setting, Value, Source string
}

// Plan is everything generated for one project.
type Plan struct {
	Root        string
	Name        string
	Profile     string
	Files       []File
	Matrix      []Row
	Assumptions []string
	Services    []string // compose service names
}

// ErrExists is returned when the project already has Docker files.
var ErrExists = errors.New("project already has Docker files")

// PlanProject inspects root and returns the files to write. It writes nothing.
func PlanProject(root string) (*Plan, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	res, err := detect.Project(abs, detect.Options{})
	if err != nil {
		return nil, err
	}
	cfg := res.Config
	for _, a := range cfg.Apps {
		if a.Runtime == config.RuntimeCompose || a.Runtime == config.RuntimeDockerfile {
			return nil, ErrExists
		}
	}

	g := &gen{
		root: abs,
		plan: &Plan{Root: abs, Name: cfg.Name},
		env:  readEnv(abs),
		used: map[int]bool{},
	}
	g.services(cfg)

	var profiles []string
	for i := range cfg.Apps {
		app := cfg.Apps[i]
		if app.Runtime == config.RuntimeNode && hasRuntime(cfg, config.RuntimePHP) && app.Path == phpPath(cfg) {
			g.phpAssets(app) // Laravel/Symfony front-end build next to PHP
			continue
		}
		var profile string
		switch app.Runtime {
		case config.RuntimePHP:
			profile = g.php(app, len(cfg.Apps) == 1 || onlyCompanions(cfg))
		case config.RuntimeNode:
			profile = g.node(app, len(cfg.Apps) == 1)
		case config.RuntimePython:
			profile = g.python(app, len(cfg.Apps) == 1)
		case config.RuntimeRuby:
			profile = g.ruby(app, len(cfg.Apps) == 1)
		case config.RuntimeGo:
			profile = g.golang(app, len(cfg.Apps) == 1)
		case config.RuntimeStatic:
			profile = g.static(app, len(cfg.Apps) == 1)
		default:
			g.assume("app %q (runtime %s) was skipped: no generator for this runtime", app.Name, app.Runtime)
			continue
		}
		profiles = append(profiles, profile)
	}
	if len(g.blocks) == 0 {
		return nil, fmt.Errorf("no app of %s could be dockerized", cfg.Name)
	}
	g.plan.Profile = strings.Join(profiles, " + ")
	if len(cfg.Apps) > 1 && !onlyCompanions(cfg) {
		g.plan.Profile = "fullstack-monorepo (" + g.plan.Profile + ")"
	}
	for _, n := range res.Notes {
		if strings.Contains(n, "⚠️") {
			g.assume("%s", strings.TrimSpace(strings.ReplaceAll(n, "⚠️", "")))
		}
	}

	g.file("compose.yaml", g.compose())
	if !exists(abs, ".env.example") {
		g.file(".env.example", g.envExample())
	} else if len(g.envVars) > 0 {
		g.assume(".env.example already exists and was not changed; the Docker variables (%s) are documented in DOCKER.md", strings.Join(g.envKeys(), ", "))
	}
	g.file("DOCKER.md", g.report())
	return g.plan, nil
}

// Conflicts lists the plan's files that already exist (never overwritten).
func (p *Plan) Conflicts() []string {
	var out []string
	for _, f := range p.Files {
		if exists(p.Root, f.Path) {
			out = append(out, f.Path)
		}
	}
	return out
}

// Write creates the plan's files. It refuses if any of them exists.
func (p *Plan) Write() error {
	if c := p.Conflicts(); len(c) > 0 {
		return fmt.Errorf("%w: %s (nothing was written)", ErrExists, strings.Join(c, ", "))
	}
	for _, f := range p.Files {
		path := filepath.Join(p.Root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(f.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// --- generator state ---

type gen struct {
	root    string
	plan    *Plan
	env     map[string]string
	blocks  []string // compose service blocks, in order
	volumes []string
	used    map[int]bool // host ports already given out
	envVars [][2]string  // compose interpolation variables: name, default

	db     string // "mysql" | "mariadb" | "postgres" | ""
	dbVer  string
	redis  bool
	ignore map[string]bool // .dockerignore files already written (by dir)
}

func (g *gen) file(path, content string) {
	g.plan.Files = append(g.plan.Files, File{Path: filepath.ToSlash(path), Content: content})
}

func (g *gen) row(setting, value, source string) {
	g.plan.Matrix = append(g.plan.Matrix, Row{setting, value, source})
}

func (g *gen) assume(format string, args ...any) {
	g.plan.Assumptions = append(g.plan.Assumptions, fmt.Sprintf(format, args...))
}

func (g *gen) service(name, block string) {
	g.plan.Services = append(g.plan.Services, name)
	g.blocks = append(g.blocks, block)
}

// hostPort gives out a unique loopback host port, configurable by variable.
func (g *gen) hostPort(variable string, preferred int) string {
	p := preferred
	for g.used[p] {
		p++
	}
	g.used[p] = true
	g.envVars = append(g.envVars, [2]string{variable, fmt.Sprint(p)})
	return fmt.Sprintf("127.0.0.1:${%s:-%d}", variable, p)
}

func (g *gen) envKeys() []string {
	var keys []string
	for _, v := range g.envVars {
		keys = append(keys, v[0])
	}
	return keys
}

// --- supporting services (evidence only) ---

func (g *gen) services(cfg *config.Config) {
	laravel := isLaravel(g.root, cfg)
	// Laravel detection assumes MySQL and Redis; here only evidence counts.
	if laravel {
		switch g.env["DB_CONNECTION"] {
		case "mysql":
			g.db = "mysql"
		case "mariadb":
			g.db = "mariadb"
		case "pgsql":
			g.db = "postgres"
		}
		for _, k := range []string{"CACHE_STORE", "CACHE_DRIVER", "SESSION_DRIVER", "QUEUE_CONNECTION"} {
			if g.env[k] == "redis" {
				g.redis = true
			}
		}
		if g.env["DB_CONNECTION"] == "" {
			g.assume("no DB_CONNECTION in .env or .env.example, so no database container was added")
		}
	} else {
		g.db = cfg.Services.Database.Type
		g.redis = cfg.Services.Redis.Enabled
	}
	if g.db != "" {
		g.dbVer = cfg.Services.Database.Version
		if g.dbVer == "" || (g.db == "mysql" && g.dbVer == "8.0") {
			// 8.0 is end of life; 8.4 is the LTS line with the same client protocol.
			g.dbVer = dbDefaultVersions[g.db]
		}
		src := "DB_CONNECTION in " + g.env["__source"]
		if !laravel {
			src = "database driver in the project's dependencies / .env"
		}
		g.row("Database", fmt.Sprintf("%s %s (container, not published)", g.db, g.dbVer), src)
		g.dbService()
	} else {
		g.row("Database", "none", "no database driver or DB_CONNECTION found")
	}
	if g.redis {
		g.row("Redis", redisImage, "Redis used for cache/session/queue")
		g.service("redis", `  redis:
    image: `+redisImage+`
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10
`)
		g.volumes = append(g.volumes, "redis-data")
	}
}

func (g *gen) dbService() {
	g.envVars = append(g.envVars, [2]string{"DOCKER_DB_PASSWORD", "local-dev-only"})
	g.assume("the database container uses user \"app\" and the password in DOCKER_DB_PASSWORD (default \"local-dev-only\"); it is not published to the host. Set a real password for anything but local development")
	var block string
	switch g.db {
	case "postgres":
		block = fmt.Sprintf(`  db:
    image: postgres:%s
    environment:
      POSTGRES_DB: ${DB_DATABASE:-app}
      POSTGRES_USER: app
      POSTGRES_PASSWORD: ${DOCKER_DB_PASSWORD:-local-dev-only}
    volumes:
      - db-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d $${POSTGRES_DB}"]
      interval: 5s
      timeout: 3s
      retries: 20
`, g.dbVer)
	case "mariadb":
		block = fmt.Sprintf(`  db:
    image: mariadb:%s
    environment:
      MARIADB_DATABASE: ${DB_DATABASE:-app}
      MARIADB_USER: app
      MARIADB_PASSWORD: ${DOCKER_DB_PASSWORD:-local-dev-only}
      MARIADB_RANDOM_ROOT_PASSWORD: "yes"
    volumes:
      - db-data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 5s
      timeout: 3s
      retries: 20
`, g.dbVer)
	default:
		block = fmt.Sprintf(`  db:
    image: mysql:%s
    environment:
      MYSQL_DATABASE: ${DB_DATABASE:-app}
      MYSQL_USER: app
      MYSQL_PASSWORD: ${DOCKER_DB_PASSWORD:-local-dev-only}
      MYSQL_RANDOM_ROOT_PASSWORD: "yes"
    volumes:
      - db-data:/var/lib/mysql
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h 127.0.0.1 -u app -p$${MYSQL_PASSWORD} --silent"]
      interval: 5s
      timeout: 3s
      retries: 20
`, g.dbVer)
	}
	g.service("db", block)
	g.volumes = append(g.volumes, "db-data")
}

// appEnv is the environment that points an app at the stack's services.
// Real environment variables take precedence over .env files in Laravel,
// Django-environ, dotenv and Rails, so the app's own .env stays untouched.
func (g *gen) appEnv(laravel bool) string {
	var b strings.Builder
	if g.db != "" {
		port := map[string]string{"mysql": "3306", "mariadb": "3306", "postgres": "5432"}[g.db]
		scheme := map[string]string{"mysql": "mysql", "mariadb": "mysql", "postgres": "postgres"}[g.db]
		fmt.Fprintf(&b, "      DB_HOST: db\n      DB_PORT: %q\n      DB_DATABASE: ${DB_DATABASE:-app}\n      DB_USERNAME: app\n      DB_PASSWORD: ${DOCKER_DB_PASSWORD:-local-dev-only}\n", port)
		if !laravel {
			fmt.Fprintf(&b, "      DATABASE_URL: %s://app:${DOCKER_DB_PASSWORD:-local-dev-only}@db:%s/${DB_DATABASE:-app}\n", scheme, port)
		}
	}
	if g.redis {
		b.WriteString("      REDIS_HOST: redis\n      REDIS_PORT: \"6379\"\n      REDIS_URL: redis://redis:6379\n")
	}
	return b.String()
}

func (g *gen) dependsOn() string {
	var parts []string
	if g.db != "" {
		parts = append(parts, "      db:\n        condition: service_healthy\n")
	}
	if g.redis {
		parts = append(parts, "      redis:\n        condition: service_healthy\n")
	}
	if len(parts) == 0 {
		return ""
	}
	return "    depends_on:\n" + strings.Join(parts, "")
}

// --- PHP ---

func (g *gen) php(app config.App, single bool) string {
	dir := appDir(app)
	ctx := ctxPath(dir)
	laravel := exists(g.root, filepath.Join(dir, "artisan"))
	composer := readComposer(filepath.Join(g.root, dir))
	hasComposer := exists(g.root, filepath.Join(dir, "composer.json"))
	hasLock := exists(g.root, filepath.Join(dir, "composer.lock"))
	profile := "php"
	if laravel {
		profile = "laravel-web"
	}
	version := app.Version
	if version == "" {
		version = "8.3"
	}
	g.row("PHP", version+" (php-fpm)", "composer.json require.php")
	if !hasLock && hasComposer {
		g.assume("%s has no composer.lock, so installs are not reproducible; commit a lockfile", orRoot(dir))
	}

	exts := phpExtensions(composer, laravel, g.db, g.redis)
	g.row("PHP extensions", strings.Join(exts, ", "), "composer.json ext-* requirements, packages and services")

	fpm, web := "app", "web"
	if !single {
		fpm, web = app.Name, app.Name+"-web"
	}
	webRoot := app.WebRoot
	if webRoot == "" {
		webRoot = "."
	}

	var df strings.Builder
	fmt.Fprintf(&df, `# syntax=docker/dockerfile:1
# Generated by SLD from the detected stack (%s). Review before production use.
ARG PHP_VERSION=%s

FROM php:${PHP_VERSION}-fpm-bookworm AS base
COPY --from=%s /usr/bin/install-php-extensions /usr/local/bin/
RUN install-php-extensions %s
COPY --from=%s /usr/bin/composer /usr/bin/composer
ENV COMPOSER_HOME=/tmp/composer
WORKDIR /var/www/html

# Development: the source is bind-mounted by compose.yaml.
FROM base AS dev
RUN cp "$PHP_INI_DIR/php.ini-development" "$PHP_INI_DIR/php.ini"
USER www-data
CMD ["php-fpm"]
`, profile, version, phpExtInstaller, strings.Join(exts, " "), composerImage)
	if hasComposer {
		lock := "composer.json"
		if hasLock {
			lock += " composer.lock"
		}
		fmt.Fprintf(&df, `
# Production: dependencies without dev packages, source baked into the image.
FROM base AS vendor
COPY %s ./
RUN composer install --no-dev --no-scripts --no-autoloader --prefer-dist --no-interaction --no-progress

FROM base AS prod
RUN cp "$PHP_INI_DIR/php.ini-production" "$PHP_INI_DIR/php.ini"
COPY --chown=www-data:www-data . .
COPY --from=vendor --chown=www-data:www-data /var/www/html/vendor ./vendor
RUN composer dump-autoload --optimize --no-dev --no-interaction
USER www-data
CMD ["php-fpm"]
`, lock)
	} else {
		df.WriteString(`
FROM base AS prod
RUN cp "$PHP_INI_DIR/php.ini-production" "$PHP_INI_DIR/php.ini"
COPY --chown=www-data:www-data . .
USER www-data
CMD ["php-fpm"]
`)
	}
	g.file(filepath.Join(dir, "Dockerfile"), df.String())
	g.dockerignore(dir, phpIgnore)

	conf := fmt.Sprintf(`# Generated by SLD: serves %s and passes PHP to the %q php-fpm service.
server {
    listen 80;
    server_name _;
    root /var/www/html/%s;
    index index.php index.html;
    client_max_body_size 64m;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        fastcgi_pass %s:9000;
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
        fastcgi_param DOCUMENT_ROOT $realpath_root;
        include fastcgi_params;
    }

    location ~ /\.(?!well-known) {
        deny all;
    }
}
`, webRoot, fpm, strings.TrimPrefix(filepath.ToSlash(filepath.Clean(webRoot)), "."), fpm)
	confPath := filepath.ToSlash(filepath.Join("docker", "nginx", fpmConfName(single, app.Name)))
	g.file(confPath, conf)
	g.row("Web server", nginxImage+", root "+webRoot, "web root detected from the framework")

	install := ""
	if hasComposer {
		// First start on a fresh clone: install vendor/ into the bind mount.
		install = `[ -f vendor/autoload.php ] || composer install --no-interaction --no-progress; `
	}
	fpmBlock := fmt.Sprintf(`  %s:
    build:
      context: %s
      target: dev
    command: ["sh", "-c", "%sexec php-fpm"]
    volumes:
      - %s:/var/www/html
`, fpm, ctx, install, ctx)
	if e := g.appEnv(laravel); e != "" {
		fpmBlock += "    environment:\n" + e
	}
	fpmBlock += g.dependsOn()
	g.service(fpm, fpmBlock)

	health := ""
	if laravel && strings.Contains(read(g.root, filepath.Join(dir, "bootstrap", "app.php")), "health:") {
		health = `    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://127.0.0.1/up"]
      interval: 10s
      timeout: 3s
      retries: 10
`
		g.row("Health check", "GET /up", "bootstrap/app.php health route")
	}
	g.service(web, fmt.Sprintf(`  %s:
    image: %s
    ports:
      - "%s:80"
    volumes:
      - %s:/var/www/html:ro
      - ./%s:/etc/nginx/conf.d/default.conf:ro
    depends_on:
      - %s
%s`, web, nginxImage, g.hostPort(portVar(single, app.Name), 8080), ctx, confPath, fpm, health))

	if laravel {
		g.laravelProcesses(app, dir, ctx, fpm)
	}
	g.row("Dev strategy", "php-fpm + nginx, source bind-mounted, vendor/ installed on first start", "")
	g.row("Prod strategy", "Dockerfile target prod: composer --no-dev, source in image, non-root", "")
	return profile
}

func (g *gen) laravelProcesses(app config.App, dir, ctx, fpm string) {
	wait := `until [ -f vendor/autoload.php ]; do sleep 2; done; `
	envBlock := ""
	if e := g.appEnv(true); e != "" {
		envBlock = "    environment:\n" + e
	}
	queue := g.env["QUEUE_CONNECTION"]
	hasJobs := dirHasFiles(filepath.Join(g.root, dir, "app", "Jobs")) || dirHasFiles(filepath.Join(g.root, dir, "app", "Listeners"))
	if hasJobs && queue != "" && queue != "sync" && queue != "null" {
		g.service("worker", fmt.Sprintf(`  worker:
    build:
      context: %s
      target: dev
    command: ["sh", "-c", "%sexec php artisan queue:work --tries=3 --max-time=3600"]
    volumes:
      - %s:/var/www/html
%s%s`, ctx, wait, ctx, envBlock, g.dependsOnWith(fpm)))
		g.row("Queue worker", "php artisan queue:work ("+queue+")", "app/Jobs or app/Listeners + QUEUE_CONNECTION")
	} else if hasJobs {
		g.assume("jobs exist but QUEUE_CONNECTION is %q, so no worker service was added", queue)
	}
	console := read(g.root, filepath.Join(dir, "routes", "console.php")) + read(g.root, filepath.Join(dir, "app", "Console", "Kernel.php"))
	if regexp.MustCompile(`Schedule::|\$schedule->`).MatchString(console) {
		g.service("scheduler", fmt.Sprintf(`  scheduler:
    build:
      context: %s
      target: dev
    command: ["sh", "-c", "%sexec php artisan schedule:work"]
    volumes:
      - %s:/var/www/html
%s%s`, ctx, wait, ctx, envBlock, g.dependsOnWith(fpm)))
		g.row("Scheduler", "php artisan schedule:work", "scheduled tasks in routes/console.php or Console/Kernel.php")
	}
}

func (g *gen) dependsOnWith(extra string) string {
	d := g.dependsOn()
	line := "      " + extra + ":\n        condition: service_started\n"
	if d == "" {
		return "    depends_on:\n" + line
	}
	return d + line
}

// phpAssets adds an on-demand service that builds the Vite assets of a PHP app.
func (g *gen) phpAssets(app config.App) {
	dir := appDir(app)
	ctx := ctxPath(dir)
	pm := pmOf(g.root, dir, app.PackageManager)
	version := orDefault(app.Version, config.DefaultVersions[config.RuntimeNode])
	g.service("assets", fmt.Sprintf(`  # Builds the front-end assets into public/build: docker compose --profile assets run --rm assets
  assets:
    image: node:%s-bookworm-slim
    profiles: ["assets"]
    working_dir: /app
    command: ["sh", "-c", "%s && %s"]
    volumes:
      - %s:/app
      - assets-node-modules:/app/node_modules
`, version, pm.installShell(), pm.run("build"), ctx))
	g.volumes = append(g.volumes, "assets-node-modules")
	g.row("Front-end assets", "node "+version+", "+pm.name+", built on demand (profile \"assets\")", "package.json with a Vite dev script")
	g.assume("Vite's dev server (hot reload) is not run in Docker; build assets with the \"assets\" profile, or run the dev server on the host")
}

// --- Node ---

func (g *gen) node(app config.App, single bool) string {
	dir := appDir(app)
	ctx := ctxPath(dir)
	pkg := readPackage(filepath.Join(g.root, dir))
	pm := pmOf(g.root, dir, app.PackageManager)
	version := orDefault(app.Version, config.DefaultVersions[config.RuntimeNode])
	port := app.Port
	if port == 0 {
		port = 3000
	}
	name := "web"
	if !single {
		name = app.Name
	}

	profile := "node"
	switch {
	case pkg.has("next"):
		profile = "nextjs"
	case pkg.has("@nestjs/core"):
		profile = "nestjs-api"
	case pkg.has("vite") && !pkg.hasAny("@sveltejs/kit", "@remix-run/dev", "nuxt", "astro"):
		profile = "react-vite"
	}
	g.row("Node.js", version, versionSource(app.Version, ".nvmrc / engines.node"))
	g.row("Package manager", pm.name+" ("+orNone(pm.lock)+")", "lockfile")
	if pm.lock == "" {
		g.assume("%s has no lockfile, so installs are not reproducible; commit one", orRoot(dir))
	}
	workspaces := pkg.Workspaces != nil || exists(g.root, filepath.Join(dir, "pnpm-workspace.yaml"))
	if workspaces {
		g.assume("%s is a workspace root: the whole folder is copied before installing, which is slower to rebuild", orRoot(dir))
	}

	devCmd := app.Command
	if devCmd == "" {
		devCmd = pm.run("dev")
	}

	var df strings.Builder
	fmt.Fprintf(&df, `# syntax=docker/dockerfile:1
# Generated by SLD from the detected stack (%s). Review before production use.
ARG NODE_VERSION=%s

FROM node:${NODE_VERSION}-bookworm-slim AS base
WORKDIR /app
%sRUN chown node:node /app
USER node

FROM base AS deps
`, profile, version, pm.setup())
	if workspaces {
		df.WriteString("COPY --chown=node:node . .\n")
	} else {
		fmt.Fprintf(&df, "COPY --chown=node:node %s ./\n", strings.Join(pm.manifests(g.root, dir), " "))
	}
	fmt.Fprintf(&df, "RUN %s\n", pm.installShell())
	fmt.Fprintf(&df, `
# Development: the source is bind-mounted by compose.yaml; node_modules stays in a volume.
FROM deps AS dev
ENV NODE_ENV=development
EXPOSE %d
CMD %s
`, port, execForm(devCmd))

	hasBuild := pkg.Scripts["build"] != ""
	switch {
	case profile == "react-vite" && hasBuild && pkg.Scripts["start"] == "":
		fmt.Fprintf(&df, `
FROM deps AS build
COPY --chown=node:node . .
RUN %s

# Production: the static build served by unprivileged nginx on port 8080.
FROM %s AS prod
COPY --from=build /app/dist /usr/share/nginx/html
COPY docker/nginx/spa.conf /etc/nginx/conf.d/default.conf
EXPOSE 8080
`, pm.run("build"), nginxUnpriv)
		g.file(filepath.Join(dir, "docker", "nginx", "spa.conf"), spaConf)
		g.row("Prod strategy", "vite build → nginx-unprivileged (static, SPA fallback)", "react-vite profile")
		g.assume("the production image serves dist/; if the build output folder differs (build.outDir), change the COPY line")
	case hasBuild && (pkg.Scripts["start"] != "" || pkg.Scripts["start:prod"] != ""):
		start := "start"
		if profile == "nestjs-api" && pkg.Scripts["start:prod"] != "" {
			start = "start:prod"
		}
		prune := ""
		if pm.name == "npm" {
			prune = "RUN npm prune --omit=dev\n"
		}
		fmt.Fprintf(&df, `
FROM deps AS build
COPY --chown=node:node . .
RUN %s
%s
# Production: the built app, run with the project's own start script.
FROM node:${NODE_VERSION}-bookworm-slim AS prod
ENV NODE_ENV=production
WORKDIR /app
COPY --from=build --chown=node:node /app ./
USER node
EXPOSE %d
CMD %s
`, pm.run("build"), prune, port, execForm(pm.run(start)))
		g.row("Prod strategy", "build, then "+pm.run(start), "package.json scripts")
		if prune == "" {
			g.assume("dev dependencies are not pruned from the %s production image (%s)", profile, pm.name)
		}
	default:
		g.assume("no production target: package.json has no build + start scripts to base it on")
	}
	g.file(filepath.Join(dir, "Dockerfile"), df.String())
	g.dockerignore(dir, nodeIgnore)

	vol := slug(name) + "-node-modules"
	g.volumes = append(g.volumes, vol)
	block := fmt.Sprintf(`  %s:
    build:
      context: %s
      target: dev
    ports:
      - "%s:%d"
    volumes:
      - %s:/app
      - %s:/app/node_modules
    environment:
      PORT: "%d"
      # File watching across the Docker Desktop / WSL2 boundary.
      CHOKIDAR_USEPOLLING: "true"
      WATCHPACK_POLLING: "true"
%s`, name, ctx, g.hostPort(portVar(single, app.Name), port), port, ctx, vol, port, g.appEnv(false))
	block += g.dependsOn()
	g.service(name, block)
	g.row("Dev strategy", devCmd+" on port "+fmt.Sprint(port)+", hot reload", "detected dev script")
	if profile == "nextjs" {
		g.assume("NEXT_PUBLIC_* variables are baked into the client bundle at build time; pass them as build args for production builds")
	}
	return profile
}

// --- Python ---

func (g *gen) python(app config.App, single bool) string {
	dir := appDir(app)
	ctx := ctxPath(dir)
	version := orDefault(app.Version, config.DefaultVersions[config.RuntimePython])
	port := orPort(app.Port, 8000)
	name := svcName(single, app.Name)
	deps := strings.ToLower(read(g.root, filepath.Join(dir, "requirements.txt")) + read(g.root, filepath.Join(dir, "pyproject.toml")))

	var apt []string
	if strings.Contains(deps, "psycopg2") && !strings.Contains(deps, "psycopg2-binary") {
		apt = append(apt, "libpq-dev", "build-essential")
	}
	if strings.Contains(deps, "mysqlclient") {
		apt = append(apt, "default-libmysqlclient-dev", "pkg-config", "build-essential")
	}
	install := ""
	switch {
	case exists(g.root, filepath.Join(dir, "requirements.txt")):
		install = "COPY requirements.txt ./\nRUN pip install -r requirements.txt\n"
		g.row("Dependencies", "requirements.txt", "")
	case exists(g.root, filepath.Join(dir, "pyproject.toml")):
		install = "COPY . .\nRUN pip install .\n"
		g.row("Dependencies", "pyproject.toml (pip install .)", "")
		g.assume("dependencies come from pyproject.toml via pip; if the project uses Poetry or uv lockfiles, switch to that tool for reproducible installs")
	default:
		g.assume("no requirements.txt or pyproject.toml; add dependencies to the Dockerfile")
	}
	g.row("Python", version, versionSource(app.Version, ".python-version"))

	var df strings.Builder
	fmt.Fprintf(&df, `# syntax=docker/dockerfile:1
# Generated by SLD from the detected stack. Review before production use.
ARG PYTHON_VERSION=%s

FROM python:${PYTHON_VERSION}-slim-bookworm AS base
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1
WORKDIR /app
`, version)
	if len(apt) > 0 {
		fmt.Fprintf(&df, "RUN apt-get update && apt-get install -y --no-install-recommends %s && rm -rf /var/lib/apt/lists/*\n", strings.Join(dedupe(apt), " "))
	}
	df.WriteString("RUN useradd --create-home --uid 1000 app\n" + install)
	fmt.Fprintf(&df, `
# Development: the source is bind-mounted by compose.yaml.
FROM base AS dev
USER app
EXPOSE %d
CMD %s
`, port, execForm(app.Command))
	prod := pythonProd(g.root, dir, deps, port)
	if prod == "" && strings.Contains(deps, "fastapi") && strings.Contains(deps, "uvicorn") && strings.HasPrefix(app.Command, "uvicorn ") {
		prod = strings.Replace(app.Command, " --reload", "", 1)
	}
	if prod != "" {
		fmt.Fprintf(&df, `
FROM base AS prod
COPY --chown=app:app . .
USER app
EXPOSE %d
CMD %s
`, port, execForm(prod))
		g.row("Prod strategy", prod, "WSGI/ASGI server in dependencies")
	} else {
		g.assume("no production target: no gunicorn/uvicorn found to run the app in production")
	}
	g.file(filepath.Join(dir, "Dockerfile"), df.String())
	g.dockerignore(dir, pythonIgnore)

	block := fmt.Sprintf(`  %s:
    build:
      context: %s
      target: dev
    ports:
      - "%s:%d"
    volumes:
      - %s:/app
`, name, ctx, g.hostPort(portVar(single, app.Name), port), port, ctx)
	if e := g.appEnv(false); e != "" {
		block += "    environment:\n" + e
	}
	g.service(name, block+g.dependsOn())
	g.row("Dev strategy", app.Command, "detected framework")
	return "python"
}

func pythonProd(root, dir, deps string, port int) string {
	switch {
	case exists(root, filepath.Join(dir, "manage.py")) && strings.Contains(deps, "gunicorn"):
		if mod := djangoWSGI(filepath.Join(root, dir)); mod != "" {
			return fmt.Sprintf("gunicorn %s --bind 0.0.0.0:%d", mod, port)
		}
	case strings.Contains(deps, "flask") && strings.Contains(deps, "gunicorn"):
		return fmt.Sprintf("gunicorn app:app --bind 0.0.0.0:%d", port)
	}
	return ""
}

// --- Ruby ---

func (g *gen) ruby(app config.App, single bool) string {
	dir := appDir(app)
	ctx := ctxPath(dir)
	version := orDefault(app.Version, config.DefaultVersions[config.RuntimeRuby])
	port := orPort(app.Port, 3000)
	name := svcName(single, app.Name)
	gemfile := strings.ToLower(read(g.root, filepath.Join(dir, "Gemfile")))
	apt := []string{"build-essential", "git", "libyaml-dev"}
	if strings.Contains(gemfile, "pg") {
		apt = append(apt, "libpq-dev")
	}
	if strings.Contains(gemfile, "mysql2") {
		apt = append(apt, "default-libmysqlclient-dev")
	}
	lock := "Gemfile"
	if exists(g.root, filepath.Join(dir, "Gemfile.lock")) {
		lock += " Gemfile.lock"
	} else {
		g.assume("no Gemfile.lock, so installs are not reproducible")
	}
	g.row("Ruby", version, versionSource(app.Version, ".ruby-version"))
	df := fmt.Sprintf(`# syntax=docker/dockerfile:1
# Generated by SLD from the detected stack. Review before production use.
ARG RUBY_VERSION=%s

FROM ruby:${RUBY_VERSION}-slim-bookworm AS dev
RUN apt-get update && apt-get install -y --no-install-recommends %s && rm -rf /var/lib/apt/lists/*
WORKDIR /app
ENV BUNDLE_PATH=/usr/local/bundle
COPY %s ./
RUN bundle install
RUN useradd --create-home --uid 1000 app
USER app
EXPOSE %d
CMD %s
`, version, strings.Join(apt, " "), lock, port, execForm(app.Command))
	g.assume("only a development target is generated for Ruby; Rails 7.1+ can generate its own production Dockerfile (bin/rails app:update)")
	g.file(filepath.Join(dir, "Dockerfile"), df)
	g.dockerignore(dir, rubyIgnore)
	block := fmt.Sprintf(`  %s:
    build:
      context: %s
      target: dev
    ports:
      - "%s:%d"
    volumes:
      - %s:/app
`, name, ctx, g.hostPort(portVar(single, app.Name), port), port, ctx)
	if e := g.appEnv(false); e != "" {
		block += "    environment:\n" + e
	}
	g.service(name, block+g.dependsOn())
	g.row("Dev strategy", app.Command, "detected framework")
	return "ruby"
}

// --- Go ---

func (g *gen) golang(app config.App, single bool) string {
	dir := appDir(app)
	ctx := ctxPath(dir)
	version := orDefault(app.Version, config.DefaultVersions[config.RuntimeGo])
	port := orPort(app.Port, 8080)
	name := svcName(single, app.Name)
	sum := ""
	if exists(g.root, filepath.Join(dir, "go.sum")) {
		sum = " go.sum"
	}
	g.row("Go", version, versionSource(app.Version, "go.mod"))
	df := fmt.Sprintf(`# syntax=docker/dockerfile:1
# Generated by SLD from the detected stack. Review before production use.
ARG GO_VERSION=%s

FROM golang:${GO_VERSION}-bookworm AS base
WORKDIR /src
COPY go.mod%s ./
RUN go mod download

# Development: the source is bind-mounted by compose.yaml.
FROM base AS dev
ENV PORT=%d
EXPOSE %d
CMD ["go", "run", "."]

FROM base AS build
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app .

# Production: a static binary on a distroless, non-root image.
FROM %s AS prod
COPY --from=build /out/app /app
ENV PORT=%d
EXPOSE %d
ENTRYPOINT ["/app"]
`, version, sum, port, port, distrolessImage, port, port)
	g.assume("the Go production image assumes the main package is the module root and builds without cgo")
	g.file(filepath.Join(dir, "Dockerfile"), df)
	g.dockerignore(dir, goIgnore)
	g.volumes = append(g.volumes, "go-cache")
	block := fmt.Sprintf(`  %s:
    build:
      context: %s
      target: dev
    ports:
      - "%s:%d"
    volumes:
      - %s:/src
      - go-cache:/root/.cache/go-build
`, name, ctx, g.hostPort(portVar(single, app.Name), port), port, ctx)
	if e := g.appEnv(false); e != "" {
		block += "    environment:\n" + e
	}
	g.service(name, block+g.dependsOn())
	g.row("Dev strategy", "go run . on port "+fmt.Sprint(port), "go.mod")
	g.row("Prod strategy", "static binary on "+distrolessImage, "")
	return "go"
}

// --- Static ---

func (g *gen) static(app config.App, single bool) string {
	dir := appDir(app)
	webRoot := orDefault(app.WebRoot, ".")
	src := filepath.ToSlash(filepath.Join(dir, webRoot))
	name := svcName(single, app.Name)
	df := fmt.Sprintf(`# syntax=docker/dockerfile:1
# Generated by SLD: serves the static site in %s with unprivileged nginx.
FROM %s AS prod
COPY %s /usr/share/nginx/html
EXPOSE 8080
`, webRoot, nginxUnpriv, filepath.ToSlash(webRoot))
	g.file(filepath.Join(dir, "Dockerfile"), df)
	g.dockerignore(dir, staticIgnore)
	g.service(name, fmt.Sprintf(`  %s:
    image: %s
    ports:
      - "%s:8080"
    volumes:
      - ./%s:/usr/share/nginx/html:ro
`, name, nginxUnpriv, g.hostPort(portVar(single, app.Name), 8080), strings.TrimPrefix(src, "./")))
	g.row("Web server", nginxUnpriv+" serving "+webRoot, "index.html location")
	return "static-site"
}

// --- compose, env, report ---

func (g *gen) compose() string {
	var b strings.Builder
	fmt.Fprintf(&b, `# Generated by SLD for %s. See DOCKER.md for the decisions behind it.
#   docker compose up -d --build     start (development)
#   docker compose logs -f           follow logs
#   docker compose down              stop and remove containers (data volumes are kept)
name: %s

services:
`, g.plan.Name, slug(g.plan.Name))
	for i, block := range g.blocks {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(block)
	}
	if len(g.volumes) > 0 {
		b.WriteString("\nvolumes:\n")
		for _, v := range dedupe(g.volumes) {
			fmt.Fprintf(&b, "  %s:\n", v)
		}
	}
	return b.String()
}

func (g *gen) envExample() string {
	var b strings.Builder
	b.WriteString("# Variables read by compose.yaml (copy to .env and adjust). Generated by SLD.\n")
	for _, v := range g.envVars {
		fmt.Fprintf(&b, "%s=%s\n", v[0], v[1])
	}
	return b.String()
}

func (g *gen) report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Docker setup for %s\n\nGenerated by Supreme Local Dev from the project's detected stack. Review it like any other change before relying on it in production.\n\n", g.plan.Name)
	fmt.Fprintf(&b, "**Profile:** %s  \n**Environments:** development (compose.yaml), production (Dockerfile target `prod`)\n\n", g.plan.Profile)
	b.WriteString("## Effective configuration\n\n| Setting | Value | Source |\n|---|---|---|\n")
	for _, r := range g.plan.Matrix {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", r.Setting, strings.ReplaceAll(r.Value, "|", "\\|"), orDash(r.Source))
	}
	b.WriteString("\n## Services\n\n")
	for _, s := range g.plan.Services {
		fmt.Fprintf(&b, "- `%s`\n", s)
	}
	b.WriteString("\n## Files\n\n")
	for _, f := range g.plan.Files {
		fmt.Fprintf(&b, "- `%s`\n", f.Path)
	}
	b.WriteString("- `DOCKER.md` (this file)\n")
	if len(g.envVars) > 0 {
		b.WriteString("\n## Variables\n\nRead by `compose.yaml` from the shell or `.env`:\n\n")
		for _, v := range g.envVars {
			fmt.Fprintf(&b, "- `%s` (default `%s`)\n", v[0], v[1])
		}
	}
	b.WriteString("\n## Commands\n\n```sh\ndocker compose up -d --build   # start\ndocker compose ps               # status\ndocker compose logs -f          # logs\ndocker compose stop             # stop, keep containers\ndocker compose down             # remove containers and network; named volumes (data) are kept\n")
	for _, s := range g.plan.Services {
		if s == "app" {
			b.WriteString("docker compose exec app sh      # shell in the PHP container\n")
			if strings.Contains(g.plan.Profile, "laravel") {
				b.WriteString("docker compose exec app php artisan migrate   # run migrations when you choose to\n")
			}
		}
		if s == "assets" {
			b.WriteString("docker compose --profile assets run --rm assets   # build front-end assets\n")
		}
	}
	b.WriteString("# production image:\ndocker build --target prod -t " + slug(g.plan.Name) + " .\n```\n\n")
	b.WriteString("Never use `docker compose down -v` as routine cleanup: it deletes the database and other data volumes.\n")
	b.WriteString("\nWhen run from SLD, the published ports are replaced by the project's domain.\n")
	if len(g.plan.Assumptions) > 0 {
		b.WriteString("\n## Assumptions and open points\n\n")
		for _, a := range g.plan.Assumptions {
			fmt.Fprintf(&b, "- %s\n", a)
		}
	}
	b.WriteString("\n## Validation\n\nNot validated at generation time. SLD validates the compose file (`docker compose config`), builds the images and starts the containers the first time you start the project.\n")
	return b.String()
}

func (g *gen) dockerignore(dir, content string) {
	if g.ignore == nil {
		g.ignore = map[string]bool{}
	}
	if g.ignore[dir] || exists(g.root, filepath.Join(dir, ".dockerignore")) {
		return
	}
	g.ignore[dir] = true
	g.file(filepath.Join(dir, ".dockerignore"), content)
}

const commonIgnore = `.git
.gitignore
.env
.env.*
!.env.example
*.log
.DS_Store
.idea
.vscode
.sld
compose.yaml
DOCKER.md
`

var (
	phpIgnore    = commonIgnore + "vendor\nnode_modules\nstorage/logs/*\nstorage/framework/cache/*\nstorage/framework/sessions/*\nstorage/framework/views/*\npublic/hot\n"
	nodeIgnore   = commonIgnore + "node_modules\n.next\ndist\nbuild\ncoverage\n.turbo\n"
	pythonIgnore = commonIgnore + "__pycache__\n*.pyc\n.venv\nvenv\n.pytest_cache\n.mypy_cache\n"
	rubyIgnore   = commonIgnore + "log/*\ntmp/*\nvendor/bundle\nnode_modules\n"
	goIgnore     = commonIgnore + "bin\n*.test\n"
	staticIgnore = commonIgnore + "node_modules\n"
)

const spaConf = `# Generated by SLD: serves the built single-page app, falling back to index.html.
server {
    listen 8080;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location ~* \.(?:js|css|woff2?|svg|png|jpg|jpeg|gif|ico)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
    }
}
`

// --- package managers ---

type packageManager struct {
	name  string // npm | pnpm | yarn
	lock  string // lockfile name, "" when none
	berry bool   // yarn 2+
}

func pmOf(root, dir, configured string) packageManager {
	at := func(f string) bool { return exists(root, filepath.Join(dir, f)) }
	switch {
	case configured == "pnpm" || at("pnpm-lock.yaml"):
		return packageManager{name: "pnpm", lock: ifThen(at("pnpm-lock.yaml"), "pnpm-lock.yaml")}
	case configured == "yarn" || at("yarn.lock"):
		return packageManager{name: "yarn", lock: ifThen(at("yarn.lock"), "yarn.lock"), berry: at(".yarnrc.yml")}
	}
	return packageManager{name: "npm", lock: ifThen(at("package-lock.json"), "package-lock.json")}
}

func (p packageManager) setup() string {
	if p.name == "npm" {
		return ""
	}
	return "RUN corepack enable\nENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0\n"
}

func (p packageManager) installShell() string {
	switch p.name {
	case "pnpm":
		if p.lock != "" {
			return "corepack pnpm install --frozen-lockfile"
		}
		return "corepack pnpm install"
	case "yarn":
		switch {
		case p.lock == "":
			return "corepack yarn install"
		case p.berry:
			return "corepack yarn install --immutable"
		}
		return "corepack yarn install --frozen-lockfile"
	}
	if p.lock != "" {
		return "npm ci"
	}
	return "npm install"
}

func (p packageManager) run(script string) string {
	if p.name == "npm" {
		return "npm run " + script
	}
	return "corepack " + p.name + " run " + script
}

func (p packageManager) manifests(root, dir string) []string {
	files := []string{"package.json"}
	for _, f := range []string{p.lock, ".npmrc", ".yarnrc.yml"} {
		if f != "" && exists(root, filepath.Join(dir, f)) {
			files = append(files, f)
		}
	}
	return files
}

// --- helpers ---

type pkgJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Workspaces      any               `json:"workspaces"`
}

func (p *pkgJSON) has(dep string) bool {
	return p.Dependencies[dep] != "" || p.DevDependencies[dep] != ""
}
func (p *pkgJSON) hasAny(deps ...string) bool {
	for _, d := range deps {
		if p.has(d) {
			return true
		}
	}
	return false
}

func readPackage(dir string) *pkgJSON {
	p := &pkgJSON{}
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		_ = json.Unmarshal(data, p)
	}
	if p.Scripts == nil {
		p.Scripts = map[string]string{}
	}
	return p
}

type composerJSON struct {
	Require map[string]string `json:"require"`
}

func readComposer(dir string) composerJSON {
	var c composerJSON
	if data, err := os.ReadFile(filepath.Join(dir, "composer.json")); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

// builtinExts ship with the official php image.
var builtinExts = map[string]bool{
	"ctype": true, "curl": true, "dom": true, "fileinfo": true, "filter": true, "hash": true, "iconv": true,
	"json": true, "libxml": true, "mbstring": true, "openssl": true, "pcre": true, "pdo": true, "pdo_sqlite": true,
	"phar": true, "posix": true, "readline": true, "reflection": true, "session": true, "simplexml": true,
	"sodium": true, "spl": true, "sqlite3": true, "standard": true, "tokenizer": true, "xml": true,
	"xmlreader": true, "xmlwriter": true, "zlib": true,
}

// packageExts maps Composer packages to the extensions they need.
var packageExts = map[string][]string{
	"intervention/image":          {"gd", "exif"},
	"maatwebsite/excel":           {"gd", "zip"},
	"phpoffice/phpspreadsheet":    {"gd", "zip"},
	"barryvdh/laravel-dompdf":     {"gd"},
	"dompdf/dompdf":               {"gd"},
	"spatie/laravel-medialibrary": {"exif", "gd"},
	"laravel/horizon":             {"pcntl"},
	"laravel/octane":              {"pcntl", "sockets"},
	"php-amqplib/php-amqplib":     {"sockets"},
	"mongodb/laravel-mongodb":     {"mongodb"},
}

func phpExtensions(c composerJSON, laravel bool, db string, redis bool) []string {
	set := map[string]bool{"opcache": true}
	if laravel {
		for _, e := range []string{"bcmath", "intl", "zip", "pcntl"} {
			set[e] = true
		}
	}
	for pkg := range c.Require {
		if strings.HasPrefix(pkg, "ext-") {
			set[strings.ToLower(strings.TrimPrefix(pkg, "ext-"))] = true
		}
		for _, e := range packageExts[pkg] {
			set[e] = true
		}
	}
	switch db {
	case "mysql", "mariadb":
		set["pdo_mysql"] = true
	case "postgres":
		set["pdo_pgsql"] = true
	}
	if redis && c.Require["predis/predis"] == "" {
		set["redis"] = true
	}
	var out []string
	for e := range set {
		if !builtinExts[e] {
			out = append(out, e)
		}
	}
	sort.Strings(out)
	return out
}

func isLaravel(root string, cfg *config.Config) bool {
	for _, a := range cfg.Apps {
		if a.Runtime == config.RuntimePHP && exists(root, filepath.Join(appDir(a), "artisan")) {
			return true
		}
	}
	return false
}

func hasRuntime(cfg *config.Config, rt string) bool {
	return len(cfg.AppsByRuntime(rt)) > 0
}

func phpPath(cfg *config.Config) string {
	if apps := cfg.AppsByRuntime(config.RuntimePHP); len(apps) > 0 {
		return apps[0].Path
	}
	return ""
}

// onlyCompanions: one PHP app plus its Vite companion in the same folder.
func onlyCompanions(cfg *config.Config) bool {
	if len(cfg.Apps) != 2 || !hasRuntime(cfg, config.RuntimePHP) {
		return false
	}
	return cfg.Apps[0].Path == cfg.Apps[1].Path
}

// readEnv reads .env.example, then .env (which wins) for service evidence.
func readEnv(root string) map[string]string {
	env := map[string]string{}
	for _, name := range []string{".env.example", ".env"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		env["__source"] = name
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if ok {
				env[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
	}
	return env
}

func djangoWSGI(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && exists(dir, filepath.Join(e.Name(), "wsgi.py")) {
			return e.Name() + ".wsgi"
		}
	}
	return ""
}

// execForm turns a simple command line into Dockerfile exec form.
func execForm(cmd string) string {
	parts := strings.Fields(cmd)
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func appDir(a config.App) string {
	if a.Path == "" {
		return "."
	}
	return filepath.FromSlash(a.Path)
}

func ctxPath(dir string) string {
	d := filepath.ToSlash(dir)
	if d == "." || d == "" {
		return "."
	}
	return "./" + d
}

func fpmConfName(single bool, app string) string {
	if single {
		return "default.conf"
	}
	return app + ".conf"
}

func svcName(single bool, app string) string {
	if single {
		return "web"
	}
	return app
}

func portVar(single bool, app string) string {
	if single {
		return "APP_PORT"
	}
	return strings.ToUpper(strings.ReplaceAll(slug(app), "-", "_")) + "_PORT"
}

func slug(s string) string {
	if v := project.Slugify(s); v != "" {
		return v
	}
	return "app"
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

func read(root, rel string) string {
	data, _ := os.ReadFile(filepath.Join(root, rel))
	return string(data)
}

func dirHasFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func orPort(p, def int) int {
	if p == 0 {
		return def
	}
	return p
}

func orRoot(dir string) string {
	if dir == "." {
		return "the project"
	}
	return filepath.ToSlash(dir)
}

func orNone(s string) string {
	if s == "" {
		return "no lockfile"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func ifThen(ok bool, s string) string {
	if ok {
		return s
	}
	return ""
}

func versionSource(v, source string) string {
	if v == "" {
		return "SLD default (no version pinned in " + source + ")"
	}
	return source
}
