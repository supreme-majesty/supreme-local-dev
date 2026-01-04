package services

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"
)

type TunnelManager struct {
	BinPath string
	Tunnels map[string]*Tunnel // Key: Site Name
	mu      sync.RWMutex
}

type Tunnel struct {
	SiteName  string             `json:"site_name"`
	PublicURL string             `json:"public_url"`
	Process   *os.Process        `json:"-"`
	Cmd       *exec.Cmd          `json:"-"`
	StartedAt time.Time          `json:"started_at"`
	Cancel    context.CancelFunc `json:"-"`
}

func NewTunnelManager(baseDir string) *TunnelManager {
	return &TunnelManager{
		BinPath: filepath.Join(baseDir, "bin", "cloudflared"),
		Tunnels: make(map[string]*Tunnel),
	}
}

// EnsureBinary checks if cloudflared is installed, downloads if not
func (tm *TunnelManager) EnsureBinary() error {
	if _, err := os.Stat(tm.BinPath); err == nil {
		return nil
	}

	// Download
	fmt.Println("Downloading cloudflared...")

	// Create bin dir if not exists
	binDir := filepath.Dir(tm.BinPath)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	url := ""
	switch runtime.GOOS {
	case "linux":
		url = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64"
		// TODO: Support ARM
	case "darwin":
		url = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-amd64"
	default:
		return fmt.Errorf("unsupported OS for auto-download: %s", runtime.GOOS)
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(tm.BinPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}

	return os.Chmod(tm.BinPath, 0755)
}

// StartTunnel starts a tunnel for a given site
func (tm *TunnelManager) StartTunnel(siteName, target string) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if t, ok := tm.Tunnels[siteName]; ok {
		return t.PublicURL, nil
	}

	if err := tm.EnsureBinary(); err != nil {
		return "", fmt.Errorf("failed to setup cloudflared: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Command: cloudflared tunnel --url TARGET --http-host-header site.test
	args := []string{"tunnel", "--url", target, "--http-host-header", siteName + ".test"}
	if len(target) > 5 && target[:5] == "https" {
		args = append(args, "--no-tls-verify")
	}

	cmd := exec.CommandContext(ctx, tm.BinPath, args...)

	// Create pipes to capture URL
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return "", err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return "", err
	}

	// Scan stderr for URL (Cloudflare outputs to stderr)
	scanner := bufio.NewScanner(stderr)
	urlChan := make(chan string)

	go func() {
		// Regex to find trycloudflare.com url
		re := regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)
		for scanner.Scan() {
			line := scanner.Text()
			if match := re.FindString(line); match != "" {
				urlChan <- match
				// Don't close, keep reading to prevent buffer fill
			}
		}
	}()

	select {
	case url := <-urlChan:
		tm.Tunnels[siteName] = &Tunnel{
			SiteName:  siteName,
			PublicURL: url,
			Process:   cmd.Process,
			Cmd:       cmd,
			StartedAt: time.Now(),
			Cancel:    cancel,
		}
		return url, nil
	case <-time.After(15 * time.Second):
		cancel()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return "", fmt.Errorf("timeout waiting for tunnel URL")
	}
}

func (tm *TunnelManager) StopTunnel(siteName string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	t, ok := tm.Tunnels[siteName]
	if !ok {
		return fmt.Errorf("tunnel not found")
	}

	t.Cancel() // Kills process context
	if t.Process != nil {
		t.Process.Kill()
	}
	delete(tm.Tunnels, siteName)
	return nil
}

func (tm *TunnelManager) GetTunnels() []*Tunnel {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	list := make([]*Tunnel, 0, len(tm.Tunnels))
	for _, t := range tm.Tunnels {
		list = append(list, t)
	}
	return list
}
