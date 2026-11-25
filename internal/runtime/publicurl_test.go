package runtime

import (
	"testing"
)

func TestPublicURLOverrides(t *testing.T) {
	const appURL = "http://shop.localhost:8888"
	hosts := []string{"shop.localhost", "api.shop.localhost"}
	cases := []struct {
		name string
		env  map[string]string
		want map[string]string
	}{
		{"old published port", map[string]string{"APP_URL": "http://localhost:8080"}, map[string]string{"APP_URL": appURL}},
		{"keeps the path", map[string]string{"ASSET_URL": "http://127.0.0.1:8080/cdn/"}, map[string]string{"ASSET_URL": appURL + "/cdn"}},
		{"own domain, wrong port", map[string]string{"WP_HOME": "http://shop.localhost"}, map[string]string{"WP_HOME": appURL}},
		{"several frameworks", map[string]string{"NEXTAUTH_URL": "http://localhost:3000", "ORIGIN": "http://localhost:5173"},
			map[string]string{"NEXTAUTH_URL": appURL, "ORIGIN": appURL}},
		{"already right", map[string]string{"APP_URL": appURL + "/"}, nil},
		{"service URLs are never touched", map[string]string{"API_URL": "http://localhost:4000", "VITE_API_URL": "http://localhost:8000"}, nil},
		{"tunnel or production URL", map[string]string{"APP_URL": "https://abc.ngrok.app"}, nil},
		{"interpolated", map[string]string{"APP_URL": "http://localhost:${APP_PORT}"}, nil},
		{"not http", map[string]string{"SITE_URL": "redis://localhost:6379"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PublicURLOverrides(tc.env, appURL, hosts)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestReadEnvFiles(t *testing.T) {
	ctx := testContext(t, "version: 2\napps:\n  web:\n    runtime: node\n    port: 3000\n", map[string]string{
		".env":       "export APP_URL=http://localhost:3000\nSITE_URL='http://localhost' # comment\nA=1\n",
		".env.local": "A=2\n",
	})
	env := ReadEnvFiles(ctx.ProjectRoot)
	if env["APP_URL"] != "http://localhost:3000" || env["SITE_URL"] != "http://localhost" || env["A"] != "2" {
		t.Errorf("env: %v", env)
	}
}

// SLD-run apps get the corrected value as an environment variable, which
// frameworks prefer over their .env files; the SLD config still wins.
func TestPublicURLEnv_InContainer(t *testing.T) {
	files := map[string]string{
		".env":     "APP_URL=http://localhost:8080\nNEXTAUTH_URL=http://localhost:3000\n",
		"artisan":  "",
		"api/.env": "APP_URL=http://localhost:8000\n",
	}
	yaml := "version: 2\napps:\n  web:\n    runtime: php\n    version: \"8.3\"\n    environment:\n      NEXTAUTH_URL: http://custom.example\n  api:\n    runtime: python\n    path: api\n    port: 8000\n    command: python main.py\n"
	ctx := testContext(t, yaml, files)

	envOf := func(app string) map[string]string {
		merged := map[string]string{}
		for _, c := range plan(t, ctx, app).Containers {
			for k, v := range c.Environment {
				merged[k] = v
			}
		}
		return merged
	}
	web, api := envOf("web"), envOf("api")
	webApp, _ := ctx.Config.App("web")
	apiApp, _ := ctx.Config.App("api")
	if want := ctx.Env.SiteURL(webApp.Domain, false); web["APP_URL"] != want {
		t.Errorf("web APP_URL = %q, want %q", web["APP_URL"], want)
	}
	if web["NEXTAUTH_URL"] != "http://custom.example" {
		t.Errorf("the SLD config must win: NEXTAUTH_URL = %q", web["NEXTAUTH_URL"])
	}
	if want := ctx.Env.SiteURL(apiApp.Domain, false); api["APP_URL"] != want {
		t.Errorf("api APP_URL = %q, want %q (its own env file and domain)", api["APP_URL"], want)
	}
}
