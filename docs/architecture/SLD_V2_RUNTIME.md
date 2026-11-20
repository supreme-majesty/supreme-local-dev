# SLD V2 Runtime & Execution Contracts

> **See also** [ADR 0007](../adr/0007-multi-runtime-drivers.md): besides PHP, projects can use node, python, ruby, go, static, image, dockerfile, proxy and compose runtimes. The PHP contracts below still apply.

This document specifies the container runtime contracts, FastCGI integration, explicit health-check verification, Node execution modes, and permission handling for SLD V2.

---

## 1. Web Runtime Contract: Traefik → Nginx → PHP-FPM

```
Browser  ──HTTP:80/443──>  Traefik  ──HTTP:80──>  Nginx  ──FastCGI TCP:9000──>  PHP-FPM
```

### Critical Invariant: PHP-FPM Speaks FastCGI, Not HTTP
Traefik is an HTTP/TCP/UDP reverse proxy. PHP-FPM is a FastCGI daemon. Traefik MUST NOT route HTTP traffic directly to PHP-FPM.

### Role of Project Nginx (`sld-nginx:1`)
Each project receives an isolated Nginx container attached to both `sld-global` and `sld-{project-id}-net`:
1. **Static Asset Offloading**: Requests for static files (`.css`, `.js`, `.png`, `.jpg`, `.svg`, `.ico`) are served directly by Nginx from `/app/public` with optimal caching headers. PHP-FPM is not invoked for static assets.
2. **Front-Controller Fallback**: Requests for dynamic routes are evaluated via:
   ```nginx
   try_files $uri $uri/ /index.php?$query_string;
   ```
3. **FastCGI Proxying**: Requests matching `.php` are forwarded over internal TCP (`sld-{project-id}-php:9000`) using standard FastCGI parameters (`SCRIPT_FILENAME`, `DOCUMENT_ROOT`).

---

## 2. Explicit Health-Check Contracts

Health checks must only rely on tools and utilities that **actually exist** within the container images. Adding arbitrary packages or making false assumptions about minimal images is prohibited.

| Service | Image | Health Check Command | Why This Strategy Was Chosen |
|---|---|---|---|
| **Nginx** | `sld-nginx:1` (Alpine or Debian-slim) | `wget -q -O - http://127.0.0.1:80/sld-health \|\| exit 1` | `wget` is pre-installed in BusyBox/Alpine and standard debian-slim without installing heavy `curl`. Tests the actual HTTP listening socket. |
| **PHP-FPM** | `sld-php:8.1..8.4` | Custom embedded script `/usr/local/bin/sld-php-healthcheck` | `php-fpm -t` only validates configuration syntax and passes even if PHP-FPM is deadlocked or unready. The healthcheck script opens a real FastCGI ping transaction to `127.0.0.1:9000` via a minimal pre-baked utility. |
| **MySQL / MariaDB** | `mysql:8.0`, `mariadb:10.11` | `mysqladmin ping -h 127.0.0.1 --silent \|\| exit 1` | `mysqladmin` is officially bundled in all MySQL/MariaDB Docker distribution images. |
| **PostgreSQL** | `postgres:16-alpine` | `pg_isready -h 127.0.0.1 -U $$POSTGRES_USER \|\| exit 1` | `pg_isready` is the official, lightweight connection-readiness utility provided by PostgreSQL. |
| **Redis** | `redis:7-alpine` | `redis-cli ping \|\| exit 1` | `redis-cli` is pre-installed in the official Redis Alpine image. |

---

## 3. Node Execution Model

Node.js is intentionally **excluded** from `sld-php` images to keep PHP images lightweight and decoupled. Node tasks operate under two distinct lifecycle contracts:

### Contract 1: One-Shot Ephemeral Tasks
- **Commands**: `sld npm install`, `sld npm run build`, `sld npm run lint`, `sld npx <tool>`.
- **Lifecycle**:
  - Container spins up with project mounted at `/app`.
  - Runs with matching host user UID/GID (`-u $(id -u):$(id -g)`).
  - Executes requested command.
  - Container is destroyed immediately upon exit (`--rm`).

### Contract 2: Managed Persistent Dev Server (Vite / Next.js)
- **Commands**: `sld npm run dev`.
- **Requirements**: Vite requires continuous execution, file watching, and WebSocket connections for Hot Module Replacement (HMR).
- **Lifecycle**:
  - Configured in `.sld/config.yaml`:
    ```yaml
    node:
      version: "20"
      dev_server:
        enabled: true
        command: "npm run dev"
        port: 5173
        expose_port: 5173
    ```
  - Reconciler creates a persistent container: `sld-{project-id}-node`.
  - Joins `sld-{project-id}-net` with restart policy `unless-stopped`.
  - **Port Allocation & Conflict Detection**:
    1. Reconciler checks if requested host port `5173` is available.
    2. If `5173` is occupied, it dynamically allocates the next available port (e.g. `5174`).
    3. Outputs the active Vite URL to the developer console.
- **TLS Limitation (Wildcard Subdomains)**:
  Single-level wildcard certificates (`*.test`) cover `my-project.test`, but **do not** cover nested subdomains such as `vite.my-project.test` (per RFC 6125). Therefore, for MVP, the Vite dev server exposes a host port directly rather than routing through Traefik with an invalid multi-level SSL certificate.

---

## 4. Filesystem Ownership & UID/GID Parity

### Principle: Zero Root Hijacking
When containers execute build commands, any file created inside the bind mount must be owned by the host developer, not `root`.

### Implementation Contract
1. The `sld` CLI passes the host user's UID and GID to container creation:
   ```ini
   WWWUSER={host-uid}
   WWWGROUP={host-gid}
   ```
2. The PHP container entrypoint maps the internal `www-data` user to match `{host-uid}` and `{host-gid}` before launching PHP-FPM.
3. The entrypoint strictly modifies only internal directories (`/var/log`, `/var/run`).
4. **The container MUST NOT execute `chown -R` across `/app` on startup.** Recursive chown operations on large project trees cause catastrophic startup latency, particularly on macOS VirtioFS and Windows WSL2 bind mounts.
