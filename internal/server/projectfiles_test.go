package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func projectID(t *testing.T, call func(method, path, body string) *httptest.ResponseRecorder, name string) string {
	t.Helper()
	rec := call("GET", "/api/v2/projects", "")
	var list []ProjectView
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("projects: %d %s", rec.Code, rec.Body)
	}
	for _, p := range list {
		if p.Name == name {
			return p.ID
		}
	}
	t.Fatalf("project %s not in %+v", name, list)
	return ""
}

func TestProjectFiles(t *testing.T) {
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"blog/.git/HEAD":        "x",
		"blog/index.html":       "<h1>blog</h1>",
		"blog/assets/app.css":   "body{}",
		"blog/logo.png":         "\x89PNG\x00\x00",
		"blog/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: static}\n",
		"secret.txt":            "outside",
	})
	call := classicServer(t, ws, nil)
	id := projectID(t, call, "blog")
	base := "/api/v2/projects/" + id

	rec := call("GET", base+"/files", "")
	var listing struct {
		Path    string      `json:"path"`
		Entries []fileEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil || rec.Code != 200 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if len(listing.Entries) < 3 || !listing.Entries[0].IsDir {
		t.Fatalf("folders should come first: %+v", listing.Entries)
	}

	rec = call("GET", base+"/files?path=assets", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"path":"assets/app.css"`) {
		t.Fatalf("sub folder: %d %s", rec.Code, rec.Body)
	}

	rec = call("GET", base+"/file?path=index.html", "")
	var file struct {
		Content string `json:"content"`
		ModTime string `json:"mod_time"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &file); err != nil || file.Content != "<h1>blog</h1>" {
		t.Fatalf("read: %d %s", rec.Code, rec.Body)
	}

	if rec = call("GET", base+"/file?path=logo.png", ""); rec.Code != 415 {
		t.Fatalf("binary file: %d %s", rec.Code, rec.Body)
	}

	for _, p := range []string{"../secret.txt", "assets/../../secret.txt", "/../secret.txt"} {
		if rec = call("GET", base+"/file?path="+p, ""); rec.Code != 400 {
			t.Fatalf("%s should be refused: %d %s", p, rec.Code, rec.Body)
		}
	}
	if rec = call("PUT", base+"/file", `{"path":"../evil.txt","content":"x"}`); rec.Code != 400 {
		t.Fatalf("write outside: %d %s", rec.Code, rec.Body)
	}
	if rec = call("PUT", base+"/file", `{"path":".git/config","content":"x"}`); rec.Code != 403 {
		t.Fatalf("write in .git: %d %s", rec.Code, rec.Body)
	}

	body, _ := json.Marshal(map[string]string{"path": "index.html", "content": "<h1>new</h1>", "mod_time": file.ModTime})
	if rec = call("PUT", base+"/file", string(body)); rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(filepath.Join(ws, "blog", "index.html")); string(data) != "<h1>new</h1>" {
		t.Fatalf("saved content = %q", data)
	}
	// Saving with an older timestamp than the file has is a conflict.
	if rec = call("PUT", base+"/file", `{"path":"index.html","content":"x","mod_time":"2000-01-01T00:00:00Z"}`); rec.Code != 409 {
		t.Fatalf("stale save: %d %s", rec.Code, rec.Body)
	}

	if rec = call("PUT", base+"/file", `{"path":"assets/new.js","content":"1"}`); rec.Code != 200 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if rec = call("PUT", base+"/file", `{"path":"missing/new.js","content":"1"}`); rec.Code != 400 {
		t.Fatalf("create in missing folder: %d %s", rec.Code, rec.Body)
	}
}

func TestProjectFilesSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"blog/.git/HEAD":        "x",
		"blog/index.html":       "x",
		"blog/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: static}\n",
	})
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "passwd"), []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, "blog", "link")); err != nil {
		t.Fatal(err)
	}
	call := classicServer(t, ws, nil)
	base := "/api/v2/projects/" + projectID(t, call, "blog")

	if rec := call("GET", base+"/file?path=link/passwd", ""); rec.Code != 400 {
		t.Fatalf("read through link: %d %s", rec.Code, rec.Body)
	}
	if rec := call("PUT", base+"/file", `{"path":"link/new","content":"x"}`); rec.Code != 400 {
		t.Fatalf("write through link: %d %s", rec.Code, rec.Body)
	}
	if rec := call("GET", base+"/files", ""); strings.Contains(rec.Body.String(), `"link"`) {
		t.Fatalf("escaping link should be hidden: %s", rec.Body)
	}
}

func TestProjectConfigEdit(t *testing.T) {
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"blog/.git/HEAD":        "x",
		"blog/index.html":       "x",
		"blog/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: static}\n",
	})
	call := classicServer(t, ws, nil)
	base := "/api/v2/projects/" + projectID(t, call, "blog")

	rec := call("PUT", base+"/config", `{"config":"version: 2\napps:\n  web: {runtime: static, comand: x}\n"}`)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "comand") {
		t.Fatalf("typo should be rejected: %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(filepath.Join(ws, "blog", ".sld", "config.yaml")); strings.Contains(string(data), "comand") {
		t.Fatal("invalid config was written")
	}

	cfg := "version: 2\nname: journal\napps:\n  web: {runtime: static}\n"
	body, _ := json.Marshal(map[string]string{"config": cfg})
	if rec = call("PUT", base+"/config", string(body)); rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	rec = call("GET", base, "")
	if !strings.Contains(rec.Body.String(), "journal.localhost") {
		t.Fatalf("rescan should pick up the new name: %s", rec.Body)
	}
}

func TestProjectEnv(t *testing.T) {
	ws := t.TempDir()
	env := "# app\nAPP_NAME=Blog\nexport DEBUG=true\n\n# db\nDB_PASSWORD=\"p@ss word\"\nOLD=1\n"
	writeTree(t, ws, map[string]string{
		"blog/.git/HEAD":        "x",
		"blog/index.html":       "x",
		"blog/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: static}\n",
		"blog/.env":             env,
	})
	call := classicServer(t, ws, nil)
	base := "/api/v2/projects/" + projectID(t, call, "blog")

	rec := call("GET", base+"/env", "")
	var resp struct {
		Files []envFileView `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || len(resp.Files) != 1 {
		t.Fatalf("env: %d %s", rec.Code, rec.Body)
	}
	vars := resp.Files[0].Variables
	if len(vars) != 4 || vars[1].Key != "DEBUG" || vars[2].Value != "p@ss word" {
		t.Fatalf("vars = %+v", vars)
	}

	rec = call("PUT", base+"/env", `{"name":".env","variables":[
		{"key":"APP_NAME","value":"Blog"},{"key":"DEBUG","value":"false"},
		{"key":"DB_PASSWORD","value":"p@ss word"},{"key":"NEW","value":"a \"b\""}]}`)
	if rec.Code != 200 {
		t.Fatalf("write: %d %s", rec.Code, rec.Body)
	}
	got, _ := os.ReadFile(filepath.Join(ws, "blog", ".env"))
	want := "# app\nAPP_NAME=Blog\nDEBUG=false\n\n# db\nDB_PASSWORD=\"p@ss word\"\nNEW=\"a \\\"b\\\"\"\n"
	if string(got) != want {
		t.Fatalf("env file =\n%s\nwant\n%s", got, want)
	}
	if v := parseEnv(got); v[3].Value != `a "b"` {
		t.Fatalf("round trip = %+v", v)
	}

	for _, bad := range []string{
		`{"name":"../.env","variables":[]}`,
		`{"name":".env","variables":[{"key":"A B","value":"1"}]}`,
		`{"name":".env","variables":[{"key":"A","value":"1"},{"key":"A","value":"2"}]}`,
		`{"name":".env","variables":[{"key":"A","value":"1\nB=2"}]}`,
	} {
		if rec = call("PUT", base+"/env", bad); rec.Code != 400 {
			t.Fatalf("%s: %d %s", bad, rec.Code, rec.Body)
		}
	}
	if _, err := os.Stat(filepath.Join(ws, "blog", ".env-backups")); err == nil {
		t.Fatal("backups must not be written into the project")
	}

	rec = call("GET", base+"/env/backups?name=.env", "")
	var backups []envBackup
	if err := json.Unmarshal(rec.Body.Bytes(), &backups); err != nil || len(backups) != 1 {
		t.Fatalf("backups: %d %s", rec.Code, rec.Body)
	}
	if rec = call("POST", base+"/env/restore", `{"id":"`+backups[0].ID+`"}`); rec.Code != 200 {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body)
	}
	if got, _ := os.ReadFile(filepath.Join(ws, "blog", ".env")); string(got) != env {
		t.Fatalf("restored =\n%s", got)
	}
	if rec = call("POST", base+"/env/restore", `{"id":"../../auth.json"}`); rec.Code != 404 {
		t.Fatalf("restore of unknown id: %d %s", rec.Code, rec.Body)
	}
}

func TestProjectsDockerizedFlag(t *testing.T) {
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"blog/.git/HEAD":        "x", // set up in SLD
		"blog/index.html":       "x",
		"blog/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: static}\n",
		"stack/.git/HEAD":       "x", // own compose file
		"stack/compose.yaml":    "services:\n  web:\n    image: nginx\n    ports: [\"8080:80\"]\n",
		"img/.git/HEAD":         "x", // own Dockerfile next to a detected stack
		"img/package.json":      `{"scripts":{"dev":"vite"}}`,
		"img/Dockerfile":        "FROM node:22\n",
		"plain/.git/HEAD":       "x", // detected, not set up, no Docker files
		"plain/package.json":    `{"scripts":{"dev":"vite"}}`,
		"notes/.git/HEAD":       "x", // unknown stack
		"notes/readme.txt":      "hi",
	})
	call := classicServer(t, ws, nil)
	rec := call("GET", "/api/v2/projects", "")
	var list []ProjectView
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("projects: %d %s", rec.Code, rec.Body)
	}
	want := map[string]bool{"blog": true, "stack": true, "img": true, "plain": false, "notes": false}
	for _, p := range list {
		if w, ok := want[p.Name]; ok && p.Dockerized != w {
			t.Errorf("%s: dockerized = %v, want %v", p.Name, p.Dockerized, w)
		}
		delete(want, p.Name)
	}
	if len(want) > 0 {
		t.Errorf("missing projects: %v", want)
	}
}
