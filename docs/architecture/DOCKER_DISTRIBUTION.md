# SLD V2 Docker Distribution Architecture

This document defines the Docker distribution and execution architecture for Supreme Local Dev (SLD) v2, transitioning it from a host-mutating daemon into a clean, reusable Docker-backed orchestrator.

---

## 1. Target Docker Architecture

**Primary Distribution Mechanism: Hybrid Host CLI + Docker Runtime**

We reject full Docker Compose dependency (like Laravel Sail) because it clutters every project repository with verbose YAML. We reject full host installation (like Laravel Herd/Valet) because it pollutes the host system and creates cross-project dependency conflicts.

The target architecture uses a **Go CLI on the host** to dynamically orchestrate **Docker Engine** containers via the Docker SDK.

1. **Global Gateway:** A background `sld-router` (Traefik) container runs constantly to handle `*.test` local DNS routing and SSL termination.
2. **Project Isolation:** When a developer runs `sld start`, the CLI uses the Docker API to spin up isolated `sld-php` containers and auxiliary services specific to that project.
3. **No Heavy Configs:** Developers do not write `docker-compose.yml`. They define high-level needs in a simple `.sld/config.yaml`.

---

## 2. Image Strategy

SLD provides official images hosted on GitHub Container Registry (GHCR).

### The Router Image
* `ghcr.io/supreme-majesty/sld-router:latest`
* **Base:** `traefik:v3.0`
* **Purpose:** Reads Docker labels to dynamically route traffic from `*.test` domains to the correct project container. Handles `mkcert` wildcard SSL termination.

### The PHP Runtime Images
* `ghcr.io/supreme-majesty/sld-php:8.1`
* `ghcr.io/supreme-majesty/sld-php:8.2`
* `ghcr.io/supreme-majesty/sld-php:8.3`
* `ghcr.io/supreme-majesty/sld-php:8.4`

**Specifications:**
* **Base:** Debian (Bullseye/Bookworm) `php:*-fpm` variants for maximum glibc extension compatibility.
* **Architectures:** Multi-arch builds natively supporting `linux/amd64` and `linux/arm64` (Apple Silicon).
* **Extensions:** Pre-compiled with common development requirements: `pdo_mysql`, `pdo_pgsql`, `redis`, `gd`, `zip`, `bcmath`, `intl`, `mbstring`, `opcache`.
* **Tools Included:** Composer (latest), Xdebug (disabled by default, toggleable via environment variable).
* **Tools Excluded:** Node.js. Including Node bloats the PHP image. Node tasks will be executed via ephemeral Node containers.
* **User Permissions:** Images will map a configurable `WWWUSER` and `WWWGROUP` (defaulting to the host's UID/GID) to prevent Docker-created files from becoming root-owned on Linux hosts.
* **Image Size:** Goal < 350MB compressed.

---

## 3. Compose Strategy

While SLD natively uses the Docker API instead of shelling out to `docker-compose`, it respects the Compose topology mentally:

**SLD Stack (Global):**
These belong to SLD itself and run once per machine.
* `sld-router` (Traefik)
* `sld-dns` (dnsmasq, port 53)

**Application Stack (Per Project):**
These belong to the specific application and are created dynamically by SLD.
* PHP-FPM / Web Server
* MySQL/Postgres
* Redis
* Mailpit

**Advanced Opt-out (Native Compose Integration):**
If a developer requires highly custom infrastructure, SLD allows them to place a standard `docker-compose.yml` in their project instead of `.sld/config.yaml`. SLD detects this, shells out to `docker compose up -d`, and ensures the project containers are attached to the `sld-global` network for Traefik routing.

---

## 4. Project Configuration Specification

Projects define their environment in a concise, version-controllable `.sld/config.yaml`.

```yaml
# .sld/config.yaml
name: "my-project"
domain: "my-project.test" # Optional, defaults to directory name

runtime:
  php: "8.3"
  web_root: "public"
  xdebug: false

services:
  database:
    type: "mysql" # mysql, postgres, mariadb
    version: "8.0"
  redis: true
  mailpit: true
```

If this file is missing, `sld start` automatically detects the framework (e.g., Laravel), infers the PHP version from `composer.json`, and generates it.

---

## 5. CLI Specification

The Go CLI is the sole interaction point.

**Global Commands:**
* `sld init` - Pulls the router image, creates the `sld-global` network, sets up `mkcert`, and starts the router.
* `sld doctor` - Validates Docker Engine connectivity, port 80/443 availability, and dnsmasq resolution.
* `sld update` - Self-updates the CLI and pulls the latest router/PHP image patches.

**Project Commands (Run inside project dir):**
* `sld start` - Reads `.sld/config.yaml`, starts isolated containers.
* `sld stop` - Stops the project containers.
* `sld restart` - Restarts the project containers.
* `sld status` - Shows service health, ports, and URLs.
* `sld logs [service]` - Tails Docker container logs.
* `sld shell` - Opens an interactive bash session as the non-root user inside the PHP container.
* `sld exec [cmd]` - Executes a raw command inside the PHP container.
* `sld open` - Opens the `.test` domain in the host's default browser.

**Passthrough Commands:**
* `sld composer [args]` - Executes Composer inside the PHP container.
* `sld artisan [args]` - Executes Laravel Artisan inside the PHP container.
* `sld npm [args]` - Spins up an ephemeral `node:latest` container mounted to the project directory to execute asset compilation, then exits.

---

## 6. Networking Model

* **`sld-global` (Bridge Network):** Created during `sld init`. The `sld-router` lives here.
* **`sld-<project>-net` (Bridge Network):** Created during `sld start`.
* **Communication:** 
  * The project's PHP container is attached to BOTH `sld-global` and `sld-<project>-net`.
  * The router communicates with PHP via `sld-global`.
  * PHP communicates with its isolated Database/Redis via `sld-<project>-net`.
  * **Host to Container:** The host browser connects to `127.0.0.1:80` (or `443`), which hits the router. Host CLI commands (`sld artisan`) use Docker API `ExecStart`.

---

## 7. PHP Runtime Model

* **Model A Implementation (One PHP container per project)**: `project-a` uses an instance of `sld-php:8.1`, while `project-b` uses an instance of `sld-php:8.4`. 
* They coexist simultaneously because Traefik dynamically routes `project-a.test` to the 8.1 container and `project-b.test` to the 8.4 container based on Docker labels.
* There is no port mapping conflict because they do not expose port 80/9000 to the host—only to the internal `sld-global` network.

---

## 8. Database / Service Model

Databases are **per-project**.
* A global shared MySQL instance introduces version locking (e.g., one project needs MySQL 5.7, another needs 8.0) and risks catastrophic data loss if the global volume is corrupted.
* SLD uses standard official Docker Hub images for services: `mysql:8.0`, `postgres:15`, `redis:alpine`, `axllent/mailpit`.
* SLD automatically maps the isolated database port to a random open port on the host (or a predictable deterministic port) and injects the connection credentials into the PHP container via standard Laravel/Symfony environment variables (e.g., `DB_HOST`, `DB_PORT`).

---

## 9. Security Model

**The Docker Socket (`/var/run/docker.sock`):**
* **Risk:** Mounting the Docker socket into a container grants root-level control over the host.
* **Mitigation:** 
  1. The `sld-router` (Traefik) is the *only* container that mounts the socket. It mounts it as **read-only** (`/var/run/docker.sock:/var/run/docker.sock:ro`).
  2. Project-level PHP containers and databases **never** mount the Docker socket.
  3. The SLD CLI runs in user-space and talks to the Docker API securely. No elevated daemon API is exposed to `localhost`.

**Host Directories:**
* The project directory is bind-mounted into the PHP container.
* To prevent root-owned files cluttering the host, the `sld-php` entrypoint uses `su-exec` or dynamic `usermod` to match the container's execution user to the host user's UID/GID.

---

## 10. CI/CD Image Publishing Design

Using **GitHub Actions**:
1. Every commit to `main` triggers a vulnerability scan (Trivy) on the Dockerfiles.
2. If tests pass, Docker Buildx compiles multi-arch images (`amd64`, `arm64`).
3. Images are pushed to GHCR.
4. **Provenance / SBOM:** GitHub Actions generates an SBOM (Software Bill of Materials) and attaches SLSA provenance attestations to the GHCR registry.

---

## 11. Versioning Strategy

* **Router:** Semantic versioning (`v2.0.0`, `latest`).
* **PHP Images:** Tagged by PHP minor version, updated silently for patches.
  * `ghcr.io/supreme-majesty/sld-php:8.3` (Always points to the latest secure patch, e.g., 8.3.11).
  * Developers rely on `8.3` and SLD automatically pulls updates during `sld update`.

---

## 12. Migration Plan from Current Architecture

1. **Deprecate the Go Daemon:** Issue an update that stops and disables the `sld-daemon.service` running on port 2025.
2. **Release SLD v2 CLI:** Deploy the new Go binary compiled with the Docker Engine SDK.
3. **Data Migration:** Since v1 ran MySQL/Nginx natively on the host, SLD v2 will provide an `sld migrate-v1` command. This command will take a MySQL dump using the local host binary and import it into the new project-isolated Docker MySQL containers.
4. **Cleanup:** The CLI will prompt to uninstall host dependencies (`apt-get remove nginx php-fpm`) safely.

---

## 13. Example Complete Laravel Project

A typical developer directory looks like this:

```text
~/Development/my-laravel-app/
├── app/
├── bootstrap/
├── config/
├── database/
├── public/
├── resources/
├── routes/
├── .env
├── composer.json
├── package.json
└── .sld/
    └── config.yaml
```

The user explicitly excludes `.sld/` from `.gitignore` so their teammates get the exact same architecture instantly.

---

## 14. Example Commands and Expected Output

**1. Initialize the system (One time)**
```bash
$ sld init
[SLD] Generating local wildcard certificates via mkcert...
[SLD] Starting global SLD router and DNS on sld-global network...
[SLD] Initialization complete. SLD is ready.
```

**2. Setup a new project**
```bash
$ git clone git@github.com:example/my-project.git
$ cd my-project
$ sld start
[SLD] Detected Laravel project. Creating .sld/config.yaml...
[SLD] Pulling image ghcr.io/supreme-majesty/sld-php:8.3...
[SLD] Creating network sld-my-project-net...
[SLD] Starting MySQL 8.0...
[SLD] Starting PHP 8.3 Runtime...
[SLD] Project is running! 
🚀 Access your site at: https://my-project.test
```

**3. Run standard commands**
```bash
$ sld composer install
[Composer] Installing dependencies from lock file...

$ sld artisan migrate
[Artisan] Running migrations...

$ sld npm run dev
[NPM] Starting Vite server... (Port forwarded automatically)
```

**4. Stop the environment**
```bash
$ sld stop
[SLD] Stopping PHP 8.3 Runtime...
[SLD] Stopping MySQL 8.0...
[SLD] Project stopped.
```
