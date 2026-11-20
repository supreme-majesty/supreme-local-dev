package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetDomainTLS(t *testing.T) {
	cases := []struct {
		name, in string
		on       bool
		want     string
	}{
		{"adds a domain block after name", "# keep me\nversion: 2\nname: shop\napps:\n  web: {runtime: static}\n", true,
			"# keep me\nversion: 2\nname: shop\ndomain:\n  tls: true\napps:\n  web: {runtime: static}\n"},
		{"adds tls to an existing block", "version: 2\ndomain:\n  name: shop.localhost # main\napps:\n  web: {runtime: static}\n", true,
			"version: 2\ndomain:\n  name: shop.localhost # main\n  tls: true\napps:\n  web: {runtime: static}\n"},
		{"flips false to true", "version: 2\ndomain:\n  tls: false\napps:\n  web: {runtime: static}\n", true,
			"version: 2\ndomain:\n  tls: true\napps:\n  web: {runtime: static}\n"},
		{"off removes the key", "version: 2\ndomain:\n  name: shop.localhost\n  tls: true\napps:\n  web: {runtime: static}\n", false,
			"version: 2\ndomain:\n  name: shop.localhost\napps:\n  web: {runtime: static}\n"},
		{"off drops an emptied block", "version: 2\ndomain:\n  tls: true\napps:\n  web: {runtime: static}\n", false,
			"version: 2\napps:\n  web: {runtime: static}\n"},
		{"off without a block is a no-op", "version: 2\napps:\n  web: {runtime: static}\n", false,
			"version: 2\napps:\n  web: {runtime: static}\n"},
	}
	for _, c := range cases {
		got, err := setDomainTLS([]byte(c.in), c.on)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if string(got) != c.want {
			t.Errorf("%s:\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}

func TestSetTLSEndpoint(t *testing.T) {
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"blog/.git/HEAD":        "x",
		"blog/index.html":       "x",
		"blog/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: static}\n",
		"new/.git/HEAD":         "x",
		"new/index.html":        "x",
	})
	call := classicServer(t, ws, nil)
	base := "/api/v2/projects/" + projectID(t, call, "blog")

	if rec := call("POST", base+"/tls", `{"enabled":true}`); rec.Code != 200 {
		t.Fatalf("enable: %d %s", rec.Code, rec.Body)
	}
	data, _ := os.ReadFile(filepath.Join(ws, "blog", ".sld", "config.yaml"))
	if !strings.Contains(string(data), "tls: true") {
		t.Fatalf("config = %s", data)
	}
	rec := call("GET", base, "")
	var resp struct {
		Project ProjectView `json:"project"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Project.TLS || !strings.HasPrefix(resp.Project.Apps[0].URL, "https://") {
		t.Fatalf("project after enabling: %+v", resp.Project)
	}

	if rec := call("POST", base+"/tls", `{"enabled":false}`); rec.Code != 200 {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(filepath.Join(ws, "blog", ".sld", "config.yaml")); strings.Contains(string(data), "tls") {
		t.Fatalf("config after disabling = %s", data)
	}

	if rec := call("POST", "/api/v2/projects/"+projectID(t, call, "new")+"/tls", `{"enabled":true}`); rec.Code != 409 {
		t.Fatalf("project without config: %d %s", rec.Code, rec.Body)
	}
}
