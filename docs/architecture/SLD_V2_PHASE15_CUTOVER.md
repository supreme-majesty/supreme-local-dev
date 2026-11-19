# SLD V2 — Phase 15 Cutover Plan: SLD as a Container

**Status:** Draft · **Date:** 2026-11-19 · **Depends on:** Phase 14 (CI E2E)

**Accepted decisions (2026-11-19):**
- Default TLD is `.localhost` (C3).
- Shared database servers are managed by SLD and replace the databases bundled in `compose.yaml` (D4).
- Binaries are untracked only, with no history rewrite (D7).

## 1. Goal

SLD started as a native tool for Linux, macOS and Windows that installed nginx, PHP and databases on the host. Phase 15 turns it into **one Docker image** that you run once and then use to manage all your projects and their databases from a dashboard.

```bash
docker run -d --name sld --restart unless-stopped \
  -p 127.0.0.1:2025:2025 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v sld-data:/data \
  -v ~/Projects:/workspace \
  ghcr.io/supreme-majesty/sld:2
# then open http://sld.localhost  (or http://127.0.0.1:2025)
```

At the end of Phase 15:

- **The product is the image** `ghcr.io/supreme-majesty/sld` (amd64 + arm64). It runs anywhere Docker runs: Linux, macOS (Docker Desktop/OrbStack) and Windows (Docker Desktop/WSL2). OS support comes from Docker, not from per-OS adapters.
- **The SLD container is a control plane.** Through the Docker socket it creates and manages sibling containers: the Traefik router, each project's runtime containers, and databases, caches and mail catchers.
- **Any web stack:** PHP, Node, Python, Ruby, Go, static sites, a custom Dockerfile or image, or a process already running on the host.
- **Database management** for project databases and shared database servers covers browsing, SQL, import/export, snapshots, clone and seeding. It reuses the existing DB manager.
- There is nothing to install on the host besides Docker. The host never runs `apt`/`brew`/`winget`, systemd units, root operations or hosts-file edits.
- The repo contains no committed binaries.

### Product definition (replaces §1 of `SLD_V2_ARCHITECTURE.md`)

> **Supreme Local Dev (SLD)** is a self-hosted local development control plane that ships as a single Docker image. Run it next to Docker and manage every project from one dashboard: each project gets a `*.localhost` (or `*.test`) domain with HTTPS, the right language runtime, isolated databases and caches, logs, a terminal, and a full database manager. No `docker-compose.yml` to write, nothing installed on the host.

## 2. Target architecture

```mermaid
graph TD
    subgraph Host
        Browser[Browser]
        WS[~/Projects]
        Sock[/var/run/docker.sock]
    end
    subgraph Docker Engine
        subgraph sld-global network
            SLD[sld control plane<br/>API + dashboard + orchestrator]
            Router[sld-router Traefik<br/>127.0.0.1:80/443]
        end
        subgraph shop project network
            Web[shop-web node]
            Api[shop-api python]
            DB[(shop-db postgres)]
        end
        subgraph shared services optional
            MySQL[(sld-shared-mariadb)]
        end
        Vol[(sld-data volume<br/>registry, certs, CA, secrets index)]
    end
    Browser -->|sld.localhost, shop.localhost| Router
    Router --> SLD
    Router --> Web
    Router --> Api
    SLD -->|Docker API| Sock
    SLD -->|DB manager over Docker network| DB
    SLD -->|DB manager| MySQL
    WS -->|bind /workspace| SLD
    WS -->|bind same host path| Web
    WS --> Api
    SLD --- Vol
    Router --- Vol
```

The control plane joins `sld-global` and attaches itself to a project's network when it needs to reach that project's database. Databases therefore don't need host ports, though `expose_port` stays available for desktop DB clients.

## 3. Container-mode constraints (what changes vs. a host CLI)

Running inside a container breaks several assumptions in the current V2 code. Each needs an explicit design.

| # | Problem | Today | Design |
|---|---|---|---|
| C1 | **Bind-mount paths are host paths.** When SLD asks Docker to mount `/workspace/shop`, the daemon looks for that path on the *host* | `buildPHP`/`buildNginx` mount `spec.ProjectRoot` directly (`orchestrator.go:405`, `:458`) | **Path mapper.** At startup SLD inspects its own container (`/proc/self/cgroup` / hostname → `ContainerInspect`) and reads its mounts. For `/workspace`, Docker reports `Source=/home/me/Projects` (or `C:\Users\me\Projects` on Docker Desktop). `pathmap.ToHost("/workspace/shop")` → `/home/me/Projects/shop` is used for every sibling bind mount. No env var is needed; `SLD_WORKSPACE_HOST` is an override |
| C2 | **SLD-owned files must not be bind-mounted from container paths** | Router mounts certs from `~/.sld/certs` (`orchestrator.go:87`), which only exists inside the container | Certs, CA and Traefik dynamic config live in a **named volume** (`sld-data`) that is mounted into both `sld` and `sld-router` |
| C3 | **No hosts-file / DNS edits** | `internal/dns` edits `/etc/hosts`, `/etc/resolver` and the Windows hosts file | Default TLD becomes **`.localhost`**, which browsers resolve to loopback with no DNS setup on any OS. `.test`/custom TLDs stay optional: the dashboard shows the hosts lines to add, and an optional `sld-dns` container (dnsmasq on `127.0.0.1:53`) can be enabled where port 53 is free |
| C4 | **TLS trust must happen on the host** | `mkcert` on the host, with self-signed fallback | SLD generates its own local CA in `sld-data` and issues `*.localhost` / `*.<tld>` certs. The dashboard offers **Download CA** with one-time trust instructions per OS. Plain HTTP works without trusting the CA |
| C5 | **Exec/shell used the host `docker` CLI** | `runDockerCLI` (`internal/v2/exec.go:237`); the image has no docker CLI | Use the SDK `ContainerExecCreate/Attach`. The dashboard gets a **web terminal** (xterm.js over websocket), and the CLI inside the image uses the same path |
| C6 | **"Open in editor" / "open browser" can't launch host apps** | `system/open-editor`, `openBrowserURL` | Emit `vscode://file/<hostPath>`, `jetbrains://…` and `cursor://…` links built from the host path (C1); the browser opens the editor |
| C7 | **Ports 80/443 bound on all interfaces** | Router `Ports: "80:80","443:443"` (`orchestrator.go:104`) | Bind `${SLD_BIND_HOST:-127.0.0.1}`, as the compose stack already does |
| C8 | **Docker socket = root on the host** | Mounted read-write, while the V1 API has no auth | Authentication is mandatory (§4 D2). Bind only to loopback. Optionally put the socket behind a socket proxy (§4 D6). Document rootless Docker / Podman socket support |
| C9 | **File ownership on Linux** | `resolveHostUID` reads the UID/GID of the SLD process, which is `sld` (10001) in the container | Use the owner of the workspace mount (`stat /workspace`) as the UID/GID for runtime containers and scaffolding |
| C10 | **Projects outside the workspace** | V1 `park` accepted any host path | Each project must be under a mounted workspace root, and several roots are allowed (`-v ~/work:/workspace/work -v ~/oss:/workspace/oss`). The dashboard explains how to add a mount |
| C11 | **Self-update** | n/a | The dashboard shows the available version. Updating means `docker compose pull && docker compose up -d`, or a one-shot helper container that recreates `sld` with the same config. SLD never replaces its own container directly |

## 4. Decisions

### D1. Control plane = the existing daemon API, rebuilt on the V2 engine (amends ADR 0002)

The compose `app` service already runs `sld daemon` in a container, and that is now the product direction. ADR 0002's goals still hold: no root on the host, no systemd, no global host state, no unauthenticated API. The process is long-running but **containerised and unprivileged** (UID 10001, `no-new-privileges`). The work is:
- **Keep** the HTTP server, websocket, dashboard embedding and the whole `db/*` surface.
- **Replace** what V1 did underneath (host nginx config generation, `state.json` routing, adapters, PHP switching) with the V2 orchestrator, Docker labels and the project registry.
- Rename `sld daemon` → `sld serve`. The old name remains as an alias for one release.

### D2. Authentication → first-run admin token, sessions after that

- On first start SLD generates an admin token, prints it once in `docker logs sld`, and stores its hash in `sld-data`. `SLD_ADMIN_TOKEN` can set it instead.
- The dashboard login exchanges the token for an HttpOnly, SameSite=Strict session cookie. API clients (CLI, scripts) use `Authorization: Bearer`.
- Host and Origin allow-lists (`sld.localhost`, `127.0.0.1:2025`, plus configured names) block DNS rebinding. Mutating routes require a CSRF header.
- File and env APIs are limited to registered project roots inside `/workspace`.

### D3. Project registry → `/data/projects.json`

Docker labels only describe projects that have been started. The registry lists every known project (`{id, containerPath, hostPath, lastSeen}`) plus "watched" workspace folders, which replace V1 `park`. It is an index only: each project's configuration stays in `.sld/config.yaml` in that project, and runtime state is read from Docker.

### D4. Databases → project databases plus optional shared servers

- **Project databases** are declared in `.sld/config.yaml`. They are isolated per project, with credentials in `.sld/secrets.json`.
- **Shared servers** are created from the dashboard (`Databases → Add server: MariaDB 11.4 / Postgres 16 / Redis / Mongo`) as SLD-managed containers named `sld-shared-<engine>-<ver>`, with named volumes. This replaces the bundled `mariadb`/`postgres`/`redis`/`mailpit`/`phpmyadmin` services in `compose.yaml`.
- **External/discovered databases:** `docker_discovery.go` already finds DB containers that SLD didn't create. The DB manager keeps supporting them as read/write "connections".
- **phpMyAdmin** becomes an optional one-click tool container. It is no longer needed, since the built-in manager covers the same features.

### D5. Distribution → image first; host binary only for migration

- Primary: `ghcr.io/supreme-majesty/sld:<semver>` (`:2`, `:2.x`, `:latest`), signed, SBOM attached. A minimal `compose.yaml` with only `sld` and optionally the socket proxy serves as the documented install.
- The `sld` binary stays in the image for `docker exec sld sld …` and scripting.
- A host binary is released **one last time** (2.2.x) to run `sld legacy migrate`, because the host's systemd unit and native MySQL can only be handled from the host. After that, releases are image-only.
- Optional later: a thin host `sld` client that calls the API with a token, for terminal users.

### D6. Docker socket proxy → optional, off by default

SLD needs containers, exec, networks, volumes, images and events, which is most of the API, so a proxy adds limited protection. Ship it as a compose profile (`--profile hardened`, using `tecnativa/docker-socket-proxy` with `SWARM/PLUGINS/SECRETS/NODES=0`). The main protections are D2 and loopback binding.

### D7. Committed binaries → untrack only (accepted)

`git rm --cached` the ~95 MB of binaries. There will be no `git filter-repo` purge, so existing clones stay valid and the old blobs remain in history.

## 5. Design: any-stack runtime model

### 5.1 Config `version: 2`

```yaml
version: 2
name: shop
domain:
  name: shop.localhost     # default: <name>.<SLD_TLD>  (SLD_TLD default "localhost")
  tls: true
  aliases: [www.shop.localhost]

apps:                      # one or more HTTP apps; first owns the root domain
  web:
    runtime: node          # php | node | python | ruby | go | static | image | dockerfile | proxy
    version: "22"
    command: npm run dev -- --host 0.0.0.0
    port: 3000
    install: npm ci        # runs on first start / when the lockfile hash changes
  api:
    runtime: python
    version: "3.12"
    command: uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload
    port: 8000
    domain: api.shop.localhost
    healthcheck: /health

services:
  database: { type: postgres, version: "16" }   # or: { shared: sld-shared-postgres-16, name: shop }
  redis: { enabled: true }
  mailpit: { enabled: true }

environment:
  APP_ENV: local
```

| Runtime | Container(s) | Routing target | Notable fields | Default image |
|---|---|---|---|---|
| `php` | `php-fpm` + `nginx` sidecar (ADR 0004) | nginx :80 | `version`, `web_root`, `xdebug` | `sld-php:<ver>`, `sld-nginx` |
| `static` | `nginx` | nginx :80 | `web_root` | `sld-nginx` (static template) |
| `node` | app | `port` | `version`, `command`, `install`, `package_manager` | `node:<ver>-bookworm-slim` |
| `python` | app | `port` | `version`, `command`, `install` | `python:<ver>-slim` |
| `ruby` | app | `port` | `version`, `command`, `install` | `ruby:<ver>-slim` |
| `go` | app | `port` | `version`, `command` | `golang:<ver>` |
| `image` | user image | `port` | `image`, `command` | user |
| `dockerfile` | built from project | `port` | `dockerfile`, `context`, `target`, `build_args` | `sld-<id>-<app>:dev` |
| `proxy` | none, Traefik → `host.docker.internal:<port>` | host `port` | `port` | n/a |

- A database can be **dedicated** (`type`/`version`, own container) or **shared** (`shared: <server>`, `name: <db>`). For shared, SLD creates the database and a user on that server and writes the credentials to `.sld/secrets.json`.
- v1 configs upgrade in memory: `runtime.php` → `apps.web {runtime: php}`, `node.dev_server` → `apps.vite`. `sld config migrate` writes the new file and keeps a `.bak`.

### 5.2 Orchestrator: runtime drivers

```go
// internal/runtime/driver.go
type Driver interface {
    Name() string
    Validate(app config.App) error
    Containers(spec *orchestrate.ProjectSpec, appName string, app config.App) ([]*reconcile.DesiredContainer, error)
    RouteTarget(spec *orchestrate.ProjectSpec, appName string, app config.App) RouteTarget
    ExecTarget(spec *orchestrate.ProjectSpec, appName string) string
}
```

- Drivers: `php` (wraps today's buildPHP + buildNginx), `static`, `generic` (node/python/ruby/go), `image`, `dockerfile` (built through the SDK `ImageBuild` with the context streamed from `/workspace`), `proxy` (Traefik file-provider entry in `sld-data`).
- `ProjectSpec` gains `HostRoot` (from the path mapper, C1). Every bind mount uses `HostRoot`; file reads and detection use `ProjectRoot`.
- One `routeLabels()` helper; router names `<id>-<app>[-secure]`.
- Naming `sld-<id>-<app>[-<role>]`, with labels `com.supreme.sld.app` and `com.supreme.sld.runtime`.
- Dependency-cache volumes per app (`node_modules`, `.venv`, `vendor/bundle`, Go module cache), invalidated by a lockfile hash label. These also avoid slow bind mounts on macOS and Windows.
- Per-app config hashes, so editing one app doesn't recreate the others (ADR 0006).
- The router (`sld-router`) and the control plane itself are on `sld-global`. SLD routes its own dashboard at `sld.<tld>`.

### 5.3 Detection

An ordered list of detectors, each returning apps plus suggested services. PHP (artisan/composer/wp/index.php), Node (next/nuxt/sveltekit/astro/remix/vite/express/nest, with version from `.nvmrc`/`engines` and the package manager from the lockfile), Python (django/fastapi/flask, version from `.python-version`/`requires-python`), Ruby (rails/sinatra), Go (`go.mod`, `air`), `Dockerfile`, static `index.html`, PHP + Vite combos, and DB/Redis from `.env`/`DATABASE_URL`. If nothing matches, the dashboard asks for the runtime instead of assuming PHP.

### 5.4 Exec, terminal, tasks

- SDK-based exec (C5) behind `POST /api/projects/{id}/exec` and the websocket `/api/projects/{id}/terminal`.
- Dashboard: terminal tab per app, plus **Tasks**: detected scripts (`npm run *`, `artisan`, `manage.py`, `rake`, `composer scripts`) that run with one click and stream output. This generalises the current `artisan/*` routes.
- CLI inside the image: `sld run [--app X] <cmd>`, `sld shell`, tool shims (`npm`, `pip`, `composer`, `artisan`, …).

## 6. Milestones

### M0 — Repo hygiene *(½ day)*
- ✅ Binaries untracked (`sld`, `sld-bin`, `sld-linux`, `sld-mac`, `sld-win.exe`, `main`, `debug-env`); already covered by `.gitignore`.
- Optional: untrack `.idea/` (also in `.gitignore`). Remove `cmd/debug-env` if unused.

### M1 — Container-mode foundations *(3–4 days)* — implemented 2026-11-19

What landed:
- `internal/hostenv`: detects container mode by self-inspection, translates bind mounts to host paths and volume paths to volume subpaths, gives stable project IDs from host paths, publishes ports on `SLD_BIND_HOST` (default `127.0.0.1`), and finds the workspace owner UID.
- `internal/tls`: SLD's own CA under `<data>/ca` (mkcert's CA is imported on the host when present). Each project gets a certificate for its exact host names, because browsers reject `*.localhost`/`*.test` wildcards. The router mounts only `<data>/certs` and `<data>/traefik`.
- `.localhost` default TLD (`SLD_TLD` overrides it), and no hosts-file edits for `.localhost` or from inside a container.
- The router binds to loopback and carries a spec hash, so existing routers are recreated with the new bindings.
- `shell`/`exec`/`php`/`composer`/`artisan`/`npm` and SQL import go through the Docker SDK (`internal/docker/exec.go`), so no `docker` CLI is needed.

- The router's host ports can be changed with `SLD_HTTP_PORT`/`SLD_HTTPS_PORT` (default 80/443), and printed URLs include non-default ports.
- The router image moved from `traefik:v3.1` to `traefik:v3.6`. Traefik 3.1's Docker provider cannot talk to Docker Engine 29, so no project was routed on current Docker.

Verified end to end on Docker Desktop 29.8 (Windows), with SLD running in a container and a workspace bind-mounted from a Windows path:
- `init` and `start` succeeded.
- `https://shop.localhost` returned PHP output through Traefik → nginx → PHP-FPM, and the certificate verified against SLD's CA.
- A wrong host name was rejected.
- Piped stdin, exit codes and the ephemeral Node container worked.
- `destroy` removed the project certificate.

Follow-ups:
- Terminal resize while a shell is open (only the initial size is sent).
- Publish the rebuilt `sld-router:2` image. Existing installs keep the broken v3.1 router until it is updated.
- A `doctor` check that the Docker Engine is ≥ 26, which volume subpaths require.

Original scope:
- `internal/pathmap` with self-inspection, multiple workspace roots, Windows/Docker Desktop source formats, and unit tests using recorded `ContainerInspect` fixtures (C1).
- Move certs, CA and Traefik dynamic config to the `sld-data` volume. Add the built-in CA plus a cert issuer, and keep mkcert only as an import path (C2, C4).
- `.localhost` default TLD, loopback router ports, UID from the workspace owner (C3, C7, C9).
- SDK exec replaces `runDockerCLI` (C5).

### M2 — Config v2 schema *(2–3 days)*
- `App` type, per-runtime validation, v1 upgrader, `config migrate`, golden tests.

### M3 — Runtime drivers *(4–6 days)*
- §5.2. The PHP driver must produce **identical** specs for upgraded v1 configs, so existing projects aren't recreated.
- `static` template in `sld-nginx`.

### M4 — Detection *(2–3 days)*
- §5.3 with fixtures per framework.

### M5 — Control plane rebuild (`sld serve`) *(7–10 days)*
- Auth (D2), registry (D3), and replacing the V1 internals of each API group:

| API group | Plan |
|---|---|
| `state`, `status`, `sites`, `services`, `services/control` | Orchestrator + registry: list, start, stop, restart, destroy per project and app |
| `park`, `forget`, `ignore` | "Watched workspace folders" on the registry |
| `link`, `unlink`, `secure`, `unsecure` | Edit `domain.aliases` / `domain.tls` in the project config |
| `php`, `php/versions` | → `runtimes` (drivers + supported versions) |
| `db/*` (~50 routes) | **Keep.** Connections now come from project DBs, shared servers (D4) and discovered containers; SLD attaches to project networks to connect. Remove the single-global-MySQL assumption (`SLD_DB_HOST` becomes one connection among several) |
| new `db/servers` | Create/remove shared DB servers (D4) |
| `logs/*`, `ws` | Docker log streams per container |
| `healer/*` | Keep; feed from container logs; add Node/Python/Ruby patterns |
| `projects/create`, `templates` | Scaffold in ephemeral runtime containers writing to the workspace (as the workspace UID); add Python/Ruby/Go/static templates |
| `projects/files`, `file/*`, `env/*` | Keep; scoped to registered roots |
| `system/editors`, `open-editor` | → editor URL schemes (C6) |
| `artisan/*` | → `tasks` + exec/terminal (§5.4) |
| `share/*` | `cloudflared` container on the project network |
| `metrics` | `ContainerStats` per project and app |
| `system/doctor` | Container-mode checks: socket access, workspace mounts, router ports, CA trust hint, Docker Desktop file-sharing |
| `plugins/*` | Drop |

### M6 — Dashboard *(4–6 days, can overlap with M5)*
- Login screen and token handling in `src/api/daemon.ts`; remove the hard-coded `localhost:2025` (`Database.tsx:2407`).
- Projects → Apps view with runtime badges, start/stop per app, terminal, tasks, logs.
- Databases → Servers (shared + project + discovered) with create-server flow.
- Setup checklist: workspace mounted, CA downloaded and trusted, TLD choice.
- Remove the Plugins page and PHP-only wording.
- Tauri: keep it as an optional shell that just points at the container URL (no sidecar).

### M7 — Image, compose, release *(2–3 days)*
- Root `Dockerfile`: `CMD ["serve"]`, `VOLUME /data`, `SLD_STATE_DIR=/data`, healthcheck on `/api/health`. Multi-arch build, cosign signing, SBOM, Trivy (the pipeline already exists in `docker-images.yml`).
- `compose.yaml` reduced to `sld` (+ `hardened` socket-proxy profile). The old bundled DB services move to docs under "shared servers".
- Release workflow: image tags; drop `.deb`/`.pkg`/`.msi`, systemd units, `postinst`. Ship a final 2.2.x host binary for `legacy migrate` (D5).
- Makefile: replace `park`/`link`/`daemon` targets with `docker-*` and `dev` targets.

### M8 — Phase 14 E2E in container mode *(3 days)*
- CI job: build the image, run it with a temp workspace, then for each fixture (Laravel, static, Next.js, Vite, FastAPI, Django, Rails, Go, Dockerfile, proxy) call the API to register, start, `curl` through the router, create a DB, run a query, stop and destroy.
- Same suite on `ubuntu-latest` and on a Docker Desktop–style path layout (fixture for C1).
- Gate for M9.

### M9 — Delete V1 *(1–2 days)*
- Delete `pkg/adapters/`, V1 parts of `pkg/daemon/` (nginx config gen, `state.json` routing), `pkg/plugins/`, `pkg/assets/runtime/`, `internal/dns` hosts-file editing (keep only the "suggest hosts lines" helper), `scripts/packaging/*`.
- Move surviving `pkg/services` code (DB drivers, SQL import, healer, env manager, tunnel, discovery) under `internal/`.
- `internal/legacy` remains only in the final host-binary build tag.

### M10 — Docs *(1–2 days, alongside M7–M9)*
- README rewritten: "Run one container" quick start; per-OS notes (Linux socket group, Docker Desktop file sharing, Windows: keep projects in the WSL filesystem for speed); trusting the CA; shared vs project databases.
- ADRs: 0007 multi-runtime drivers, 0008 containerised control plane (amends 0001/0002), 0009 path mapping and workspace mounts, 0010 auth.
- Update `SLD_V2_ARCHITECTURE.md`, mark the audit as historical, close `dockerization_progress.md`.

## 7. Release sequencing

| Release | Contents |
|---|---|
| `2.2.0-rc.1` | M0–M4 + M7 image work: multi-runtime engine running in container mode; existing host CLI still published |
| `2.2.0` | M5 + M6 + M8: `sld serve` with auth is the product; last host binary (with `legacy migrate`) |
| `2.3.0` | M9: V1 code removed; image-only releases |

Estimate: about 5–7 weeks for one person. M5 (control plane) and M3 (drivers) dominate.

## 8. Risks & mitigations

| Risk | Mitigation |
|---|---|
| Wrong host-path translation mounts the wrong folder or an empty one | Path mapper refuses to start a project outside a known mount; `doctor` shows the mapping; E2E fixture for Docker Desktop paths |
| Docker socket in a web-reachable container | Mandatory auth, loopback binding, Host/Origin checks, CSRF, optional socket proxy; security test suite in CI |
| Existing PHP projects recreated or losing data | Golden-spec test (M3); named DB volumes untouched |
| Dev servers bind `127.0.0.1` inside the container | Drivers set `HOST=0.0.0.0` / framework flags; doctor probes the port |
| HMR websockets through Traefik | Per-app domains; framework HMR host env; Vite in E2E |
| Slow bind mounts on macOS/Windows | Dependency-cache volumes; docs recommend VirtioFS / WSL filesystem |
| Users can't or won't trust the CA | HTTP works by default on `.localhost`; HTTPS opt-in per project |
| V1 users stranded | Final host binary with `legacy migrate` that dumps V1 host MySQL databases and imports them into the new project or shared servers |

## 9. Definition of done

- [ ] `docker run … ghcr.io/supreme-majesty/sld:2` on a clean Linux, macOS and Windows machine opens a working dashboard, with nothing else installed.
- [ ] A Laravel, Next.js, Django, Rails, Go, static and Dockerfile project each start from the dashboard and are reachable at `<name>.localhost`.
- [ ] Project and shared databases can be created, browsed, queried, imported, snapshotted and cloned from the dashboard.
- [ ] Every API route requires auth; the router and API bind to loopback by default.
- [ ] Upgraded v1-config PHP projects start with zero container recreation.
- [ ] No `pkg/adapters`, systemd units or host-mutation code in the image build; no binaries in `git ls-files`.
- [ ] README, ADRs 0007–0010 merged.
