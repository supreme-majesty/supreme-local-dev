package v2

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	appruntime "github.com/supreme-majesty/supreme-local-dev/internal/runtime"
)

// selfURLKeys are settings in which a framework expects the app's own public
// URL. Links, assets, redirects and auth callbacks are built from them.
var selfURLKeys = map[string]bool{
	"APP_URL": true, "ASSET_URL": true, // Laravel
	"DEFAULT_URI": true,                     // Symfony
	"WP_HOME":     true, "WP_SITEURL": true, // WordPress (Bedrock)
	"NEXTAUTH_URL": true, "AUTH_URL": true, // Auth.js / NextAuth
	"NEXT_PUBLIC_SITE_URL": true, "NEXT_PUBLIC_APP_URL": true, "NEXT_PUBLIC_BASE_URL": true,
	"NUXT_PUBLIC_SITE_URL": true, "NUXT_SITE_URL": true,
	"PUBLIC_SITE_URL": true, "ORIGIN": true, // SvelteKit, Astro
	"SITE_URL": true, "BASE_URL": true, "APP_BASE_URL": true,
}

// envFiles are the env files frameworks load in development, in load order.
var envFiles = []string{".env", ".env.local", ".env.development", ".env.development.local"}

// localHosts are addresses a project typically used before SLD served it on
// its own domain (e.g. a published port such as localhost:8080).
var localHosts = map[string]bool{"localhost": true, "127.0.0.1": true, "0.0.0.0": true, "::1": true}

// EnvURLProblems warns about URLs in a project's env files that point where
// SLD no longer serves the project. Two cases are reported:
//
//   - a setting that holds the app's own URL (APP_URL, NEXTAUTH_URL, ORIGIN,
//     ...) that points at localhost, or at the app's SLD domain with the wrong
//     port or scheme;
//   - any other http(s)://localhost:PORT where PORT is a port SLD now routes
//     on a domain: an app's own port, or a host port the app's compose service
//     published (SLD removes those). VITE_API_URL=http://localhost:8000 then
//     points at the api app's domain.
//
// Tunnels and production URLs are never reported. siteURL returns the URL SLD
// serves a domain on (scheme, host and router port).
func EnvURLProblems(root string, apps []AppInfo, siteURL func(domain string) string) []string {
	ownDomains := map[string]bool{}
	byPort := map[int]AppInfo{} // localhost port the project used -> the app now serving it
	for _, a := range apps {
		if a.Domain == "" {
			continue
		}
		ownDomains[strings.ToLower(a.Domain)] = true
		if a.Port != 0 && a.Runtime != "compose" { // compose: Port is the container port
			byPort[a.Port] = a
		}
	}
	for _, a := range apps {
		if a.Runtime == "compose" && a.Service != "" {
			dir := filepath.Join(root, filepath.FromSlash(a.Path))
			for _, p := range composeHostPorts(dir, a.ComposeFiles, a.Service) {
				byPort[p] = a
			}
		}
	}

	var problems []string
	seen := map[string]bool{}
	check := func(dir string, self *AppInfo) {
		for _, name := range envFiles {
			file := filepath.Join(dir, name)
			rel, err := filepath.Rel(root, file)
			if err != nil {
				rel = name
			}
			rel = filepath.ToSlash(rel)
			env := readDotenv(file)
			keys := make([]string, 0, len(env))
			for k := range env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				value := env[key]
				got := parseHTTPURL(value)
				if got == nil {
					continue
				}
				host := strings.ToLower(got.Hostname())
				var target *AppInfo
				switch {
				case self != nil && selfURLKeys[key] && (localHosts[host] || ownDomains[host]):
					target = self
				case localHosts[host]:
					if a, ok := byPort[portOf(got)]; ok {
						target = &a
					}
				}
				if target == nil {
					continue
				}
				want, err := url.Parse(siteURL(target.Domain))
				if err != nil || origin(got) == origin(want) {
					continue
				}
				id := rel + "|" + key
				if seen[id] {
					continue
				}
				seen[id] = true
				base := want.Scheme + "://" + want.Host
				fix := base + strings.TrimRight(got.EscapedPath(), "/")
				problems = append(problems, fmt.Sprintf(
					"%s sets %s=%s, but SLD serves %s at %s, so links built from it point to an address that no longer works. Set %s=%s and restart the project.",
					rel, key, value, target.Name, base, key, fix))
			}
		}
	}

	scanned := map[string]bool{}
	for i := range apps {
		dir := filepath.Join(root, filepath.FromSlash(apps[i].Path))
		if scanned[dir] || apps[i].Domain == "" {
			continue
		}
		scanned[dir] = true
		check(dir, &apps[i])
	}
	if !scanned[root] {
		check(root, nil) // a monorepo's shared env files: only port-based matches
	}
	return problems
}

// parseHTTPURL returns value as an absolute http(s) URL, or nil.
func parseHTTPURL(value string) *url.URL {
	if value == "" || strings.Contains(value, "${") {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil
	}
	return u
}

func portOf(u *url.URL) int {
	if p, err := strconv.Atoi(u.Port()); err == nil {
		return p
	}
	if u.Scheme == "https" {
		return 443
	}
	return 80
}

// origin is scheme://host:port with the default port made explicit.
func origin(u *url.URL) string {
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Hostname()) + ":" + strconv.Itoa(portOf(u))
}

// composeHostPorts returns the host ports a compose service publishes (which
// SLD removes, routing a domain instead). ${VAR:-default} is resolved from the
// stack's .env, then from the default.
func composeHostPorts(dir string, files []string, service string) []int {
	if len(files) == 0 {
		files = appruntime.DefaultComposeFiles(dir)
	}
	env := readDotenv(filepath.Join(dir, ".env"))
	var ports []int
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		var doc struct {
			Services map[string]struct {
				Ports []any `yaml:"ports"`
			} `yaml:"services"`
		}
		if yaml.Unmarshal(data, &doc) != nil {
			continue
		}
		for _, entry := range doc.Services[service].Ports {
			if p := hostPort(entry, env); p != 0 {
				ports = append(ports, p)
			}
		}
	}
	return ports
}

var composeVar = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::?-([^}]*))?\}`)

// hostPort extracts the published port of a compose port entry:
// "8080:80", "${APP_PORT:-8080}:80", "127.0.0.1:8080:80/tcp" or {published: 8080}.
func hostPort(entry any, env map[string]string) int {
	var s string
	switch v := entry.(type) {
	case string:
		s = v
	case map[string]any:
		s = fmt.Sprint(v["published"])
	default:
		return 0
	}
	s = composeVar.ReplaceAllStringFunc(s, func(m string) string {
		parts := composeVar.FindStringSubmatch(m)
		if v := env[parts[1]]; v != "" {
			return v
		}
		return parts[2]
	})
	s, _, _ = strings.Cut(s, "/")
	parts := strings.Split(s, ":")
	if _, isMap := entry.(map[string]any); !isMap {
		if len(parts) < 2 {
			return 0 // only a container port: nothing is published on a fixed host port
		}
		s = parts[len(parts)-2]
	}
	p, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return p
}

// readDotenv reads KEY=VALUE lines of a .env file. It handles comments,
// `export`, and single or double quotes; anything fancier is ignored.
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
			// Quoted: up to the closing quote; a trailing comment may follow.
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
