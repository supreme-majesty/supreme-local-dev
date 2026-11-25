package orchestrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
	appruntime "github.com/supreme-majesty/supreme-local-dev/internal/runtime"
)

// ComposeImage runs `docker compose` for projects with their own compose stack,
// so neither the host nor the SLD image needs a docker CLI.
const ComposeImage = "docker:27.5.1-cli"

const composeMountDir = "/sld-compose"

// mergeComposeStacks combines apps that route to the same compose stack.
func mergeComposeStacks(stacks []*appruntime.ComposeStack) []*appruntime.ComposeStack {
	byKey := map[string]*appruntime.ComposeStack{}
	var out []*appruntime.ComposeStack
	for _, s := range stacks {
		key := s.Dir + "|" + strings.Join(s.Files, ",")
		if existing, ok := byKey[key]; ok {
			existing.Routes = append(existing.Routes, s.Routes...)
			existing.KeepPorts = existing.KeepPorts || s.KeepPorts
			continue
		}
		copied := *s
		copied.Routes = append([]appruntime.ComposeRoute(nil), s.Routes...)
		byKey[key] = &copied
		out = append(out, &copied)
	}
	return out
}

// composeStackID names a stack's override file.
func composeStackID(projectID string, s *appruntime.ComposeStack) string {
	h := sha256.Sum256([]byte(s.Dir + "|" + strings.Join(s.Files, ",")))
	return projectID + "-" + hex.EncodeToString(h[:])[:8]
}

func (o *Orchestrator) composeDir() (string, error) {
	dir := filepath.Join(o.env.DataDir(), "compose")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", dir, err)
	}
	return dir, nil
}

// upComposeStack generates the SLD override for a stack and runs `up -d`.
func (o *Orchestrator) upComposeStack(ctx context.Context, spec *ProjectSpec, stack *appruntime.ComposeStack) error {
	var cfgOut, errOut bytes.Buffer
	if err := o.runCompose(ctx, spec, stack, "", []string{"config", "--format", "json"}, &cfgOut, &errOut); err != nil {
		return fmt.Errorf("docker compose config failed: %w\n%s", err, tail(errOut.String()))
	}
	var resolved composeConfig
	if err := json.Unmarshal(cfgOut.Bytes(), &resolved); err != nil {
		return fmt.Errorf("could not read docker compose config: %w", err)
	}
	for _, r := range stack.Routes {
		if _, ok := resolved.Services[r.Service]; !ok {
			return fmt.Errorf("app %q: compose service %q not found (services: %s)", r.App, r.Service, strings.Join(resolved.serviceNames(), ", "))
		}
	}

	override, err := buildComposeOverride(spec, stack, &resolved)
	if err != nil {
		return err
	}
	dir, err := o.composeDir()
	if err != nil {
		return err
	}
	name := composeStackID(spec.Identity.ID, stack) + ".yaml"
	if err := os.WriteFile(filepath.Join(dir, name), override, 0o644); err != nil {
		return fmt.Errorf("failed to write compose override: %w", err)
	}

	out := o.buildLog
	if out == nil {
		out = io.Discard
	}
	errOut.Reset()
	if err := o.runCompose(ctx, spec, stack, name, []string{"up", "-d"}, out, io.MultiWriter(out, &errOut)); err != nil {
		return fmt.Errorf("docker compose up failed for %s: %w\n%s", stack.Dir, err, tail(errOut.String()))
	}
	return o.connectComposeRoutes(ctx, spec, stack, resolved.Name)
}

// connectComposeRoutes attaches the routed services of a compose stack to the
// router network. SLD does this itself rather than listing the network in the
// override, because Compose registers a service's name (app, web, api, ...) as
// a DNS alias on every network the service joins. On the shared network those
// names collide between projects: another stack's nginx asking for "app" could
// reach this project's "app". Connected through the API, a container is known
// there only by its own unique name, which is all the router needs.
func (o *Orchestrator) connectComposeRoutes(ctx context.Context, spec *ProjectSpec, stack *appruntime.ComposeStack, composeProject string) error {
	routed := map[string]bool{}
	for _, r := range stack.Routes {
		routed[r.Service] = true
	}
	containers, err := o.engine.ListProjectContainers(ctx, spec.Identity.ID)
	if err != nil {
		return fmt.Errorf("listing compose containers: %w", err)
	}
	for _, c := range containers {
		if c.Labels[composeProjectLabel] != composeProject || !routed[c.Labels[docker.LabelService]] {
			continue
		}
		if err := o.engine.ConnectNetwork(ctx, GlobalNetwork, c.ID); err != nil {
			return fmt.Errorf("connecting %s to %s: %w", c.Name, GlobalNetwork, err)
		}
	}
	return nil
}

// composeProjectLabel is the label Docker Compose puts on a stack's containers.
const composeProjectLabel = "com.docker.compose.project"

// StopComposeStacks stops (down=false) or removes (down=true) the compose
// stacks of a project. removeVolumes adds `--volumes` to `down`.
func (o *Orchestrator) StopComposeStacks(ctx context.Context, spec *ProjectSpec, down, removeVolumes bool) error {
	plan, err := o.planProject(spec)
	if err != nil {
		return err
	}
	dir, err := o.composeDir()
	if err != nil {
		return err
	}
	for _, stack := range plan.composeStacks {
		name := composeStackID(spec.Identity.ID, stack) + ".yaml"
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			name = "" // never started by SLD; use the project's files only
		}
		args := []string{"stop"}
		if down {
			args = []string{"down"}
			if removeVolumes {
				args = append(args, "--volumes")
			}
		}
		var errOut bytes.Buffer
		if err := o.runCompose(ctx, spec, stack, name, args, io.Discard, &errOut); err != nil {
			return fmt.Errorf("docker compose %s failed for %s: %w\n%s", args[0], stack.Dir, err, tail(errOut.String()))
		}
		if down && name != "" {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	return nil
}

// runCompose runs `docker compose` in a helper container. The project is
// mounted at the path the Docker daemon uses for it (see hostenv.MirrorMount),
// so relative bind mounts in the user's compose files resolve correctly.
func (o *Orchestrator) runCompose(ctx context.Context, spec *ProjectSpec, stack *appruntime.ComposeStack, overrideName string, args []string, stdout, stderr io.Writer) error {
	executor, ok := o.engine.(docker.Executor)
	if !ok {
		return fmt.Errorf("docker engine does not support running helper containers")
	}

	projectMount, _, err := o.env.MirrorMount(spec.ProjectRoot, false)
	if err != nil {
		return fmt.Errorf("compose projects must live in a host directory mounted into SLD: %w", err)
	}
	_, workDir, err := o.env.MirrorMount(stack.Dir, false)
	if err != nil {
		return err
	}

	cmd := []string{"docker", "compose"}
	for _, f := range stack.Files {
		cmd = append(cmd, "-f", f)
	}
	mounts := []reconcile.Mount{projectMount}
	if overrideName != "" {
		dir, err := o.composeDir()
		if err != nil {
			return err
		}
		overrideMount, err := o.env.MountFor(dir, composeMountDir, true)
		if err != nil {
			return err
		}
		mounts = append(mounts, overrideMount)
		cmd = append(cmd, "-f", composeMountDir+"/"+overrideName)
	}
	cmd = append(cmd, args...)

	code, err := executor.RunEphemeral(ctx, &reconcile.DesiredContainer{
		Image:      ComposeImage,
		WorkingDir: workDir,
		Volumes:    []string{"/var/run/docker.sock:/var/run/docker.sock"},
		Mounts:     mounts,
	}, docker.ExecOptions{Cmd: cmd, Stdout: stdout, Stderr: stderr})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exit code %d", code)
	}
	return nil
}

// composeConfig is the subset of `docker compose config --format json` SLD reads.
type composeConfig struct {
	Name     string `json:"name"`
	Services map[string]struct {
		Networks map[string]any `json:"networks"`
	} `json:"services"`
}

func (c *composeConfig) serviceNames() []string {
	names := make([]string, 0, len(c.Services))
	for n := range c.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// buildComposeOverride labels every service as part of the SLD project, turns
// off Docker's automatic restarts, drops published host ports (unless
// keep_ports), and connects routed services to
// the router network with Traefik labels.
func buildComposeOverride(spec *ProjectSpec, stack *appruntime.ComposeStack, resolved *composeConfig) ([]byte, error) {
	id := spec.Identity
	routes := map[string][]appruntime.ComposeRoute{}
	for _, r := range stack.Routes {
		routes[r.Service] = append(routes[r.Service], r)
	}

	str := func(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }
	mapping := func() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }
	add := func(m *yaml.Node, k string, v *yaml.Node) { m.Content = append(m.Content, str(k), v) }

	services := mapping()
	for _, name := range resolved.serviceNames() {
		svc := mapping()

		labels := mapping()
		base := map[string]string{
			docker.LabelSLD:          "true",
			docker.LabelVersion:      docker.CurrentVersion,
			docker.LabelProject:      id.ID,
			docker.LabelProjectPath:  id.CanonicalPath,
			docker.LabelManagedBy:    docker.ManagedBySLD,
			docker.LabelResourceType: docker.ResourceTypeContainer,
			docker.LabelService:      name,
			docker.LabelRuntime:      "compose",
		}
		if rs := routes[name]; len(rs) > 0 {
			base[docker.LabelApp] = rs[0].App
			base["traefik.enable"] = "true"
			base["traefik.docker.network"] = GlobalNetwork
			for _, r := range rs {
				rule := appruntime.HostRule(r.Hosts...)
				base["traefik.http.routers."+r.RouterName+".rule"] = rule
				base["traefik.http.routers."+r.RouterName+".entrypoints"] = "web"
				base["traefik.http.routers."+r.RouterName+".service"] = r.RouterName
				base["traefik.http.services."+r.RouterName+".loadbalancer.server.port"] = fmt.Sprint(r.Port)
				if r.TLS {
					base["traefik.http.routers."+r.RouterName+"-secure.rule"] = rule
					base["traefik.http.routers."+r.RouterName+"-secure.entrypoints"] = "websecure"
					base["traefik.http.routers."+r.RouterName+"-secure.tls"] = "true"
					base["traefik.http.routers."+r.RouterName+"-secure.service"] = r.RouterName
				}
			}
		}
		keys := make([]string, 0, len(base))
		for k := range base {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			add(labels, k, str(base[k]))
		}
		add(svc, "labels", labels)

		// Start only when asked to (from SLD), never on its own when Docker
		// starts, whatever restart policy the project's compose file sets.
		// Quoted: a bare `no` is the boolean false in YAML 1.1 parsers.
		add(svc, "restart", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "no", Style: yaml.DoubleQuotedStyle})

		if !stack.KeepPorts {
			add(svc, "ports", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!reset", Style: yaml.FlowStyle})
		}

		// Routed services are connected to the router network after `up`
		// (connectComposeRoutes), not here: see there for why.
		add(services, name, svc)
	}

	root := mapping()
	add(root, "services", services)

	var buf bytes.Buffer
	buf.WriteString("# Generated by SLD for " + id.ID + ". Do not edit; regenerated on every start.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("failed to encode compose override: %w", err)
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

// tail returns the last lines of command output for error messages.
func tail(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 15 {
		lines = lines[len(lines)-15:]
	}
	return strings.Join(lines, "\n")
}
