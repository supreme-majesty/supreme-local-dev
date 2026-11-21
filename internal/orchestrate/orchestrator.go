package orchestrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
	appruntime "github.com/supreme-majesty/supreme-local-dev/internal/runtime"
	"github.com/supreme-majesty/supreme-local-dev/internal/secret"
	"github.com/supreme-majesty/supreme-local-dev/internal/tls"
)

const (
	// Image Registry
	ImageRegistry = "ghcr.io/supreme-majesty"

	// Default image tags
	RouterImageTag = "2"
	NginxImageTag  = "1"

	// Third-party images used for built-in services and runtimes.
	StaticImage  = "nginx:1.27-alpine"
	MailpitImage = "axllent/mailpit:v1.31.2"
)

// Shared resources. Overridable so a second SLD instance (such as the end-to-end
// tests) can run beside a real one without taking over its router.
var (
	GlobalNetwork   = envOr("SLD_GLOBAL_NETWORK", "sld-global")
	RouterContainer = envOr("SLD_ROUTER_CONTAINER", "sld-router")
)

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

// Orchestrator manages the lifecycle of Docker containers for SLD projects.
type Orchestrator struct {
	engine             docker.EngineClient
	env                *hostenv.Env
	buildLog           io.Writer // image build output (nil = discard)
	healthTimeout      time.Duration
	healthPollInterval time.Duration
}

// NewOrchestrator creates an Orchestrator backed by the given Docker engine.
// It assumes SLD runs on the host; call SetEnv with the result of
// hostenv.Detect when SLD may be running inside a container.
func NewOrchestrator(engine docker.EngineClient) *Orchestrator {
	return &Orchestrator{
		engine:             engine,
		env:                hostenv.Host(""),
		healthTimeout:      30 * time.Second,
		healthPollInterval: 500 * time.Millisecond,
	}
}

// Engine returns the underlying Docker engine client.
func (o *Orchestrator) Engine() docker.EngineClient {
	return o.engine
}

// Env returns the runtime environment used for path translation.
func (o *Orchestrator) Env() *hostenv.Env {
	return o.env
}

// SetEnv replaces the runtime environment (nil restores host mode).
func (o *Orchestrator) SetEnv(env *hostenv.Env) {
	if env == nil {
		env = hostenv.Host("")
	}
	o.env = env
}

// SetBuildLog sets where image build output is written (nil discards it).
func (o *Orchestrator) SetBuildLog(w io.Writer) { o.buildLog = w }

// CertManager returns the TLS manager for SLD's data directory.
func (o *Orchestrator) CertManager() (*tls.Manager, error) {
	return tls.NewManager(o.env.DataDir())
}

// ProjectSpec bundles the resolved identity, config, and root path for a project.
type ProjectSpec struct {
	Identity    *project.Identity
	Config      *config.Config
	ConfigHash  string
	ProjectRoot string
}

// EnsureGlobalInfra ensures the sld-global network and router container exist.
func (o *Orchestrator) EnsureGlobalInfra(ctx context.Context) error {
	// 1. Ensure global network
	globalLabels := map[string]string{
		docker.LabelSLD:       "true",
		docker.LabelVersion:   docker.CurrentVersion,
		docker.LabelManagedBy: docker.ManagedBySLD,
	}

	_, err := o.engine.EnsureNetwork(ctx, GlobalNetwork, globalLabels)
	if err != nil {
		return fmt.Errorf("failed to ensure global network: %w", err)
	}

	// When SLD runs in a container, join sld-global so it can reach the router
	// and project services by container name.
	if o.env.InContainer() && o.env.ContainerID() != "" {
		if err := o.engine.ConnectNetwork(ctx, GlobalNetwork, o.env.ContainerID()); err != nil {
			return fmt.Errorf("failed to attach SLD container to %s: %w", GlobalNetwork, err)
		}
	}

	// 2. Ensure the local CA, the router's default certificate and TLS config.
	certMgr, err := o.CertManager()
	if err != nil {
		return err
	}
	if !o.env.InContainer() {
		// On the host, reuse mkcert's CA if present: browsers usually trust it already.
		if imported, err := certMgr.ImportMkcertCA(); err != nil {
			return err
		} else if imported {
			fmt.Println("[SLD TLS] Using mkcert's local CA for SLD certificates.")
		}
	}
	if err := certMgr.EnsureDefault(); err != nil {
		return fmt.Errorf("failed to prepare TLS certificates: %w", err)
	}

	certMount, err := o.env.MountFor(certMgr.CertDir(), "/etc/traefik/certs", true)
	if err != nil {
		return fmt.Errorf("router cannot access SLD certificates (mount SLD's data directory as a volume): %w", err)
	}
	dynMount, err := o.env.MountFor(certMgr.TraefikDir(), "/etc/traefik/dynamic", true)
	if err != nil {
		return fmt.Errorf("router cannot access SLD Traefik config (mount SLD's data directory as a volume): %w", err)
	}

	// The static config is SLD's (not the one baked into the image), so it can
	// add the JSON access log X-Ray reads without publishing a new image.
	staticDir := filepath.Join(o.env.DataDir(), "traefik-static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", staticDir, err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "traefik.yml"), []byte(RouterStaticConfig), 0o644); err != nil {
		return fmt.Errorf("failed to write the router config: %w", err)
	}
	staticMount, err := o.env.MountFor(staticDir, "/etc/traefik/static", true)
	if err != nil {
		return fmt.Errorf("router cannot access SLD Traefik config (mount SLD's data directory as a volume): %w", err)
	}
	staticSum := sha256.Sum256([]byte(RouterStaticConfig))

	// 3. Ensure router container is running
	routerImage := fmt.Sprintf("%s/sld-router:%s", ImageRegistry, RouterImageTag)
	desired := &reconcile.DesiredContainer{
		Name:  RouterContainer,
		Image: routerImage,
		Cmd:   []string{"traefik", "--configFile=/etc/traefik/static/traefik.yml"},
		Labels: map[string]string{
			docker.LabelSLD:          "true",
			docker.LabelVersion:      docker.CurrentVersion,
			docker.LabelManagedBy:    docker.ManagedBySLD,
			docker.LabelService:      docker.ServiceRouter,
			docker.LabelResourceType: docker.ResourceTypeContainer,
			// A changed static config needs a new router (Traefik reads it once).
			"com.supreme.sld.router-config": hex.EncodeToString(staticSum[:8]),
		},
		// Every request writes an access log line: cap what Docker keeps.
		LogOptions: map[string]string{"max-size": "10m", "max-file": "3"},
		Ports: []string{
			o.env.PublishPort(o.env.HTTPPort(), "80"),
			o.env.PublishPort(o.env.HTTPSPort(), "443"),
		},
		Networks: []string{GlobalNetwork},
		// Lets file routes (proxy runtime) reach processes on the host.
		ExtraHosts: []string{"host.docker.internal:host-gateway"},
		// The Docker socket path is resolved on the Docker host, which is what Traefik needs.
		Volumes: []string{"/var/run/docker.sock:/var/run/docker.sock:ro"},
		Mounts:  []reconcile.Mount{certMount, dynMount, staticMount},
		// SLD infrastructure, not a project: keep it up across Docker restarts.
		RestartPolicy: "unless-stopped",
	}
	// The router has no project config file, so hash its spec: a changed bind
	// address or mount (e.g. after upgrading SLD) recreates the router.
	desired.ConfigHash = specHash(desired)
	desired.Labels[docker.LabelConfigHash] = desired.ConfigHash

	// Check if router already exists
	containers, err := o.engine.ListAllContainers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	var routerActual *reconcile.ActualContainer
	for _, c := range containers {
		if c.Name == RouterContainer {
			routerActual = c
			break
		}
	}

	plan := reconcile.DiffContainer(desired, routerActual)

	_, err = o.applyPlan(ctx, plan)
	return err
}

// RecreateRouter forces recreation of the global sld-router container (e.g. after image update or cert refresh).
func (o *Orchestrator) RecreateRouter(ctx context.Context) error {
	containers, err := o.engine.ListAllContainers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}
	for _, c := range containers {
		if c.Name == RouterContainer {
			if c.State == "running" {
				_ = o.engine.StopContainer(ctx, c.ID, 10)
			}
			if err := o.engine.RemoveContainer(ctx, c.ID, true); err != nil {
				return fmt.Errorf("failed to remove old router: %w", err)
			}
			break
		}
	}
	return o.EnsureGlobalInfra(ctx)
}

// StartProject reconciles a project's containers, image builds and routes
// with its configuration.
func (o *Orchestrator) StartProject(ctx context.Context, spec *ProjectSpec) error {
	id := spec.Identity

	// 1. Plan services and apps
	plan, err := o.planProject(spec)
	if err != nil {
		return fmt.Errorf("failed to build desired state: %w", err)
	}

	// 2. Project network — only when SLD runs containers of its own. Compose
	// stacks bring their own networks, and Docker's pool of network address
	// ranges is small, so an unused network is not created.
	if len(plan.containers) > 0 {
		netLabels, err := docker.BuildNetworkLabels(docker.LabelParams{
			ProjectID:     id.ID,
			CanonicalPath: id.CanonicalPath,
			ConfigHash:    spec.ConfigHash,
		})
		if err != nil {
			return fmt.Errorf("failed to build network labels: %w", err)
		}
		if _, err := o.engine.EnsureNetwork(ctx, id.NetworkName(), netLabels); err != nil {
			return fmt.Errorf("failed to ensure project network: %w", err)
		}
	} else if err := o.engine.RemoveNetwork(ctx, id.NetworkName()); err != nil {
		// Drop the unused network an earlier SLD version created.
		return fmt.Errorf("failed to remove unused project network: %w", err)
	}

	// 3. Per-project TLS certificate (browsers reject *.localhost / *.test wildcards).
	if spec.Config.Domain.TLS {
		certMgr, err := o.CertManager()
		if err != nil {
			return err
		}
		if _, err := certMgr.EnsureCert(id.ID, ProjectTLSHosts(spec.Config)); err != nil {
			return fmt.Errorf("failed to issue TLS certificate: %w", err)
		}
	}

	// 4. Build images for dockerfile apps. The image ID becomes a label so a
	// rebuilt image changes the container's spec hash and recreates it.
	for _, b := range plan.builds {
		imageID, err := o.buildImage(ctx, b)
		if err != nil {
			return fmt.Errorf("app %q: %w", b.App, err)
		}
		for _, c := range plan.containers {
			if c.Labels[docker.LabelApp] == b.App && c.Image == b.Tag {
				c.Labels[docker.LabelImageID] = imageID
				finalizeContainer(c)
			}
		}
	}

	// 5. Volumes
	if err := o.ensureVolumes(ctx, spec, plan.volumes); err != nil {
		return fmt.Errorf("failed to ensure volumes: %w", err)
	}

	// 6. Reconcile containers in dependency order
	actual, err := o.engine.ListProjectContainers(ctx, id.ID)
	if err != nil {
		return fmt.Errorf("failed to list existing containers: %w", err)
	}
	actualMap := make(map[string]*reconcile.ActualContainer, len(actual))
	for _, a := range actual {
		// Compose-managed containers carry the project label too, but docker
		// compose owns them: never treat them as obsolete SLD containers.
		if a.Labels[docker.LabelRuntime] == "compose" {
			continue
		}
		actualMap[a.Name] = a
	}
	for _, p := range reconcile.ComputeContainerPlan(plan.containers, actualMap) {
		containerID, err := o.applyPlan(ctx, p)
		if err != nil {
			return fmt.Errorf("failed to apply plan for %s: %w", p.ResourceName, err)
		}
		if containerID == "" || p.Desired == nil || p.Desired.SkipHealthWait {
			continue
		}
		if p.Action == reconcile.ActionCreate || p.Action == reconcile.ActionStart || p.Action == reconcile.ActionRecreate {
			if err := o.waitForHealthy(ctx, containerID, p.ResourceName); err != nil {
				return err
			}
		}
	}

	// 7. The project's own compose stacks, with SLD's override.
	for _, stack := range plan.composeStacks {
		if err := o.upComposeStack(ctx, spec, stack); err != nil {
			return err
		}
	}

	// 8. File-based routes (proxy runtime), replacing any from a previous start.
	return o.writeFileRoutes(id.ID, plan.routes)
}

// DestroyProject tears down all containers, the project network, and optionally persistent volumes.
func (o *Orchestrator) DestroyProject(ctx context.Context, identity *project.Identity, removeVolumes bool) error {
	if err := o.StopProject(ctx, identity.ID); err != nil {
		return err
	}

	if err := o.engine.RemoveNetwork(ctx, identity.NetworkName()); err != nil {
		return fmt.Errorf("failed to remove project network %s: %w", identity.NetworkName(), err)
	}

	if certMgr, err := o.CertManager(); err == nil {
		if err := certMgr.RemoveCert(identity.ID); err != nil {
			return fmt.Errorf("failed to remove project certificate: %w", err)
		}
	}

	if removeVolumes {
		seen := make(map[string]bool)
		if labeledVols, err := o.engine.ListProjectVolumes(ctx, identity.ID); err == nil {
			for _, v := range labeledVols {
				seen[v] = true
			}
		}
		// Also include deterministic default volume names in case labels are missing
		seen[identity.DBVolumeName()] = true
		seen[identity.RedisVolumeName()] = true

		for volName := range seen {
			if err := o.engine.RemoveVolume(ctx, volName, true); err != nil {
				return fmt.Errorf("failed to remove volume %s: %w", volName, err)
			}
		}
	}

	return nil
}

// StopProject stops and removes all containers and routes of a project.
func (o *Orchestrator) StopProject(ctx context.Context, projectID string) error {
	containers, err := o.engine.ListProjectContainers(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to list project containers: %w", err)
	}

	for _, c := range containers {
		// Compose-managed containers are stopped through docker compose (StopComposeStacks).
		if c.Labels[docker.LabelRuntime] == "compose" {
			continue
		}
		if c.State == "running" {
			if err := o.engine.StopContainer(ctx, c.ID, 10); err != nil {
				return fmt.Errorf("failed to stop %s: %w", c.Name, err)
			}
		}
		if err := o.engine.RemoveContainer(ctx, c.ID, false); err != nil {
			return fmt.Errorf("failed to remove %s: %w", c.Name, err)
		}
	}

	return o.writeFileRoutes(projectID, nil)
}

// ProjectStatus returns the current state of all containers belonging to a project.
func (o *Orchestrator) ProjectStatus(ctx context.Context, projectID string) ([]*reconcile.ActualContainer, error) {
	return o.engine.ListProjectContainers(ctx, projectID)
}

// projectPlan is everything needed to bring a project up.
type projectPlan struct {
	containers    []*reconcile.DesiredContainer // in start order
	volumes       []string                      // extra named volumes (dependency caches)
	builds        []*appruntime.Build
	routes        []*appruntime.FileRoute
	composeStacks []*appruntime.ComposeStack // projects' own compose stacks
}

// CheckEngineSupport reports configuration this SLD version cannot start yet.
func CheckEngineSupport(cfg *config.Config) error {
	if cfg.Services.Database.Shared != "" {
		return fmt.Errorf("services.database.shared: shared database servers are not available in this SLD version yet; use 'type' for a dedicated database container")
	}
	return nil
}

// buildDesiredContainers returns the project's containers in start order:
// services (database, Redis, Mailpit) first, then apps in config order.
func (o *Orchestrator) buildDesiredContainers(spec *ProjectSpec) ([]*reconcile.DesiredContainer, error) {
	plan, err := o.planProject(spec)
	if err != nil {
		return nil, err
	}
	return plan.containers, nil
}

func (o *Orchestrator) planProject(spec *ProjectSpec) (*projectPlan, error) {
	cfg := spec.Config
	if err := CheckEngineSupport(cfg); err != nil {
		return nil, err
	}
	plan := &projectPlan{}

	// --- Services ---
	if cfg.Services.Database.Type != "" {
		db, err := o.buildDatabase(spec)
		if err != nil {
			return nil, err
		}
		plan.containers = append(plan.containers, db)
	}
	if cfg.Services.Redis.Enabled {
		redis, err := o.buildRedis(spec)
		if err != nil {
			return nil, err
		}
		plan.containers = append(plan.containers, redis)
	}
	if cfg.Services.Mailpit.Enabled {
		mail, err := o.buildMailpit(spec)
		if err != nil {
			return nil, err
		}
		plan.containers = append(plan.containers, mail)
	}

	// --- Apps ---
	serviceEnv, err := o.serviceEnv(spec)
	if err != nil {
		return nil, err
	}
	uid, gid := o.ProjectOwner(spec.ProjectRoot)
	rctx := &appruntime.Context{
		Identity:       spec.Identity,
		Config:         cfg,
		ProjectRoot:    spec.ProjectRoot,
		Env:            o.env,
		UID:            uid,
		GID:            gid,
		ProjectNetwork: spec.Identity.NetworkName(),
		GlobalNetwork:  GlobalNetwork,
		ServiceEnv:     serviceEnv,
		PHPImage:       func(v string) string { return fmt.Sprintf("%s/sld-php:%s", ImageRegistry, v) },
		NginxImage:     fmt.Sprintf("%s/sld-nginx:%s", ImageRegistry, NginxImageTag),
		StaticImage:    StaticImage,
		StaticConfig:   o.staticNginxConfig,
	}
	for i := range cfg.Apps {
		appPlan, err := appruntime.PlanApp(rctx, &cfg.Apps[i])
		if err != nil {
			return nil, err
		}
		plan.containers = append(plan.containers, appPlan.Containers...)
		plan.volumes = append(plan.volumes, appPlan.Volumes...)
		if appPlan.Build != nil {
			plan.builds = append(plan.builds, appPlan.Build)
		}
		if appPlan.Route != nil {
			plan.routes = append(plan.routes, appPlan.Route)
		}
		if appPlan.Compose != nil {
			plan.composeStacks = append(plan.composeStacks, appPlan.Compose)
		}
	}

	plan.composeStacks = mergeComposeStacks(plan.composeStacks)

	for _, c := range plan.containers {
		finalizeContainer(c)
	}
	return plan, nil
}

// finalizeContainer stamps a container with the hash of its own spec. A
// container is recreated exactly when its spec changes, so editing one app
// leaves the project's other containers running (ADR 0006).
func finalizeContainer(c *reconcile.DesiredContainer) {
	delete(c.Labels, docker.LabelConfigHash)
	c.ConfigHash = specHash(c)
	c.Labels[docker.LabelConfigHash] = c.ConfigHash
}

// serviceEnv returns the connection variables apps use to reach the project's
// services, in both Laravel-style (DB_HOST, ...) and URL form (DATABASE_URL, ...).
func (o *Orchestrator) serviceEnv(spec *ProjectSpec) (map[string]string, error) {
	id, cfg := spec.Identity, spec.Config
	env := map[string]string{}

	if db := cfg.Services.Database; db.Type != "" {
		pass, err := secret.GetOrGenerate(spec.ProjectRoot, "db_password", 16)
		if err != nil {
			return nil, fmt.Errorf("failed to generate DB password: %w", err)
		}
		host, port, name, user := id.DBContainerName(), resolveDBPort(db.Type), cfg.Name, cfg.Name
		env["DB_CONNECTION"] = resolveDBConnection(db.Type)
		env["DB_HOST"] = host
		env["DB_PORT"] = port
		env["DB_DATABASE"] = name
		env["DB_USERNAME"] = user
		env["DB_PASSWORD"] = pass
		scheme := "mysql"
		if resolveDBConnection(db.Type) == "pgsql" {
			scheme = "postgresql"
		}
		u := url.URL{Scheme: scheme, User: url.UserPassword(user, pass), Host: host + ":" + port, Path: "/" + name}
		env["DATABASE_URL"] = u.String()
	}
	if cfg.Services.Redis.Enabled {
		env["REDIS_HOST"] = id.RedisContainerName()
		env["REDIS_PORT"] = "6379"
		env["REDIS_URL"] = "redis://" + id.RedisContainerName() + ":6379"
	}
	if cfg.Services.Mailpit.Enabled {
		env["MAIL_MAILER"] = "smtp"
		env["MAIL_HOST"] = id.MailpitContainerName()
		env["MAIL_PORT"] = "1025"
		env["SMTP_HOST"] = id.MailpitContainerName()
		env["SMTP_PORT"] = "1025"
	}
	return env, nil
}

// buildImage builds a dockerfile app's image, streaming the build log.
func (o *Orchestrator) buildImage(ctx context.Context, b *appruntime.Build) (string, error) {
	builder, ok := o.engine.(docker.Builder)
	if !ok {
		return "", fmt.Errorf("docker engine does not support image builds")
	}
	fmt.Printf("[SLD] Building image for app %q (%s)...\n", b.App, b.Tag)
	return builder.BuildImage(ctx, docker.BuildOptions{
		ContextDir: b.ContextDir,
		Dockerfile: b.Dockerfile,
		Target:     b.Target,
		BuildArgs:  b.BuildArgs,
		Tag:        b.Tag,
		Output:     o.buildLog,
	})
}

// writeFileRoutes replaces a project's Traefik file routes with routes.
func (o *Orchestrator) writeFileRoutes(projectID string, routes []*appruntime.FileRoute) error {
	certMgr, err := o.CertManager()
	if err != nil {
		return err
	}
	dir := certMgr.TraefikDir()
	// Files are route-<router>.yml; router names are "<id>" or "<id>-<app>".
	belongs := func(name string) bool {
		return name == "route-"+projectID+".yml" || strings.HasPrefix(name, "route-"+projectID+"-")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read router config: %w", err)
	}
	for _, e := range entries {
		if belongs(e.Name()) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				return fmt.Errorf("failed to remove stale route %s: %w", e.Name(), err)
			}
		}
	}
	for _, r := range routes {
		path := filepath.Join(dir, "route-"+r.Name+".yml")
		if err := os.WriteFile(path, []byte(appruntime.RenderFileRoute(r)), 0o644); err != nil {
			return fmt.Errorf("failed to write route %s: %w", r.Name, err)
		}
	}
	return nil
}

// buildDatabase creates the database container spec (MySQL, MariaDB, or PostgreSQL).
func (o *Orchestrator) buildDatabase(spec *ProjectSpec) (*reconcile.DesiredContainer, error) {
	id := spec.Identity
	dbCfg := spec.Config.Services.Database

	image, env, err := resolveDatabaseImage(dbCfg, spec)
	if err != nil {
		return nil, err
	}

	labels, err := docker.BuildContainerLabels(docker.LabelParams{
		ProjectID:     id.ID,
		CanonicalPath: id.CanonicalPath,
		ConfigHash:    spec.ConfigHash,
		Service:       docker.ServiceDB,
	})
	if err != nil {
		return nil, err
	}

	var ports []string
	if dbCfg.ExposePort > 0 {
		ports = []string{o.env.PublishPort(dbCfg.ExposePort, resolveDBPort(dbCfg.Type))}
	}

	return &reconcile.DesiredContainer{
		Name:        id.DBContainerName(),
		Service:     docker.ServiceDB,
		Image:       image,
		ConfigHash:  spec.ConfigHash,
		Labels:      labels,
		Environment: env,
		Ports:       ports,
		Networks:    []string{id.NetworkName()},
		Volumes: []string{
			fmt.Sprintf("%s:%s", id.DBVolumeName(), resolveDBDataDir(dbCfg.Type)),
		},
		Healthcheck: resolveDBHealthcheck(dbCfg.Type),
	}, nil
}

// buildRedis creates the Redis container spec.
func (o *Orchestrator) buildRedis(spec *ProjectSpec) (*reconcile.DesiredContainer, error) {
	id := spec.Identity

	version := spec.Config.Services.Redis.Version
	if version == "" {
		version = "7-alpine"
	}
	redisImage := fmt.Sprintf("redis:%s", version)

	labels, err := docker.BuildContainerLabels(docker.LabelParams{
		ProjectID:     id.ID,
		CanonicalPath: id.CanonicalPath,
		ConfigHash:    spec.ConfigHash,
		Service:       docker.ServiceRedis,
	})
	if err != nil {
		return nil, err
	}

	return &reconcile.DesiredContainer{
		Name:       id.RedisContainerName(),
		Service:    docker.ServiceRedis,
		Image:      redisImage,
		ConfigHash: spec.ConfigHash,
		Labels:     labels,
		Networks:   []string{id.NetworkName()},
		Volumes: []string{
			fmt.Sprintf("%s:/data", id.RedisVolumeName()),
		},
		Healthcheck: &reconcile.HealthcheckConfig{
			Test:        []string{"CMD", "redis-cli", "ping"},
			Interval:    5 * time.Second,
			Timeout:     2 * time.Second,
			StartPeriod: 2 * time.Second,
			Retries:     3,
		},
	}, nil
}

// buildMailpit creates the Mailpit container spec; its UI is served on mail.<domain>.
func (o *Orchestrator) buildMailpit(spec *ProjectSpec) (*reconcile.DesiredContainer, error) {
	id := spec.Identity

	labels, err := docker.BuildContainerLabels(docker.LabelParams{
		ProjectID:     id.ID,
		CanonicalPath: id.CanonicalPath,
		Service:       "mailpit",
	})
	if err != nil {
		return nil, err
	}

	// App routers are "<id>-<app>" and app names start with a letter, so "--" cannot collide.
	router := id.ID + "--mailpit"
	rule := appruntime.HostRule("mail." + spec.Config.Domain.Name)
	labels["traefik.enable"] = "true"
	labels["traefik.docker.network"] = GlobalNetwork
	labels["traefik.http.routers."+router+".rule"] = rule
	labels["traefik.http.routers."+router+".entrypoints"] = "web"
	labels["traefik.http.services."+router+".loadbalancer.server.port"] = "8025"
	if spec.Config.Domain.TLS {
		labels["traefik.http.routers."+router+"-secure.rule"] = rule
		labels["traefik.http.routers."+router+"-secure.entrypoints"] = "websecure"
		labels["traefik.http.routers."+router+"-secure.tls"] = "true"
	}

	return &reconcile.DesiredContainer{
		Name:     id.MailpitContainerName(),
		Service:  "mailpit",
		Image:    MailpitImage,
		Labels:   labels,
		Networks: []string{id.NetworkName(), GlobalNetwork},
	}, nil
}

// ensureVolumes creates the project's named volumes: database, Redis and app dependency caches.
func (o *Orchestrator) ensureVolumes(ctx context.Context, spec *ProjectSpec, extra []string) error {
	id := spec.Identity
	cfg := spec.Config

	volLabels, err := docker.BuildVolumeLabels(docker.LabelParams{
		ProjectID:     id.ID,
		CanonicalPath: id.CanonicalPath,
		ConfigHash:    spec.ConfigHash,
	})
	if err != nil {
		return err
	}

	names := append([]string(nil), extra...)
	if cfg.Services.Database.Type != "" {
		names = append(names, id.DBVolumeName())
	}
	if cfg.Services.Redis.Enabled {
		names = append(names, id.RedisVolumeName())
	}
	for _, name := range names {
		if _, err := o.engine.EnsureVolume(ctx, name, volLabels); err != nil {
			return fmt.Errorf("failed to ensure volume %s: %w", name, err)
		}
	}
	return nil
}

// applyPlan executes a single reconciliation plan entry and returns the resulting container ID (if started/created).
func (o *Orchestrator) applyPlan(ctx context.Context, plan reconcile.DesiredResourcePlan) (string, error) {
	switch plan.Action {
	case reconcile.ActionNoOp:
		return "", nil

	case reconcile.ActionCreate:
		id, err := o.engine.CreateContainer(ctx, plan.Desired)
		if err != nil {
			return "", err
		}
		if err := o.engine.StartContainer(ctx, id); err != nil {
			return "", err
		}
		return id, nil

	case reconcile.ActionStart:
		if err := o.engine.StartContainer(ctx, plan.Actual.ID); err != nil {
			return "", err
		}
		return plan.Actual.ID, nil

	case reconcile.ActionRecreate:
		if plan.Actual != nil {
			if plan.Actual.State == "running" {
				_ = o.engine.StopContainer(ctx, plan.Actual.ID, 10)
			}
			_ = o.engine.RemoveContainer(ctx, plan.Actual.ID, true)
		}
		id, err := o.engine.CreateContainer(ctx, plan.Desired)
		if err != nil {
			return "", err
		}
		if err := o.engine.StartContainer(ctx, id); err != nil {
			return "", err
		}
		return id, nil

	case reconcile.ActionRemove:
		if plan.Actual != nil {
			if plan.Actual.State == "running" {
				_ = o.engine.StopContainer(ctx, plan.Actual.ID, 10)
			}
			return "", o.engine.RemoveContainer(ctx, plan.Actual.ID, true)
		}
		return "", nil

	default:
		return "", fmt.Errorf("unknown action: %s", plan.Action)
	}
}

// waitForHealthy polls a container's Docker health check until healthy, unhealthy, exited, or timeout.
// Containers without a Docker healthcheck return immediately.
func (o *Orchestrator) waitForHealthy(ctx context.Context, containerID, name string) error {
	timeout := o.healthTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	interval := o.healthPollInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}

	deadline := time.Now().Add(timeout)
	for {
		inspect, err := o.engine.InspectContainer(ctx, containerID)
		if err != nil {
			return fmt.Errorf("failed to inspect %s during health check: %w", name, err)
		}

		// No state or no healthcheck configured (e.g. Mailpit, Node, or mock engine) -> ready immediately
		if inspect.ContainerJSONBase == nil || inspect.State == nil {
			return nil
		}
		if inspect.State.Status == "exited" || inspect.State.Status == "dead" {
			stderrTail := o.readContainerStderrTail(ctx, containerID)
			if stderrTail != "" {
				return fmt.Errorf("container %s exited unexpectedly (exit code %d):\n%s", name, inspect.State.ExitCode, stderrTail)
			}
			return fmt.Errorf("container %s exited unexpectedly (exit code %d)", name, inspect.State.ExitCode)
		}
		if inspect.State.Health == nil {
			return nil
		}

		switch inspect.State.Health.Status {
		case "healthy":
			return nil
		case "unhealthy":
			stderrTail := o.readContainerStderrTail(ctx, containerID)
			if stderrTail != "" {
				return fmt.Errorf("container %s became unhealthy:\n%s", name, stderrTail)
			}
			return fmt.Errorf("container %s became unhealthy", name)
		}

		if time.Now().After(deadline) {
			stderrTail := o.readContainerStderrTail(ctx, containerID)
			if stderrTail != "" {
				return fmt.Errorf("timed out waiting for %s to become healthy:\n%s", name, stderrTail)
			}
			return fmt.Errorf("timed out waiting for %s to become healthy", name)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
			if interval < 2*time.Second {
				interval += 250 * time.Millisecond
			}
		}
	}
}

func (o *Orchestrator) readContainerStderrTail(ctx context.Context, containerID string) string {
	reader, err := o.engine.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStderr: true,
		ShowStdout: false,
		Tail:       "20",
	})
	if err != nil || reader == nil {
		return ""
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, 4096))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func resolveDBHealthcheck(dbType string) *reconcile.HealthcheckConfig {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return &reconcile.HealthcheckConfig{
			Test:        []string{"CMD-SHELL", `pg_isready -h 127.0.0.1 -U "$POSTGRES_USER" || exit 1`},
			Interval:    5 * time.Second,
			Timeout:     3 * time.Second,
			StartPeriod: 10 * time.Second,
			Retries:     5,
		}
	default:
		return &reconcile.HealthcheckConfig{
			Test:        []string{"CMD-SHELL", "mysqladmin ping -h 127.0.0.1 --silent || exit 1"},
			Interval:    5 * time.Second,
			Timeout:     3 * time.Second,
			StartPeriod: 10 * time.Second,
			Retries:     5,
		}
	}
}

// --- Helpers ---

// fallbackUID is used when the real owner is unknown or root (Docker Desktop VMs).
const fallbackUID = "1000"

// ProjectOwner returns the UID/GID that project containers should run as so
// files they create belong to the developer. Inside a container SLD's own UID
// is meaningless, so it uses the owner of the mounted project directory.
func (o *Orchestrator) ProjectOwner(projectRoot string) (string, string) {
	if o.env.InContainer() {
		if uid, gid, ok := hostenv.OwnerOf(projectRoot); ok && uid != "0" {
			return uid, gid
		}
		return fallbackUID, fallbackUID
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return fallbackUID, fallbackUID // Docker Desktop VM default
	}
	return strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())
}

// ProjectTLSHosts lists the host names a project's certificate must cover.
func ProjectTLSHosts(cfg *config.Config) []string {
	return cfg.Hostnames()
}

// specHash fingerprints the parts of a container spec that require recreation.
//
// Everything that defines the container counts: command, environment, user
// and labels too, so editing an app's env or command, its domain or HTTPS
// (router labels) in .sld/config.yaml takes effect on the next start instead
// of being ignored by an existing container. fmt prints maps in sorted key
// order, so the hash is stable.
func specHash(d *reconcile.DesiredContainer) string {
	labels := make(map[string]string, len(d.Labels))
	for k, v := range d.Labels {
		if k != docker.LabelConfigHash {
			labels[k] = v
		}
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s|%q|%q|%q|%+v", d.Image, d.Ports, d.Networks, d.Volumes, d.Mounts)
	fmt.Fprintf(h, "|%q|%q|%q|%q|%q|%q", d.Cmd, d.WorkingDir, d.User, d.Environment, d.ExtraHosts, labels)
	if d.Healthcheck != nil {
		fmt.Fprintf(h, "|%+v", *d.Healthcheck)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func resolveDBConnection(dbType string) string {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return "pgsql"
	default:
		return "mysql"
	}
}

func resolveDBPort(dbType string) string {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return "5432"
	default:
		return "3306"
	}
}

func resolveDBDataDir(dbType string) string {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return "/var/lib/postgresql/data"
	default:
		return "/var/lib/mysql"
	}
}

func resolveDatabaseImage(dbCfg config.DatabaseConfig, spec *ProjectSpec) (string, map[string]string, error) {
	dbPass, err := secret.GetOrGenerate(spec.ProjectRoot, "db_password", 16)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate DB password: %w", err)
	}

	dbName := spec.Config.Name
	dbUser := spec.Config.Name

	switch strings.ToLower(dbCfg.Type) {
	case "mysql":
		return fmt.Sprintf("mysql:%s", dbCfg.Version), map[string]string{
			"MYSQL_ROOT_PASSWORD": dbPass,
			"MYSQL_DATABASE":      dbName,
			"MYSQL_USER":          dbUser,
			"MYSQL_PASSWORD":      dbPass,
		}, nil

	case "mariadb":
		return fmt.Sprintf("mariadb:%s", dbCfg.Version), map[string]string{
			"MYSQL_ROOT_PASSWORD": dbPass,
			"MYSQL_DATABASE":      dbName,
			"MYSQL_USER":          dbUser,
			"MYSQL_PASSWORD":      dbPass,
		}, nil

	case "postgres", "postgresql":
		return fmt.Sprintf("postgres:%s", dbCfg.Version), map[string]string{
			"POSTGRES_DB":       dbName,
			"POSTGRES_USER":     dbUser,
			"POSTGRES_PASSWORD": dbPass,
		}, nil

	default:
		return "", nil, fmt.Errorf("unsupported database type: %s", dbCfg.Type)
	}
}
