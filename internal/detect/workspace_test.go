package detect

import (
	"strings"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
)

func TestProject_PnpmWorkspace(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	root := fixture(t, "logistics-os", map[string]string{
		"package.json":             `{"name":"root","private":true,"scripts":{"dev":"turbo dev"},"devDependencies":{"turbo":"2"}}`,
		"pnpm-lock.yaml":           "lockfileVersion: '9.0'\n",
		"pnpm-workspace.yaml":      "packages:\n  - apps/*\n  - packages/*\n",
		"turbo.json":               "{}",
		".nvmrc":                   "22\n",
		"apps/api/package.json":    `{"name":"@lo/api","scripts":{"start:dev":"nest start --watch"},"dependencies":{"@nestjs/core":"10","pg":"8"}}`,
		"apps/web/package.json":    `{"name":"@lo/web","scripts":{"dev":"next dev"},"dependencies":{"next":"15"}}`,
		"apps/mobile/package.json": `{"name":"@lo/mobile","dependencies":{"expo":"52"}}`,
		"packages/ui/package.json": `{"name":"@lo/ui","main":"index.ts"}`,
	})
	res, err := Project(root, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	out, _ := config.Marshal(res.Config)
	cfg, err := config.LoadFromBytes(out, "logistics-os")
	if err != nil {
		t.Fatalf("workspace config does not validate: %v\n%s", err, out)
	}
	if len(cfg.Apps) != 2 {
		t.Fatalf("apps:\n%s", out)
	}
	web, api := cfg.Apps[0], cfg.Apps[1]
	// web is front-ranked so it owns the root domain.
	if web.Name != "web" || web.Path != "" || web.Port != 3000 || web.PackageManager != "pnpm" || web.Version != "22" ||
		web.Command != "corepack pnpm --filter ./apps/web run dev -H 0.0.0.0 -p 3000" {
		t.Errorf("web = %+v", web)
	}
	if api.Name != "api" || api.Command != "corepack pnpm --filter ./apps/api run start:dev" {
		t.Errorf("api = %+v", api)
	}
	if cfg.PrimaryApp().Domain != "logistics-os.localhost" {
		t.Errorf("primary domain = %s", cfg.PrimaryApp().Domain)
	}
	if cfg.Services.Database.Type != "postgres" {
		t.Errorf("pg in apps/api should add postgres, got %q", cfg.Services.Database.Type)
	}
	notes := strings.Join(res.Notes, "\n")
	for _, want := range []string{"apps/mobile/: skipped — Expo", "packages/ui/: skipped — package without", "pnpm workspace: 2 app(s) of 4 package(s)"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes missing %q:\n%s", want, notes)
		}
	}
}

func TestProject_NpmAndYarnWorkspaces(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	npm := fixture(t, "shop", map[string]string{
		"package.json":           `{"workspaces":["apps/*"]}`,
		"package-lock.json":      "{}",
		"apps/site/package.json": `{"name":"site","scripts":{"dev":"vite"},"devDependencies":{"vite":"6"}}`,
	})
	res, err := Project(npm, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if a := res.Config.Apps[0]; a.Command != "npm run dev -w apps/site -- --host 0.0.0.0 --port 5173" || a.PackageManager != "" {
		t.Errorf("npm app = %+v", a)
	}

	yarn := fixture(t, "blog", map[string]string{
		"package.json":                 `{"workspaces":{"packages":["packages/*"]}}`,
		"yarn.lock":                    "",
		"packages/front/package.json":  `{"name":"@blog/front","scripts":{"dev":"astro dev"},"dependencies":{"astro":"5"}}`,
		"packages/noname/package.json": `{"scripts":{"dev":"node x.js"}}`,
	})
	res, err = Project(yarn, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Config.Apps) != 1 || res.Config.Apps[0].Command != "corepack yarn workspace @blog/front run dev --host 0.0.0.0 --port 4321" {
		t.Errorf("yarn apps = %+v", res.Config.Apps)
	}
	if !strings.Contains(strings.Join(res.Notes, "\n"), "packages/noname/: skipped — yarn workspaces need a package name") {
		t.Errorf("notes: %v", res.Notes)
	}
}

// A workspace of libraries whose root runs everything (turbo dev) stays one app.
func TestProject_WorkspaceOfLibrariesRunsRoot(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	root := fixture(t, "kit", map[string]string{
		"package.json":            `{"workspaces":["packages/*"],"scripts":{"dev":"turbo dev"}}`,
		"packages/a/package.json": `{"name":"a"}`,
	})
	res, err := Project(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Config.Apps) != 1 || res.Config.Apps[0].Command != "npm run dev" {
		t.Errorf("apps = %+v", res.Config.Apps)
	}
}

func TestPackageManagerWithoutLockfile(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	byField := fixture(t, "a", map[string]string{
		"package.json":          `{"packageManager":"yarn@4.5.0","workspaces":["apps/*"]}`,
		"apps/web/package.json": `{"name":"web","scripts":{"dev":"vite"},"devDependencies":{"vite":"6"}}`,
	})
	if res, err := Project(byField, Options{}); err != nil || res.Config.Apps[0].PackageManager != "yarn" {
		t.Errorf("packageManager field: %+v %v", res, err)
	}
	byWorkspaceFile := fixture(t, "b", map[string]string{
		"package.json":          `{}`,
		"pnpm-workspace.yaml":   "packages: [apps/*]\n",
		"apps/web/package.json": `{"scripts":{"dev":"vite"},"devDependencies":{"vite":"6"}}`,
	})
	if res, err := Project(byWorkspaceFile, Options{}); err != nil || res.Config.Apps[0].PackageManager != "pnpm" {
		t.Errorf("pnpm-workspace.yaml: %+v %v", res, err)
	}
}
