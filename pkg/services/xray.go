package services

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/hpcloud/tail"
	"github.com/supreme-majesty/supreme-local-dev/pkg/events"
)

type XRayLogEntry struct {
	Time            string `json:"time_iso"`
	Msec            string `json:"msec"`
	RemoteAddr      string `json:"remote_addr"`
	Method          string `json:"method"`
	Host            string `json:"host"`
	URI             string `json:"uri"`
	Status          int    `json:"status"`
	BodyBytes       int    `json:"body_bytes"`
	Latency         string `json:"latency"`
	UpstreamLatency string `json:"upstream_latency"`
	Agent           string `json:"agent"`
}

type XRayService struct {
	LogPath string
	Bus     *events.Bus
	Tail    *tail.Tail
}

func NewXRayService(bus *events.Bus) *XRayService {
	return &XRayService{
		LogPath: "/var/log/nginx/sld-xray.log",
		Bus:     bus,
	}
}

func (x *XRayService) Start() error {
	// Ensure log file exists to prevent tail error
	if _, err := os.Stat(x.LogPath); os.IsNotExist(err) {
		os.WriteFile(x.LogPath, []byte(""), 0666)
	}
	os.Chmod(x.LogPath, 0666) // Always ensure it's writable by Nginx

	t, err := tail.TailFile(x.LogPath, tail.Config{
		Follow: true,
		ReOpen: true, // Handle log rotation
		Poll:   true, // Needed for mounted filesystems sometimes
		Location: &tail.SeekInfo{
			Offset: 0,
			Whence: io.SeekEnd,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to tail xray log: %w", err)
	}

	x.Tail = t

	// Process logs in background
	go func() {
		for line := range t.Lines {
			if line.Text == "" {
				continue
			}

			// Parse JSON
			var entry XRayLogEntry
			if err := json.Unmarshal([]byte(line.Text), &entry); err != nil {
				// Raw log if parsing fails (fallback)
				// fmt.Println("XRay Parse Error:", err)
				continue
			}

			// Broadcast
			x.Bus.Publish(events.Event{
				Type:    events.XRayLog,
				Payload: entry,
			})
		}
	}()

	fmt.Println("X-Ray Service started 📡")
	return nil
}

func (x *XRayService) Stop() {
	if x.Tail != nil {
		x.Tail.Cleanup()
		x.Tail.Stop()
	}
}
