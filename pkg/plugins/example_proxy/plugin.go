// Package example_proxy provides an example plugin demonstrating the NginxHook interface.
// This plugin adds a reverse proxy location to all sites.
package example_proxy

import (
	"fmt"

	"github.com/supreme-majesty/supreme-local-dev/pkg/plugins"
)

// ExampleProxyPlugin demonstrates how to implement the NginxHook interface.
type ExampleProxyPlugin struct {
	status      plugins.Status
	proxyTarget string
}

// NewExampleProxyPlugin creates a new instance of the example proxy plugin.
func NewExampleProxyPlugin() *ExampleProxyPlugin {
	return &ExampleProxyPlugin{
		status:      plugins.StatusStopped,
		proxyTarget: "http://127.0.0.1:3000",
	}
}

// --- Plugin Interface Implementation ---

func (p *ExampleProxyPlugin) ID() string {
	return "example-proxy"
}

func (p *ExampleProxyPlugin) Name() string {
	return "Example Proxy"
}

func (p *ExampleProxyPlugin) Description() string {
	return "Demonstrates NginxHook by adding a /api proxy location to all sites"
}

func (p *ExampleProxyPlugin) Version() string {
	return "1.0.0"
}

func (p *ExampleProxyPlugin) Status() plugins.Status {
	return p.status
}

func (p *ExampleProxyPlugin) Install() error {
	// Nothing to install for this example
	return nil
}

func (p *ExampleProxyPlugin) Start() error {
	p.status = plugins.StatusRunning
	return nil
}

func (p *ExampleProxyPlugin) Stop() error {
	p.status = plugins.StatusStopped
	return nil
}

func (p *ExampleProxyPlugin) IsInstalled() bool {
	return true // Always "installed" since it's built-in
}

// --- NginxHook Implementation ---

// NginxConfig returns Nginx configuration blocks to be included.
// This example adds a location block that proxies /api/* to a backend service.
func (p *ExampleProxyPlugin) NginxConfig() (map[string]string, error) {
	if p.status != plugins.StatusRunning {
		return nil, nil
	}

	// Return a named configuration block
	config := fmt.Sprintf(`
# Example Proxy Plugin - Proxies /api/* to backend
location /api/ {
    proxy_pass %s;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection 'upgrade';
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_cache_bypass $http_upgrade;
}
`, p.proxyTarget)

	return map[string]string{
		"api-proxy": config,
	}, nil
}

// SetProxyTarget allows configuring the backend URL at runtime.
func (p *ExampleProxyPlugin) SetProxyTarget(target string) {
	p.proxyTarget = target
}
