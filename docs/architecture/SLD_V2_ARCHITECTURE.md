# SLD V2 Architecture

## 1. Product Definition

**Supreme Local Dev (SLD) is a high-performance, Docker-backed local development orchestrator that bridges the seamless speed of Laravel Herd with the reproducible isolation of DDEV.**

- **Target Users:** Professional web developers working primarily with PHP (Laravel, Symfony, WordPress) and Node.js (Vite, React) who need isolated environments without writing complex `docker-compose.yml` files.
- **Supported OS:** Linux (Tier 1), macOS (Tier 2), Windows via WSL2 (Tier 3).
- **Docker Requirements:** Docker Engine (Linux/WSL2) or Docker Desktop/OrbStack (macOS).
- **Supported PHP Versions:** 8.1, 8.2, 8.3, 8.4 via official lightweight SLD images.
- **What SLD Manages:** Docker container lifecycle, dynamic `.test` HTTP/HTTPS routing, local SSL generation (mkcert), isolated database/Redis provisioning, and seamless CLI passthrough (composer, artisan, npm).
- **What SLD Intentionally Does NOT Manage:** Host-level package managers (`apt`, `brew`, `winget`), global host web servers, system PHP-FPM installations, host database daemons, or production deployments.

---

## 2. Architecture Principles

1. **Host Purity & Zero Host Mutation:** No runtime dependencies (Nginx, PHP, databases, Redis) are installed on the host. SLD never executes host package managers.
2. **Go CLI as the Control Plane:** The CLI uses the Docker Engine API directly for deterministic state control rather than generating and shelling out to `docker-compose`.
3. **No Elevated Daemon:** The V1 background daemon (port 2025 HTTP API) and systemd service are eliminated. The CLI runs in user-space. Docker Engine is the sole runtime engine.
4. **Declarative State:** Projects explicitly define their requirements in `.sld/config.yaml`. Docker Engine state is the sole runtime source of truth, discovered via standardized Docker labels.
5. **FastCGI Separation:** Traefik terminates HTTP/HTTPS and proxies to project Nginx; Traefik MUST NOT route HTTP directly to PHP-FPM. PHP-FPM speaks FastCGI. Project Nginx serves static assets directly and proxies PHP requests over FastCGI.
6. **Deterministic Isolation:** Every project runs in its own isolated Docker bridge network. Auxiliary services (databases, Redis) are private to the project network and do not publish host ports by default.

---

## 3. Component Architecture

```mermaid
graph TD
    subgraph Host OS
        CLI[SLD Go CLI]
        Browser[Web Browser]
        MK[mkcert]
        Files[Project Source Code]
        Secrets[~/.sld/ or .sld/secrets]
    end

    subgraph Docker Engine
        subgraph sld-global Network
            Router[sld-router: Traefik v3.1]
        end
        
        subgraph Project: App A (sld-app-a-3f7b49a1-net)
            NginxA[sld-app-a-3f7b49a1-nginx]
            PHPA[sld-app-a-3f7b49a1-php: PHP-FPM 8.3]
            DBA[(sld-app-a-3f7b49a1-db: MySQL 8.0)]
            RedisA[(sld-app-a-3f7b49a1-redis: Redis 7)]
        end
        
        subgraph Project: App B (sld-app-b-e81a029c-net)
            NginxB[sld-app-b-e81a029c-nginx]
            PHPB[sld-app-b-e81a029c-php: PHP-FPM 8.4]
        end
    end

    Browser -->|*.test HTTP/S| Router
    Router -->|HTTP :80| NginxA
    Router -->|HTTP :80| NginxB
    NginxA -->|FastCGI :9000| PHPA
    NginxB -->|FastCGI :9000| PHPB
    PHPA --> DBA
    PHPA --> RedisA
    Files <-->|Bind Mount /app| NginxA
    Files <-->|Bind Mount /app| PHPA
    CLI -->|Docker Engine API| Router
    CLI -->|Docker Engine API| NginxA
    CLI -->|Docker Engine API| PHPA
```

### Request Flow Path
1. **DNS Resolution**: `my-app.test` resolves to `127.0.0.1`.
2. **Gateway**: Request hits host port 80/443, received by `sld-router` (Traefik) on `sld-global`.
3. **TLS Termination**: Traefik terminates TLS using wildcard certificate (`*.test`) mounted from `~/.sld/certs/`.
4. **Project Web Server**: Traefik routes HTTP request to `sld-{id}-nginx` over `sld-global`.
5. **Static vs Dynamic**:
   - Static files (`.css`, `.js`, images): Nginx delivers directly from `/app/public` without waking PHP.
   - Dynamic / PHP requests: Nginx proxies over FastCGI (TCP port 9000) to `sld-{id}-php` on `sld-{id}-net`.
6. **Backend Execution**: PHP-FPM processes application code, communicates with internal DB/Redis, and returns response via Nginx and Traefik to the browser.

---

## 4. Docker Architecture & Resource Model

### Engine API
SLD communicates directly with the Docker socket via the official Docker Go SDK (`github.com/docker/docker/client`).

### Networks
- `sld-global`: Global Docker bridge network for Traefik ingress to project Nginx containers.
- `sld-{project-id}-net`: Private per-project bridge network for internal communication between Nginx, PHP-FPM, Database, and Redis.

### Resource Naming Standards
- Project Network: `sld-{project-id}-net`
- Nginx Container: `sld-{project-id}-nginx`
- PHP Container: `sld-{project-id}-php`
- Database Container: `sld-{project-id}-db`
- Redis Container: `sld-{project-id}-redis`
- Node Container: `sld-{project-id}-node`
- Database Volume: `sld-{project-id}-db-data`

### Label Strategy
Standardized labels distinguish metadata, services, and resource types:
```ini
com.supreme.sld=true
com.supreme.sld.version=2
com.supreme.sld.project=<project-id>
com.supreme.sld.project-path=<canonical-absolute-path>
com.supreme.sld.managed-by=sld
com.supreme.sld.config-hash=<sha256>
com.supreme.sld.service=<nginx|php|db|redis|node|router>
com.supreme.sld.resource-type=<container|network|volume>
```
*Note*: `service` identifies the functional service; `resource-type` describes the Docker entity type. Networks and volumes are never labeled as services.

---

## 5. Router Architecture (Traefik)

- **Selected Router:** Traefik v3.1 (`ghcr.io/supreme-majesty/sld-router:2`).
- **Dynamic Configuration:** Traefik discovers project Nginx containers dynamically via Docker provider labels on the `sld-global` network.
- **Docker Socket Security:** Traefik mounts `/var/run/docker.sock:ro` (read-only). Project containers NEVER mount the Docker socket.
- **HTTPS Termination:** Traefik uses a dynamic file provider pointing to `~/.sld/certs/` for wildcard TLS termination.

---

## 6. PHP & Nginx Runtime Architecture

- **Nginx (`sld-nginx:1`):** Lightweight web server dual-homed on `sld-global` and `sld-{id}-net`. Contains `/sld-health` health endpoint, static asset delivery, and FastCGI proxy rules.
- **PHP-FPM (`sld-php:8.1..8.4`):** Dedicated per-project container on `sld-{id}-net`.
  - Extensions pre-installed: `pdo_mysql`, `pdo_pgsql`, `redis`, `gd`, `zip`, `bcmath`, `intl`, `mbstring`, `opcache`, `pcntl`, `exif`, `soap`, `sockets`.
  - Composer bundled globally.
  - Xdebug pre-installed, toggled via `xdebug.enabled: true` / `XDEBUG_MODE=debug`.
  - Node.js is strictly excluded.
  - User permissions: Mapped host UID/GID via `WWWUSER`/`WWWGROUP` env vars; no full-tree `chown -R` on startup.

---

## 7. Node Execution Model

- **One-Shot Commands (`sld npm install`, `sld npm run build`):** Executed in ephemeral containers (`--rm`) with matching host UID/GID.
- **Persistent Dev Server (`sld npm run dev` / Vite):** Executed in a managed, persistent container (`sld-{id}-node`) with automatic host port conflict detection and allocation. Nested subdomains (`vite.app.test`) are avoided in MVP because `*.test` wildcard certs do not cover multi-level subdomains.

---

## 8. Database & Secret Management

- **Lifecycle:** Created on `sld start` when `services.database` is defined. Attached only to `sld-{id}-net`.
- **Volumes:** Backed by persistent named Docker volume `sld-{id}-db-data`. Survives `sld stop`, `sld restart`, and normal `sld destroy`.
- **Secrets:** Passwords are generated using cryptographically random bytes upon initialization, saved to local gitignored storage (`.sld/secrets.json` or `.sld/.env`), and injected into container environments. Deterministic or hardcoded credentials (such as `sld_secret`) are strictly forbidden.
- **Destroy Semantics:** `sld destroy` leaves data volumes intact. `sld destroy --volumes` requires explicit interactive confirmation and clearly warns that database data will be lost.

---

## 9. Platform DNS Strategy

- **Separation of Concerns:** DNS answers "What IP does `*.test` resolve to?" (`127.0.0.1`). Traefik answers "Which container receives HTTP for `my-app.test`?".
- **Platform Implementations:**
  - **macOS:** `/etc/resolver/test` pointing to `127.0.0.1`.
  - **Linux:** `/etc/hosts` synchronization or systemd-resolved domain routing (`Domains=~test`) only when an active DNS resolver is present on port 53. DNS must never point to a nonexistent port 53 resolver.
  - **Windows WSL2:** Synchronize Windows hosts file for active `.test` domains.
