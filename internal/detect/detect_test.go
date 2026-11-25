package detect

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
)

func fixture(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

type want struct {
	apps     string // Describe() output
	command  string // first app's command (substring)
	port     int    // first app's port
	webRoot  string
	db       string
	redis    bool
	mailpit  bool
	pkgMgr   string
	noteLike string
}

func TestProject(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	cases := []struct {
		name  string
		files map[string]string
		want  want
	}{
		{
			name: "laravel with vite",
			files: map[string]string{
				"artisan":       "#!/usr/bin/env php",
				"composer.json": `{"require":{"php":"^8.2","laravel/framework":"^11.0"}}`,
				"package.json":  `{"scripts":{"dev":"vite","build":"vite build"},"devDependencies":{"vite":"^5"}}`,
				".nvmrc":        "v20.11.0\n",
				".env":          "DB_CONNECTION=pgsql\nCACHE_STORE=redis\n",
			},
			want: want{apps: "web (php 8.2), vite (node 20)", webRoot: "public", db: "postgres", redis: true, mailpit: true, noteLike: "Laravel"},
		},
		{
			name: "laravel 11 sqlite default",
			files: map[string]string{
				"artisan":       "",
				"composer.json": `{"require":{"php":"^8.3","laravel/framework":"^11.0"}}`,
				".env.example":  "DB_CONNECTION=sqlite\n",
			},
			want: want{apps: "web (php 8.3)", webRoot: "public", db: "", redis: true, mailpit: true},
		},
		{
			name:  "wordpress",
			files: map[string]string{"wp-config-sample.php": "<?php", "index.php": "<?php"},
			want:  want{apps: "web (php 8.3)", webRoot: ".", db: "mysql", noteLike: "WordPress"},
		},
		{
			name:  "plain php with public dir",
			files: map[string]string{"public/index.php": "<?php echo 1;"},
			want:  want{apps: "web (php 8.3)", webRoot: "public"},
		},
		{
			name: "next.js with pnpm",
			files: map[string]string{
				"package.json":   `{"scripts":{"dev":"next dev"},"dependencies":{"next":"15","react":"19","pg":"8"},"engines":{"node":">=22"}}`,
				"pnpm-lock.yaml": "lockfileVersion: 9",
			},
			want: want{apps: "web (node 22)", command: "corepack pnpm run dev -H 0.0.0.0 -p 3000", port: 3000, db: "postgres", pkgMgr: "pnpm", noteLike: "Next.js"},
		},
		{
			name:  "vite spa with npm",
			files: map[string]string{"package.json": `{"scripts":{"dev":"vite"},"devDependencies":{"vite":"6"}}`, "package-lock.json": "{}"},
			want:  want{command: "npm run dev -- --host 0.0.0.0 --port 5173", port: 5173, noteLike: "Vite"},
		},
		{
			name:  "astro",
			files: map[string]string{"package.json": `{"scripts":{"dev":"astro dev"},"dependencies":{"astro":"5"}}`},
			want:  want{command: "--port 4321", port: 4321, noteLike: "Astro"},
		},
		{
			name:  "nestjs",
			files: map[string]string{"package.json": `{"scripts":{"start:dev":"nest start --watch"},"dependencies":{"@nestjs/core":"10","ioredis":"5"}}`},
			want:  want{command: "npm run start:dev", port: 3000, redis: true, noteLike: "NestJS"},
		},
		{
			name:  "express without dev script falls back to start",
			files: map[string]string{"package.json": `{"scripts":{"start":"node server.js"},"dependencies":{"express":"4"}}`},
			want:  want{command: "npm run start", port: 3000, noteLike: "read PORT"},
		},
		{
			name:  "django",
			files: map[string]string{"manage.py": "", "requirements.txt": "Django==5.1\npsycopg[binary]\n", ".python-version": "3.12.4\n"},
			want:  want{apps: "web (python 3.12)", command: "manage.py runserver 0.0.0.0:8000", port: 8000, db: "postgres", noteLike: "Django"},
		},
		{
			name: "fastapi in app package",
			files: map[string]string{
				"pyproject.toml": "[project]\ndependencies = [\"fastapi\", \"uvicorn\", \"redis\"]\n",
				"app/main.py":    "from fastapi import FastAPI\napi = FastAPI()\n",
			},
			want: want{apps: "web (python 3.13)", command: "uvicorn app.main:api --host 0.0.0.0 --port 8000 --reload", port: 8000, redis: true},
		},
		{
			name:  "flask",
			files: map[string]string{"requirements.txt": "flask\nPyMySQL\n"},
			want:  want{command: "flask run --host 0.0.0.0", port: 8000, db: "mysql"},
		},
		{
			name:  "rails",
			files: map[string]string{"Gemfile": "source 'https://rubygems.org'\ngem 'rails', '~> 7.2'\ngem 'pg'\n", ".ruby-version": "ruby-3.3.5\n"},
			want:  want{apps: "web (ruby 3.3)", command: "bin/rails server -b 0.0.0.0 -p 3000", port: 3000, db: "postgres", noteLike: "Rails on port 3000"},
		},
		{
			name:  "go with air",
			files: map[string]string{"go.mod": "module example.com/api\n\ngo 1.24\n\nrequire github.com/jackc/pgx/v5 v5.7.0\n", ".air.toml": ""},
			want:  want{apps: "web (go 1.24)", command: "air", port: 8080, db: "postgres"},
		},
		{
			name:  "dockerfile with expose",
			files: map[string]string{"Dockerfile": "FROM caddy:2\nEXPOSE 2015\n"},
			want:  want{apps: "web (dockerfile)", port: 2015},
		},
		{
			name:  "static site in dist",
			files: map[string]string{"dist/index.html": "<h1>hi</h1>"},
			want:  want{apps: "web (static)", webRoot: "dist"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Project(fixture(t, "my-app", tc.files), Options{})
			if err != nil {
				t.Fatalf("Project: %v", err)
			}
			// The proposal must be a valid config once defaults are applied.
			out, err := config.Marshal(res.Config)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.LoadFromBytes(out, "my-app")
			if err != nil {
				t.Fatalf("detected config does not validate: %v\n%s", err, out)
			}
			app := cfg.Apps[0]
			w := tc.want
			if w.apps != "" && cfg.Describe() != w.apps {
				t.Errorf("apps = %q, want %q", cfg.Describe(), w.apps)
			}
			if w.command != "" && !strings.Contains(app.Command, w.command) {
				t.Errorf("command = %q, want it to contain %q", app.Command, w.command)
			}
			if w.port != 0 && app.Port != w.port {
				t.Errorf("port = %d, want %d", app.Port, w.port)
			}
			if w.webRoot != "" && app.WebRoot != w.webRoot {
				t.Errorf("web_root = %q, want %q", app.WebRoot, w.webRoot)
			}
			if cfg.Services.Database.Type != w.db {
				t.Errorf("database = %q, want %q", cfg.Services.Database.Type, w.db)
			}
			if cfg.Services.Redis.Enabled != w.redis {
				t.Errorf("redis = %v, want %v", cfg.Services.Redis.Enabled, w.redis)
			}
			if cfg.Services.Mailpit.Enabled != w.mailpit {
				t.Errorf("mailpit = %v, want %v", cfg.Services.Mailpit.Enabled, w.mailpit)
			}
			if w.pkgMgr != "" && app.PackageManager != w.pkgMgr {
				t.Errorf("package_manager = %q, want %q", app.PackageManager, w.pkgMgr)
			}
			if w.noteLike != "" && !strings.Contains(strings.Join(res.Notes, "\n"), w.noteLike) {
				t.Errorf("notes %q do not mention %q", res.Notes, w.noteLike)
			}
			if cfg.Domain.Name != "my-app.localhost" {
				t.Errorf("domain = %q", cfg.Domain.Name)
			}
		})
	}
}

func TestProject_NothingDetected(t *testing.T) {
	_, err := Project(fixture(t, "empty", map[string]string{"README.md": "hi"}), Options{})
	if !errors.Is(err, ErrNothingDetected) {
		t.Fatalf("err = %v, want ErrNothingDetected", err)
	}
}

func TestProject_RuntimeFilter(t *testing.T) {
	root := fixture(t, "mixed", map[string]string{
		"composer.json": `{"require":{"php":"^8.3","laravel/framework":"^11"}}`,
		"package.json":  `{"scripts":{"dev":"vite"}}`,
	})
	res, err := Project(root, Options{Runtime: config.RuntimeNode})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Config.Apps) != 1 || res.Config.Apps[0].Runtime != config.RuntimeNode {
		t.Errorf("runtime filter kept %+v", res.Config.Apps)
	}
	if _, err := Project(root, Options{Runtime: config.RuntimeGo}); !errors.Is(err, ErrNothingDetected) {
		t.Errorf("filtering to an absent runtime should fail, got %v", err)
	}
}

func TestRunScript(t *testing.T) {
	cases := map[string]string{
		runScript("npm", "dev", "--host 0.0.0.0"): "npm run dev -- --host 0.0.0.0",
		runScript("npm", "start", ""):             "npm run start",
		runScript("pnpm", "dev", "--port 1"):      "corepack pnpm run dev --port 1",
		runScript("yarn", "start:dev", ""):        "corepack yarn run start:dev",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestProject_Monorepo(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	root := fixture(t, "smart-docx", map[string]string{
		"README.md":             "# monorepo",
		"docker-compose.yml":    "services: {}",
		"docs/index.html":       "<h1>docs are skipped</h1>",
		"backend/composer.json": `{"require":{"php":"^7.3|^8.2","laravel/framework":"^10"}}`,
		"backend/package.json":  `{"scripts":{"dev":"vite"}}`,
		"backend/.env":          "DB_CONNECTION=pgsql\n",
		"frontend/package.json": `{"scripts":{"dev":"vite"},"devDependencies":{"vite":"6"}}`,
		"frontend/index.html":   "<div id=app></div>",
		"driver/package.json":   `{"dependencies":{"react-native":"0.76"}}`,
		"driver/Gemfile":        "source 'https://rubygems.org'\ngem 'cocoapods'\n",
		"mobile/pubspec.yaml":   "name: app",
		"shared/package.json":   `{"name":"@x/shared","main":"index.js"}`,
		"2fa-service/go.mod":    "module x\n\ngo 1.24\n",
	})
	res, err := Project(root, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	out, err := config.Marshal(res.Config)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromBytes(out, "smart-docx")
	if err != nil {
		t.Fatalf("monorepo config does not validate: %v\n%s", err, out)
	}

	if got := cfg.Describe(); got != "frontend (node 22), app-2fa-service (go 1.24), backend (php 8.2), backend-vite (node 22)" {
		t.Errorf("apps = %s\n%s", got, out)
	}
	paths := map[string]string{}
	for _, a := range cfg.Apps {
		paths[a.Name] = a.Path
	}
	if paths["frontend"] != "frontend" || paths["backend"] != "backend" || paths["backend-vite"] != "backend" || paths["app-2fa-service"] != "2fa-service" {
		t.Errorf("paths = %v", paths)
	}
	if cfg.PrimaryApp().Name != "frontend" || cfg.PrimaryApp().Domain != "smart-docx.localhost" {
		t.Errorf("frontend should own the root domain, primary = %s (%s)", cfg.PrimaryApp().Name, cfg.PrimaryApp().Domain)
	}
	if b, _ := cfg.App("backend"); b.Domain != "backend.smart-docx.localhost" {
		t.Errorf("backend domain = %s", b.Domain)
	}
	if cfg.Services.Database.Type != "postgres" {
		t.Errorf("backend/.env should set postgres, got %q", cfg.Services.Database.Type)
	}
	notes := strings.Join(res.Notes, "\n")
	for _, want := range []string{"driver/: skipped — React Native", "mobile/: skipped — Flutter", "backend/: Laravel", "shared/: skipped — package without"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes missing %q:\n%s", want, notes)
		}
	}
}
