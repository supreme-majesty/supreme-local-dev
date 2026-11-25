package orchestrate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// RouterURL is how SLD itself reaches the router: by container name over the
// shared network when SLD runs in Docker, otherwise through the host port.
func (o *Orchestrator) RouterURL() string {
	if o.env.InContainer() {
		return "http://" + RouterContainer + ":80"
	}
	return fmt.Sprintf("http://127.0.0.1:%d", o.env.HTTPPort())
}

// WaitForRoutes waits until each host is served: the router has a route and
// the app behind it answers. Traefik picks up new containers a moment after
// they start, and a proxy in front of the app (nginx before PHP-FPM, ...)
// answers 502 until the app is up; waiting here means "started" also means
// "reachable". Hosts still not served after timeout are returned.
func (o *Orchestrator) WaitForRoutes(ctx context.Context, hosts []string, timeout time.Duration) []string {
	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // a redirect means the app answered
		},
	}
	pending := append([]string(nil), hosts...)
	deadline := time.Now().Add(timeout)
	for len(pending) > 0 {
		var still []string
		for _, host := range pending {
			if !o.routed(ctx, client, host) {
				still = append(still, host)
			}
		}
		pending = still
		if len(pending) == 0 || time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return pending
		case <-time.After(time.Second):
		}
	}
	return pending
}

// routed reports whether the router has a route for host. Traefik's own 404
// body is exactly "404 page not found"; an app's 404 page is a valid route.
func (o *Orchestrator) routed(ctx context.Context, client *http.Client, host string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.RouterURL()+"/", nil)
	if err != nil {
		return false
	}
	req.Host = host
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	return served(resp.StatusCode, string(body))
}

// served reports whether a response through the router comes from the app.
// Not yet: Traefik's own 404 (no route; an app's 404 page has another body),
// and 502/503/504, which a proxy in front of the app (or Traefik itself)
// returns while the app behind it is still starting. Anything else, including
// redirects and the app's own errors, means the app answered.
func served(status int, body string) bool {
	switch status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return false
	case http.StatusNotFound:
		return body != "404 page not found\n"
	}
	return true
}

// staticSiteConf is served by static apps instead of nginx's default
// config: same document root, but hidden files (.git, .env, .sld) are refused.
const staticSiteConf = `# Managed by SLD. Do not edit.
server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html index.htm;

    # Never serve hidden files or folders (.git, .env, .sld, ...).
    location ~ /\.(?!well-known/) {
        return 404;
    }

    location / {
        try_files $uri $uri/ =404;
    }
}
`

// staticNginxConfig writes the static-site nginx config into SLD's data
// directory and returns a read-only mount of it at /etc/nginx/conf.d.
func (o *Orchestrator) staticNginxConfig() (reconcile.Mount, error) {
	dir := filepath.Join(o.env.DataDir(), "nginx-static")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return reconcile.Mount{}, err
	}
	path := filepath.Join(dir, "default.conf")
	if existing, err := os.ReadFile(path); err != nil || string(existing) != staticSiteConf {
		if err := os.WriteFile(path, []byte(staticSiteConf), 0o644); err != nil {
			return reconcile.Mount{}, fmt.Errorf("failed to write static site config: %w", err)
		}
	}
	return o.env.MountFor(dir, "/etc/nginx/conf.d", true)
}
