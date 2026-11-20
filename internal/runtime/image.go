package runtime

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// imageDriver runs a user-supplied image (runtime image) or an image built
// from the project's Dockerfile (runtime dockerfile). The project is mounted
// at /app so code changes are visible without rebuilding.
type imageDriver struct {
	build bool
}

func (d imageDriver) Plan(ctx *Context, app *config.App) (*AppPlan, error) {
	id := ctx.Identity
	plan := &AppPlan{}

	image := app.Image
	if d.build {
		contextDir := filepath.Join(AppRoot(ctx, app), filepath.FromSlash(app.Context))
		image = id.AppImageTag(app.Name)
		plan.Build = &Build{
			App:        app.Name,
			ContextDir: contextDir,
			Dockerfile: filepath.ToSlash(app.Dockerfile),
			Target:     app.Target,
			BuildArgs:  app.BuildArgs,
			Tag:        image,
		}
	}

	codeMount, err := appMount(ctx, app, "", "/app", false)
	if err != nil {
		return nil, err
	}

	labels, err := appLabels(ctx, app, app.Runtime)
	if err != nil {
		return nil, err
	}
	addRouteLabels(ctx, app, labels, app.Port)

	c := &reconcile.DesiredContainer{
		Name:        id.AppContainerName(app.Name, ""),
		Service:     app.Runtime,
		Image:       image,
		Labels:      labels,
		Environment: appEnv(ctx, app, map[string]string{"PORT": fmt.Sprint(app.Port)}),
		Networks:    []string{ctx.ProjectNetwork, ctx.GlobalNetwork},
		ExtraHosts:  []string{"host.docker.internal:host-gateway"},
		Mounts:      []reconcile.Mount{codeMount},
		// Images may run migrations or builds at start; don't block on them.
		SkipHealthWait: true,
	}
	if cmd := strings.TrimSpace(app.Command); cmd != "" {
		c.Cmd = []string{"sh", "-c", cmd}
	}
	if app.ExposePort > 0 {
		c.Ports = []string{ctx.Env.PublishPort(app.ExposePort, fmt.Sprint(app.Port))}
	}
	plan.Containers = []*reconcile.DesiredContainer{c}
	return plan, nil
}

// proxyDriver routes a domain to a process already running on the host
// (e.g. `npm run dev` started outside Docker). No container is created.
type proxyDriver struct{}

func (proxyDriver) Plan(ctx *Context, app *config.App) (*AppPlan, error) {
	return &AppPlan{Route: &FileRoute{
		Name:  RouterName(ctx, app),
		Hosts: AppHosts(ctx.Config, app),
		URL:   fmt.Sprintf("http://host.docker.internal:%d", app.Port),
		TLS:   ctx.Config.Domain.TLS,
	}}, nil
}

// RenderFileRoute renders a Traefik dynamic configuration for a file route.
func RenderFileRoute(r *FileRoute) string {
	var b strings.Builder
	rule := HostRule(r.Hosts...)
	b.WriteString("# Managed by SLD. Do not edit.\nhttp:\n  routers:\n")
	fmt.Fprintf(&b, "    %s:\n      rule: %q\n      entryPoints: [web]\n      service: %s\n", r.Name, rule, r.Name)
	if r.TLS {
		fmt.Fprintf(&b, "    %s-secure:\n      rule: %q\n      entryPoints: [websecure]\n      service: %s\n      tls: {}\n", r.Name, rule, r.Name)
	}
	fmt.Fprintf(&b, "  services:\n    %s:\n      loadBalancer:\n        servers:\n          - url: %q\n", r.Name, r.URL)
	return b.String()
}
