package runtime

import (
	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// phpDriver runs PHP-FPM plus an nginx sidecar that serves static files and
// forwards PHP requests over FastCGI (ADR 0004). Only nginx is routed.
type phpDriver struct{}

func (phpDriver) Plan(ctx *Context, app *config.App) (*AppPlan, error) {
	id := ctx.Identity
	phpName := id.AppContainerName(app.Name, docker.ServicePHP)

	rwMount, err := projectMount(ctx, "", "/app", false)
	if err != nil {
		return nil, err
	}
	roMount, err := projectMount(ctx, "", "/app", true)
	if err != nil {
		return nil, err
	}

	// --- PHP-FPM ---
	phpLabels, err := appLabels(ctx, app, docker.ServicePHP)
	if err != nil {
		return nil, err
	}
	extra := map[string]string{
		"WWWUSER":     ctx.UID,
		"WWWGROUP":    ctx.GID,
		"XDEBUG_MODE": "off",
	}
	if app.Xdebug {
		extra["XDEBUG_MODE"] = "debug"
	}
	if ctx.Config.Services.Redis.Enabled {
		extra["CACHE_DRIVER"] = "redis"
		extra["SESSION_DRIVER"] = "redis"
	}
	php := &reconcile.DesiredContainer{
		Name:        phpName,
		Service:     docker.ServicePHP,
		Image:       ctx.PHPImage(app.Version),
		WorkingDir:  "/app",
		Labels:      phpLabels,
		Environment: appEnv(ctx, app, extra),
		Networks:    []string{ctx.ProjectNetwork},
		ExtraHosts:  []string{"host.docker.internal:host-gateway"},
		Mounts:      []reconcile.Mount{rwMount},
	}

	// --- nginx sidecar ---
	nginxLabels, err := appLabels(ctx, app, docker.ServiceNginx)
	if err != nil {
		return nil, err
	}
	addRouteLabels(ctx, app, nginxLabels, 80)
	nginx := &reconcile.DesiredContainer{
		Name:    id.AppContainerName(app.Name, docker.ServiceNginx),
		Service: docker.ServiceNginx,
		Image:   ctx.NginxImage,
		Labels:  nginxLabels,
		Environment: map[string]string{
			"WEB_ROOT": app.WebRoot,
			"PHP_HOST": phpName,
		},
		Networks: []string{ctx.ProjectNetwork, ctx.GlobalNetwork},
		Mounts:   []reconcile.Mount{roMount},
	}

	return &AppPlan{Containers: []*reconcile.DesiredContainer{php, nginx}}, nil
}

// staticDriver serves files with stock nginx; no build step, no PHP.
type staticDriver struct{}

func (staticDriver) Plan(ctx *Context, app *config.App) (*AppPlan, error) {
	webRoot, err := projectMount(ctx, app.WebRoot, "/usr/share/nginx/html", true)
	if err != nil {
		return nil, err
	}
	labels, err := appLabels(ctx, app, "web")
	if err != nil {
		return nil, err
	}
	addRouteLabels(ctx, app, labels, 80)
	return &AppPlan{Containers: []*reconcile.DesiredContainer{{
		Name:     ctx.Identity.AppContainerName(app.Name, ""),
		Service:  "web",
		Image:    ctx.StaticImage,
		Labels:   labels,
		Networks: []string{ctx.ProjectNetwork, ctx.GlobalNetwork},
		Mounts:   []reconcile.Mount{webRoot},
	}}}, nil
}
