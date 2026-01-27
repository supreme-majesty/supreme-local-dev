package metrics

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
)

// Stats holds all dashboard metrics
type Stats struct {
	CPUPercent        float64 `json:"cpu_percent"`
	RAMUsage          string  `json:"ram_usage"`
	RAMTotal          string  `json:"ram_total"`
	RAMPercent        float64 `json:"ram_percent"`
	ActiveConnections int     `json:"active_connections"` // From Nginx
	SitesParked       int     `json:"sites_parked"`
	SitesLinked       int     `json:"sites_linked"`
	ServicesRunning   int     `json:"services_running"` // Approximate
}

// Collect gathers current system and daemon stats
func Collect(d *daemon.Daemon) (*Stats, error) {
	stats := &Stats{}

	// 1. System Stats
	v, err := mem.VirtualMemory()
	if err == nil {
		stats.RAMPercent = v.UsedPercent
		stats.RAMUsage = fmt.Sprintf("%.1f GB", float64(v.Used)/1024/1024/1024)
		stats.RAMTotal = fmt.Sprintf("%.1f GB", float64(v.Total)/1024/1024/1024)
	}

	c, err := cpu.Percent(0, false)
	if err == nil && len(c) > 0 {
		stats.CPUPercent = c[0]
	}

	// 2. Daemon Stats
	stats.SitesParked = len(d.State.Data.Paths)
	stats.SitesLinked = len(d.State.Data.Links)

	// Count services (simple check)
	services := []string{"nginx", "dnsmasq"}
	// Add php-fpm if version set
	if d.State.Data.PHPVersion != "" {
		services = append(services, fmt.Sprintf("php%s-fpm", d.State.Data.PHPVersion))
	}

	runningCount := 0
	for _, s := range services {
		if running, _ := d.Adapter.IsServiceRunning(s); running {
			runningCount++
		}
	}
	stats.ServicesRunning = runningCount

	// 3. Nginx Stats (stub_status)
	if active, err := getNginxActiveConnections(); err == nil {
		stats.ActiveConnections = active
	}

	return stats, nil
}

func getNginxActiveConnections() (int, error) {
	client := http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := client.Get("http://127.0.0.1/sld-nginx-status")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// Format:
	// Active connections: 2
	// server accepts handled requests
	//  12 12 12
	// Reading: 0 Writing: 1 Waiting: 1

	content := string(body)
	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		// Parse "Active connections: X"
		parts := strings.Split(lines[0], ":")
		if len(parts) == 2 {
			val := strings.TrimSpace(parts[1])
			return strconv.Atoi(val)
		}
	}

	return 0, fmt.Errorf("could not parse nginx status")
}
