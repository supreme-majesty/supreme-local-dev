# SLD V2 Current State Audit

> **Historical.** This audit describes the V1 code as it was before V2 (2026). That code was deleted in Phase 15 M9; see [`SLD_V2_ARCHITECTURE.md`](../architecture/SLD_V2_ARCHITECTURE.md) for the current system.

## Executive Summary

The Supreme Local Dev (SLD) repository currently implements a **V1 architecture** that is highly coupled to the host operating system. It operates as an elevated background daemon that mutates host state—specifically installing dependencies via OS package managers (`apt-get`, `brew`), directly modifying system DNS (`/etc/hosts`, `dnsmasq`), and dynamically generating host Nginx configuration files to proxy to host UNIX sockets (`php-fpm`). 

The codebase currently contains **zero Docker integration**. Transitioning to the SLD V2 Docker-backed orchestrator will require removing the entire OS abstraction layer (`pkg/adapters`) and the privileged background daemon (`pkg/daemon/api`), replacing them with a purely user-space CLI that orchestrates the Docker Engine API. 

## Repository Structure

* **`cmd/sld/main.go`**: The entrypoint using Cobra. Defines all commands but primarily acts as a thin wrapper sending REST requests to `localhost:2025`. Includes an `elevate()` function to request `sudo`.
* **`pkg/daemon/daemon.go`**: Core V1 logic. Holds global state, generates Nginx `server {}` blocks in memory, and writes them to the filesystem.
* **`pkg/daemon/api/server.go`**: An unauthenticated HTTP server on port 2025. It executes daemon actions (parking, restarting services, DB management) based on HTTP requests from the CLI and GUI.
* **`pkg/adapters/*`**: Operating system mutators (`linux`, `macos`, `windows`). Responsible for running `apt-get`, `brew`, `winget`, and `systemctl`.
* **`pkg/services/*`**: Contains project detection logic, artisan execution, log watching, and database interaction.
* **`pkg/daemon/state/*`**: Manages the global JSON state file located at `/var/lib/sld/state.json`.
* **`.github/workflows/*`**: Contains CI and Release pipelines, utilizing `fpm` for `.deb` and WiX for `.msi` native installers.

## Runtime Execution Flows

**`sld install`**:
1. Checks for root (`elevate()` to `sudo`).
2. Calls `daemon.GetClient().EnsureInstalled()`.
3. Calls OS-specific `Adapter.InstallDependencies()` (e.g., `apt-get install nginx php-fpm`).
4. Extracts Nginx templates to `/var/lib/sld/`.
5. Creates and enables `/etc/systemd/system/sld-daemon.service`.

**`sld park` (and domain routing)**:
1. CLI sends POST to `http://localhost:2025/api/park`.
2. Daemon adds the path to `/var/lib/sld/state.json`.
3. Daemon triggers a state refresh.
4. Daemon iterates over all parked paths, detects projects, and strings together a giant Nginx configuration containing `fastcgi_pass unix:/run/php/phpX.Y-fpm.sock`.
5. Daemon writes this config to `/etc/nginx/sites-available` and runs `systemctl reload nginx`.

## Existing Components

- **Cobra CLI**: Reusable, but commands need to be rewired away from the daemon.
- **ProjectManager**: Detects Laravel/PHP web roots. Reusable.
- **Adapters**: Highly tied to host OS. Must be discarded.
- **Daemon API**: Unauthenticated command executor. Must be discarded.
- **Systemd Service**: Must be discarded.

## Docker Readiness

**NONE.**
- The repository does not import the Docker SDK (`github.com/docker/docker/client`).
- There are no Dockerfiles.
- There is no network or volume management logic.
- All processes currently assume host execution (e.g., `exec.Command("tail", ...)`).

## V1 → V2 Migration Matrix

| Existing Component | Current Purpose | V2 Equivalent | Action | Reason |
| --- | --- | --- | --- | --- |
| `cmd/sld/main.go` | Command routing to Daemon | Docker Orchestrator | **ADAPT** | Keep Cobra UI, change implementation. |
| `pkg/daemon/api/` | REST API for CLI/GUI | Direct Docker API | **REMOVE** | Background daemon is no longer needed. |
| `pkg/adapters/` | OS Package Management | Docker Images | **REMOVE** | SLD V2 does not install host binaries. |
| `pkg/daemon/daemon.go` | Nginx Config Generation | Traefik Labels | **REPLACE** | Traefik dynamically routes via Docker labels. |
| `pkg/services/project.go`| Web root / Framework detection | Same | **ADAPT** | Expand to generate/read `.sld/config.yaml`. |
| `pkg/daemon/state/` | `/var/lib/sld/state.json` | Docker Engine State | **REPLACE** | Docker is the source of truth for running services. |

## Architecture Conflicts

| Conflict | Severity | Description |
| --- | --- | --- |
| **Host Nginx/PHP** | CRITICAL | `adapters` force global installation of PHP and Nginx, preventing Docker isolation. |
| **Privileged Daemon** | CRITICAL | The daemon runs as root/sudo to write to `/etc/nginx`. Docker orchestration runs in user-space. |
| **UNIX Sockets** | HIGH | Nginx generation hardcodes `fastcgi_pass unix:...`. Docker requires TCP networking. |
| **Unauthenticated API**| HIGH | Port 2025 API is wide open locally. |
| **Global State** | HIGH | `/var/lib/sld/state.json` dictates all routing, rather than per-project files. |

## Configuration Audit

**Current:** 
Configuration is entirely global, stored in `/var/lib/sld/state.json`. Projects are discovered by scanning directories listed in the `ParkedPaths` array. Individual projects cannot easily override their PHP version or database requirements through code.

**V2 Target:**
Projects must define `.sld/config.yaml`. SLD must parse this file to spin up isolated resources. The global state should only be used to remember which projects exist, but the configuration source of truth must move into the project directory.

## Project Detection

Currently handled in `pkg/services/project_manager.go`.
- Scans a directory.
- If `artisan` exists -> Laravel (`public` is web root).
- If `index.php` exists -> Generic PHP (`.` is web root).
- PHP version is determined by running `php -v` on the host, not per-project.

**V2 Collision Risk:** 
When spinning up Docker containers, container names like `sld-api-php` might collide if two projects are named `api`. V2 must hash the absolute path or use UUIDs combined with project names for deterministic, isolated network and container naming.

## Networking

**Current:** 
- Nginx binds to host `80`/`443`.
- Daemon binds to `2025`.
- Database binds to `3306`.
- dnsmasq binds to `53`.

**V2 Risks:**
Port `80`/`443` bindings for the Traefik container will conflict if the user previously installed V1 (which installed host Nginx). The migration MUST stop and disable host Nginx/Apache before spinning up the Docker router.

## Security

- **sudo Usage**: Pervasive. `cmd/sld/main.go` uses `elevate()` heavily.
- **Unauthenticated API**: `pkg/daemon/api/server.go` has no authentication and allows executing destructive host commands via HTTP POST.
- **Permissions**: `/var/lib/sld` requires root to manage, complicating the developer experience.

## Testing

**Coverage is virtually non-existent.**
Only two test files exist in the entire repository (`ghost_test.go` and `relationships_test.go`).
There are no E2E tests, no integration tests, and no tests for Nginx generation or CLI commands.
**V2 Impact:** Moving to V2 requires building a test suite from scratch, specifically Docker integration tests.

## Build & Release

- **Current:** `.github/workflows/release.yml` builds Go binaries and wraps them in OS-specific installers (`.deb` via `fpm`, `.msi` via `wix`, `.pkg` via `pkgbuild`).
- **V2 Impact:** The OS-specific installers can be simplified. We only need the Go binary. However, we must add new GitHub Actions to build and push the `sld-router` and `sld-php` Docker images to GHCR using `buildx` for multi-arch support.

## Documentation

- **Current:** `README.md` describes the V1 host-mutating architecture ("Automatic Nginx configuration", "Linux-first", "systemctl").
- **Status:** **OUTDATED** (for V2). The entire architectural premise must be rewritten.

## Dependencies

- **Keep:** `cobra` (CLI), `gorilla/websocket` (if keeping GUI).
- **Remove:** Host dependencies (`nginx`, `php-fpm`, `dnsmasq` OS packages).
- **Add:** `github.com/docker/docker/client` (Docker Go SDK), `gopkg.in/yaml.v3` (for config parsing).

## Technical Debt

- **BLOCKERS:** The heavy coupling to `adapters` running `apt-get`/`brew`.
- **MIGRATION RISKS:** Users have existing MySQL databases running on the host. Transitioning to Dockerized per-project MySQL requires a data migration path, or SLD V2 must support routing to the host database temporarily.
- **CLEANUP:** 15+ HTTP endpoints in `server.go` that manipulate the host system must be deleted.

## Migration Risks

The highest risk in this migration is the **Database State**. Users rely on the host MySQL instance installed by V1. If V2 immediately switches to Dockerized MySQL, users will lose access to their data. A database import/export utility must be written as part of the migration tooling.

## Recommended Migration Sequence

1. **Preserve working behavior:** Fork the command structure so V1 continues to work during development.
2. **Establish configuration schema:** Implement `.sld/config.yaml` parsing.
3. **Establish Docker abstraction:** Add the Docker Go SDK and create wrapper interfaces for managing containers, networks, and volumes.
4. **Establish Traefik & DNS:** Create the `sld init` flow that spins up the `sld-global` network and Traefik container.
5. **Establish Nginx + PHP runtime:** Create `sld start` to spin up project-isolated `sld-php` containers.
6. **Add migration tooling:** Build `sld migrate` to dump host MySQL and import to Docker volumes.
7. **Add developer commands:** Map `sld artisan` and `sld composer` to `docker exec`.
8. **Remove obsolete V1 components:** Delete `pkg/adapters`, the background daemon, and host Nginx generation.

## Files to Preserve
- `cmd/sld/main.go` (Command definitions)
- `pkg/services/project_manager.go` (Framework detection logic)

## Files to Adapt
- `.github/workflows/release.yml` (Simplify installers, add Docker pushes)
- `pkg/daemon/state/manager.go` (Convert to reading `.sld/config.yaml` and querying Docker)

## Files to Replace
- `pkg/daemon/daemon.go` (Nginx generation replaced by Traefik labels)

## Files to Remove Eventually
- `pkg/daemon/api/server.go`
- `pkg/adapters/linux/*`
- `pkg/adapters/macos/*`
- `pkg/adapters/windows/*`
- `pkg/assets/runtime/nginx/*`

## New Components Required
- `pkg/docker/client.go` (Docker SDK wrapper)
- `pkg/docker/reconciler.go` (State comparison)
- Dockerfiles for `sld-router`, `sld-php`, `sld-node`.

## Open Questions

- How will we smoothly migrate users' existing `.test` host databases into the isolated Docker volumes?
- Should the GUI (`sld-dashboard`) be preserved, and if so, how does it query status without the background daemon? (Likely by executing the SLD CLI directly).

## Audit Conclusion

### Confidence: HIGH

The repository structure is straightforward. The complete reliance on host-level mutations and the complete absence of Docker integrations are clearly evident in the source code (`pkg/adapters` and `pkg/daemon/daemon.go`). The path forward to a declarative, Docker-backed architecture requires stripping away the V1 daemon/adapter model entirely and replacing it with Docker SDK bindings.
