package v2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

var testSiteURL = func(domain string) string { return "http://" + domain + ":8888" }

func expectProblems(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d problem(s), got %d: %q", len(want), len(got), got)
	}
	for i, sub := range want {
		if !strings.Contains(got[i], sub) {
			t.Errorf("problem %q does not contain %q", got[i], sub)
		}
	}
}

// The sunport case: a compose stack that published ${APP_PORT:-8080}:80, with
// APP_URL still pointing there after SLD routed it to sunport.localhost.
func TestEnvURLProblems_ComposeOldPort(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"backend/docker-compose.yml": "services:\n  nginx:\n    image: nginx\n    ports:\n      - \"${APP_PORT:-8080}:80\"\n  app:\n    image: php\n",
		"backend/.env":               "APP_NAME=Sunport\nAPP_URL=http://localhost:8080\nMAIL_HOST=127.0.0.1\nPAYMENT_CALLBACK=http://localhost:8080/api/pay\n",
	})
	apps := []AppInfo{{Name: "backend", Runtime: "compose", Domain: "sunport.localhost", Path: "backend", Service: "nginx", Port: 80}}
	expectProblems(t, EnvURLProblems(root, apps, testSiteURL),
		"backend/.env sets APP_URL=http://localhost:8080, but SLD serves backend at http://sunport.localhost:8888",
		"Set PAYMENT_CALLBACK=http://sunport.localhost:8888/api/pay ",
	)
}

// Any stack: a Vite frontend that calls the api app on its old localhost port,
// and a Next.js app whose NEXTAUTH_URL is its old dev server address.
func TestEnvURLProblems_MultiApp(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"web/.env.local": "VITE_API_URL=http://localhost:8000/v1\nVITE_OLLAMA=http://localhost:11434\n",
		"admin/.env":     "NEXTAUTH_URL=http://localhost:3000\nNEXT_PUBLIC_SITE_URL=http://admin.shop.localhost:8888\n",
	})
	apps := []AppInfo{
		{Name: "web", Runtime: "node", Domain: "shop.localhost", Path: "web", Port: 5173},
		{Name: "api", Runtime: "python", Domain: "api.shop.localhost", Path: "api", Port: 8000},
		{Name: "admin", Runtime: "node", Domain: "admin.shop.localhost", Path: "admin", Port: 3000},
	}
	expectProblems(t, EnvURLProblems(root, apps, testSiteURL),
		"web/.env.local sets VITE_API_URL=http://localhost:8000/v1, but SLD serves api at http://api.shop.localhost:8888",
		"admin/.env sets NEXTAUTH_URL=http://localhost:3000, but SLD serves admin at http://admin.shop.localhost:8888",
	)
}

func TestEnvURLProblems_SelfKeys(t *testing.T) {
	apps := []AppInfo{{Name: "web", Runtime: "php", Domain: "shop.localhost", Port: 80}}
	cases := []struct {
		name, env string
		want      []string
	}{
		{"matches", "APP_URL=http://shop.localhost:8888\n", nil},
		{"matches with quotes and slash", "APP_URL=\"http://shop.localhost:8888/\"\n", nil},
		{"own domain, wrong port", "APP_URL=http://shop.localhost\n", []string{"APP_URL=http://shop.localhost,"}},
		{"keeps the path", "ASSET_URL='http://127.0.0.1:9000/cdn' # local\n", []string{"Set ASSET_URL=http://shop.localhost:8888/cdn "}},
		{"wordpress", "WP_HOME=http://localhost\n", []string{"Set WP_HOME=http://shop.localhost:8888 "}},
		{"sveltekit origin", "ORIGIN=http://localhost:5173\n", []string{"Set ORIGIN=http://shop.localhost:8888 "}},
		{"tunnel is the user's choice", "APP_URL=https://abc.ngrok.app\n", nil},
		{"interpolated", "APP_URL=http://localhost:${APP_PORT}\n", nil},
		{"commented out", "# APP_URL=http://localhost:8080\n", nil},
		{"not a URL setting", "DB_HOST=localhost\nREDIS_URL=redis://localhost:6379\n", nil},
		{"unknown local port", "OLLAMA_URL=http://localhost:11434\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeFiles(t, map[string]string{".env": tc.env})
			expectProblems(t, EnvURLProblems(root, apps, testSiteURL), tc.want...)
		})
	}
}

func TestHostPort(t *testing.T) {
	env := map[string]string{"WEB_PORT": "9090"}
	cases := map[any]int{
		"8080:80":               8080,
		"127.0.0.1:8081:80/tcp": 8081,
		"${APP_PORT:-8082}:80":  8082,
		"${WEB_PORT:-1}:80":     9090,
		"80":                    0,
		"${MISSING}:80":         0,
	}
	for entry, want := range cases {
		if got := hostPort(entry, env); got != want {
			t.Errorf("hostPort(%q) = %d, want %d", entry, got, want)
		}
	}
	if got := hostPort(map[string]any{"target": 80, "published": 8083}, env); got != 8083 {
		t.Errorf("long syntax: got %d", got)
	}
}
