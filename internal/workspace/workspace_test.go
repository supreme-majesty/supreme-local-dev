package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, c := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPackagesPnpm(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"pnpm-workspace.yaml":                  "packages:\n  - 'apps/*'\n  - 'packages/**'\n  - '!packages/legacy'\n",
		"package.json":                         `{"name":"root"}`,
		"apps/web/package.json":                "{}",
		"apps/admin/package.json":              "{}",
		"apps/notes/README.md":                 "no package.json",
		"packages/ui/package.json":             "{}",
		"packages/cfg/eslint/package.json":     "{}",
		"packages/legacy/package.json":         "{}",
		"apps/web/node_modules/x/package.json": "{}",
	})
	got := Packages(root)
	want := []string{"apps/admin", "apps/web", "packages/cfg/eslint", "packages/ui"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Packages = %v, want %v", got, want)
	}
}

func TestPackagesNpmAndYarn(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"package.json":              `{"workspaces":["apps/*","services/api"]}`,
		"apps/site/package.json":    "{}",
		"services/api/package.json": "{}",
	})
	if got := Packages(root); !reflect.DeepEqual(got, []string{"apps/site", "services/api"}) {
		t.Errorf("npm workspaces = %v", got)
	}

	yarn := t.TempDir()
	write(t, yarn, map[string]string{
		"package.json":            `{"workspaces":{"packages":["packages/*"],"nohoist":["**/x"]}}`,
		"packages/a/package.json": "{}",
	})
	if got := Packages(yarn); !reflect.DeepEqual(got, []string{"packages/a"}) {
		t.Errorf("yarn workspaces = %v", got)
	}
}

func TestNotAWorkspace(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"package.json": `{"name":"plain"}`})
	if got := Packages(root); got != nil {
		t.Errorf("Packages = %v, want nil", got)
	}
}
