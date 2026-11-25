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
  insightifytech/sld:latest
# then open http://sld.localhost  (or http://127.0.0.1:2025)
```

At the end of Phase 15:

- **The product is the image** `insightifytech/sld` on Docker Hub (amd64 + arm64). It runs anywhere Docker runs: Linux, macOS (Docker Desktop/OrbStack) and Windows (Docker Desktop/WSL2). OS support comes from Docker, not from per-OS adapters.
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

- Primary: `insightifytech/sld:<version>` and `:latest` on Docker Hub, signed, SBOM attached. *(Amended 2026-11-22: GitHub Packages is disabled for the `supreme-majesty` account, so the images moved from GHCR to Docker Hub. `SLD_IMAGE_REGISTRY` points SLD at another registry, e.g. `ghcr.io/supreme-majesty`.)* A minimal `compose.yaml` with only `sld` and optionally the socket proxy serves as the documented install.
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

### M2 — Config v2 schema *(2–3 days)* — implemented 2026-11-19

What landed:
- **`version: 2`** with an ordered `apps:` map and nine runtimes (`php`, `static`, `node`, `python`, `ruby`, `go`, `image`, `dockerfile`, `proxy`). There are per-runtime defaults and field rules, `domain.aliases`, `services.database.shared`/`name`, and `tools.node`.
- **Error reporting:** unknown keys are rejected with their line number (typo protection), and validation reports every problem at once.
- **Version 1 upgrade:** version 1 files are upgraded in memory (`runtime.php` → `apps.web`, `node.dev_server` → `apps.vite`, `node.version` → `tools.node`). `sld v2 config migrate` rewrites the file and keeps `config.v1.yaml.bak`, and `init-project` now writes version 2.
- **Guard test:** a version 1 file and its migrated version 2 file produce identical container specs.

Engine note (superseded by M3): until M3, `start` ran one `php` app plus one `node` dev server. Shared databases still validate but cannot be started until M5.

Original scope:
- `App` type, per-runtime validation, v1 upgrader, `config migrate`, golden tests.

### M3 — Runtime drivers *(4–6 days)* — implemented 2026-11-20

What landed:
- **`internal/runtime`:** one driver per runtime.
  - `php`: PHP-FPM plus an nginx sidecar.
  - `static`: stock `nginx:1.27-alpine`.
  - `node`/`python`/`ruby`/`go`: dependencies install into a per-app volume when the lockfile changes, then the process drops from root to the developer's UID with `setpriv`. User commands are passed through environment variables and are never spliced into the script.
  - `image`, `dockerfile`: built through the SDK, honouring `.dockerignore`. The image ID becomes a label, so a rebuild recreates the container.
  - `proxy`: a Traefik file route to `host.docker.internal`, removed on stop.
- **Container names** are `sld-<id>-<app>[-<role>]`. The primary app keeps the bare `<id>` router name, and the Mailpit router is `<id>--mailpit`.
- **Per-container spec hash** (ADR 0006): editing or adding one app leaves the other containers running. This was verified live.
- **Service URLs:** apps get `DATABASE_URL`, `REDIS_URL` and `SMTP_*` alongside the Laravel-style variables.
- **CLI:**
  - `exec`, `shell` and `logs` take app names, and `logs` reads stopped containers and splits stdout/stderr.
  - New passthroughs: `python`, `pip`, `bundle`, `rails`, `go`, `npx`.
  - `npm` runs inside the Node app when one is running, where `node_modules` lives.
  - `start` prints one URL per app.

Verified end to end on Docker Desktop 29.8 with SLD in a container:
- `static`, `node` (healthy, `npm install` in its volume), `python` (venv), `dockerfile` (build argument applied) and `proxy` (to the dashboard on host port 2025) were all served on their own `*.localhost` names.
- Processes and `exec` ran as UID 1000.
- Adding two apps did not recreate the existing three.
- `stop` removed the proxy route, and `destroy --volumes` removed the dependency volumes.

Upgrade effect: existing projects are recreated once (new names and spec hashes). Database and Redis volumes are kept.

Follow-ups:
- `destroy` does not remove images built for `dockerfile` apps.
- `bun` is not supported as a package manager yet.
- Ruby `-slim` images lack build tools for native gems; detection (M4) should offer the full image.

Original scope:
- §5.2. The PHP driver must produce **identical** specs for upgraded v1 configs, so existing projects aren't recreated.
- `static` template in `sld-nginx`.

### M4 — Detection *(2–3 days)* — implemented 2026-11-20

What landed:
- **`internal/detect`:** an ordered detector chain.
  - PHP: Laravel, Symfony, WordPress/Bedrock or plain PHP, plus a Vite companion.
  - Node: Next.js, Nuxt, Astro, SvelteKit, Remix, NestJS, Vite, Express, Fastify, Hono, Koa. The package manager comes from the lockfile, and the version from `.nvmrc`/`.node-version`/`engines`.
  - Python: Django, FastAPI (finds `module:app`), Flask. Ruby: Rails, Sinatra. Go, using Air when configured.
  - Fallbacks: a Dockerfile (port from `EXPOSE`), then static `index.html`.
- **Services** are inferred from `.env`/`.env.example` (`DB_CONNECTION`, `DATABASE_URL`, `REDIS_URL`, cache/session/queue settings) and from database and Redis drivers in the dependency files.
- **Monorepos:** when the root is not an app, each first-level folder is detected as its own app with the new `apps.<name>.path`. Mobile folders (React Native, Expo, Flutter, native) and Node packages with nothing to run are skipped. Frontend-like folders come first, so they own the root domain.
- **New config field `path`:** an app's own folder is mounted at `/app`, and its lockfiles, web root and Dockerfile context are resolved from there.
- **`init-project --runtime <rt>` and `--dry-run`.** If nothing is detected, SLD explains what it looked for instead of assuming PHP.
- **Other fixes:** composer alternatives like `^7.3|^8.2` resolve to the highest supported version, and CLI errors print once, without the usage dump.

Verified read-only against 10 real projects in `Documents/Development`:
- Laravel: swiftsms, sunport backend.
- Laravel monorepos with React Native and Flutter folders: cecadoe-ride, insightify-lms.
- Vite frontend plus Laravel backend: smart-docx, gifec, tailorly.
- pnpm projects: logistics-os, lgs_operationaltools. Rails: rails_learning.

Follow-ups:
- Turborepo/pnpm workspaces are run as one root `dev` script; per-package apps are not split out.
- Two Laravel backends with Vite companions would both expose host port 5173.

Original scope:
- §5.3 with fixtures per framework.

### M4b — Existing Docker Compose projects — implemented 2026-11-20

Decision (2026-11-20): projects that already have their own compose stack run **as-is** through it, not converted to SLD runtimes. A stack in a sub-folder is named after the main project folder (`sunport-apps/backend` → `sunport.localhost`).

What landed:
- **`runtime: compose`** (fields: `service`, `port`, `path`, `compose_files`, `keep_ports`). SLD runs the project's compose files plus a generated override kept in `<data>/compose/`, outside the repo. The override:
  - removes every published host port, so stacks no longer collide on 80/3306/6379/... (`keep_ports: true` opts out);
  - adds SLD labels to all services;
  - joins the routed service(s) to `sld-global` with Traefik labels while keeping their own networks.
  
  The compose project name is unchanged, so existing volumes and data are reused.
- **Compose runs in a `docker:27.5.1-cli` helper container.** The project is mounted at the path the daemon uses (`hostenv.MirrorMount`; `/run/desktop/mnt/host/c/...` on Docker Desktop for Windows), so relative bind mounts resolve to real host paths. Neither the host nor the SLD image needs a docker CLI.
- **Lifecycle:** `stop` → `docker compose stop`; `destroy` → `docker compose down`, plus `--volumes` only with `--volumes` (the warning names the stack's volumes).
- **Detection:** a compose file wins over framework detection. The web service(s) come from published ports; databases, caches, admin and backup tools are excluded, and nginx/web/app are ranked first. Project names come from the main folder, with generic folders (`backend`, `api`, ...) taking their parent's name and group suffixes (`-apps`, `-builds`, `-projects`) dropped. Domains are no longer cut to 20 characters.
- **`sld v2 scan [dir] [--write]`** finds every project (git roots, SLD configs, compose stacks), searches group folders up to 3 levels deep, honours `.sldignore`, and never overwrites existing configs.
- **PHP constraints:** `>=7.4` → 8.4, and `>=8.1 <8.4` → 8.3.

Verified:
- A read-only scan of `Documents/Development` found 18 projects. Each compose stack resolved to the service and port its running containers use today.
- A live fixture (stack in `backend/`, relative bind mount, published ports) was served on `demo.localhost` from the bind-mounted folder, with ports removed, the project name kept and the database left unrouted; `stop` and `destroy` worked.

Follow-ups:
- `start` returns a few seconds before Traefik picks up new containers.
- Projects not yet started under SLD keep binding their own ports and can still block the router's port 80.

### M5 — Control plane rebuild (`sld serve`) *(7–10 days)* — backend implemented 2026-11-20

What landed:
- **`sld serve`** (`cmd/sld/serve.go`) mounts the existing routes (dashboard files, `/api/db/*` database manager, WebSocket) through the new `api.Server.Mux()`, plus `/api/v2` from `internal/server`. Everything sits behind the auth middleware. It listens on `:2025` / `SLD_LISTEN` and scans `SLD_WORKSPACES` (default `/workspace` in the image).
- **Auth (`internal/server/auth.go`):**
  - The admin token is printed once on first start and only its SHA-256 is stored in `<data>/auth.json`. `SLD_ADMIN_TOKEN` and `--reset-token` are supported.
  - Browsers get an HttpOnly, SameSite=Strict session cookie; scripts use `Authorization: Bearer`.
  - Host allow-list (localhost, `*.localhost`, `SLD_ALLOWED_HOSTS`) against DNS rebinding.
  - Cookie writes need `X-SLD-Request` and a local Origin.
  - Static dashboard files and `/api/health` are public.
- **API (`internal/server/api.go`):**
  - `GET /api/v2/projects`: scan plus live status (running/partial/stopped/not-configured/undetected) and URLs.
  - `GET /api/v2/projects/{id}`: detail including the config YAML.
  - `POST /api/v2/projects/{id}/setup|start|stop|restart|destroy`: background jobs, one per project, with `remove_volumes` for destroy.
  - `GET /api/v2/jobs/{id}` and `/events` (SSE, resumable with `offset`).
  - `GET /api/v2/projects/{id}/logs?target=&tail=&follow=1`.
  - `GET /api/v2/system`, `POST /api/v2/system/init`, `GET /api/v2/system/ca.crt`.
- **Runner output** goes to a per-job writer (`Runner.SetOutput`), so job output is captured instead of printed to the server's stdout.

Fix found during the live test: `StartProject` treated compose-managed containers as obsolete SLD containers and **removed them on every start**, so compose rebuilt the stack each time. They are now skipped, with a regression test added.

Verified live with `sld serve` in a container against the real workspace:
- Auth gates: public health/static, 401 without sign-in, 403 for a foreign Host, 403 for a cookie POST without the CSRF header.
- Cookie sign-in; 17 projects listed (SLD's repo excluded via `.sldignore`), with swiftsms `running`.
- A start job streamed over SSE, logs were read per app, and a second start left all 5 swiftsms containers untouched.

Next (M6): the dashboard's sign-in screen and Projects/Setup pages on `/api/v2`, sending `X-SLD-Request` on writes.

Original scope:
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

### M6 — Dashboard *(4–6 days, can overlap with M5)* — first version implemented 2026-11-20

What landed:
- **Sign-in gate** (`components/auth/AuthGate.tsx`): a token form that reappears whenever the API answers 401.
- **One fetch interceptor** (`lib/fetch-interceptor.ts`) adds `X-SLD-Request` to every same-origin API write and reports 401s, so the existing database pages work behind auth without per-page changes.
- **`src/api/v2.ts`:** typed client for `/api/v2`, with SSE job following and streamed logs.
- **Projects page (home):**
  - status badges, filters, search, per-app links;
  - set up / start / stop / restart, with a live job console;
  - a log viewer per container, with follow and pause;
  - destroy keeps data by default, and deleting volumes requires typing the project name.
- **Setup page:** Docker and router status (with a "Start router" button), ports, workspaces, CA certificate download with install steps, sign out.
- **Topbar** shows SLD health (Docker plus router) instead of V1's PHP/nginx host status.
- **Navigation** is reduced to Projects, Databases and Setup. V1-only pages (Domains, Plugins, X-Ray, Share, Doctor, host Logs/Settings) are no longer routed; their files are still in the repo.
- **Build:** 0 TypeScript errors. The production build is embedded in `pkg/assets/gui`.

Follow-ups:
- A browser end-to-end check (the Chrome extension was not connected during implementation).
- Delete the unrouted V1 page files.
- ~~Per-project detail view (config editor, file manager, env editor).~~ Done 2026-11-20: `/projects/:id` with Overview, Config, Files, Environment and Logs tabs, backed by `/api/v2/projects/{id}/{config,files,file,env}` (`internal/server/projectfiles.go`). Paths are confined to the project folder (symlinks included), configs are validated before saving, env edits keep comments and order, and env backups live in the data volume.

Original scope:
- Login screen and token handling in `src/api/daemon.ts`; remove the hard-coded `localhost:2025` (`Database.tsx:2407`).
- Projects → Apps view with runtime badges, start/stop per app, terminal, tasks, logs.
- Databases → Servers (shared + project + discovered) with create-server flow.
- Setup checklist: workspace mounted, CA downloaded and trusted, TLD choice.
- Remove the Plugins page and PHP-only wording.
- Tauri: keep it as an optional shell that just points at the container URL (no sidecar).

### M7 — Image, compose, release *(2–3 days)* — local part done 2026-11-20; published to Docker Hub by hand 2026-11-22

What landed:
- **Image and compose:**
  - The image's default command is `serve`, and the compose `app` service runs `serve`.
  - Compose mounts the projects folder at `/workspace` (`SLD_WORKSPACE`, default: the folder containing the repo) and passes router ports (`SLD_HTTP_PORT`/`SLD_HTTPS_PORT`).
  - The bundled MariaDB, Redis, Mailpit and phpMyAdmin stay. They are the database manager's default servers until shared servers are managed from the dashboard (D4).
- **Release workflow:** `.deb`/`.pkg`/`.msi` host installers are no longer built. Releases carry the CLI binaries plus `SHA256SUMS` and the Tauri desktop app. (`scripts/packaging` is deleted with V1 in M9.)
- **README** is rewritten around "run one container": quick start (including `--group-add` for the socket), compose, dashboard, domains/ports, HTTPS, config examples, CLI, security.
- **Makefile** gains `test`, `serve`, `docker-token` and `docker-image-app`. Host-mode targets are kept.
- **Formatting:** three pre-existing files are now gofmt'd, so CI's format check passes.

**Publishing: Docker Hub, by hand for now.** GitHub Packages is disabled for the `supreme-majesty` account, so the images live on Docker Hub under `insightifytech` (SLD's default `SLD_IMAGE_REGISTRY`). `make docker-publish VERSION=…` builds and pushes every image for amd64 + arm64. `docker-images.yml` pushes to Docker Hub with the `DOCKERHUB_USERNAME`/`DOCKERHUB_TOKEN` secrets, but every workflow still fails before starting because *"your account is locked due to a billing issue"* (GitHub).

Published as of 2026-11-25: `sld:latest`, `sld:v3.0.0`, `sld-router:2`/`latest`, `sld-nginx:1`/`latest`, `sld-php:8.1`–`8.3` (amd64 + arm64) and `sld-php:8.4` (**amd64 only**). The arm64 build of 8.4 runs under emulation and did not fit in the build machine's memory; until it is pushed, PHP 8.4 projects (the default) cannot start on Apple Silicon Macs or arm64 Linux.

Original scope:
- Root `Dockerfile`: `CMD ["serve"]`, `VOLUME /data`, `SLD_STATE_DIR=/data`, healthcheck on `/api/health`. Multi-arch build, cosign signing, SBOM, Trivy (the pipeline already exists in `docker-images.yml`).
- `compose.yaml` reduced to `sld` (+ `hardened` socket-proxy profile). The old bundled DB services move to docs under "shared servers".
- Release workflow: image tags; drop `.deb`/`.pkg`/`.msi`, systemd units, `postinst`. Ship a final 2.2.x host binary for `legacy migrate` (D5).
- Makefile: replace `park`/`link`/`daemon` targets with `docker-*` and `dev` targets.

### M8 — Phase 14 E2E in container mode *(3 days)* — implemented 2026-11-20

What landed:
- **`scripts/e2e.sh`** (`make e2e`) runs an SLD image with `serve` against `test/e2e/fixtures` (static, node, python, dockerfile, compose-in-a-subfolder, plus php with `E2E_PHP=1`). It drives everything through `/api/v2`: discovery → setup → start → request the domain through the router → destroy → no containers left. It also checks the auth gates, unprivileged app users, compose host-port removal and that static sites do not serve `.git`.
- **Isolation:** the test runs beside a real SLD installation without touching it. The router and shared network names are configurable (`SLD_ROUTER_CONTAINER`, `SLD_GLOBAL_NETWORK`), and the test uses its own data volume.
- **CI:** `.github/workflows/e2e.yml` builds the router, nginx, PHP 8.3 and SLD images and runs the suite with PHP on pushes and PRs. It is blocked, like every workflow, by the GitHub billing lock (still in place on 2026-11-25).

Fixes found by the tests:
- **Unused networks:** compose-only projects created an unused SLD project network. Docker Desktop's network address pool is small (~31), and this machine ran out ("all predefined address pools have been fully subnetted"). Project networks are now created only when SLD runs containers of its own, and leftovers are removed on start.
- **Unreachable right after start:** a project could be "started" while Traefik had not yet picked it up (first request 404, sometimes for over a minute). `start` now waits, up to 90 s, until the router serves each domain (`Orchestrator.WaitForRoutes`).
- **Hidden files served:** static sites served hidden files (`.git/`, `.sld/`) with stock nginx's config. They now use an SLD-managed config that refuses dotfiles, except `.well-known/`.

Verified locally: 37/37 checks on two consecutive runs (without PHP: the SLD PHP images are not built on this machine).

Original scope:
- CI job: build the image, run it with a temp workspace, then for each fixture (Laravel, static, Next.js, Vite, FastAPI, Django, Rails, Go, Dockerfile, proxy) call the API to register, start, `curl` through the router, create a DB, run a query, stop and destroy.
- Same suite on `ubuntu-latest` and on a Docker Desktop–style path layout (fixture for C1).
- Gate for M9.

### M9 — Delete V1 *(1–2 days)* — implemented 2026-11-20

Decisions (2026-11-20): keep every dashboard page and back the host-dependent ones with Docker; make the V2 commands the top-level CLI.

Deleted:
- **Host layer:** `pkg/adapters` (Linux/macOS/Windows), the V1 daemon's nginx/PHP-FPM/hosts/cert logic and `state.json` (`pkg/daemon/state`, `pkg/daemon/metrics`), `pkg/plugins` and the host plugins (redis/mailhog/postgres binaries), the cloudflared download (`tunnel.go`), `pkg/assets/runtime` (nginx templates, `router.php`), `scripts/packaging`, `cmd/debug-env`, `pkg/project`, `pkg/util`.
- **Unauthenticated listener:** the V1 daemon's own listener (`api.Server.Start`), which served the API without auth and with `Access-Control-Allow-Origin: *`.
- **CLI:** the V1 host commands (install, uninstall, daemon, park, link, secure, php, service, plugin, share, sites ...). The project commands are top level (`sld init`, `sld start`, `sld scan`, ...); `sld v2 …` stays as a hidden alias. `sld db clone|snapshot`, `sld serve` and `sld legacy …` (for moving off V1) remain.
- **Host changes:** `internal/dns` no longer writes `/etc/hosts` or `/etc/resolver`. For TLDs other than `.localhost`, `sld start` prints the hosts line to add. The healer no longer runs `apt-get install` or `fuser -k`; issues can only be dismissed, and their text says what to do.

Re-backed with Docker (in `internal/server`: `classic.go`, `plugins.go`, `share.go`, `create.go`), with the same JSON shapes the pages already use:
- **Overview, Sites, Domains:** `/api/state`, `/api/sites` come from the workspace scan. Tags and categories are stored in `settings.json`, and sites carry a `url` that includes the router port. "Remove" writes `.sldignore`; park/link un-hide a folder inside a workspace and refuse paths outside it.
- **Services, Doctor, Metrics:** services are SLD's own containers: the router, the other services of SLD's compose stack, and plugin containers. Start/stop/restart only accept those. Doctor checks Docker, the router, workspaces, the CA and domain resolution. "Restart" restarts the router.
- **Plugins:** Redis, Mailpit and PostgreSQL run as `sld-plugin-<id>` containers on the router network. Redis and Mailpit reuse the SLD compose stack's own containers when present.
- **Share:** a `cloudflare/cloudflared` container on the router network runs a quick tunnel to `sld-router` with the project's Host header.
- **New project / Ghost Mode:** templates run in throwaway `composer:2.8` / `node:22` containers as the folder owner. WordPress is downloaded and unpacked in Go. Clones are copied in Go, get their own `.sld/config.yaml` and a `.sld/ghost-of` marker. Discard only deletes folders that carry the marker. Every path must be inside a workspace.
- **Pages:** global PHP switch replaced by the list of available versions (PHP is set per project). "Park folder" opens Projects, "Enable HTTPS" opens Setup.

Not moved yet: `pkg/services` and `pkg/daemon` (now only the database manager, logs, env, artisan, healer and X-Ray services) still live under `pkg/`. Moving them under `internal/` is a rename with no behaviour change, left for a quiet moment. Artisan (the console drawer) and X-Ray still expect a host PHP and nginx log; the CLI's `sld artisan` runs in the project container.

Original scope:
- Delete `pkg/adapters/`, V1 parts of `pkg/daemon/` (nginx config gen, `state.json` routing), `pkg/plugins/`, `pkg/assets/runtime/`, `internal/dns` hosts-file editing (keep only the "suggest hosts lines" helper), `scripts/packaging/*`.
- Move surviving `pkg/services` code (DB drivers, SQL import, healer, env manager, tunnel, discovery) under `internal/`.
- `internal/legacy` remains only in the final host-binary build tag.

### M10 — Docs *(1–2 days, alongside M7–M9)* — implemented 2026-11-20

What landed:
- **README:** per-OS notes (Windows/WSL filesystem, macOS file sharing, Linux socket group, several workspaces), databases (per project vs shared), the dashboard pages as they are after M9, `make e2e`.
- **ADRs:** 0007 multi-runtime drivers, 0008 containerised control plane (0001 and 0002 marked as amended), 0009 path mapping and workspace mounts, 0010 control plane authentication.
- **Architecture:** `SLD_V2_ARCHITECTURE.md` rewritten for the current system. The audit, the implementation blueprint and the distribution doc are marked historical; the DNS, security and runtime docs point to what changed.
- `dockerization_progress.md` closed.

Original scope:
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

Status 2026-11-20 (after M10). Nothing is ticked that has not been verified.

- [ ] `docker run … insightifytech/sld:latest` on a clean Linux, macOS and Windows machine opens a working dashboard, with nothing else installed. *Verified on Windows + Docker Desktop with a locally built image. The image is on Docker Hub (2026-11-22) but has not been tried on a clean machine yet, and `sld-php:8.4` is published for amd64 only.*
- [ ] A Laravel, Next.js, Django, Rails, Go, static and Dockerfile project each start from the dashboard and are reachable at `<name>.localhost`. *The end-to-end suite covers static, Node, Python, Dockerfile and compose, plus PHP in CI; real Laravel/compose projects were verified by hand (swiftsms, credora). Next.js 15, Django 5.1, Rails 8 and Go fixtures were added on 2026-11-25 and pass locally (Rails needed the full `ruby` image). The PHP fixture is plain PHP, not Laravel, and has not been run locally against the published `sld-php` images yet.*
- [ ] Project and shared databases can be created, browsed, queried, imported, snapshotted and cloned from the dashboard. *The database manager is in daily use against the bundled MariaDB. Project database containers and the PostgreSQL plugin are found by the same Docker discovery but were not tested end to end. `services.database.shared` (SLD-managed shared servers per project) is not implemented.*
- [x] Every API route requires auth (except health, login and auth status); the router and API bind to loopback by default.
- [ ] Upgraded v1-config PHP projects start with zero container recreation. *Covered by the config migration golden test only; no test compares container spec hashes before and after.*
- [x] No `pkg/adapters`, systemd units or host-mutation code in the image build; no binaries in `git ls-files`. *(`sld legacy remove`, for leaving a V1 install, still offers to disable the old systemd unit on the host it runs on.)*
- [x] README and ADRs 0007–0010 written (committed locally, not pushed).
