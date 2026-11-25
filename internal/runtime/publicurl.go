package runtime

import (
	"bufio"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// publicURLKeys are settings in which a framework expects the app's own
// public address, used only to build links, assets, redirects and callbacks.
// URLs of other services (API_URL, VITE_API_URL, ...) are deliberately not
// here: they may be called from inside a container, where the right address
// is the service name, not the public domain.
var publicURLKeys = map[string]bool{
	"APP_URL": true, "ASSET_URL": true, // Laravel
	"DEFAULT_URI": true,                     // Symfony
	"WP_HOME":     true, "WP_SITEURL": true, // WordPress (Bedrock)
	"NEXTAUTH_URL": true, "AUTH_URL": true, // Auth.js / NextAuth
	"NEXT_PUBLIC_SITE_URL": true, "NEXT_PUBLIC_APP_URL": true,
	"NUXT_PUBLIC_SITE_URL": true, "NUXT_SITE_URL": true,
	"PUBLIC_SITE_URL": true, "SITE_URL": true, "ORIGIN": true, // Astro, SvelteKit
}

// envFiles are the env files frameworks read in development, in load order
// (later files override earlier ones).
var envFiles = []string{".env", ".env.development", ".env.local", ".env.development.local"}

var localHosts = map[string]bool{"localhost": true, "127.0.0.1": true, "0.0.0.0": true, "::1": true}

// PublicURLOverrides returns corrected values for env settings that hold the
// app's own public address but still point where the app was served before
// SLD (localhost:8080, 127.0.0.1:3000, ...) or at its SLD domain with the
// wrong port or scheme. The path of the old value is kept. Tunnels, production
// URLs and ${...} values are left alone.
//
// env is what the app sees; appURL is the URL SLD serves it on; ownHosts are
// the project's SLD domains.
func PublicURLOverrides(env map[string]string, appURL string, ownHosts []string) map[string]string {
	want, err := url.Parse(appURL)
	if err != nil || want.Host == "" {
		return nil
	}
	own := map[string]bool{}
	for _, h := range ownHosts {
		own[strings.ToLower(h)] = true
	}
	var out map[string]string
	for key, value := range env {
		if !publicURLKeys[key] || value == "" || strings.Contains(value, "${") {
			continue
		}
		got, err := url.Parse(value)
		if err != nil || got.Host == "" || (got.Scheme != "http" && got.Scheme != "https") {
			continue
		}
		host := strings.ToLower(got.Hostname())
		if !localHosts[host] && !own[host] {
			continue
		}
		if origin(got) == origin(want) {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[key] = want.Scheme + "://" + want.Host + strings.TrimRight(got.EscapedPath(), "/")
	}
	return out
}

func origin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

// ReadEnvFiles merges an app directory's development env files.
func ReadEnvFiles(dir string) map[string]string {
	env := map[string]string{}
	for _, name := range envFiles {
		for k, v := range readDotenv(filepath.Join(dir, name)) {
			env[k] = v
		}
	}
	return env
}

// readDotenv reads KEY=VALUE lines of a .env file. It handles comments,
// `export`, and quoted values followed by a comment.
func readDotenv(file string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(file)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
			if end := strings.IndexByte(value[1:], value[0]); end >= 0 {
				value = value[1 : end+1]
			}
		} else if i := strings.Index(value, " #"); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}
		out[strings.TrimSpace(key)] = value
	}
	return out
}
