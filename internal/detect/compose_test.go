package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
)

func TestContainerPort(t *testing.T) {
	cases := map[any]int{
		"8080":                 8080,
		"${APP_PORT:-8080}:80": 80,
		"127.0.0.1:${DB_EXTERNAL_PORT:-3307}:3306": 3306,
		"127.0.0.1:9000:9000/tcp":                  9000,
		"${WEB_HOST_PORT:-5173}:5173":              5173,
		"8000-8010:8000-8010":                      0,
		"80:${INNER}":                              0,
		5173:                                       5173,
	}
	for in, want := range cases {
		if got := containerPort(in); got != want {
			t.Errorf("containerPort(%v) = %d, want %d", in, got, want)
		}
	}
	if got := containerPort(map[string]any{"target": 80, "published": "8080"}); got != 80 {
		t.Errorf("long syntax = %d", got)
	}
}

func TestProjectName(t *testing.T) {
	cases := map[string]string{
		"/dev/sunport-apps":                        "sunport",
		"/dev/sunport-apps/backend":                "sunport",
		"/dev/laravel-builds/cecadoe-ride/backend": "cecadoe-ride",
		"/dev/react-apps/tailorly":                 "tailorly",
		"/dev/gifec-it-asset -tracking-system":     "gifec-it-asset -tracking-system",
		"/dev/laravel-builds":                      "laravel",
		"/dev/my-app":                              "my-app",
		"/dev/swiftsms":                            "swiftsms",
	}
	for in, want := range cases {
		if got := ProjectName(filepath.FromSlash(in)); got != want {
			t.Errorf("ProjectName(%s) = %q, want %q", in, got, want)
		}
	}
}

const sunportCompose = `name: sunport-backend
x-app: &app
  image: sunport-backend:latest
  networks: [sunport]
services:
  nginx:
    image: nginx:1.25-alpine
    container_name: sunport-nginx
    ports:
      - "${APP_PORT:-8080}:80"
  app:
    <<: *app
    container_name: sunport-app
  queue:
    <<: *app
  mysql:
    image: sunport-mysql:8.0
    ports:
      - "127.0.0.1:${DB_EXTERNAL_PORT:-3307}:3306"
  backup:
    image: mysql:8.0
  offsite-backup:
    image: rclone/rclone:1.68.2
`

func TestProject_ComposeInSubfolderNamedAfterMainFolder(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	root := filepath.Join(t.TempDir(), "sunport-apps")
	fixtureAt(t, root, map[string]string{
		".git/HEAD":                  "ref: refs/heads/main",
		"backend/docker-compose.yml": sunportCompose,
		"backend/composer.json":      `{"require":{"php":"^8.2","laravel/framework":"^11"}}`,
		"backend/artisan":            "",
		"driver-app/package.json":    `{"dependencies":{"expo":"52"}}`,
		"user-app/package.json":      `{"dependencies":{"react-native":"0.76"}}`,
	})
	res, err := Project(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := loadDetected(t, res)
	if cfg.Name != "sunport" || cfg.Domain.Name != "sunport.localhost" {
		t.Errorf("name=%q domain=%q, want sunport / sunport.localhost", cfg.Name, cfg.Domain.Name)
	}
	if len(cfg.Apps) != 1 {
		t.Fatalf("apps = %s", cfg.Describe())
	}
	app := cfg.Apps[0]
	if app.Runtime != config.RuntimeCompose || app.Service != "nginx" || app.Port != 80 || app.Path != "backend" || app.Domain != "sunport.localhost" {
		t.Errorf("compose app = %+v", app)
	}
	// The stack brings its own database: SLD must not add one.
	if cfg.Services.Database.Type != "" || cfg.Services.Redis.Enabled {
		t.Errorf("compose project should not get SLD services: %+v", cfg.Services)
	}
}

func TestProject_ComposeWithTwoWebServices(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	root := filepath.Join(t.TempDir(), "tailorly")
	fixtureAt(t, root, map[string]string{
		"docker-compose.yml": `name: tailormeasure
services:
  mysql: {image: "mysql:8.4", ports: ["127.0.0.1:${MYSQL_HOST_PORT:-33066}:3306"]}
  phpmyadmin: {image: "phpmyadmin:5.2", ports: ["127.0.0.1:8066:80"]}
  minio: {image: "cgr.dev/chainguard/minio:latest", ports: ["127.0.0.1:9000:9000", "127.0.0.1:9001:9001"]}
  api: {image: "tailorly-api:dev", ports: ["${API_HOST_PORT:-8000}:8000"]}
  queue: {image: "tailorly-api:dev"}
  web: {image: "node:24-bookworm", ports: ["${WEB_HOST_PORT:-5173}:5173"]}
`,
		"backend/composer.json": `{"require":{"php":"^8.4"}}`,
	})
	res, err := Project(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := loadDetected(t, res)
	if got := cfg.Describe(); got != "web (compose), api (compose)" {
		t.Fatalf("apps = %s", got)
	}
	web, api := cfg.Apps[0], cfg.Apps[1]
	if web.Service != "web" || web.Port != 5173 || web.Domain != "tailorly.localhost" {
		t.Errorf("web = %+v", web)
	}
	if api.Service != "api" || api.Port != 8000 || api.Domain != "api.tailorly.localhost" {
		t.Errorf("api = %+v", api)
	}
}

func TestFindProjects(t *testing.T) {
	ws := t.TempDir()
	fixtureAt(t, ws, map[string]string{
		"laravel-builds/swiftsms/.git/HEAD":         "x",
		"laravel-builds/swiftsms/compose.yaml":      "services: {}",
		"laravel-builds/credora/docker-compose.yml": "services: {}",
		"sunport-apps/.git/HEAD":                    "x",
		"sunport-apps/backend/docker-compose.yml":   "services: {}",
		"react-apps/tailorly/.git/HEAD":             "x",
		"notes/readme.md":                           "not a project",
		"node_modules/pkg/.git/HEAD":                "skipped",
		"tools/sld/.git/HEAD":                       "x",
		"tools/sld/.sldignore":                      "",
	})
	var got []string
	for _, p := range FindProjects(ws, 3) {
		rel, _ := filepath.Rel(ws, p)
		got = append(got, filepath.ToSlash(rel))
	}
	want := "laravel-builds/credora,laravel-builds/swiftsms,react-apps/tailorly,sunport-apps"
	if strings.Join(got, ",") != want {
		t.Errorf("FindProjects = %v, want %s", got, want)
	}
}

func fixtureAt(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(path)), content)
	}
}

func loadDetected(t *testing.T, res *Result) *config.Config {
	t.Helper()
	out, err := config.Marshal(res.Config)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromBytes(out, "x")
	if err != nil {
		t.Fatalf("detected config does not validate: %v\n%s", err, out)
	}
	return cfg
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
