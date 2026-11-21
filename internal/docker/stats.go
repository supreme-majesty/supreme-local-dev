package docker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/docker/docker/api/types/container"
)

// Usage is a container's resource use at one moment.
type Usage struct {
	CPUPercent float64 `json:"cpu_percent"` // of one CPU (200 = two cores busy), like `docker stats`
	MemBytes   uint64  `json:"mem_bytes"`   // working set (page cache excluded)
	MemLimit   uint64  `json:"mem_limit"`
	NetRxBytes uint64  `json:"net_rx_bytes"`
	NetTxBytes uint64  `json:"net_tx_bytes"`
}

// UsageReader reads container resource use. DockerEngine implements it; it
// is separate from EngineClient so fakes need not.
type UsageReader interface {
	ContainerUsage(ctx context.Context, containerID string) (Usage, error)
}

var _ UsageReader = (*DockerEngine)(nil)

// ContainerUsage takes one stats sample. The Engine waits for a second
// sample (about a second) so the CPU percentage covers a real interval.
func (e *DockerEngine) ContainerUsage(ctx context.Context, containerID string) (Usage, error) {
	resp, err := e.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return Usage{}, fmt.Errorf("stats for %s: %w", containerID, err)
	}
	defer resp.Body.Close()
	var s container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return Usage{}, fmt.Errorf("decoding stats for %s: %w", containerID, err)
	}
	return UsageFromStats(&s), nil
}

// UsageFromStats computes usage the way the docker CLI does.
func UsageFromStats(s *container.StatsResponse) Usage {
	u := Usage{MemLimit: s.MemoryStats.Limit}

	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	cpus := float64(s.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpuDelta > 0 && sysDelta > 0 && cpus > 0 {
		u.CPUPercent = cpuDelta / sysDelta * cpus * 100
	}

	// Page cache is reclaimable; cgroup v2 reports it as inactive_file,
	// v1 as total_inactive_file.
	u.MemBytes = s.MemoryStats.Usage
	inactive := s.MemoryStats.Stats["inactive_file"]
	if v, ok := s.MemoryStats.Stats["total_inactive_file"]; ok {
		inactive = v
	}
	if inactive < u.MemBytes {
		u.MemBytes -= inactive
	}

	for _, n := range s.Networks {
		u.NetRxBytes += n.RxBytes
		u.NetTxBytes += n.TxBytes
	}
	return u
}
