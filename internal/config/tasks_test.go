package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const tasksConfig = `
version: 2
name: shop
apps:
  web:  { runtime: php, version: "8.3" }
  vite: { runtime: node, command: "npm run dev", port: 5173 }
tasks:
  migrate: php artisan migrate
  build:
    command: npm run build
    app: vite
    description: Production build
`

func TestTasks_LoadInOrder(t *testing.T) {
	cfg := mustLoad(t, tasksConfig)
	if len(cfg.Tasks) != 2 {
		t.Fatalf("tasks: %+v", cfg.Tasks)
	}
	if got := cfg.Tasks[0]; got.Name != "migrate" || got.Command != "php artisan migrate" || got.App != "" {
		t.Errorf("first task: %+v", got)
	}
	if got := cfg.Tasks[1]; got.Name != "build" || got.App != "vite" || got.Description != "Production build" {
		t.Errorf("second task: %+v", got)
	}
}

func TestTasks_RoundTrip(t *testing.T) {
	cfg := mustLoad(t, tasksConfig)
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "migrate: php artisan migrate") {
		t.Errorf("a command-only task should be written as a plain string:\n%s", out)
	}
	again := mustLoad(t, string(out))
	if len(again.Tasks) != 2 || again.Tasks[1].App != "vite" {
		t.Errorf("round trip lost tasks: %+v", again.Tasks)
	}
}

func TestTasks_Errors(t *testing.T) {
	base := "version: 2\nname: shop\napps:\n  web: { runtime: php, version: \"8.3\" }\n"
	expectError(t, base+"tasks:\n  build: { command: x, app: nope }\n", `tasks.build.app: no app named "nope"`)
	expectError(t, base+"tasks:\n  build: { comand: x }\n", `unknown field "comand"`)
	expectError(t, base+"tasks:\n  Bad Name: x\n", "tasks.Bad Name")
	expectError(t, base+"tasks:\n  empty: { app: web }\n", "tasks.empty.command: required")
}
