package config

import (
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func mustLoad(t *testing.T, data string) *Config {
	t.Helper()
	cfg, err := LoadFromBytes([]byte(data), "default-name")
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	return cfg
}

func expectError(t *testing.T, data string, wants ...string) {
	t.Helper()
	_, err := LoadFromBytes([]byte(data), "test")
	if err == nil {
		t.Fatalf("expected an error containing %q, got nil", wants)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

const fullV1 = `
version: 1
name: "demo-app"
runtime:
  php: "8.3"
  web_root: "public"
domain:
  name: "demo.test"
  tls: true
services:
  database:
    type: "mysql"
  redis:
    enabled: true
node:
  version: "20"
  dev_server:
    enabled: true
xdebug:
  enabled: true
environment:
  APP_ENV: "local"
`

func TestLoadV1_UpgradesToApps(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	cfg := mustLoad(t, fullV1)

	if cfg.Version != CurrentVersion || cfg.SourceVersion != 1 {
		t.Errorf("version=%d source=%d, want %d/1", cfg.Version, cfg.SourceVersion, CurrentVersion)
	}
	if cfg.Name != "demo-app" || cfg.Domain.Name != "demo.test" || !cfg.Domain.TLS {
		t.Errorf("project fields not carried over: %+v", cfg.Domain)
	}
	if len(cfg.Apps) != 2 {
		t.Fatalf("expected web + vite apps, got %d (%s)", len(cfg.Apps), cfg.Describe())
	}

	web := cfg.Apps[0]
	if web.Name != "web" || web.Runtime != RuntimePHP || web.Version != "8.3" || web.WebRoot != "public" || !web.Xdebug {
		t.Errorf("web app = %+v", web)
	}
	if web.Domain != "demo.test" {
		t.Errorf("first app should own the root domain, got %q", web.Domain)
	}

	vite := cfg.Apps[1]
	if vite.Name != "vite" || vite.Runtime != RuntimeNode || vite.Version != "20" ||
		vite.Command != "npm run dev" || vite.Port != 5173 || vite.ExposePort != 5173 {
		t.Errorf("vite app = %+v (version 1 dev-server defaults must be kept)", vite)
	}
	if vite.Domain != "vite.demo.test" {
		t.Errorf("vite domain = %q", vite.Domain)
	}

	if cfg.Services.Database.Version != "8.0" || cfg.Services.Redis.Version != "7-alpine" {
		t.Errorf("service defaults not applied: %+v", cfg.Services)
	}
	if cfg.Environment["APP_ENV"] != "local" {
		t.Errorf("environment lost: %v", cfg.Environment)
	}
}

func TestLoadV1_NodeVersionWithoutDevServerBecomesTool(t *testing.T) {
	cfg := mustLoad(t, "version: 1\nruntime:\n  php: \"8.2\"\nnode:\n  version: \"18\"\n")
	if len(cfg.Apps) != 1 || cfg.Tools.Node != "18" || cfg.NodeVersion() != "18" {
		t.Errorf("apps=%s tools.node=%q NodeVersion=%q", cfg.Describe(), cfg.Tools.Node, cfg.NodeVersion())
	}
}

func TestLoadV1_Errors(t *testing.T) {
	expectError(t, "version: 1\nname: no-php\n", "runtime.php is required", "version: 2")
	expectError(t, "version: 1\nruntime:\n  php: \"7.4\"\n", "unsupported PHP version")
}

func TestDefaults(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	cfg, err := LoadFromBytes([]byte("version: 2\napps:\n  web:\n    runtime: php\n    version: \"8.2\"\n"), "My Service")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "My Service" || cfg.Domain.Name != "my-service.localhost" || cfg.Domain.TLS {
		t.Errorf("name=%q domain=%q tls=%v", cfg.Name, cfg.Domain.Name, cfg.Domain.TLS)
	}
	if cfg.Apps[0].WebRoot != "public" {
		t.Errorf("php web_root default = %q", cfg.Apps[0].WebRoot)
	}

	t.Setenv("SLD_TLD", "test")
	cfg, err = LoadFromBytes([]byte("version: 2\napps:\n  web:\n    runtime: static\n"), "site")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Domain.Name != "site.test" || cfg.Apps[0].WebRoot != "." {
		t.Errorf("SLD_TLD / static defaults: domain=%q web_root=%q", cfg.Domain.Name, cfg.Apps[0].WebRoot)
	}
}

func TestLoadV2_MultiRuntime(t *testing.T) {
	t.Setenv("SLD_TLD", "")
	cfg := mustLoad(t, `
version: 2
name: shop
domain:
  tls: true
  aliases: [www.shop.localhost]
apps:
  web:
    runtime: node
    command: npm run dev -- --host 0.0.0.0
    install: npm ci
    package_manager: pnpm
  api:
    runtime: python
    command: uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload
    port: 8000
    healthcheck: /health
  legacy:
    runtime: proxy
    port: 9000
    domain: old.shop.localhost
  worker:
    runtime: dockerfile
    port: 8080
    target: dev
    build_args: {GO_VERSION: "1.25"}
  mail:
    runtime: image
    image: axllent/mailpit:v1.31.2
    port: 8025
services:
  database: {shared: sld-shared-postgres-16}
  redis: {enabled: true}
`)

	if got := cfg.Describe(); got != "web (node 22), api (python 3.13), legacy (proxy), worker (dockerfile), mail (image)" {
		t.Errorf("Describe() = %q (order or defaults wrong)", got)
	}
	web, _ := cfg.App("web")
	if web.Port != 3000 || web.Domain != "shop.localhost" {
		t.Errorf("node defaults: port=%d domain=%q", web.Port, web.Domain)
	}
	api, _ := cfg.App("api")
	if api.Domain != "api.shop.localhost" {
		t.Errorf("api domain = %q", api.Domain)
	}
	worker, _ := cfg.App("worker")
	if worker.Dockerfile != "Dockerfile" || worker.Context != "." {
		t.Errorf("dockerfile defaults: %+v", worker)
	}
	if cfg.PrimaryApp().Name != "web" {
		t.Errorf("primary app = %s", cfg.PrimaryApp().Name)
	}
	if cfg.Services.Database.Name != "shop" {
		t.Errorf("shared database name default = %q", cfg.Services.Database.Name)
	}
	hosts := strings.Join(cfg.Hostnames(), ",")
	for _, h := range []string{"shop.localhost", "api.shop.localhost", "old.shop.localhost", "www.shop.localhost"} {
		if !strings.Contains(hosts, h) {
			t.Errorf("Hostnames() missing %s: %s", h, hosts)
		}
	}
}

func TestValidateV2_Errors(t *testing.T) {
	cases := []struct {
		name  string
		yaml  string
		wants []string
	}{
		{"no apps", "version: 2\n", []string{"define at least one app"}},
		{"unknown runtime", "version: 2\napps:\n  web: {runtime: cobol}\n", []string{"unknown runtime \"cobol\""}},
		{"missing runtime", "version: 2\napps:\n  web: {port: 80}\n", []string{"apps.web.runtime: required"}},
		{"php version", "version: 2\napps:\n  web: {runtime: php, version: \"7.4\"}\n", []string{"unsupported PHP version"}},
		{"python needs command and port", "version: 2\napps:\n  api: {runtime: python}\n", []string{"apps.api.command: required", "apps.api.port: required"}},
		{"image needs image", "version: 2\napps:\n  x: {runtime: image, port: 80}\n", []string{"apps.x.image: required"}},
		{"proxy needs port", "version: 2\napps:\n  x: {runtime: proxy}\n", []string{"apps.x.port: required"}},
		{"field for wrong runtime", "version: 2\napps:\n  x: {runtime: proxy, port: 80, command: run, xdebug: true}\n", []string{"apps.x.command: not supported", "apps.x.xdebug: not supported"}},
		{"bad app name", "version: 2\napps:\n  Web_App: {runtime: static}\n", []string{"app names must be"}},
		{"duplicate domain", "version: 2\ndomain: {name: a.localhost}\napps:\n  one: {runtime: static}\n  two: {runtime: static, domain: a.localhost}\n", []string{"already used by apps.one"}},
		{"web_root escapes project", "version: 2\napps:\n  web: {runtime: static, web_root: ../outside}\n", []string{"must be a path inside the project"}},
		{"bad port", "version: 2\napps:\n  web: {runtime: node, port: 70000}\n", []string{"70000 is not a valid port"}},
		{"db type and shared", "version: 2\napps:\n  web: {runtime: static}\nservices:\n  database: {type: mysql, shared: s}\n", []string{"either 'type'"}},
		{"unknown field (typo)", "version: 2\napps:\n  web: {runtime: node, comand: x}\n", []string{"comand"}},
		{"duplicate app", "version: 2\napps:\n  web: {runtime: static}\n  web: {runtime: static}\n", []string{"web"}},
		{"unsupported version", "version: 3\n", []string{"unsupported configuration version 3"}},
		{"missing version", "name: x\n", []string{"missing configuration version"}},
		{"malformed", "version: 2\napps: [unclosed\n", []string{"YAML"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { expectError(t, tc.yaml, tc.wants...) })
	}
}

func TestValidate_ReportsAllProblemsAtOnce(t *testing.T) {
	_, err := LoadFromBytes([]byte("version: 2\napps:\n  a: {runtime: python}\n  b: {runtime: cobol}\n"), "x")
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"apps.a.command", "apps.a.port", "apps.b.runtime"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("combined error missing %s:\n%v", want, err)
		}
	}
}

func TestMigrateV1_GoldenOutput(t *testing.T) {
	out, err := MigrateV1([]byte(fullV1))
	if err != nil {
		t.Fatalf("MigrateV1: %v", err)
	}
	body := string(out)
	if i := strings.Index(body, "version:"); i >= 0 {
		body = body[i:] // drop the comment header
	}
	want := `version: 2
name: demo-app
domain:
  name: demo.test
  tls: true
apps:
  web:
    runtime: php
    version: "8.3"
    web_root: public
    xdebug: true
  vite:
    runtime: node
    version: "20"
    command: npm run dev
    port: 5173
    expose_port: 5173
services:
  database:
    type: mysql
  redis:
    enabled: true
environment:
  APP_ENV: local
`
	if body != want {
		t.Errorf("migrated YAML mismatch.\n--- got ---\n%s\n--- want ---\n%s", body, want)
	}

	// The migrated file loads to the same resolved config as the original.
	fromV1 := mustLoad(t, fullV1)
	fromV2 := mustLoad(t, string(out))
	fromV1.SourceVersion, fromV2.SourceVersion = 0, 0
	a, _ := yaml.Marshal(fromV1)
	b, _ := yaml.Marshal(fromV2)
	if string(a) != string(b) {
		t.Errorf("resolved configs differ after migration:\n%s\n---\n%s", a, b)
	}
}

func TestMigrateV1_AlreadyCurrent(t *testing.T) {
	_, err := MigrateV1([]byte("version: 2\napps:\n  web: {runtime: static}\n"))
	if !errors.Is(err, ErrAlreadyCurrent) {
		t.Errorf("err = %v, want ErrAlreadyCurrent", err)
	}
}

func TestApps_RoundTripPreservesOrder(t *testing.T) {
	cfg := mustLoad(t, "version: 2\napps:\n  zeta: {runtime: static}\n  alpha: {runtime: static}\n  mid: {runtime: static}\n")
	out, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	again := mustLoad(t, string(out))
	var names []string
	for _, a := range again.Apps {
		names = append(names, a.Name)
	}
	if strings.Join(names, ",") != "zeta,alpha,mid" {
		t.Errorf("app order not preserved: %v", names)
	}
}

func TestNodeVersion(t *testing.T) {
	cfg := mustLoad(t, "version: 2\napps:\n  web: {runtime: php, version: \"8.3\"}\n")
	if cfg.NodeVersion() != DefaultVersions[RuntimeNode] {
		t.Errorf("default NodeVersion = %q", cfg.NodeVersion())
	}
	cfg = mustLoad(t, "version: 2\napps:\n  web: {runtime: php, version: \"8.3\"}\n  ui: {runtime: node, version: \"20\"}\ntools: {node: \"18\"}\n")
	if cfg.NodeVersion() != "20" {
		t.Errorf("node app version should win over tools.node, got %q", cfg.NodeVersion())
	}
}

func TestResolvePHPConstraint(t *testing.T) {
	cases := []struct {
		constraint string
		expected   string
		shouldErr  bool
	}{
		{"^8.3", "8.3", false},
		{"~8.3.0", "8.3", false},
		{"^8.1", "8.1", false},
		{"8.2", "8.2", false},
		{">=8.2", "8.4", false},
		{"^7.4", "", true}, // Fatal error: no silent downgrade
		{"<8.0", "", true}, // Fatal error: no silent downgrade
		{"invalid", "", true},
	}

	for _, c := range cases {
		t.Run(c.constraint, func(t *testing.T) {
			got, err := ResolvePHPConstraint(c.constraint)
			if c.shouldErr {
				if err == nil {
					t.Errorf("expected error for constraint %q, got version %q", c.constraint, got)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for constraint %q: %v", c.constraint, err)
				}
				if got != c.expected {
					t.Errorf("ResolvePHPConstraint(%q) = %q; want %q", c.constraint, got, c.expected)
				}
			}
		})
	}
}
