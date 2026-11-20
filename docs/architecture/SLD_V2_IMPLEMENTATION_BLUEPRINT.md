# SLD V2 Implementation Blueprint

> **Historical.** The plan V2 was built from. Where it differs from the code, the code and [`SLD_V2_ARCHITECTURE.md`](SLD_V2_ARCHITECTURE.md) are current. In particular: SLD now also runs as a container with an authenticated dashboard ([ADR 0008](../adr/0008-containerised-control-plane.md)), supports runtimes beyond PHP ([ADR 0007](../adr/0007-multi-runtime-drivers.md)), and defaults to `.localhost` with its own CA instead of `.test` and mkcert.

This document defines the final implementation-ready architecture and engineering blueprint for Supreme Local Dev (SLD) V2. It resolves all outstanding architectural decisions, incorporates findings from `docs/audit/SLD_V2_CURRENT_STATE_AUDIT.md`, and serves as the authoritative specification before any code is implemented.

---

## 1. Architecture Principles

1. **Host Purity & Zero Host Mutation**: The host system must only host the `sld` CLI binary, the Docker Engine, and developer project source code. SLD V2 shall NEVER install, mutate, or manage host-level packages (`nginx`, `php-fpm`, `mysql`, `redis`, `dnsmasq`) via OS package managers (`apt`, `brew`, `winget`).
2. **No Privileged Daemon or Background Service**: The V1 elevated background daemon (`pkg/daemon/api/server.go`, running on port 2025 as root) and systemd unit (`sld-daemon.service`) are permanently eliminated. Docker Engine is the execution engine; the `sld` CLI runs exclusively in user space as a short-lived control plane.
3. **Declarative State with Docker as Runtime Source of Truth**: Project configuration is declared in version-controlled `.sld/config.yaml`. Docker Engine state—identified and tracked via deterministic Docker labels—is the sole runtime source of truth. There is no global mutable state file (eliminating `/var/lib/sld/state.json`).
4. **FastCGI Separation (Strict Web Architecture)**: Traefik routes HTTP traffic to an internal project Nginx web server; Traefik MUST NOT route HTTP directly to PHP-FPM. PHP-FPM speaks FastCGI. Project Nginx serves static assets directly and proxies PHP requests over FastCGI to PHP-FPM.
5. **Deterministic Isolation**: Every project is isolated in its own private Docker bridge network. Auxiliary services (databases, Redis) are private to the project network and do not expose host ports by default.
6. **Idempotent Reconciliation**: Every state-changing command (`sld start`, `sld restart`) acts as a declarative reconciler: Desired State vs. Actual State → Diff → Minimal Actions → Health Verification. Running `sld start` repeatedly is completely idempotent.

---

## 2. Final Runtime Architecture

### Infrastructure Topology

```
Host OS
├── sld CLI (User space Go binary)
├── Docker Engine API (/var/run/docker.sock)
└── Project Source Code (/path/to/project)

Docker Engine
├── Global Infrastructure (Created by `sld init` / managed by SLD)
│   ├── Network: `sld-global` (bridge)
│   └── Container: `sld-router` (Traefik v3.6, binds host ports 80 & 443)
│
├── Project Infrastructure: App 1 (`api-3f7b49a1`)
│   ├── Network: `sld-api-3f7b49a1-net` (bridge, isolated)
│   ├── Container: `sld-api-3f7b49a1-nginx` (Joins BOTH `sld-global` AND `sld-api-3f7b49a1-net`)
│   ├── Container: `sld-api-3f7b49a1-php`   (Joins ONLY `sld-api-3f7b49a1-net`)
│   ├── Container: `sld-api-3f7b49a1-db`    (Joins ONLY `sld-api-3f7b49a1-net`, Volume: `sld-api-3f7b49a1-db-data`)
│   └── Container: `sld-api-3f7b49a1-redis` (Joins ONLY `sld-api-3f7b49a1-net`)
│
└── Project Infrastructure: App 2 (`web-e81a029c`)
    ├── Network: `sld-web-e81a029c-net` (bridge, isolated)
    ├── Container: `sld-web-e81a029c-nginx` (Joins BOTH `sld-global` AND `sld-web-e81a029c-net`)
    └── Container: `sld-web-e81a029c-php`   (Joins ONLY `sld-web-e81a029c-net`)
```

### Request Flow Path

```
Browser
   ↓ HTTP / HTTPS (e.g., http://my-app.test)
DNS Resolution (*.test → 127.0.0.1)
   ↓
Host Port 80 / 443
   ↓
`sld-router` (Traefik on `sld-global` network)
   │  Traefik terminates TLS (using wildcard mkcert certificate)
   │  Inspects Host header: `my-app.test`
   │  Routes HTTP traffic over `sld-global` network
   ↓ HTTP :80
`sld-my-app-3f7b49a1-nginx` (Project Nginx)
   │  1. Static Assets (.css, .js, images): Served directly from /app/public
   │  2. Front Controller / .php requests: Proxied via FastCGI
   ↓ FastCGI :9000 (Internal TCP over `sld-my-app-3f7b49a1-net`)
`sld-my-app-3f7b49a1-php` (Project PHP-FPM)
   │  Executes PHP application code mounted at /app
   │  Communicates with internal MySQL (`sld-my-app-3f7b49a1-db:3306`)
   │  Communicates with internal Redis (`sld-my-app-3f7b49a1-redis:6379`)
   ↓
Application Response returned via FastCGI → Nginx → Traefik → Browser
```

---

## 3. Component Responsibilities

| Component | Execution Context | Responsibilities | Forbidden Actions |
|---|---|---|---|
| **`sld` CLI** | Host (User space) | CLI commands, parses `.sld/config.yaml`, computes project identity, calls Docker Engine API via Go SDK, diffs state, executes reconciliation, streams logs, proxies `artisan`/`composer`/`npm` commands. | Must NOT run background daemon; must NOT require sudo for standard dev commands; must NOT modify OS package managers. |
| **`sld-router` (Traefik)** | Docker (`sld-global`) | Binds host ports 80/443; discovers backends dynamically via Docker labels on `sld-global`; terminates TLS using mounted certs; proxies HTTP requests to project Nginx containers. | MUST NOT route directly to PHP-FPM; must NOT have write access to Docker socket; must NOT manage DNS. |
| **`sld-nginx`** | Docker (`sld-global` + project network) | Dual-homed network bridge; serves static assets; routes application requests via Laravel/Symfony front-controller rules; proxies `.php` requests to PHP-FPM over FastCGI; exposes `/sld-health`. | Must NOT execute PHP directly; must NOT mount host Docker socket. |
| **`sld-php`** | Docker (Project network only) | PHP-FPM runtime (8.1, 8.2, 8.3, 8.4); pre-packaged extensions; Composer runtime; handles FastCGI on `0.0.0.0:9000`; runs with mapped UID/GID matching host user. | Must NOT join `sld-global`; must NOT bundle Node.js; must NOT expose host ports. |
| **Project Database** | Docker (Project network only) | MySQL, MariaDB, or PostgreSQL engine; isolated to project network; stores data in named persistent Docker volume. | Must NOT expose host ports by default; must NOT bind to `sld-global`. |
| **Project Redis** | Docker (Project network only) | In-memory key-value store; isolated to project network. | Must NOT expose host ports by default; must NOT bind to `sld-global`. |
| **Node Container** | Docker (Ephemeral or Managed) | Ephemeral one-shots for `sld npm install/build`; persistent managed service for `sld npm run dev` (Vite dev server) with dedicated port routing. | Must NOT be baked into `sld-php` image. |

---

## 4. Repository Mapping

Every file and package from V1 is mapped to its exact V2 disposition:

```
Existing V1 Repository Structure vs. V2 Strategy
```

| File / Package | Lines | V1 Role | V2 Disposition | Action Timing |
|---|---|---|---|---|
| `cmd/sld/main.go` | 1064 | CLI entrypoint, sudo elevator, REST client to port 2025 | **ADAPT**: Refactor to direct Cobra commands calling `internal/reconcile` and `internal/docker`. Strip `elevate()`, `autoInstall()`, and HTTP calls. | Phase 11 |
| `pkg/daemon/daemon.go` | 1089 | In-memory Nginx generator, service supervisor | **REPLACE**: Superseded by `internal/reconcile` and `internal/runtime`. | Phase 4 |
| `pkg/daemon/api/server.go` | 2374 | Unauthenticated HTTP REST API on port 2025 | **QUARANTINE & REMOVE**: Deprecate immediately; remove when V2 CLI commands are complete. | Phase 15 |
| `pkg/daemon/api/*` | ~1000 | WebSocket hub, event stream, API handlers | **REMOVE**: No daemon HTTP API in V2. | Phase 15 |
| `pkg/daemon/state/manager.go` | 250 | Manages `/var/lib/sld/state.json` | **REPLACE**: Superseded by Docker label queries and minimal `~/.sld/config.json`. | Phase 3 |
| `pkg/adapters/interface.go` | 58 | Host OS mutation interface (`SystemAdapter`) | **REMOVE**: Replaced by Docker SDK client. | Phase 15 |
| `pkg/adapters/linux/*` | ~600 | Runs `apt-get`, `systemctl`, `systemd-resolved` | **REMOVE**: Host package installation is eliminated. | Phase 15 |
| `pkg/adapters/macos/*` | 397 | Runs `brew install`, `brew services` | **REMOVE**: Host package installation is eliminated. | Phase 15 |
| `pkg/adapters/windows/*` | 365 | Runs `winget`, `sc query` | **REMOVE**: Host package installation is eliminated. | Phase 15 |
| `pkg/project/config.go` | 92 | Parses `.sld.yaml` and `composer.json` | **ADAPT**: Move to `internal/project/detect.go` to infer PHP version from `composer.json` and web root from directory structure. | Phase 2 |
| `pkg/services/project_manager.go` | 1016 | Framework detection, editor launch | **ADAPT**: Extract framework detection logic into `internal/project/framework.go`. | Phase 2 |
| `pkg/services/database.go` | ~420 | Connects to host MySQL via `127.0.0.1:3306` | **REPLACE**: Replaced by Dockerized database container lifecycle in `internal/runtime/services.go`. | Phase 9 |
| `pkg/services/artisan.go` | ~100 | Shells out to host PHP binary | **ADAPT**: Rewrite to execute `docker exec` against `sld-<project-id>-php`. | Phase 11 |
| `pkg/services/env_manager.go` | ~170 | Reads and modifies `.env` files | **KEEP & ADAPT**: Reused in `internal/project/env.go` for managing database credentials in project `.env`. | Phase 9 |
| `pkg/services/log_watcher.go` | ~300 | Tails host log files | **REPLACE**: Replaced by `internal/docker/logs.go` streaming Docker container logs. | Phase 12 |
| `pkg/assets/runtime/router.php` | 201 | PHP-based router reading `state.json` | **REMOVE**: Traefik + Nginx completely replace this. | Phase 15 |
| `pkg/assets/runtime/nginx/*` | ~200 | Host Nginx configuration templates | **REPLACE**: Replaced by containerized Nginx template in `docker/nginx/default.conf.template`. | Phase 5 |
| `sld-dashboard/` | TS/Tauri | Frontend GUI communicating with port 2025 | **DEPRECATE (V2.0)**: Quarantined during MVP; adapt in V2.1 to use Tauri IPC/CLI JSON. | Phase 15 |
| `Makefile` | 108 | Compiles binaries, runs `sudo sld install` | **ADAPT**: Strip `sudo` targets; add Docker image build and testing targets. | Phase 1 |
| `go.mod` | 32 | Go 1.25 dependencies | **ADAPT**: Add `github.com/docker/docker` and `github.com/docker/go-connections`. | Phase 1 |

---

## 5. Project Identity

### Collision Problem & Objectives
V1 identified projects purely by folder name (e.g., `api`). Two projects located at `/home/user/client-a/api` and `/home/user/client-b/api` would collide on container names, domains, and network namespaces.
The V2 project identity solution must:
1. Remain strictly stable across repeated `sld start` invocations.
2. Prevent collisions between identically named folders across different paths.
3. Produce human-readable Docker container and network names.
4. Avoid random UUIDs so identity is 100% reproducible from filesystem location.

### Identity Algorithm Specification

```
Input: Absolute directory path (string)
Output: Deterministic Project ID (string), Project Slug (string), Project Hash (string)
```

1. **Path Canonicalization**:
   - Resolve all symlinks: `realPath = filepath.EvalSymlinks(inputPath)`
   - Convert to clean absolute path: `absPath = filepath.Clean(realPath)`
   - On Windows, normalize volume drive letter to lowercase and convert backslashes to forward slashes.
2. **Slug Extraction**:
   - Extract base folder name: `base = filepath.Base(absPath)`
   - Normalize: lowercase all characters.
   - Replace any non-alphanumeric character with a hyphen (`-`).
   - Collapse multiple consecutive hyphens into a single hyphen.
   - Trim leading and trailing hyphens.
   - If slug becomes empty, default to `"project"`.
   - Truncate slug to a maximum of 20 characters to respect Docker/DNS name limits.
3. **Deterministic Hash**:
   - Compute SHA-256 of canonical path: `sum = sha256.Sum256([]byte(absPath))`
   - Encode to hex string: `hexStr = hex.EncodeToString(sum[:])`
   - Extract the first 8 characters: `hash8 = hexStr[0:8]`
4. **Project ID Assembly**:
   - `ProjectID = fmt.Sprintf("%s-%s", slug, hash8)`

### Examples

| Canonical Path | Slug | Hash | Resulting Project ID |
|---|---|---|---|
| `/home/developer/code/client-a/api` | `api` | `3f7b49a1` | `api-3f7b49a1` |
| `/home/developer/code/client-b/api` | `api` | `e81a029c` | `api-e81a029c` |
| `/var/www/Super_E-Commerce.Store` | `super-e-commerce-s` | `9b21d450` | `super-e-commerce-s-9b21d450` |
| `C:\Users\Dev\Projects\crm` | `crm` | `5c4a7e12` | `crm-5c4a7e12` |

---

## 6. Configuration Schema

Project configuration lives in `.sld/config.yaml` at the project root.

### Schema Definition (`.sld/config.yaml`)

```yaml
version: 1

name: "my-project"          # Optional. Defaults to directory slug.

runtime:
  php: "8.3"                # Required for PHP projects. Supported: "8.1", "8.2", "8.3", "8.4"
  web_root: "public"        # Optional. Default: "public" (Laravel/Symfony) or "." (generic PHP)

domain:
  name: "my-project.test"   # Optional. Default: {name}.test
  tls: false                # Optional. Default: false. True enables HTTPS via Traefik.

services:
  database:
    type: "mysql"           # Optional: "mysql", "postgres", "mariadb"
    version: "8.0"          # Optional. Defaults: mysql: "8.0", postgres: "16", mariadb: "10.11"
    expose_port: 0          # Optional. Default: 0 (no host port). Set to e.g. 33060 to expose.
  redis:
    enabled: false          # Optional. Default: false.
    version: "7-alpine"     # Optional. Default: "7-alpine"
  mailpit:
    enabled: false          # Optional. Default: false.

node:
  version: "20"             # Optional. Supported: "18", "20", "22".
  dev_server:
    enabled: false          # Optional. Default: false.
    command: "npm run dev"  # Command to execute for persistent Vite/dev server.
    port: 5173              # Internal dev server port.
    expose_port: 5173       # Host port mapping for Vite HMR.

xdebug:
  enabled: false            # Optional. Default: false. When true, sets XDEBUG_MODE=debug.

environment:                # Optional non-secret environment variables injected into PHP.
  APP_ENV: "local"
  APP_DEBUG: "true"
```

### Schema Rules & Validation

1. **Required Fields**:
   - `version`: Must equal `1`.
   - `runtime.php`: Must be provided or successfully inferred from `composer.json`. Must strictly match supported versions: `["8.1", "8.2", "8.3", "8.4"]`.
2. **Field Defaults**:
   - `name`: Fallback to directory basename slug.
   - `runtime.web_root`: If directory `public` exists, defaults to `"public"`; otherwise `"."`.
   - `domain.name`: Defaults to `<name>.test`.
   - `domain.tls`: Defaults to `false`.
3. **Strict Secret Hygiene**:
   - Database credentials, API tokens, and private keys MUST NOT be committed to `.sld/config.yaml`.
   - Secret injection follows standard 12-factor pattern: framework `.env` files are preserved.
   - Database passwords are generated as cryptographically secure random strings upon initial provisioning, stored in local gitignored `.sld/secrets.json` (permissions 0600), and injected into the container environment. Deterministic passwords derived from project identity or static values (e.g. `sld_secret`) are strictly prohibited.

### PHP Version Detection Precedence

When initializing a project or running `sld start` without explicit `runtime.php`:

```
1. Explicit .sld/config.yaml (`runtime.php`)
   ↓ if missing
2. Project `composer.json` (`require.php` constraint)
   ↓ if missing
3. Supported Framework Inference (Laravel 11+ → PHP 8.2+, etc.)
   ↓ if missing
4. Abort with actionable error: "PHP version required. Please specify runtime.php in .sld/config.yaml"
```

**Constraint Resolution Rules**:
- `^8.3` or `~8.3.0` → resolves to `8.3`
- `>=8.2` → resolves to highest available: `8.4`
- `^8.1` → resolves to `8.1`
- `^7.4` or `<8.0` → **FATAL ERROR**: SLD V2 does NOT silently downgrade or run unsupported PHP versions.
  *Error output*: `Project requires PHP ^7.4, but SLD V2 only supports PHP 8.1, 8.2, 8.3, 8.4. Please upgrade project dependencies or configure a supported PHP version.`

---

## 7. Docker Resource Model

Every project is isolated into a dedicated resource cluster identified by `com.supreme.sld.project=<project-id>`.

### Resource Naming Standards

| Resource | Name Pattern | Concrete Example |
|---|---|---|
| **Project Network** | `sld-{project-id}-net` | `sld-api-3f7b49a1-net` |
| **Nginx Container** | `sld-{project-id}-nginx` | `sld-api-3f7b49a1-nginx` |
| **PHP Container** | `sld-{project-id}-php` | `sld-api-3f7b49a1-php` |
| **Database Container** | `sld-{project-id}-db` | `sld-api-3f7b49a1-db` |
| **Redis Container** | `sld-{project-id}-redis` | `sld-api-3f7b49a1-redis` |
| **Node Container** | `sld-{project-id}-node` | `sld-api-3f7b49a1-node` |
| **Database Volume** | `sld-{project-id}-db-data` | `sld-api-3f7b49a1-db-data` |
| **Redis Volume** | `sld-{project-id}-redis-data` | `sld-api-3f7b49a1-redis-data` |
| **Global Network** | `sld-global` | `sld-global` |
| **Router Container** | `sld-router` | `sld-router` |

### Mandatory Docker Labels

Every container, network, and volume created by SLD must be labeled:

```ini
com.supreme.sld=true
com.supreme.sld.version=2
com.supreme.sld.project={project-id}
com.supreme.sld.project-path={canonical-abs-path}
com.supreme.sld.service={nginx|php|db|redis|node|router}
com.supreme.sld.resource-type={container|network|volume}
com.supreme.sld.config-hash={sha256-of-sld-config-yaml}
com.supreme.sld.managed-by=sld
```

*Label Semantics*:
- `com.supreme.sld.project` is the canonical identity key for resource discovery and grouping.
- `com.supreme.sld.project-path` is diagnostic metadata and MUST NOT be used as the primary identity key.
- `com.supreme.sld.service` describes the functional service role (only on containers). Networks and volumes must NOT be labeled as services.
- `com.supreme.sld.resource-type` describes the Docker entity type (`container`, `network`, `volume`).

### Container Health Checks

Health checks are explicit contracts that must only use tools guaranteed to exist in the image:

- **`sld-nginx`**: `wget -q -O - http://127.0.0.1:80/sld-health || exit 1`
  *(Interval: 5s, Timeout: 2s, Retries: 3, StartPeriod: 3s — uses wget which exists in slim/alpine images)*
- **`sld-php`**: `/usr/local/bin/sld-php-healthcheck || exit 1`
  *(Interval: 5s, Timeout: 2s, Retries: 3, StartPeriod: 5s — uses a dedicated readiness script that tests FastCGI responsiveness; `php-fpm -t` only validates syntax, not runtime readiness)*
- **`sld-db` (MySQL/MariaDB)**: `mysqladmin ping -h 127.0.0.1 --silent || exit 1`
  *(Interval: 5s, Timeout: 3s, Retries: 5, StartPeriod: 10s)*
- **`sld-db` (Postgres)**: `pg_isready -h 127.0.0.1 -U $$POSTGRES_USER || exit 1`
  *(Interval: 5s, Timeout: 3s, Retries: 5, StartPeriod: 10s)*
- **`sld-redis`**: `redis-cli ping || exit 1`
  *(Interval: 5s, Timeout: 2s, Retries: 3, StartPeriod: 2s)*

---

## 8. Networking

### Two-Tier Network Architecture

```
                    [ HOST PORT 80 / 443 ]
                              │
                    ┌─────────▼────────┐
                    │    sld-router    │
                    │    (Traefik)     │
                    └─────────┬────────┘
                              │
                    ══════════╪════════════════════ sld-global Network
                              │
                    ┌─────────▼────────────────────┐
                    │    sld-{id}-nginx            │
                    │ (Dual-Homed Network Gateway) │
                    └─────────┬────────────────────┘
                              │
    ══════════════════════════╪════════════════════ sld-{id}-net (Project Network)
         ┌────────────────────┼────────────────────┐
         │                    │                    │
┌────────▼────────┐  ┌────────▼────────┐  ┌────────▼────────┐
│  sld-{id}-php   │  │   sld-{id}-db   │  │ sld-{id}-redis  │
│    (PHP-FPM)    │  │ (MySQL/Postgres)│  │     (Redis)     │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

### Strict Isolation Rules

1. **Traefik Scope**: `sld-router` connects ONLY to `sld-global`. It has zero visibility into `sld-{id}-net`.
2. **Dual-Homed Nginx**: `sld-{id}-nginx` is the ONLY container attached to both `sld-global` and `sld-{id}-net`. It bridges public ingress to internal services.
3. **Isolated Backends**: `sld-{id}-php`, `sld-{id}-db`, and `sld-{id}-redis` are attached ONLY to `sld-{id}-net`.
   - PHP cannot communicate with another project's database or PHP container.
   - Databases and Redis cannot be contacted from the global network.
4. **No Host Port Bindings by Default**:
   - Nginx, PHP, MySQL, and Redis do NOT publish host ports by default.
   - Database host port publishing is strictly opt-in via `services.database.expose_port` (e.g. `33060`) for local GUI clients (TablePlus/DBeaver).

---

## 9. Nginx/PHP Runtime

### FastCGI Communication Contract

```
Traefik  ──HTTP:80──>  Nginx  ──FastCGI TCP:9000──>  PHP-FPM
```

Traefik terminates incoming HTTP/HTTPS and passes the HTTP request to Nginx. Nginx checks if the requested URI corresponds to a static file inside the bind-mounted project `/app/public`. If the file exists, Nginx delivers it immediately without waking PHP. If the request requires dynamic processing or matches a `.php` extension, Nginx constructs a FastCGI request and forwards it to `sld-{project-id}-php:9000` via internal Docker DNS.

### Container Images

#### 1. `sld-nginx:1`
- **Base**: `nginx:1.27-bookworm-slim`
- **Dynamic Configuration**: At container startup, an entrypoint script executes `envsubst` to replace `${WEB_ROOT}` and `${PHP_HOST}` in `default.conf.template`:
  ```nginx
  server {
      listen 80;
      server_name _;
      root /app/${WEB_ROOT};
      index index.php index.html;

      client_max_body_size 128M;

      # SLD internal health check endpoint
      location = /sld-health {
          access_log off;
          return 200 'ok';
      }

      location / {
          try_files $uri $uri/ /index.php?$query_string;
      }

      location ~ \.php$ {
          fastcgi_pass ${PHP_HOST}:9000;
          fastcgi_index index.php;
          fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
          fastcgi_param DOCUMENT_ROOT $realpath_root;
          fastcgi_read_timeout 600;
          include fastcgi_params;
      }

      location ~ /\.(?!well-known).* {
          deny all;
      }
  }
  ```

#### 2. `sld-php:{8.1, 8.2, 8.3, 8.4}`
- **Base**: `php:{version}-fpm-bookworm`
- **Architectures**: Multi-arch `linux/amd64` and `linux/arm64`.
- **Pre-installed Extensions**:
  `pdo_mysql`, `pdo_pgsql`, `redis`, `gd`, `zip`, `bcmath`, `intl`, `mbstring`, `opcache`, `pcntl`, `exif`, `soap`, `sockets`
- **Composer**: Pre-bundled latest stable Composer binary.
- **Xdebug**: Pre-installed but inactive by default (`xdebug.mode=off`). Enabled dynamically when `xdebug.enabled: true` via environment variable `XDEBUG_MODE=debug` and `XDEBUG_CONFIG=client_host=host.docker.internal`.
- **User / UID Mapping**: The container entrypoint inspects `WWWUSER` and `WWWGROUP` environment variables (passed by the CLI matching host UID/GID) and dynamically adjusts the internal `www-data` user UID/GID. This prevents any file created by Composer or Artisan from becoming root-owned on the host.
- **Excluded from PHP Image**: Node.js, npm, Nginx, database servers.

---

## 10. Traefik

### Gateway Configuration (`sld-router`)
- **Image**: `ghcr.io/supreme-majesty/sld-router:2` (based on `traefik:v3.6`).
- **Static Config (`/etc/traefik/traefik.yml`)**:
  ```yaml
  entryPoints:
    web:
      address: ":80"
    websecure:
      address: ":443"

  providers:
    docker:
      exposedByDefault: false
      network: "sld-global"
      watch: true
    file:
      directory: "/etc/traefik/dynamic"
      watch: true

  api:
    dashboard: false
  ```
- **Socket Security**: Mounts `/var/run/docker.sock:ro` (read-only). Project containers never receive the Docker socket.

### Dynamic Routing Labels on Nginx Container

When SLD starts `sld-{project-id}-nginx`, it applies the following Traefik labels:

```ini
# Enable Traefik Discovery
traefik.enable=true
traefik.docker.network=sld-global

# HTTP Router
traefik.http.routers.sld-{project-id}.rule=Host(`{domain}`)
traefik.http.routers.sld-{project-id}.entrypoints=web
traefik.http.services.sld-{project-id}.loadbalancer.server.port=80

# HTTPS Router (When domain.tls: true)
traefik.http.routers.sld-{project-id}-tls.rule=Host(`{domain}`)
traefik.http.routers.sld-{project-id}-tls.entrypoints=websecure
traefik.http.routers.sld-{project-id}-tls.tls=true
traefik.http.services.sld-{project-id}-tls.loadbalancer.server.port=80
```

---

## 11. DNS

### Responsibility Separation
- **DNS responsibility**: Answers "What IP does `my-app.test` resolve to?" → `127.0.0.1`.
- **Traefik responsibility**: Answers "Which container receives the HTTP request with Host `my-app.test`?" → `sld-{project-id}-nginx`.

### Platform Strategy Matrix

| Platform | MVP Strategy | Port 53 Conflicts | Sudo Requirement | Future Strategy |
|---|---|---|---|---|
| **macOS** | `/etc/resolver/test` file containing `nameserver 127.0.0.1` (used only when local forwarder is running). | None. Resolvers work without binding port 53. | One-time sudo during `sld init`. | Maintained. Native macOS per-TLD routing. |
| **Linux (Ubuntu / Debian / Arch)** | Direct `/etc/hosts` synchronization for active `.test` domains. **Note**: Setting `DNS=127.0.0.1` and `Domains=~test` in `systemd-resolved` without a live DNS server actively listening on `127.0.0.1:53` is strictly prohibited, as it breaks local domain resolution. | Zero port 53 conflicts. Bypasses `systemd-resolved` port 53 collision entirely. | One-time host helper integration. | Optional containerized DNS forwarder on non-conflicting loopback IP (`127.0.0.54:53`) integrated with resolved. |
| **Windows WSL2** | Update Windows hosts file (`C:\Windows\System32\drivers\etc\hosts`) for active domains via WSL2 helper. | Port 53 binding in WSL2 does not route from Windows host browser reliably. | One-time Windows elevation. | Automated Windows-side helper utility. |

---

## 12. TLS

### Certificate Management via `mkcert`
- **Tool**: `mkcert` (local trusted CA utility).
- **Certificate Location**: `~/.sld/certs/`
  - `sld-wildcard.crt`: Certificate covering `*.test`, `test`, `localhost`, `127.0.0.1`.
  - `sld-wildcard.key`: Private key.
- **Workflow (`sld secure`)**:
  1. CLI checks if `mkcert` is installed on host `$PATH`.
     - If missing: aborts with message `mkcert is not installed. Please install mkcert (https://github.com/FiloSottile/mkcert) to enable local TLS.` SLD will NEVER run `apt install mkcert` automatically.
  2. Executes `mkcert -install` (installs local CA into system & browser trust stores).
  3. Generates wildcard cert into `~/.sld/certs/`.
  4. Writes dynamic Traefik TLS configuration file in `~/.sld/certs/traefik-tls.yml`:
     ```yaml
     tls:
       certificates:
         - certFile: "/certs/sld-wildcard.crt"
           keyFile: "/certs/sld-wildcard.key"
     ```
  5. Mounts `~/.sld/certs` as `/certs:ro` and `/etc/traefik/dynamic:ro` into `sld-router`.
- **Per-Project TLS**: Set `domain.tls: true` in `.sld/config.yaml`. Traefik immediately serves valid, browser-trusted HTTPS.

---

## 13. Database and Services

### Normal V2 Database Lifecycle

```
1. `sld start` reads `services.database` configuration.
2. Checks/Creates named volume: `sld-{project-id}-db-data`.
3. Creates isolated container: `sld-{project-id}-db` on `sld-{project-id}-net`.
4. Database initializes system tables in persistent volume.
5. SLD awaits database health check (`mysqladmin ping` / `pg_isready`).
6. SLD injects connection credentials into PHP-FPM environment.
```

### Supported Engines and Versions
- **MySQL**: Images `mysql:8.0`, `mysql:8.4`
- **MariaDB**: Image `mariadb:10.11`
- **PostgreSQL**: Images `postgres:16-alpine`, `postgres:15-alpine`

### Standard Injected Environment Variables

The PHP container receives the following environment variables:

```ini
DB_CONNECTION=mysql
DB_HOST=sld-{project-id}-db
DB_PORT=3306
DB_DATABASE=app
DB_USERNAME=sld
DB_PASSWORD=<randomly-generated-stored-in-secrets-json>
```

### Redis Service
- When `services.redis.enabled: true`:
  - Spins up container `sld-{project-id}-redis` (image `redis:7-alpine`).
  - Attached ONLY to `sld-{project-id}-net`.
  - Injects `REDIS_HOST=sld-{project-id}-redis` and `REDIS_PORT=6379` into PHP.

### Non-Destructive Storage Policy
- Running `sld stop`, `sld restart`, or `sld destroy` leaves database volumes completely intact.
- Volumes are ONLY removed if the developer explicitly executes:
  `sld destroy --volumes` with an interactive confirmation prompt.

---

## 14. Node Runtime

### Two Execution Models

#### 1. One-Shot Commands (`sld npm install`, `sld npm run build`)
- **Behavior**: Ephemeral container.
- **Execution**:
  ```bash
  docker run --rm -it \
    -u "$(id -u):$(id -g)" \
    -v "$(pwd)":/app \
    -w /app \
    node:20-bookworm-slim \
    npm "$@"
  ```
- **Lifecycle**: Mounts project, executes command, cleans up immediately upon exit.

#### 2. Long-Running Managed Dev Server (`sld npm run dev` / Vite HMR)
- **Problem**: Vite dev servers run continuously, watch files, and serve hot module replacement (HMR) over WebSockets. They must NOT be executed as ephemeral one-shot commands.
- **Solution**: Managed Node Service.
  - The reconciler creates a persistent container: `sld-{project-id}-node`.
  - Joins `sld-{project-id}-net` and publishes an allocated host port (default: 5173).
  - Note on Subdomains: RFC 6125 wildcard certificates (`*.test`) do NOT cover nested subdomains (`vite.app.test`). For MVP, Vite dev server traffic routes directly through an exposed host port rather than Traefik.
  - Mounts project root into `/app`.
  - Container runs `npm run dev` with restart policy `unless-stopped`.
  - Managed by `sld start`, `sld stop`, and `sld logs node`.

---

## 15. Reconciliation Engine

### Declarative Reconciliation Algorithm

```
.sld/config.yaml
       │
       ▼
 [Config Loader]  ── Validates schema, resolves PHP & project identity
       │
       ▼
 [Desired State]  ── Struct defining required Networks, Volumes, Containers, Labels
       │
       ▼
 [Actual State]   ── Query Docker Engine API for label: com.supreme.sld.project={id}
       │
       ▼
   [Diffing]      ── Computes resource delta (Create, Recreate, Start, Stop, Remove, NoOp)
       │
       ▼
 [Apply Actions]  ── Execute Docker API calls in strict dependency order:
                     1. Create project network (if missing)
                     2. Create database/redis volumes (if missing)
                     3. Reconcile auxiliary services (DB, Redis)
                     4. Reconcile PHP-FPM container
                     5. Reconcile Nginx container (attach both networks)
                     6. Reconcile Node container (if enabled)
       │
       ▼
[Health Check]    ── Poll container health checks with exponential backoff (max 30s)
       │
       ▼
   Complete
```

### Reconciliation State Matrix

| State Trigger | Condition | Reconciliation Action |
|---|---|---|
| **Missing Resource** | Container or network does not exist in Docker. | Create and start resource. |
| **Matching Hash & Running** | Container exists, is running, and `com.supreme.sld.config-hash` matches. | **NoOp** (Do nothing; instant completion). |
| **Matching Hash & Stopped** | Container exists, stopped, and config-hash matches. | Call `containerStart` (no recreate needed). |
| **Changed Configuration** | Container exists, but `config-hash` differs (e.g. PHP version changed). | Gracefully stop old container, remove container, create new container, start. |
| **Obsolete Service** | Redis was enabled in config, now disabled. | Stop and remove `sld-{project-id}-redis` container. Preserve volume. |
| **Missing Global Network** | `sld-global` network absent. | Lazily create `sld-global` bridge network. |
| **Router Missing** | `sld-router` container stopped or absent. | Trigger global router reconciliation. |
| **Health Check Failure** | Container fails health check within timeout. | Emit actionable error; stream container stderr; do NOT tear down healthy siblings. |

---

## 16. CLI Architecture

### Command Tree & Implementation Strategy

```
sld
├── init                     # Start global Traefik router, create sld-global, setup DNS
├── doctor                   # Run system diagnostics (Docker, ports, DNS, mkcert)
├── update                   # Pull latest official SLD Docker images
├── start                    # Declaratively reconcile and start project containers
├── stop                     # Stop all project containers (preserves state)
├── restart                  # Restart project containers
├── status                   # Display tabular status of current project (or --all)
├── logs [service]           # Stream logs from container (default: php)
├── inspect                  # Output detailed JSON representation of Docker state
├── destroy [--volumes]      # Tear down project containers/networks (volumes optional)
├── open                     # Open project domain in default browser
├── secure                   # Generate wildcard certificates via mkcert
├── config
│   ├── validate             # Validate .sld/config.yaml syntax and constraints
│   └── show                 # Print resolved configuration with applied defaults
├── composer [args...]       # Run Composer in project PHP container
├── artisan [args...]        # Run Laravel Artisan in project PHP container
├── php [args...]            # Run PHP CLI in project PHP container
├── npm [args...]            # Run npm in ephemeral or managed Node container
├── exec [service] [cmd...]  # Execute command in specified container
├── shell [service]          # Open interactive bash/sh shell in container
└── legacy                   # V1 Migration Toolset
    ├── status               # Inspect host for V1 installations, DBs, and configs
    ├── migrate [--dry-run]  # Migrate host V1 MySQL databases to Docker volumes
    └── remove               # Stop and purge V1 systemd daemon and host configs
```

### V1 Command Disposition

| V1 Command | Action | Rationale |
|---|---|---|
| `sld install` | **REMOVED** | Replaced by `sld init`. SLD never installs host packages. |
| `sld uninstall` | **REMOVED** | Replaced by `sld legacy remove`. |
| `sld daemon` | **REMOVED** | Background host daemon is permanently eliminated. |
| `sld service *` | **REMOVED** | Systemd unit management is obsolete. |
| `sld park` / `sld unpark` | **DEPRECATED (MVP)** | V2 projects self-contain identity; `sld start` registers routing. |
| `sld link` / `sld unlink` | **DEPRECATED (MVP)** | Configured declaratively via `domain.name` in `.sld/config.yaml`. |
| `sld sites` / `sld paths` | **REPLACED** | Replaced by `sld status --all` querying Docker labels. |
| `sld refresh` | **REMOVED** | Declarative reconciler automatically handles updates on `sld start`. |
| `sld share` | **DEFERRED** | Cloudflare tunnel integration deferred to post-V2 MVP. |
| `sld db *` | **DEFERRED** | Database management commands deferred to post-V2 MVP. |

---

## 17. Security Model

1. **Privilege Demotion**:
   - Zero root/sudo privileges required for normal development commands (`sld start`, `sld stop`, `sld composer`, `sld artisan`, `sld logs`).
   - Sudo is strictly restricted to one-time `sld init` for `/etc/resolver/test` or systemd-resolved DNS setup. Every sudo invocation requires explicit user confirmation and prints the exact command being executed.
2. **Docker Socket Protection & Trust Boundary**:
   - The Docker socket (`/var/run/docker.sock`) is accessed by the `sld` CLI on the host.
   - `sld-router` (Traefik) mounts the Docker socket **strictly read-only** (`:ro`).
   - *Security Reality*: A read-only socket mount is not a least-privilege boundary, as it still allows reading container inspect metadata and secrets. For MVP, Docker socket access is trusted by the SLD runtime. In hardened V2.x, an API-filtering Docker socket proxy will be introduced.
   - **ZERO project containers** (`sld-php`, `sld-nginx`, `sld-db`, `sld-redis`, `sld-node`) ever receive the Docker socket under any circumstance. Application vulnerabilities cannot escalate to host container control.
3. **UID / GID Parity**:
   - Containers run processes using host user UID/GID via dynamic user switching in entrypoint scripts. This eliminates file permission mismatches on Linux/WSL2 hosts.
4. **Internal Network Boundary**:
   - Databases and key-value stores do not bind to `0.0.0.0` on the host, preventing accidental public exposure on local Wi-Fi networks.

---

## 18. V1 Migration

### Migration Philosophy
V1 migration is an **explicit, user-initiated operation**. V2 will NEVER automatically touch, alter, or delete existing host databases, host Nginx sites, or systemd services without explicit invocation of `sld legacy` commands.

### Migration Commands
1. **`sld legacy status`**:
   - Detects existence of `/var/lib/sld/state.json`.
   - Checks status of `/etc/systemd/system/sld-daemon.service`.
   - Checks if host Nginx, PHP-FPM, or MySQL services are active.
   - Lists all parked directories, site configs, and detected databases.
2. **`sld legacy migrate [--dry-run]`**:
   - Defaults to `--dry-run`, presenting a full preview of actions.
   - When executed with `--execute`:
     - Reads each parked project.
     - Creates `.sld/config.yaml` with detected PHP version and web root.
     - For databases: executes `mysqldump` against host MySQL (`127.0.0.1:3306`), creates target V2 project database volume, and streams SQL dump into the containerized database.
3. **`sld legacy remove`**:
   - Prompts for confirmation.
   - Stops and disables `sld-daemon.service`.
   - Removes `/etc/systemd/system/sld-daemon.service`.
   - Archives `/var/lib/sld/state.json` to `~/.sld/backup/state.v1.json`.
   - Prints manual commands if user wishes to uninstall host packages (`apt remove nginx php-fpm`). Does not execute package managers automatically.

---

## 19. GUI / Dashboard

### Architecture Evaluation
- **V1 Reality**: `sld-dashboard` is a Tauri application that communicates with an unauthenticated HTTP server on `localhost:2025` (`pkg/daemon/api/server.go`).
- **Conflict**: Running an unauthenticated HTTP server on the host violates the core V2 security and daemonless principles.
- **Architectural Decision**: **DEPRECATE for V2.0 MVP; ADAPT in V2.1**.

### V2.1 Adaptation Blueprint
The GUI will be decoupled from any background HTTP daemon:
1. When launched, the Tauri desktop application uses native Tauri Rust IPC commands.
2. Rust commands execute the `sld` CLI in non-interactive JSON mode (`sld status --json`, `sld start --json`) or directly invoke the shared Go engine via compiled C-archive (`c-shared`).
3. No daemon is required to run in the background when the GUI is closed.
4. In V2.0 MVP, the CLI is the primary user interface. `sld-dashboard/` remains quarantined in the repository until Phase 15.

---

## 20. Testing Strategy

### Testing Pyramid

```
                ▲
               ╱ ╲
              ╱ E2E ╲            (Full Docker + Laravel + MySQL + Curl)
             ╱───────╲
            ╱   CLI   ╲          (Cobra command executions, output formatting)
           ╱───────────╲
          ╱ Reconciler  ╲        (Desired vs Actual state mutation in Docker)
         ╱───────────────╲
        ╱   Integration   ╲      (Docker SDK client, network/container CRUD)
       ╱───────────────────╲
      ╱     Unit Tests      ╲    (Config schema, project ID, diff computation)
     ─────────────────────────
```

### Test Coverage Requirements

1. **Unit Tests (Fast, zero dependencies)**:
   - `internal/project/identity_test.go`: Determinism, collision resistance, path sanitization.
   - `internal/config/loader_test.go`: Schema parsing, default values, validation rules.
   - `internal/project/detect_test.go`: Composer constraint resolution (`^8.2`, `>=8.3`, unsupported `^7.4`).
   - `internal/reconcile/diff_test.go`: State diffing logic under all permutation of Desired vs Actual.
2. **Integration Tests (Requires Docker Engine)**:
   - Resource isolation: Create containers with test prefix `sld-test-*`; verify labels and networks.
   - Container CRUD & health checks: Spin up Nginx and PHP; verify health check transitions.
   - Idempotency test: Run reconciliation twice with identical config; assert zero containers recreated.
   - Recreate test: Change config hash; assert container is recreated seamlessly.
3. **End-to-End Tests (GitHub Actions CI)**:
   - Complete lifecycle:
     1. `sld init`
     2. Create temporary Laravel project directory.
     3. Generate `.sld/config.yaml`.
     4. `sld start`
     5. Await health verification.
     6. `curl -sf -H "Host: my-app.test" http://127.0.0.1:80` → Expect HTTP 200.
     7. `sld artisan --version` → Verify execution inside PHP container.
     8. `sld stop` → Verify containers stop.
     9. `sld destroy --volumes` → Verify clean teardown with zero dangling resources.

---

## 21. Build and Release

### Release Matrix
- **CLI Binaries**:
  - `linux/amd64` (Standard Linux)
  - `linux/arm64` (ARM / Raspberry Pi / Cloud)
  - `darwin/amd64` (Intel Mac)
  - `darwin/arm64` (Apple Silicon Mac)
- **Windows Policy**: Windows developers execute the native Linux binary inside WSL2. Native Windows binary (`sld-win.exe`) is deprecated.
- **Docker Images (GHCR)**:
  - `ghcr.io/supreme-majesty/sld-router:2`
  - `ghcr.io/supreme-majesty/sld-nginx:1`
  - `ghcr.io/supreme-majesty/sld-php:8.1`
  - `ghcr.io/supreme-majesty/sld-php:8.2`
  - `ghcr.io/supreme-majesty/sld-php:8.3`
  - `ghcr.io/supreme-majesty/sld-php:8.4`
- **Image Build Pipeline**:
  - GitHub Actions with QEMU & Docker Buildx for native multi-arch builds (`linux/amd64`, `linux/arm64`).
  - Vulnerability scanning via Trivy.
  - Software Bill of Materials (SBOM) generated via Syft.
  - Cosign signatures for image provenance.

---

## 22. Implementation Phases

```
Phase 0 ──► Phase 1 ──► Phase 2 ──► Phase 3 ──► Phase 4 ──► Phase 5 ──► Phase 6
  Docs        Docker      Config      Identity    Reconciler   Nginx/PHP   Traefik
 Freeze        SDK        Schema       Engine       Core       Runtime     Gateway
   │
   ▼
Phase 7 ──► Phase 8 ──► Phase 9 ──► Phase 10 ──► Phase 11 ──► Phase 12 ──► Phase 13
  DNS         TLS          DB &      Node.js      Developer    Doctor &     Legacy
Resolver     mkcert      Services    Runtime      Commands      Status     Migration
   │
   ▼
Phase 14 ──► Phase 15 ──► Phase 16
  E2E         Legacy       Build &
Testing       Purge        Release
```

### Phase Schedule

- **Phase 0: Architecture & Documentation Freeze**
  - Lock specifications in `docs/architecture/SLD_V2_IMPLEMENTATION_BLUEPRINT.md`.
- **Phase 1: Go Docker Abstraction & Client Wrapper**
  - Add Docker Go SDK to `go.mod`. Build `internal/docker` package (containers, networks, volumes, labels, queries).
- **Phase 2: Configuration Schema & Validation**
  - Implement `internal/config` and `internal/project/detect.go`. Add `sld config validate` and `sld config show`.
- **Phase 3: Project Identity & Deterministic Naming**
  - Implement canonical path hashing and slug generation in `internal/project/identity.go`.
- **Phase 4: Declarative Reconciliation Engine**
  - Implement Desired State, Actual State query, Diff computation, and Apply pipeline in `internal/reconcile`.
- **Phase 5: Nginx + PHP-FPM Runtime**
  - Build `docker/nginx` and `docker/php` Dockerfiles. Implement container spec builders in `internal/runtime`.
- **Phase 6: Traefik Router & Global Gateway**
  - Build `docker/router` image. Implement `sld init` and Traefik label generation in `internal/router`.
- **Phase 7: DNS Architecture & Platform Integration**
  - Implement macOS resolver, Linux systemd-resolved integration, and WSL2 hosts helper in `internal/dns`.
- **Phase 8: TLS Certificate Automation**
  - Implement `mkcert` wrapper and Traefik dynamic TLS configuration in `internal/tls`. Implement `sld secure`.
- **Phase 9: Database, Redis & Auxiliary Services**
  - Implement MySQL, Postgres, MariaDB, and Redis container/volume management in `internal/runtime/services.go`.
- **Phase 10: Node.js Runtime**
  - Implement ephemeral runner (`sld npm`) and managed long-running dev server container in `internal/runtime/node.go`.
- **Phase 11: Developer CLI Tooling**
  - Implement `sld composer`, `sld artisan`, `sld php`, `sld shell`, `sld exec` via Docker exec API.
- **Phase 12: Diagnostics, Status & Observability**
  - Implement `sld doctor`, `sld status`, `sld logs`, and `sld inspect`.
- **Phase 13: V1 Legacy Migration Toolset**
  - Implement `sld legacy status`, `sld legacy migrate`, and `sld legacy remove`.
- **Phase 14: End-to-End Testing & CI Automation**
  - Implement full E2E test suite running in GitHub Actions against real Docker Engine.
- **Phase 15: V1 Legacy Code Removal & Package Cleanup**
  - Safely delete `pkg/adapters`, `pkg/daemon`, and obsolete V1 templates.
- **Phase 16: Multi-Arch Build, Packaging & Distribution**
  - Configure GitHub Actions Buildx for multi-arch Docker image pushes and CLI release binaries.

---

## 23. Open Decisions

1. **WSL2 Port 80 Ingress**: On Windows 11 WSL2 with "Mirrored" networking, host port 80 maps directly to WSL2 localhost. On older WSL2 NAT networking, Windows host port forwarding requires a PowerShell portproxy rule. Recommendation: Detect WSL2 networking mode during `sld doctor` and output exact configuration instructions.
2. **Database Credentials Storage for Team Repositories**: Should `.sld/.env` be standardized as the default local secret store, or should credentials strictly live inside the framework's `.env`? Recommendation: Framework `.env` remains the standard for application secrets; `.sld/.secrets` (gitignored) stores SLD-generated database root credentials.
3. **Vite Port Collisions**: When two projects run `sld npm run dev` simultaneously with host port `5173`. Recommendation: Detect host port availability; if 5173 is occupied, allocate next sequential free port (e.g. 5174) and update project environment dynamically.

---

## 24. Risks and Mitigations

| Risk | Impact | Mitigation Strategy |
|---|---|---|
| **Host Port 80/443 Collision** | Traefik fails to start if host Apache/Nginx or V1 Nginx is running. | `sld doctor` and `sld init` explicitly check port availability before starting Traefik, identifying the conflicting process PID. |
| **Systemd-resolved Port 53 Collision** | Linux DNS binding fails. | Avoid binding port 53 directly. Configure systemd-resolved domain routing (`Domains=~test`) to route queries to `127.0.0.1:80`. |
| **Docker Permission Denied** | Developer cannot execute `sld` commands without sudo. | `sld doctor` checks Docker socket permissions and outputs exact command: `sudo usermod -aG docker $USER`. |
| **Bind Mount Performance (macOS)** | Slow PHP file execution on large codebases. | Ensure VirtioFS is enabled in Docker Desktop/OrbStack. Document optimal mount flags. |
| **Data Loss during V1 Migration** | Host databases overwritten or lost. | `sld legacy migrate` enforces `--dry-run` by default and creates full SQL backup files in `~/.sld/backup/` before importing. |

---

## 25. Acceptance Criteria

SLD V2 shall be considered production-ready when:
1. `sld start` in a PHP/Laravel directory idempotently brings up Nginx, PHP-FPM, and Database containers without error.
2. Visiting `http://<project>.test` in a browser successfully renders the Laravel welcome page via Traefik → Nginx → PHP-FPM FastCGI.
3. Static files are served directly by Nginx without invoking PHP-FPM.
4. `sld secure` generates a trusted wildcard certificate, and `https://<project>.test` displays a green padlock in Google Chrome/Firefox.
5. `sld composer install` and `sld artisan migrate` execute cleanly within the PHP container and modify host files with correct non-root permissions.
6. Auxiliary database and Redis services are isolated to the project network and unreachable from other projects.
7. Database data persists across `sld stop`, `sld start`, and system reboots.
8. The entire automated test suite (Unit, Integration, E2E) passes in GitHub Actions CI with zero host dependency mutations.
9. All V1 host mutation adapters (`pkg/adapters`) and the V1 daemon HTTP server (`pkg/daemon/api`) have been purged from the repository.
