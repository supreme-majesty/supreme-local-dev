package dockerize

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/detect"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type composeDoc struct {
	Name     string `yaml:"name"`
	Services map[string]struct {
		Image    string   `yaml:"image"`
		Ports    []string `yaml:"ports"`
		Profiles []string `yaml:"profiles"`
		Build    *struct {
			Context string `yaml:"context"`
			Target  string `yaml:"target"`
		} `yaml:"build"`
	} `yaml:"services"`
	Volumes map[string]any `yaml:"volumes"`
}

func generate(t *testing.T, root string) (*Plan, composeDoc) {
	t.Helper()
	plan, err := PlanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Write(); err != nil {
		t.Fatal(err)
	}
	var doc composeDoc
	data, err := os.ReadFile(filepath.Join(root, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("compose.yaml is not valid YAML: %v\n%s", err, data)
	}
	// The generated stack must be picked up as a compose project by SLD.
	res, err := detect.Project(root, detect.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.Apps[0].Runtime != config.RuntimeCompose {
		t.Fatalf("not detected as compose: %+v", res.Config.Apps)
	}
	return plan, doc
}

func TestLaravel(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shop")
	writeTree(t, root, map[string]string{
		"artisan":               "#!/usr/bin/env php",
		"composer.json":         `{"require":{"php":"^8.2","laravel/framework":"^11.0","ext-gd":"*","maatwebsite/excel":"^3"}}`,
		"composer.lock":         "{}",
		"public/index.php":      "<?php",
		"bootstrap/app.php":     "->withRouting(health: '/up')",
		"routes/console.php":    "Schedule::command('x')->daily();",
		"app/Jobs/SendMail.php": "<?php",
		".env.example":          "DB_CONNECTION=mysql\nCACHE_STORE=redis\nQUEUE_CONNECTION=database\n",
		"package.json":          `{"scripts":{"dev":"vite","build":"vite build"},"devDependencies":{"vite":"^5"}}`,
		"package-lock.json":     "{}",
	})
	plan, doc := generate(t, root)

	for _, s := range []string{"app", "web", "db", "redis", "worker", "scheduler", "assets"} {
		if _, ok := doc.Services[s]; !ok {
			t.Errorf("missing service %s: %v", s, plan.Services)
		}
	}
	if doc.Services["db"].Image != "mysql:8.4" {
		t.Errorf("db image = %s", doc.Services["db"].Image)
	}
	if len(doc.Services["db"].Ports) != 0 || len(doc.Services["redis"].Ports) != 0 {
		t.Error("databases must not be published")
	}
	if p := doc.Services["web"].Ports; len(p) != 1 || !strings.HasPrefix(p[0], "127.0.0.1:${APP_PORT:-8080}:80") {
		t.Errorf("web ports = %v", p)
	}
	if got := doc.Services["assets"].Profiles; len(got) != 1 || got[0] != "assets" {
		t.Errorf("assets must be on demand: %v", got)
	}
	df := read(root, "Dockerfile")
	for _, want := range []string{"php:${PHP_VERSION}-fpm-bookworm", "ARG PHP_VERSION=8.2", "AS dev", "AS prod", "pdo_mysql", "redis", "gd", "zip", "USER www-data", "composer install --no-dev"} {
		if !strings.Contains(df, want) {
			t.Errorf("Dockerfile lacks %q:\n%s", want, df)
		}
	}
	if strings.Contains(df, "latest") {
		t.Error("no latest tags")
	}
	conf := read(root, "docker/nginx/default.conf")
	if !strings.Contains(conf, "root /var/www/html/public;") || !strings.Contains(conf, "fastcgi_pass app:9000;") {
		t.Errorf("nginx conf:\n%s", conf)
	}
	report := read(root, "DOCKER.md")
	for _, want := range []string{"laravel-web", "Effective configuration", "GET /up", "down -v"} {
		if !strings.Contains(report, want) {
			t.Errorf("DOCKER.md lacks %q", want)
		}
	}
	// .env.example existed: it is left alone.
	if read(root, ".env.example") != "DB_CONNECTION=mysql\nCACHE_STORE=redis\nQUEUE_CONNECTION=database\n" {
		t.Error(".env.example was changed")
	}
}

func TestLaravelWithoutEvidenceAddsNoServices(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"artisan":          "",
		"composer.json":    `{"require":{"php":"^8.3","laravel/framework":"^11.0"}}`,
		"public/index.php": "<?php",
	})
	_, doc := generate(t, root)
	for _, s := range []string{"db", "redis", "worker", "scheduler"} {
		if _, ok := doc.Services[s]; ok {
			t.Errorf("%s added without evidence", s)
		}
	}
}

func TestNextJS(t *testing.T) {
	root := filepath.Join(t.TempDir(), "site")
	writeTree(t, root, map[string]string{
		"package.json":   `{"scripts":{"dev":"next dev","build":"next build","start":"next start"},"dependencies":{"next":"15","pg":"8"}}`,
		"pnpm-lock.yaml": "",
		".nvmrc":         "20",
	})
	plan, doc := generate(t, root)
	if plan.Profile != "nextjs" {
		t.Errorf("profile = %s", plan.Profile)
	}
	if doc.Services["web"].Build == nil || doc.Services["web"].Build.Target != "dev" {
		t.Errorf("web build: %+v", doc.Services["web"].Build)
	}
	if doc.Services["db"].Image != "postgres:16" {
		t.Errorf("pg dependency should add postgres: %v", doc.Services)
	}
	df := read(root, "Dockerfile")
	for _, want := range []string{"node:${NODE_VERSION}-bookworm-slim", "ARG NODE_VERSION=20", "corepack pnpm install --frozen-lockfile", "USER node", "AS prod", `"corepack", "pnpm", "run", "start"`} {
		if !strings.Contains(df, want) {
			t.Errorf("Dockerfile lacks %q:\n%s", want, df)
		}
	}
	if !strings.Contains(read(root, ".env.example"), "DOCKER_DB_PASSWORD=") {
		t.Error(".env.example should list the compose variables")
	}
}

func TestViteSPA(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"package.json":      `{"scripts":{"dev":"vite","build":"vite build"},"devDependencies":{"vite":"5","react":"18"}}`,
		"package-lock.json": "{}",
	})
	plan, _ := generate(t, root)
	if plan.Profile != "react-vite" {
		t.Errorf("profile = %s", plan.Profile)
	}
	if !strings.Contains(read(root, "Dockerfile"), "nginxinc/nginx-unprivileged") || read(root, "docker/nginx/spa.conf") == "" {
		t.Error("vite SPA should be served statically in production")
	}
}

func TestRefusesExistingFiles(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"index.html": "<h1>x</h1>",
		"DOCKER.md":  "mine",
	})
	plan, err := PlanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Write(); !errors.Is(err, ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
	if exists(root, "compose.yaml") || read(root, "DOCKER.md") != "mine" {
		t.Error("nothing may be written when a file exists")
	}
}

func TestAlreadyDockerized(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"compose.yaml": "services:\n  web:\n    image: nginx\n    ports: [\"80:80\"]\n"})
	if _, err := PlanProject(root); !errors.Is(err, ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
}
