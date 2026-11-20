package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveAllowed(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "blog"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewServer(0).WithRoots([]string{ws})

	ok := []string{filepath.Join(ws, "blog"), filepath.Join(ws, "blog", "new.txt")}
	for _, p := range ok {
		if _, err := s.resolveAllowed(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	bad := []string{"", "relative/path", filepath.Join(outside, "x"), filepath.Join(ws, "..", filepath.Base(outside)), filepath.Join(ws, "missing", "x")}
	for _, p := range bad {
		if _, err := s.resolveAllowed(p); err == nil {
			t.Errorf("%q should be refused", p)
		}
	}
	if _, err := NewServer(0).resolveAllowed(filepath.Join(ws, "blog")); err == nil {
		t.Error("without roots every path must be refused")
	}

	if runtime.GOOS != "windows" {
		if err := os.Symlink(outside, filepath.Join(ws, "blog", "link")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.resolveAllowed(filepath.Join(ws, "blog", "link", "x")); err == nil {
			t.Error("a symlink out of the workspace must be refused")
		}
	}
}

func TestLegacyRoutesRefuseOutsidePaths(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	env := filepath.Join(ws, ".env")
	mux := NewServer(0).WithRoots([]string{ws}).Mux()
	outsideFile := filepath.Join(outside, "secret")

	cases := []struct{ method, path, body string }{
		{"GET", "/api/projects/files?path=" + url.QueryEscape(outside), ""},
		{"GET", "/api/projects/file/content?path=" + url.QueryEscape(outsideFile), ""},
		{"POST", "/api/projects/file/save", `{"path":` + q(outsideFile) + `,"content":"x"}`},
		{"GET", "/api/env/files?project=" + url.QueryEscape(outside), ""},
		{"GET", "/api/env/read?path=" + url.QueryEscape(outsideFile), ""},
		{"GET", "/api/env/read?path=" + url.QueryEscape(filepath.Join(ws, "config.php")), ""}, // not an env file
		{"PUT", "/api/env/write", `{"path":` + q(filepath.Join(outside, ".env")) + `,"variables":{}}`},
		{"GET", "/api/env/backups?path=" + url.QueryEscape(filepath.Join(outside, ".env")), ""},
		{"POST", "/api/env/restore", `{"backup_path":` + q(outsideFile) + `,"target_path":` + q(env) + `}`},
		{"POST", "/api/env/restore", `{"backup_path":` + q(filepath.Join(ws, "notes.txt")) + `,"target_path":` + q(env) + `}`},
		{"POST", "/api/artisan/run", `{"project_path":` + q(outside) + `,"command":"list"}`},
		{"POST", "/api/system/open-editor", `{"path":` + q(outside) + `,"editor":"code"}`},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
}

func q(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}
