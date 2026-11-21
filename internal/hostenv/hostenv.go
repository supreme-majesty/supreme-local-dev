// Package hostenv describes where SLD is running and translates paths that SLD
// can see into mounts that sibling containers can use.
//
// When SLD runs on the host, a project path is also a host path and can be
// bind-mounted as-is. When SLD runs inside a container (the primary V2
// distribution), the Docker daemon resolves bind-mount sources on the *host*,
// so "/workspace/shop" must become the host directory that backs /workspace,
// and files under SLD's data volume must be mounted as a volume subpath.
package hostenv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

const (
	// DefaultBindHost is the interface host ports are published on by default.
	DefaultBindHost = "127.0.0.1"

	envInContainer = "SLD_IN_CONTAINER"
	envContainerID = "SLD_CONTAINER_ID"
	envStateDir    = "SLD_STATE_DIR"
	envBindHost    = "SLD_BIND_HOST"
	envHTTPPort    = "SLD_HTTP_PORT"
	envHTTPSPort   = "SLD_HTTPS_PORT"
)

// ErrNotMounted is returned when a path is not backed by any mount of the SLD
// container, so sibling containers cannot be given access to it.
var ErrNotMounted = errors.New("path is not inside a directory mounted into the SLD container")

// ErrNotHostPath is returned by HostPath when the path lives in a named volume
// and therefore has no directory on the host.
var ErrNotHostPath = errors.New("path is stored in a Docker volume, not a host directory")

// Mount is one mount of the SLD container as reported by `docker inspect`.
type Mount struct {
	Type        string // "bind" or "volume"
	Source      string // host path for bind mounts
	Name        string // volume name for volume mounts
	Destination string // path inside the SLD container
}

// Env is the resolved runtime environment of the SLD process.
type Env struct {
	inContainer   bool
	containerID   string
	containerName string
	mounts        []Mount // sorted by Destination length, longest first
	dataDir       string
	bindHost      string
}

// Inspector is the subset of the Docker client needed for self-inspection.
type Inspector interface {
	InspectContainer(ctx context.Context, containerID string) (types.ContainerJSON, error)
}

// Host returns an Env for SLD running directly on the host. An empty dataDir
// selects DefaultDataDir().
func Host(dataDir string) *Env {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	return &Env{dataDir: dataDir, bindHost: bindHostFromEnv()}
}

// Container returns an Env for SLD running inside container containerID with
// the given mounts. Mainly useful for tests; production code uses Detect.
func Container(containerID string, mounts []Mount, dataDir string) *Env {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	sorted := append([]Mount(nil), mounts...)
	for i := range sorted {
		sorted[i].Destination = path.Clean(sorted[i].Destination)
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Destination) > len(sorted[j].Destination)
	})
	return &Env{
		inContainer: true,
		containerID: containerID,
		mounts:      sorted,
		dataDir:     dataDir,
		bindHost:    bindHostFromEnv(),
	}
}

// Detect determines whether SLD is running in a container and, if so, reads
// the container's own mounts through the Docker API.
func Detect(ctx context.Context, inspector Inspector) (*Env, error) {
	if !runningInContainer() {
		return Host(""), nil
	}

	id := detectContainerID()
	if id == "" {
		return Container("", nil, ""), fmt.Errorf("running in a container but could not determine its ID; set %s", envContainerID)
	}

	info, err := inspector.InspectContainer(ctx, id)
	if err != nil {
		return Container(id, nil, ""), fmt.Errorf("failed to inspect own container %s (is the Docker socket mounted?): %w", id, err)
	}

	var mounts []Mount
	for _, m := range info.Mounts {
		mounts = append(mounts, Mount{
			Type:        string(m.Type),
			Source:      m.Source,
			Name:        m.Name,
			Destination: m.Destination,
		})
	}
	env := Container(info.ID, mounts, "")
	env.containerName = strings.TrimPrefix(info.Name, "/")
	return env, nil
}

// ContainerName is the name of SLD's own container, such as "sld" or
// "sld-app-1" ("" on the host or when it could not be inspected).
func (e *Env) ContainerName() string { return e.containerName }

// InContainer reports whether SLD runs inside a container.
func (e *Env) InContainer() bool { return e.inContainer }

// ContainerID is the ID of SLD's own container ("" on the host).
func (e *Env) ContainerID() string { return e.containerID }

// DataDir is where SLD keeps its state (certificates, CA, registry).
func (e *Env) DataDir() string { return e.dataDir }

// BindHost is the interface published ports bind to (SLD_BIND_HOST, default 127.0.0.1).
func (e *Env) BindHost() string { return e.bindHost }

// Mounts returns the SLD container's mounts (nil on the host).
func (e *Env) Mounts() []Mount { return append([]Mount(nil), e.mounts...) }

// HTTPPort is the host port the router serves HTTP on: SLD_HTTP_PORT, then the
// port saved by `sld init` (see SaveRouterPorts), then 80.
func (e *Env) HTTPPort() int { return portFromEnv(envHTTPPort, e.savedPorts().HTTP, 80) }

// HTTPSPort is the host port the router serves HTTPS on: SLD_HTTPS_PORT, then
// the saved port, then 443.
func (e *Env) HTTPSPort() int { return portFromEnv(envHTTPSPort, e.savedPorts().HTTPS, 443) }

// RouterPorts are the router's host ports, remembered between commands so a
// later command without SLD_HTTP_PORT does not move the router back to 80.
type RouterPorts struct {
	HTTP  int `json:"http_port"`
	HTTPS int `json:"https_port"`
}

func (e *Env) routerPortsFile() string { return filepath.Join(e.dataDir, "router.json") }

func (e *Env) savedPorts() RouterPorts {
	var p RouterPorts
	if data, err := os.ReadFile(e.routerPortsFile()); err == nil {
		_ = json.Unmarshal(data, &p)
	}
	return p
}

// SaveRouterPorts remembers the router's current host ports.
func (e *Env) SaveRouterPorts() error {
	data, err := json.Marshal(RouterPorts{HTTP: e.HTTPPort(), HTTPS: e.HTTPSPort()})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(e.dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(e.routerPortsFile(), data, 0o644)
}

// SiteURL builds the browser URL for a domain, adding the router port when it
// is not the scheme's default.
func (e *Env) SiteURL(domain string, https bool) string {
	if https {
		if p := e.HTTPSPort(); p != 443 {
			return fmt.Sprintf("https://%s:%d", domain, p)
		}
		return "https://" + domain
	}
	if p := e.HTTPPort(); p != 80 {
		return fmt.Sprintf("http://%s:%d", domain, p)
	}
	return "http://" + domain
}

// PublishPort formats a "bindHost:hostPort:containerPort" port spec.
func (e *Env) PublishPort(hostPort int, containerPort string) string {
	return fmt.Sprintf("%s:%d:%s", e.bindHost, hostPort, containerPort)
}

// MountFor returns a mount that makes localPath (a path SLD can see) available
// at target inside a sibling container.
func (e *Env) MountFor(localPath, target string, readOnly bool) (reconcile.Mount, error) {
	localPath = absPath(localPath)
	if !e.inContainer {
		return reconcile.Mount{Type: reconcile.MountTypeBind, Source: localPath, Target: target, ReadOnly: readOnly}, nil
	}

	m, rel, err := e.findMount(localPath)
	if err != nil {
		return reconcile.Mount{}, err
	}

	switch m.Type {
	case "volume":
		return reconcile.Mount{
			Type:     reconcile.MountTypeVolume,
			Source:   m.Name,
			Target:   target,
			ReadOnly: readOnly,
			Subpath:  rel,
		}, nil
	case "bind":
		return reconcile.Mount{
			Type:     reconcile.MountTypeBind,
			Source:   joinHostPath(m.Source, rel),
			Target:   target,
			ReadOnly: readOnly,
		}, nil
	default:
		return reconcile.Mount{}, fmt.Errorf("%w: %s is on a %q mount", ErrNotMounted, localPath, m.Type)
	}
}

// HostPath returns the host directory that backs localPath.
func (e *Env) HostPath(localPath string) (string, error) {
	localPath = absPath(localPath)
	if !e.inContainer {
		return localPath, nil
	}
	m, rel, err := e.findMount(localPath)
	if err != nil {
		return "", err
	}
	if m.Type != "bind" {
		return "", fmt.Errorf("%w: %s (volume %s)", ErrNotHostPath, localPath, m.Name)
	}
	return joinHostPath(m.Source, rel), nil
}

// ProjectIdentity computes the identity for a project directory. Inside a
// container the identity is derived from the host path, so a project keeps the
// same ID (and therefore the same database volumes) whichever container path
// it is mounted at, and matches the ID a host-run SLD would compute.
func (e *Env) ProjectIdentity(localPath string) (*project.Identity, error) {
	if !e.inContainer {
		return project.ComputeIdentity(localPath)
	}
	abs := absPath(localPath)
	if hostPath, err := e.HostPath(abs); err == nil {
		return project.ComputeIdentityFromHostPath(hostPath)
	}
	m, rel, err := e.findMount(abs)
	if err != nil {
		return nil, err
	}
	// Volume-backed project: no host path exists, key it on volume + subpath.
	return project.ComputeIdentityFromHostPath("/volume/" + m.Name + "/" + rel)
}

// OwnerOf returns the numeric owner of path as strings, or ok=false when it
// cannot be determined (Windows, missing path).
func OwnerOf(p string) (uid, gid string, ok bool) {
	return ownerOf(p)
}

func (e *Env) findMount(localPath string) (Mount, string, error) {
	p := path.Clean(filepath.ToSlash(localPath))
	if !path.IsAbs(p) {
		return Mount{}, "", fmt.Errorf("%w: %s is not an absolute path", ErrNotMounted, localPath)
	}
	for _, m := range e.mounts {
		if m.Destination == "/" {
			continue // a root mount would match everything; never translate through it
		}
		if p == m.Destination {
			return m, "", nil
		}
		if strings.HasPrefix(p, m.Destination+"/") {
			return m, strings.TrimPrefix(p, m.Destination+"/"), nil
		}
	}
	return Mount{}, "", fmt.Errorf("%w: %s (mount the folder with -v <host-dir>:/workspace)", ErrNotMounted, localPath)
}

// absPath makes p absolute relative to the working directory (best effort).
func absPath(p string) string {
	if p == "" {
		return p
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// joinHostPath appends a slash-separated relative path to a host path, keeping
// the host's separator style (Docker Desktop on Windows may report C:\...).
func joinHostPath(source, rel string) string {
	if rel == "" {
		return source
	}
	if strings.Contains(source, `\`) {
		return strings.TrimRight(source, `\`) + `\` + strings.ReplaceAll(rel, "/", `\`)
	}
	return strings.TrimRight(source, "/") + "/" + rel
}

// DefaultDataDir is SLD_STATE_DIR, or ~/.sld when unset.
func DefaultDataDir() string {
	if dir := os.Getenv(envStateDir); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".sld"
	}
	return filepath.Join(home, ".sld")
}

func portFromEnv(name string, saved, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
			return p
		}
	}
	if saved > 0 && saved < 65536 {
		return saved
	}
	return fallback
}

func bindHostFromEnv() string {
	if h := strings.TrimSpace(os.Getenv(envBindHost)); h != "" {
		return h
	}
	return DefaultBindHost
}

func runningInContainer() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envInContainer))) {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

var containerIDPattern = regexp.MustCompile(`/containers/([0-9a-f]{64})/`)

// detectContainerID finds this container's ID: SLD_CONTAINER_ID, then the
// Docker-managed hostname/resolv.conf bind mounts in /proc/self/mountinfo
// (works with cgroup v2), then the hostname (Docker's default is the short ID).
func detectContainerID() string {
	if id := strings.TrimSpace(os.Getenv(envContainerID)); id != "" {
		return id
	}
	if f, err := os.Open("/proc/self/mountinfo"); err == nil {
		defer f.Close()
		if id := containerIDFromMountinfo(bufio.NewScanner(f)); id != "" {
			return id
		}
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return ""
}

func containerIDFromMountinfo(sc *bufio.Scanner) string {
	for sc.Scan() {
		if m := containerIDPattern.FindStringSubmatch(sc.Text()); m != nil {
			return m[1]
		}
	}
	return ""
}

// MirrorMount mounts localPath into a helper container at the path the Docker
// daemon itself uses for that directory, and returns that path. Tools that
// pass host paths to the daemon (docker compose resolving `./src:/var/www`)
// then produce paths the daemon can bind-mount:
//
//	Linux/macOS:            /home/me/app -> /home/me/app
//	Windows Docker Desktop: C:\Users\me\app -> /run/desktop/mnt/host/c/Users/me/app
func (e *Env) MirrorMount(localPath string, readOnly bool) (reconcile.Mount, string, error) {
	hostPath, err := e.HostPath(localPath)
	if err != nil {
		return reconcile.Mount{}, "", err
	}
	target := MirrorPath(hostPath)
	return reconcile.Mount{Type: reconcile.MountTypeBind, Source: hostPath, Target: target, ReadOnly: readOnly}, target, nil
}

// MirrorPath is the Linux path under which the Docker daemon sees hostPath.
func MirrorPath(hostPath string) string {
	norm := project.NormalizeHostPath(hostPath)
	if len(norm) >= 2 && norm[1] == ':' {
		return "/run/desktop/mnt/host/" + norm[:1] + norm[2:]
	}
	return norm
}
