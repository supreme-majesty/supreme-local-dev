package server

// X-Ray's HTTP view: every request that goes through the router. The router
// writes a JSON access log line per request to stdout (see
// orchestrate.RouterStaticConfig); this streams it from the router container
// and publishes each request to the dashboard's event bus.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/pkg/events"
	"github.com/supreme-majesty/supreme-local-dev/pkg/services"
)

// accessLogLine is the part of Traefik's JSON access log X-Ray shows.
type accessLogLine struct {
	StartUTC       string `json:"StartUTC"`
	ClientHost     string `json:"ClientHost"`
	RequestHost    string `json:"RequestHost"`
	RequestMethod  string `json:"RequestMethod"`
	RequestPath    string `json:"RequestPath"`
	RequestScheme  string `json:"RequestScheme"`
	Status         int    `json:"DownstreamStatus"`
	Size           int    `json:"DownstreamContentSize"`
	Duration       int64  `json:"Duration"`       // nanoseconds
	OriginDuration int64  `json:"OriginDuration"` // nanoseconds spent in the app
	EntryPoint     string `json:"entryPointName"`
	UserAgent      string `json:"request_User-Agent"`
}

// parseAccessLog turns one router log line into an X-Ray entry. Traefik's own
// log lines and its internal requests (ping, healthcheck) are skipped.
func parseAccessLog(line []byte) (services.XRayLogEntry, bool) {
	var l accessLogLine
	if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &l) != nil || l.RequestHost == "" || l.EntryPoint == "traefik" {
		return services.XRayLogEntry{}, false
	}
	seconds := func(ns int64) string { return strconv.FormatFloat(float64(ns)/1e9, 'f', 6, 64) }
	started, _ := time.Parse(time.RFC3339Nano, l.StartUTC)
	return services.XRayLogEntry{
		Time:            l.StartUTC,
		Msec:            strconv.FormatInt(started.UnixMilli(), 10),
		RemoteAddr:      l.ClientHost,
		Method:          l.RequestMethod,
		Host:            l.RequestHost,
		URI:             l.RequestPath,
		Scheme:          l.RequestScheme,
		Status:          l.Status,
		BodyBytes:       l.Size,
		Latency:         seconds(l.Duration),
		UpstreamLatency: seconds(l.OriginDuration),
		Agent:           l.UserAgent,
	}, true
}

// StartXRay streams the router's access log until ctx ends, reconnecting
// when the router restarts or is recreated.
func (s *Server) StartXRay(ctx context.Context, bus *events.Bus) {
	go func() {
		since := time.Now()
		for ctx.Err() == nil {
			if err := s.followAccessLog(ctx, bus, since); err != nil && ctx.Err() == nil {
				fmt.Printf("[SLD] X-Ray: router access log unavailable (%v); retrying\n", err)
			}
			since = time.Now()
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
			}
		}
	}()
}

func (s *Server) followAccessLog(ctx context.Context, bus *events.Bus, since time.Time) error {
	rc, err := s.opts.Engine.ContainerLogs(ctx, orchestrate.RouterContainer, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: true,
		Since: strconv.FormatInt(since.Unix(), 10),
	})
	if err != nil {
		return err
	}
	defer rc.Close()
	pr, pw := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, rc)
		pw.CloseWithError(err)
	}()
	defer pr.Close()
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if entry, ok := parseAccessLog(sc.Bytes()); ok {
			bus.Publish(events.Event{Type: events.XRayLog, Payload: entry})
		}
	}
	return sc.Err()
}
