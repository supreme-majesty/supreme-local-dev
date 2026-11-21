package server

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// ContainerHealth is one container's state, resource use and recent trouble.
type ContainerHealth struct {
	Name         string        `json:"name"`
	Service      string        `json:"service"`
	App          string        `json:"app,omitempty"`
	State        string        `json:"state"`
	Health       string        `json:"health,omitempty"` // healthy | unhealthy | starting (when the image has a healthcheck)
	RestartCount int           `json:"restart_count"`
	ExitCode     int           `json:"exit_code"`
	OOMKilled    bool          `json:"oom_killed,omitempty"`
	StartedAt    time.Time     `json:"started_at,omitzero"`
	FinishedAt   time.Time     `json:"finished_at,omitzero"`
	CrashLoop    bool          `json:"crash_loop,omitempty"`
	Problem      string        `json:"problem,omitempty"` // one-line summary of what is wrong, if anything
	Usage        *docker.Usage `json:"usage,omitempty"`
	LastError    *LogLine      `json:"last_error,omitempty"`
}

// LogLine is one line of a container's log.
type LogLine struct {
	Time time.Time `json:"time,omitzero"`
	Line string    `json:"line"`
}

// ProjectUsage sums the resource use of one project's running containers.
type ProjectUsage struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemBytes   uint64  `json:"mem_bytes"`
	Running    int     `json:"running"`
	Problems   int     `json:"problems"` // containers crash-looping, unhealthy, OOM-killed or exited with an error
}

// usageCache keeps the latest stats sample per container for a few seconds:
// taking one costs the Engine about a second per container.
type usageCache struct {
	mu      sync.Mutex
	sampled time.Time
	byID    map[string]docker.Usage
	running chan struct{} // closed when the sampling in progress finishes
}

const usageTTL = 5 * time.Second

// sample returns usage for the given running containers, sampling them in
// parallel when the cached values are stale. Concurrent callers share one
// sampling round.
func (uc *usageCache) sample(ctx context.Context, reader docker.UsageReader, ids []string) map[string]docker.Usage {
	uc.mu.Lock()
	fresh := time.Since(uc.sampled) < usageTTL && containsAll(uc.byID, ids)
	if fresh {
		out := uc.byID
		uc.mu.Unlock()
		return out
	}
	if uc.running != nil {
		wait := uc.running
		uc.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
		}
		uc.mu.Lock()
		out := uc.byID
		uc.mu.Unlock()
		return out
	}
	done := make(chan struct{})
	uc.running = done
	uc.mu.Unlock()

	results := make(map[string]docker.Usage, len(ids))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if u, err := reader.ContainerUsage(sctx, id); err == nil {
				mu.Lock()
				results[id] = u
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	uc.mu.Lock()
	uc.byID, uc.sampled, uc.running = results, time.Now(), nil
	uc.mu.Unlock()
	close(done)
	return results
}

func containsAll(m map[string]docker.Usage, ids []string) bool {
	for _, id := range ids {
		if _, ok := m[id]; !ok {
			return false
		}
	}
	return true
}

// handleUsage sums resource use per project: GET /api/v2/usage.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	reader, ok := s.opts.Engine.(docker.UsageReader)
	if !ok {
		writeError(w, http.StatusNotImplemented, "the Docker engine cannot report usage")
		return
	}
	all, err := s.opts.Engine.ListAllContainers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var ids []string
	for _, c := range all {
		if c.State == "running" {
			ids = append(ids, c.ID)
		}
	}
	usage := s.usage.sample(r.Context(), reader, ids)

	projects := map[string]*ProjectUsage{}
	var total ProjectUsage
	for _, c := range all {
		pid := c.Labels[docker.LabelProject]
		if pid == "" {
			pid = "sld" // SLD's own containers: router, plugins, tunnels
		}
		p := projects[pid]
		if p == nil {
			p = &ProjectUsage{}
			projects[pid] = p
		}
		if c.State == "running" {
			p.Running++
			total.Running++
		}
		if u, ok := usage[c.ID]; ok {
			p.CPUPercent += u.CPUPercent
			p.MemBytes += u.MemBytes
			total.CPUPercent += u.CPUPercent
			total.MemBytes += u.MemBytes
		}
	}
	// Problems come from inspecting each container (cheap next to stats).
	problem := make([]bool, len(all))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i, c := range all {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			problem[i] = s.inspectHealth(r.Context(), c).Problem != ""
		}()
	}
	wg.Wait()
	for i, c := range all {
		if !problem[i] {
			continue
		}
		pid := c.Labels[docker.LabelProject]
		if pid == "" {
			pid = "sld"
		}
		projects[pid].Problems++
		total.Problems++
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects, "total": total, "sampled_at": time.Now().UTC()})
}

// handleProjectHealth reports each container of a project:
// GET /api/v2/projects/{id}/health.
func (s *Server) handleProjectHealth(w http.ResponseWriter, r *http.Request) {
	p, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	identity, err := s.opts.Env.ProjectIdentity(p.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	containers, err := s.opts.Engine.ListProjectContainers(r.Context(), identity.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var usage map[string]docker.Usage
	if reader, ok := s.opts.Engine.(docker.UsageReader); ok {
		var ids []string
		for _, c := range containers {
			if c.State == "running" {
				ids = append(ids, c.ID)
			}
		}
		usage = s.usage.sample(r.Context(), reader, ids)
	}

	out := make([]ContainerHealth, len(containers))
	var wg sync.WaitGroup
	for i, c := range containers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := s.inspectHealth(r.Context(), c)
			if u, ok := usage[c.ID]; ok {
				h.Usage = &u
			}
			h.LastError = s.lastErrorLine(r.Context(), c.ID)
			out[i] = h
		}()
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	var sum ProjectUsage
	for _, h := range out {
		if h.State == "running" {
			sum.Running++
		}
		if h.Usage != nil {
			sum.CPUPercent += h.Usage.CPUPercent
			sum.MemBytes += h.Usage.MemBytes
		}
		if h.Problem != "" {
			sum.Problems++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"containers": out, "total": sum})
}

// inspectHealth reads a container's state and names what is wrong with it.
func (s *Server) inspectHealth(ctx context.Context, c *reconcile.ActualContainer) ContainerHealth {
	h := ContainerHealth{Name: c.Name, Service: c.Service, App: c.Labels[docker.LabelApp], State: c.State}
	info, err := s.opts.Engine.InspectContainer(ctx, c.ID)
	if err != nil || info.ContainerJSONBase == nil || info.State == nil {
		return h
	}
	st := info.State
	h.RestartCount = info.RestartCount
	h.ExitCode = st.ExitCode
	h.OOMKilled = st.OOMKilled
	h.StartedAt = parseDockerTime(st.StartedAt)
	h.FinishedAt = parseDockerTime(st.FinishedAt)
	if st.Health != nil {
		h.Health = st.Health.Status
	}
	h.CrashLoop, h.Problem = diagnose(h, st.Restarting, time.Now())
	return h
}

// diagnose names the most important problem of a container, if any.
func diagnose(h ContainerHealth, restarting bool, now time.Time) (crashLoop bool, problem string) {
	recentlyStarted := !h.StartedAt.IsZero() && now.Sub(h.StartedAt) < 2*time.Minute
	crashLoop = restarting || (h.RestartCount >= 3 && recentlyStarted)
	switch {
	case crashLoop:
		problem = "keeps restarting (crash loop)"
		if h.ExitCode != 0 {
			problem += ", last exit code " + strconv.Itoa(h.ExitCode)
		}
	case h.OOMKilled:
		problem = "killed for running out of memory"
	case h.Health == "unhealthy" && h.State == "running":
		// Docker keeps the last result after a container stops; only a
		// running container can be unhealthy.
		problem = "healthcheck failing"
	case h.State == "exited" && h.ExitCode != 0 && h.ExitCode <= 128:
		// Codes above 128 mean a signal ended it (128+N: SIGTERM 143, SIGKILL
		// 137, SIGQUIT 131 for php-fpm...), which is how docker stop ends
		// containers, not a crash. OOM kills are reported above.
		problem = "exited with code " + strconv.Itoa(h.ExitCode)
	case h.State == "dead":
		problem = "container is dead"
	}
	return crashLoop, problem
}

func parseDockerTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil || t.Year() < 2000 { // "0001-01-01T00:00:00Z" means never
		return time.Time{}
	}
	return t
}

var (
	errorLine = regexp.MustCompile(`(?i)(\berror\b|\bexception\b|\bfatal\b|\bpanic\b|traceback|uncaught|segmentation fault|SQLSTATE\[)`)
	// Lines that mention errors without being one.
	notAnError = regexp.MustCompile(`(?i)(\b0 errors?\b|\bno errors?\b|error_reporting|display_errors|log_errors|error_log\s*=|errors?=0\b|\bwithout errors\b)`)
)

// lastErrorLine scans the end of a container's log for the latest line that
// looks like an error.
func (s *Server) lastErrorLine(ctx context.Context, id string) *LogLine {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rc, err := s.opts.Engine.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: "300", Timestamps: true})
	if err != nil {
		return nil
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	if err != nil && len(raw) == 0 {
		return nil
	}
	return findLastError(demuxLogs(raw))
}

// demuxLogs strips Docker's stream headers when the log is multiplexed
// (containers without a TTY); TTY logs are returned as they are.
func demuxLogs(raw []byte) []byte {
	if len(raw) >= 8 && (raw[0] == 1 || raw[0] == 2) && raw[1] == 0 && raw[2] == 0 && raw[3] == 0 {
		var out bytes.Buffer
		if _, err := stdcopy.StdCopy(&out, &out, bytes.NewReader(raw)); err == nil || out.Len() > 0 {
			return out.Bytes()
		}
	}
	return raw
}

// findLastError returns the last error-looking line of a timestamped log.
func findLastError(logs []byte) *LogLine {
	var last *LogLine
	sc := bufio.NewScanner(bytes.NewReader(logs))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		var ts time.Time
		if i := strings.IndexByte(line, ' '); i > 0 {
			if t, err := time.Parse(time.RFC3339Nano, line[:i]); err == nil {
				ts, line = t, line[i+1:]
			}
		}
		if errorLine.MatchString(line) && !notAnError.MatchString(line) {
			text := strings.TrimSpace(line)
			if len(text) > 300 {
				text = text[:300] + "…"
			}
			last = &LogLine{Time: ts, Line: text}
		}
	}
	return last
}
