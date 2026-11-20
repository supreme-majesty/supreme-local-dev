// Package detect inspects a project directory and proposes an SLD config:
// which apps it contains (framework, runtime version, dev command, port) and
// which services (database, Redis, Mailpit) it needs.
package detect

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
)

// ErrNothingDetected means no detector recognised the project.
var ErrNothingDetected = errors.New("could not detect the project's stack")

// Result is a proposed configuration plus human-readable findings.
type Result struct {
	Config *config.Config
	Notes  []string // e.g. "Laravel (PHP 8.3, from composer.json)"
}

// Options adjust detection.
type Options struct {
	// Runtime keeps only apps of this runtime (e.g. when a repo mixes stacks).
	Runtime string
}

// proj is the parsed view of one directory shared by detectors.
type proj struct {
	root   string
	pkg    *packageJSON
	notes  *[]string
	prefix string // e.g. "backend/: " when detecting inside a sub-directory
}

func (p *proj) exists(name string) bool {
	_, err := os.Stat(filepath.Join(p.root, filepath.FromSlash(name)))
	return err == nil
}

func (p *proj) read(name string) string {
	data, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(name)))
	if err != nil {
		return ""
	}
	return string(data)
}

func (p *proj) note(format string, args ...any) {
	*p.notes = append(*p.notes, p.prefix+fmt.Sprintf(format, args...))
}

// Project detects the apps and services of the project at root. When the root
// itself is not an app (a monorepo with backend/, frontend/, ...), each
// first-level sub-directory is detected as its own app with `path:` set.
func Project(root string, opts Options) (*Result, error) {
	name := filepath.Base(root)
	cfg := &config.Config{
		Version: config.CurrentVersion,
		Name:    name,
		Domain:  config.DomainConfig{Name: fmt.Sprintf("%s.%s", project.Slugify(name), config.DefaultTLD())},
	}
	var notes []string

	apps := detectDir(&proj{root: root, pkg: readPackageJSON(root), notes: &notes}, cfg)
	if len(apps) == 0 {
		apps = detectSubdirs(root, cfg, &notes)
	} else {
		detectServicesFromEnv(&proj{root: root, notes: &notes}, cfg)
	}

	if opts.Runtime != "" {
		var kept []config.App
		for _, a := range apps {
			if a.Runtime == opts.Runtime {
				kept = append(kept, a)
			}
		}
		if len(kept) == 0 {
			return nil, fmt.Errorf("%w as runtime %q", ErrNothingDetected, opts.Runtime)
		}
		apps = kept
	}
	if len(apps) == 0 {
		return nil, ErrNothingDetected
	}
	cfg.Apps = apps
	return &Result{Config: cfg, Notes: notes}, nil
}

// detectDir runs the detector chain on one directory. Order matters: a PHP
// project with a package.json is a PHP app plus a Vite companion, not a Node
// app; a Dockerfile or static files are fallbacks.
func detectDir(p *proj, cfg *config.Config) []config.App {
	for _, detector := range []func(*proj, *config.Config) []config.App{
		detectPHP, detectNode, detectPython, detectRuby, detectGo,
	} {
		if found := detector(p, cfg); len(found) > 0 {
			return found
		}
	}
	if found := detectDockerfile(p); len(found) > 0 {
		return found
	}
	return detectStatic(p)
}

// skipDirs are never apps.
var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "docs": true, "doc": true, "documentation": true,
	"docker": true, "deploy": true, "deployment": true, "bin": true, "scripts": true,
	"assets": true, "audit": true, "agents": true, "infra": true, "terraform": true,
	"k8s": true, "helm": true, "tests": true, "test": true, "storage": true,
}

// frontFirst lists directory names that usually hold the user-facing app; they
// are ordered first so they own the project's root domain.
var frontFirst = []string{"frontend", "web", "client", "ui", "app", "site", "www"}

func detectSubdirs(root string, cfg *config.Config, notes *[]string) []config.App {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !skipDirs[strings.ToLower(e.Name())] {
			dirs = append(dirs, e.Name())
		}
	}
	sort.SliceStable(dirs, func(i, j int) bool {
		return frontRank(dirs[i]) < frontRank(dirs[j])
	})

	var apps []config.App
	used := map[string]bool{}
	for _, dir := range dirs {
		sub := filepath.Join(root, dir)
		p := &proj{root: sub, pkg: readPackageJSON(sub), notes: notes, prefix: dir + "/: "}
		if kind := mobileKind(p); kind != "" {
			p.note("skipped — %s app (not served over HTTP)", kind)
			continue
		}
		found := detectDir(p, cfg)
		if len(found) == 0 {
			continue
		}
		detectServicesFromEnv(p, cfg)
		base := appName(dir)
		for _, app := range found {
			name := base
			if app.Name != "web" {
				name = truncateName(base + "-" + app.Name)
			}
			for i := 2; used[name]; i++ {
				name = truncateName(fmt.Sprintf("%s%d", base, i))
			}
			used[name] = true
			app.Name = name
			app.Path = filepath.ToSlash(dir)
			apps = append(apps, app)
		}
	}
	return apps
}

func frontRank(dir string) int {
	for i, name := range frontFirst {
		if strings.EqualFold(dir, name) {
			return i
		}
	}
	return len(frontFirst)
}

// mobileKind recognises mobile app directories, which SLD does not serve.
func mobileKind(p *proj) string {
	switch {
	case p.exists("pubspec.yaml"):
		return "Flutter"
	case p.pkg != nil && p.pkg.has("expo"):
		return "Expo"
	case p.pkg != nil && p.pkg.has("react-native"):
		return "React Native"
	case p.exists("android/build.gradle") && p.exists("ios"):
		return "native mobile"
	}
	return ""
}

// appName turns a directory name into a valid app name.
func appName(dir string) string {
	name := project.Slugify(dir)
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		name = "app-" + name
	}
	return truncateName(name)
}

func truncateName(name string) string {
	if len(name) > 20 {
		name = strings.TrimRight(name[:20], "-")
	}
	return name
}

// --- PHP ---

func detectPHP(p *proj, cfg *config.Config) []config.App {
	var composer struct {
		Require map[string]string `json:"require"`
	}
	hasComposer := json.Unmarshal([]byte(p.read("composer.json")), &composer) == nil && p.exists("composer.json")
	isWordPress := p.exists("wp-config.php") || p.exists("wp-config-sample.php")
	if !hasComposer && !p.exists("artisan") && !isWordPress && !p.exists("index.php") && !p.exists("public/index.php") {
		return nil
	}

	web := config.App{Name: "web", Runtime: config.RuntimePHP, Version: "8.3"}
	versionSource := "default"
	if c, ok := composer.Require["php"]; ok {
		if v, err := config.ResolvePHPConstraint(c); err == nil {
			web.Version, versionSource = v, "composer.json "+c
		} else {
			p.note("⚠️  %v — using PHP %s", err, web.Version)
		}
	}

	framework := "PHP"
	switch {
	case composer.Require["laravel/framework"] != "" || p.exists("artisan"):
		framework, web.WebRoot = "Laravel", "public"
		if cfg.Services.Database.Type == "" {
			cfg.Services.Database = config.DatabaseConfig{Type: "mysql", Version: "8.0"}
		}
		cfg.Services.Redis.Enabled = true
		cfg.Services.Mailpit.Enabled = true
	case composer.Require["symfony/framework-bundle"] != "":
		framework, web.WebRoot = "Symfony", "public"
	case composer.Require["roots/wordpress"] != "":
		framework, web.WebRoot = "WordPress (Bedrock)", "web"
		cfg.Services.Database = config.DatabaseConfig{Type: "mysql", Version: "8.0"}
	case isWordPress:
		framework, web.WebRoot = "WordPress", "."
		cfg.Services.Database = config.DatabaseConfig{Type: "mysql", Version: "8.0"}
	case p.exists("public/index.php"):
		web.WebRoot = "public"
	default:
		web.WebRoot = "."
	}
	p.note("%s (PHP %s, from %s), web root %s", framework, web.Version, versionSource, web.WebRoot)

	apps := []config.App{web}
	// Laravel/Symfony front-end tooling runs next to PHP as a Vite dev server.
	if p.pkg != nil && strings.Contains(p.pkg.Scripts["dev"], "vite") {
		vite := config.App{
			Name: "vite", Runtime: config.RuntimeNode, Version: nodeVersion(p),
			PackageManager: packageManager(p), Port: 5173, ExposePort: 5173,
		}
		vite.Command = runScript(vite.PackageManager, "dev", "--host 0.0.0.0")
		if vite.PackageManager == "npm" {
			vite.PackageManager = ""
		}
		p.note("Vite dev server (node %s) on port 5173", displayVersion(vite.Version))
		apps = append(apps, vite)
	}
	return apps
}

// --- Node ---

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         struct {
		Node string `json:"node"`
	} `json:"engines"`
}

func (pkg *packageJSON) has(dep string) bool {
	return pkg.Dependencies[dep] != "" || pkg.DevDependencies[dep] != ""
}

func readPackageJSON(root string) *packageJSON {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil
	}
	var pkg packageJSON
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	return &pkg
}

// nodeFramework describes how to run a Node framework's dev server.
type nodeFramework struct {
	name   string
	deps   []string
	script string // npm script to run
	args   string // arguments appended to the script; {port} is replaced
	port   int
}

var nodeFrameworks = []nodeFramework{
	{"Next.js", []string{"next"}, "dev", "-H 0.0.0.0 -p {port}", 3000},
	{"Nuxt", []string{"nuxt"}, "dev", "--host 0.0.0.0 --port {port}", 3000},
	{"Astro", []string{"astro"}, "dev", "--host 0.0.0.0 --port {port}", 4321},
	{"SvelteKit", []string{"@sveltejs/kit"}, "dev", "--host 0.0.0.0 --port {port}", 5173},
	{"Remix", []string{"@remix-run/dev"}, "dev", "--host 0.0.0.0 --port {port}", 5173},
	{"NestJS", []string{"@nestjs/core"}, "start:dev", "", 3000},
	{"Vite", []string{"vite"}, "dev", "--host 0.0.0.0 --port {port}", 5173},
	{"Express", []string{"express"}, "dev", "", 3000},
	{"Fastify", []string{"fastify"}, "dev", "", 3000},
	{"Hono", []string{"hono"}, "dev", "", 3000},
	{"Koa", []string{"koa"}, "dev", "", 3000},
}

func detectNode(p *proj, cfg *config.Config) []config.App {
	if p.pkg == nil {
		return nil
	}
	pm := packageManager(p)
	app := config.App{Name: "web", Runtime: config.RuntimeNode, Version: nodeVersion(p), PackageManager: pm}

	fw := nodeFramework{name: "Node.js", script: "dev", port: 3000}
	for _, candidate := range nodeFrameworks {
		matched := false
		for _, dep := range candidate.deps {
			if p.pkg.has(dep) {
				matched = true
			}
		}
		if matched {
			fw = candidate
			break
		}
	}
	// Fall back to whichever script exists.
	if p.pkg.Scripts[fw.script] == "" {
		switch {
		case p.pkg.Scripts["dev"] != "":
			fw.script, fw.args = "dev", ""
		case p.pkg.Scripts["start"] != "":
			fw.script, fw.args = "start", ""
		case p.prefix != "":
			// Inside a monorepo, a package with nothing to run is a shared library.
			p.note("skipped — package without a dev or start script (shared library?)")
			return nil
		default:
			p.note("⚠️  package.json has no dev or start script; set apps.web.command")
		}
	}

	app.Port = fw.port
	app.Command = runScript(pm, fw.script, strings.ReplaceAll(fw.args, "{port}", fmt.Sprint(fw.port)))
	if pm == "npm" {
		app.PackageManager = "" // npm is the default; keep the file minimal
	}
	if fw.args == "" && fw.script != "" {
		p.note("%s: the server must listen on 0.0.0.0 and read PORT (SLD sets PORT=%d)", fw.name, fw.port)
	}
	p.note("%s (node %s, %s) on port %d", fw.name, displayVersion(app.Version), pm, app.Port)

	detectServicesFromDeps(p, cfg, p.pkg.Dependencies, map[string]string{
		"pg": "postgres", "postgres": "postgres", "mysql2": "mysql", "mysql": "mysql",
		"ioredis": "redis", "redis": "redis", "@prisma/client": "",
	})
	return []config.App{app}
}

func packageManager(p *proj) string {
	switch {
	case p.exists("pnpm-lock.yaml"):
		return "pnpm"
	case p.exists("yarn.lock"):
		return "yarn"
	case p.exists("bun.lockb") || p.exists("bun.lock"):
		p.note("⚠️  bun lockfile found; SLD runs bun projects with npm for now")
	}
	return "npm"
}

// runScript builds the command that runs a package.json script with extra args.
func runScript(pm, script, args string) string {
	var cmd string
	switch pm {
	case "pnpm":
		cmd = "corepack pnpm run " + script
	case "yarn":
		cmd = "corepack yarn run " + script
	default:
		cmd = "npm run " + script
		if args != "" {
			cmd += " --"
		}
	}
	if args != "" {
		cmd += " " + args
	}
	return cmd
}

var majorVersion = regexp.MustCompile(`(\d+)`)

func nodeVersion(p *proj) string {
	for _, f := range []string{".nvmrc", ".node-version"} {
		if v := strings.TrimSpace(p.read(f)); v != "" {
			if m := majorVersion.FindString(strings.TrimPrefix(v, "v")); m != "" {
				return m
			}
		}
	}
	if p.pkg != nil && p.pkg.Engines.Node != "" {
		if m := majorVersion.FindString(p.pkg.Engines.Node); m != "" {
			return m
		}
	}
	return "" // driver default
}

// --- Python ---

func detectPython(p *proj, cfg *config.Config) []config.App {
	deps := strings.ToLower(p.read("requirements.txt") + "\n" + p.read("pyproject.toml") + "\n" + p.read("Pipfile"))
	if !p.exists("manage.py") && strings.TrimSpace(deps) == "" {
		return nil
	}
	app := config.App{Name: "web", Runtime: config.RuntimePython, Version: pythonVersion(p), Port: 8000}

	switch {
	case p.exists("manage.py"):
		app.Command = "python manage.py runserver 0.0.0.0:8000"
		p.note("Django on port 8000")
	case strings.Contains(deps, "fastapi"):
		module := findASGIApp(p, "FastAPI(")
		app.Command = fmt.Sprintf("uvicorn %s --host 0.0.0.0 --port 8000 --reload", module)
		p.note("FastAPI (%s) on port 8000", module)
	case strings.Contains(deps, "flask"):
		app.Command = "flask run --host 0.0.0.0 --port 8000 --debug"
		p.note("Flask on port 8000 (set FLASK_APP in environment if it is not app.py/wsgi.py)")
	default:
		p.note("⚠️  Python project without a recognised framework; set apps.web.command and port")
		app.Command = "python main.py"
	}

	detectServicesFromText(p, cfg, deps, map[string]string{
		"psycopg": "postgres", "asyncpg": "postgres", "mysqlclient": "mysql", "pymysql": "mysql", "redis": "redis",
	})
	return []config.App{app}
}

func pythonVersion(p *proj) string {
	if v := strings.TrimSpace(p.read(".python-version")); v != "" {
		if m := regexp.MustCompile(`^(\d+\.\d+)`).FindString(v); m != "" {
			return m
		}
	}
	return ""
}

// findASGIApp locates "module:app" for an ASGI app constructed with marker.
func findASGIApp(p *proj, marker string) string {
	for _, candidate := range []string{"main.py", "app.py", "app/main.py", "src/main.py", "api/main.py"} {
		content := p.read(candidate)
		if !strings.Contains(content, marker) {
			continue
		}
		module := strings.ReplaceAll(strings.TrimSuffix(candidate, ".py"), "/", ".")
		varName := "app"
		if m := regexp.MustCompile(`(?m)^(\w+)\s*=\s*` + regexp.QuoteMeta(marker)).FindStringSubmatch(content); m != nil {
			varName = m[1]
		}
		return module + ":" + varName
	}
	return "main:app"
}

// --- Ruby ---

func detectRuby(p *proj, cfg *config.Config) []config.App {
	gemfile := strings.ToLower(p.read("Gemfile"))
	if gemfile == "" {
		return nil
	}
	app := config.App{Name: "web", Runtime: config.RuntimeRuby, Version: rubyVersion(p)}
	switch {
	case strings.Contains(gemfile, "'rails'") || strings.Contains(gemfile, `"rails"`):
		app.Command, app.Port = "bin/rails server -b 0.0.0.0 -p 3000", 3000
		p.note("Rails on port 3000")
	case strings.Contains(gemfile, "sinatra"):
		app.Command, app.Port = "bundle exec rackup -o 0.0.0.0 -p 9292", 9292
		p.note("Sinatra on port 9292")
	default:
		app.Command, app.Port = "bundle exec rackup -o 0.0.0.0 -p 9292", 9292
		p.note("Rack app on port 9292")
	}
	if strings.Contains(p.read("Gemfile.lock"), "nokogiri") || strings.Contains(gemfile, "pg") || strings.Contains(gemfile, "mysql2") {
		p.note("⚠️  gems with native extensions may need build tools; if install fails, use runtime: dockerfile")
	}
	detectServicesFromText(p, cfg, gemfile, map[string]string{"'pg'": "postgres", `"pg"`: "postgres", "mysql2": "mysql", "redis": "redis"})
	return []config.App{app}
}

func rubyVersion(p *proj) string {
	if v := strings.TrimSpace(p.read(".ruby-version")); v != "" {
		if m := regexp.MustCompile(`(\d+\.\d+)`).FindString(v); m != "" {
			return m
		}
	}
	return ""
}

// --- Go ---

func detectGo(p *proj, cfg *config.Config) []config.App {
	gomod := p.read("go.mod")
	if gomod == "" {
		return nil
	}
	app := config.App{Name: "web", Runtime: config.RuntimeGo, Port: 8080, Command: "go run ."}
	if m := regexp.MustCompile(`(?m)^go\s+(\d+\.\d+)`).FindStringSubmatch(gomod); m != nil {
		app.Version = m[1]
	}
	if p.exists(".air.toml") {
		app.Command = "go run github.com/air-verse/air@latest"
		p.note("Go (%s) with Air live reload on port 8080 — the server must read PORT", displayVersion(app.Version))
	} else {
		p.note("Go (%s) on port 8080 — the server must listen on 0.0.0.0 and read PORT", displayVersion(app.Version))
	}
	detectServicesFromText(p, cfg, gomod, map[string]string{
		"jackc/pgx": "postgres", "lib/pq": "postgres", "go-sql-driver/mysql": "mysql", "redis/go-redis": "redis",
	})
	return []config.App{app}
}

// --- Fallbacks ---

func detectDockerfile(p *proj) []config.App {
	content := p.read("Dockerfile")
	if content == "" {
		return nil
	}
	port := 8080
	if m := regexp.MustCompile(`(?mi)^\s*EXPOSE\s+(\d+)`).FindStringSubmatch(content); m != nil {
		fmt.Sscan(m[1], &port)
	}
	p.note("Dockerfile, serving port %d", port)
	return []config.App{{Name: "web", Runtime: config.RuntimeDockerfile, Port: port}}
}

func detectStatic(p *proj) []config.App {
	for _, dir := range []string{".", "public", "dist", "build", "site"} {
		if p.exists(filepath.ToSlash(filepath.Join(dir, "index.html"))) {
			p.note("static site in %s", dir)
			return []config.App{{Name: "web", Runtime: config.RuntimeStatic, WebRoot: dir}}
		}
	}
	return nil
}

// --- Services ---

func detectServicesFromDeps(p *proj, cfg *config.Config, deps map[string]string, mapping map[string]string) {
	for dep, service := range mapping {
		if deps[dep] != "" && service != "" {
			applyService(p, cfg, service, dep)
		}
	}
}

func detectServicesFromText(p *proj, cfg *config.Config, text string, mapping map[string]string) {
	for needle, service := range mapping {
		if strings.Contains(text, needle) {
			applyService(p, cfg, service, strings.Trim(needle, `'"`))
		}
	}
}

func applyService(p *proj, cfg *config.Config, service, source string) {
	switch service {
	case "redis":
		if !cfg.Services.Redis.Enabled {
			cfg.Services.Redis.Enabled = true
			p.note("Redis (uses %s)", source)
		}
	case "postgres", "mysql", "mariadb":
		if cfg.Services.Database.Type == "" {
			cfg.Services.Database = config.DatabaseConfig{Type: service}
			p.note("%s database (uses %s)", service, source)
		}
	}
}

// detectServicesFromEnv reads .env (or .env.example) for database and cache
// settings. Values found here override guesses from dependencies.
func detectServicesFromEnv(p *proj, cfg *config.Config) {
	content := p.read(".env")
	source := ".env"
	if content == "" {
		content, source = p.read(".env.example"), ".env.example"
	}
	if content == "" {
		return
	}
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), `"'`)

		switch key {
		case "DB_CONNECTION":
			switch value {
			case "mysql":
				cfg.Services.Database = config.DatabaseConfig{Type: "mysql", Version: "8.0"}
			case "mariadb":
				cfg.Services.Database = config.DatabaseConfig{Type: "mariadb", Version: "10.11"}
			case "pgsql":
				cfg.Services.Database = config.DatabaseConfig{Type: "postgres", Version: "16"}
			case "sqlite":
				cfg.Services.Database = config.DatabaseConfig{}
				p.note("SQLite database (no database container, from %s)", source)
			}
		case "DATABASE_URL":
			switch {
			case strings.HasPrefix(value, "postgres"):
				cfg.Services.Database = config.DatabaseConfig{Type: "postgres"}
			case strings.HasPrefix(value, "mysql"):
				cfg.Services.Database = config.DatabaseConfig{Type: "mysql"}
			}
		case "REDIS_URL", "CACHE_DRIVER", "CACHE_STORE", "SESSION_DRIVER", "QUEUE_CONNECTION":
			if key == "REDIS_URL" || value == "redis" {
				cfg.Services.Redis.Enabled = true
			}
		}
	}
}

func displayVersion(v string) string {
	if v == "" {
		return "default"
	}
	return v
}
