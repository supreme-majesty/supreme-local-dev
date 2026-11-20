package detect

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	appruntime "github.com/supreme-majesty/supreme-local-dev/internal/runtime"
)

// composeService is what detection needs from a compose service.
type composeService struct {
	Image string `yaml:"image"`
	Ports []any  `yaml:"ports"`
}

// nonWebMarkers identify databases, caches and admin/backup tools; SLD does
// not give them a domain (they stay reachable inside the stack).
var nonWebMarkers = []string{
	"mysql", "mariadb", "postgres", "redis", "valkey", "mongo", "memcached", "rabbitmq",
	"elasticsearch", "opensearch", "meilisearch", "typesense", "minio", "phpmyadmin",
	"adminer", "mailpit", "mailhog", "backup", "rclone", "clamav", "selenium",
}

// nonWebPorts are container ports that are never the HTTP entry point.
var nonWebPorts = map[int]bool{
	3306: true, 5432: true, 6379: true, 27017: true, 9000: true, 11211: true,
	5672: true, 9200: true, 1025: true, 9001: true, 33060: true,
}

// serviceRank orders services so the usual HTTP entry point comes first.
var serviceRank = []string{"nginx", "web", "webserver", "caddy", "httpd", "apache", "frontend", "app", "api", "server"}

// composeCandidate is a service that can be served on a domain.
type composeCandidate struct {
	service string
	port    int
}

// detectCompose returns one compose app per HTTP service of the directory's
// own compose stack (if it has one).
func detectCompose(p *proj, _ *config.Config) []config.App {
	files := appruntime.DefaultComposeFiles(p.root)
	if len(files) == 0 {
		return nil
	}

	services := map[string]composeService{}
	for _, f := range files {
		var doc struct {
			Services map[string]composeService `yaml:"services"`
		}
		data, err := os.ReadFile(filepath.Join(p.root, f))
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			p.note("⚠️  could not parse %s: %v", f, err)
			continue
		}
		for name, svc := range doc.Services {
			merged := services[name]
			if svc.Image != "" {
				merged.Image = svc.Image
			}
			if len(svc.Ports) > 0 {
				merged.Ports = svc.Ports
			}
			services[name] = merged
		}
	}

	candidates := webCandidates(services)
	if len(candidates) == 0 {
		p.note("⚠️  %s has no service publishing an HTTP port; set apps.<name>.service and port", strings.Join(files, " + "))
		return nil
	}

	var apps []config.App
	for i, c := range candidates {
		name := "web"
		if i > 0 {
			name = appName(c.service)
		}
		apps = append(apps, config.App{Name: name, Runtime: config.RuntimeCompose, Service: c.service, Port: c.port})
		p.note("docker compose service %q (port %d)", c.service, c.port)
	}
	p.note("uses its own compose stack (%s); SLD removes its published host ports and routes the domain(s) instead", strings.Join(files, " + "))
	return apps
}

func webCandidates(services map[string]composeService) []composeCandidate {
	names := make([]string, 0, len(services))
	for n := range services {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		ri, rj := rankOf(names[i]), rankOf(names[j])
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})

	var out []composeCandidate
	for _, name := range names {
		svc := services[name]
		if isNonWeb(name, svc.Image) {
			continue
		}
		if port := httpPort(svc.Ports); port != 0 {
			out = append(out, composeCandidate{service: name, port: port})
		}
	}
	return out
}

func rankOf(name string) int {
	for i, n := range serviceRank {
		if strings.EqualFold(name, n) {
			return i
		}
	}
	return len(serviceRank)
}

func isNonWeb(name, image string) bool {
	s := strings.ToLower(name + " " + image)
	for _, m := range nonWebMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// httpPort picks the container port of the first published port that looks
// like HTTP, preferring 80 over 443.
func httpPort(ports []any) int {
	var found []int
	for _, p := range ports {
		if port := containerPort(p); port != 0 && !nonWebPorts[port] {
			found = append(found, port)
		}
	}
	for _, port := range found {
		if port != 443 {
			return port
		}
	}
	if len(found) > 0 {
		return found[0]
	}
	return 0
}

// containerPort extracts the container side of a compose port entry:
// "8080", "${APP_PORT:-8080}:80", "127.0.0.1:3307:3306/tcp", or {target: 80}.
func containerPort(entry any) int {
	switch v := entry.(type) {
	case int:
		return v
	case string:
		s := strings.TrimSpace(v)
		if i := strings.Index(s, "/"); i >= 0 {
			s = s[:i]
		}
		// The container port follows the last ':' that is not inside ${...}.
		depth, last := 0, -1
		for i, r := range s {
			switch r {
			case '{':
				depth++
			case '}':
				depth--
			case ':':
				if depth == 0 {
					last = i
				}
			}
		}
		if last >= 0 {
			s = s[last+1:]
		}
		if strings.Contains(s, "-") || strings.Contains(s, "$") {
			return 0 // ranges and interpolated container ports are not guessed
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0
		}
		return n
	case map[string]any:
		if t, ok := v["target"]; ok {
			return containerPort(fmt.Sprint(t))
		}
	}
	return 0
}

// --- project naming ---

// genericDirs are folder names that describe a part of a project rather than
// the project; their parent folder names the project instead.
var genericDirs = map[string]bool{
	"backend": true, "frontend": true, "api": true, "server": true, "app": true,
	"web": true, "client": true, "src": true, "site": true, "laravel": true, "www": true,
}

// nameSuffixes are dropped from folder names: "sunport-apps" -> "sunport".
var nameSuffixes = []string{"-apps", "_apps", "-builds", "_builds", "-projects", "_projects", "-monorepo"}

// ProjectName derives a project's name from its folder: a generic folder
// (backend, api, ...) takes its parent's name, and generic suffixes are dropped.
func ProjectName(root string) string {
	clean := filepath.Clean(root)
	name := filepath.Base(clean)
	if genericDirs[strings.ToLower(name)] {
		if parent := filepath.Base(filepath.Dir(clean)); parent != "." && parent != string(filepath.Separator) && parent != "" {
			name = parent
		}
	}
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)
	for _, suffix := range nameSuffixes {
		if strings.HasSuffix(lower, suffix) && len(name) > len(suffix) {
			name = name[:len(name)-len(suffix)]
			break
		}
	}
	return name
}
