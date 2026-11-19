# SLD V2 Dockerization — Progress Tracker

## ✅ Completed Components

| Component | Files | Description & Status |
|-----------|-------|----------------------|
| **PHP Runtime Image** | [`docker/php/Dockerfile`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/docker/php/Dockerfile) | Multi-arch (`linux/amd64`, `linux/arm64`), PHP 8.1–8.4, Xdebug toggle, Composer, non-root `WWWUSER`/`WWWGROUP` dynamic UID/GID mapping |
| **Nginx Sidecar Image** | [`docker/nginx/Dockerfile`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/docker/nginx/Dockerfile) | Template-based config, health check, `WEB_ROOT`/`PHP_HOST` env substitution, zero-config FastCGI routing |
| **Router Image (Traefik)** | [`docker/router/Dockerfile`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/docker/router/Dockerfile), [`docker/router/traefik.yml`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/docker/router/traefik.yml) | Traefik v3.6, Docker label auto-discovery on `sld-global` network, dynamic TLS configuration watching `/etc/traefik/dynamic` |
| **Docker Engine Client** | [`internal/docker/client.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/docker/client.go) | Official Docker SDK client wrapper: ping, network/volume CRUD (`EnsureNetwork`, `RemoveNetwork`, `EnsureVolume`, `RemoveVolume`, `ListProjectVolumes`), container lifecycle with `Cmd`/`WorkingDir`/`ExtraHosts`/`Healthcheck`, logs streaming, image pull |
| **Label System** | [`internal/docker/labels.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/docker/labels.go) | Standardized `com.supreme.sld.*` labels for project containers, networks, volumes |
| **Config Schema & Defaults** | [`internal/config/schema.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/config/schema.go) | `.sld/config.yaml` schema: runtime, domain, services (DB, Redis, Mailpit), Node dev server, Xdebug, custom env |
| **Config Loader & Resolver** | [`internal/config/loader.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/config/loader.go) | Parse, validate, defaults, composer PHP constraint resolver, deterministic SHA-256 config hashing |
| **Project Identity** | [`internal/project/identity.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/project/identity.go) | Deterministic `<slug>-<hash8>` project IDs, resource naming helpers (`PHPContainerName`, `NetworkName`, etc.) |
| **Reconciliation Engine** | [`internal/reconcile/diff.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/reconcile/diff.go) | Declarative desired-vs-actual state reconciliation: CREATE, START, RECREATE (on config change), REMOVE, NOOP |
| **Secret Manager** | [`internal/secret/secret.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/secret/secret.go) | `.sld/secrets.json`, cryptographic random generator, idempotent `GetOrGenerate` |
| **TLS & Certificate Manager** | [`internal/tls/cert.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/tls/cert.go) | Automatic `mkcert` wildcard SSL generation with Go `crypto/x509` self-signed fallback, Traefik dynamic TLS configuration generator mounted into router |
| **DNS Resolution Helper** | [`internal/dns/resolver.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/dns/resolver.go) | Cross-platform DNS check, hosts file add/check/remove lifecycle, WSL2 Windows hosts auto-detection, macOS `/etc/resolver/test` helper, wired into `init`/`start`/`destroy` |
| **Core Orchestrator** | [`internal/orchestrate/orchestrator.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/orchestrate/orchestrator.go) | Strict dependency order (DB/Redis/Mailpit → PHP → Nginx → Node), container health-check polling with stderr diagnostics, managed Node dev server (`Cmd`/`WorkingDir`), Linux Xdebug `host.docker.internal:host-gateway`, `DestroyProject` (optional volume purge), `RecreateRouter` |
| **V2 CLI Commands & Runner** | [`internal/v2/commands.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/v2/commands.go), [`internal/v2/runner.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/v2/runner.go) | Cobra command tree under `sld v2`: `init`, `start`, `stop`, `destroy [--volumes]`, `restart`, `status [--all] [--json]`, `inspect`, `secure`, `config validate/show`, `doctor` (Docker, ports 80/443, TLS, DNS, WSL2), `init-project`, `logs`, `open`, `update`, `legacy status/migrate/remove`, `migrate-v1` |
| **CLI Exec & Passthrough** | [`internal/v2/exec.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/v2/exec.go) | TTY-attached interactive exec: `sld v2 shell [--service]`, `sld v2 exec [--service] <cmd...>`, `sld v2 php`, `sld v2 composer`, `sld v2 artisan`, `sld v2 npm` (ephemeral Node container with Linux UID/GID mapping) |
| **Framework Auto-Detector** | [`internal/v2/detect.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/v2/detect.go) | Auto-detects Laravel, Symfony, WordPress, PHP versions from `composer.json`, Node dev servers from `package.json`, and databases from `.env` |
| **Root CLI Integration** | [`cmd/sld/main.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/cmd/sld/main.go) | Wired `v2.RegisterCommands(rootCmd)` into root CLI, coexisting with V1 commands during migration |
| **CI/CD Pipeline** | [`.github/workflows/docker-images.yml`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/.github/workflows/docker-images.yml), [`.github/workflows/ci.yml`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/.github/workflows/ci.yml), [`.github/workflows/release.yml`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/.github/workflows/release.yml) | Multi-arch Docker Buildx with SBOM/provenance attestations and Trivy image scanning; Go version synced with `go.mod`; `linux/arm64` CLI binary added |
| **Test Suite** | Unit tests + Docker integration test | `config_test.go`, `labels_test.go`, `identity_test.go`, `diff_test.go`, `secret_test.go`, `orchestrator_test.go`, `cert_test.go`, `resolver_test.go`, `detect_test.go`, `runner_test.go`, `docker_discovery_test.go`, `integration_test.go` (`-tags=integration`) |

## ⏳ Remaining Post-MVP Items
- **Phase 14 (CI E2E)**: Wire `-tags=integration` and a full Laravel HTTP E2E job into GitHub Actions CI.
- **Phase 15 (V1 Cutover & Purge)**: Promote `sld v2 <cmd>` to top-level `sld <cmd>`, decide on keeping vs. purging V1 `pkg/adapters` and `pkg/daemon` (and migrating `sld-dashboard` to Tauri IPC / `--json`), update `README.md`/`Makefile`, and clean up committed binaries in repo root.

## 🚀 Available V2 Commands

```bash
# Global management
sld v2 init              # Initialize sld-global network, wildcard certs, and Traefik router
sld v2 secure            # Generate/refresh wildcard TLS certs and reload Traefik router
sld v2 doctor            # Validate Docker Engine, ports 80/443, global router, TLS certs, DNS, WSL2
sld v2 update            # Pull latest GHCR images (router, nginx, php 8.1-8.4) and recreate router

# Project management (run inside project directory)
sld v2 init-project      # Auto-detect framework & generate .sld/config.yaml
sld v2 config validate   # Validate .sld/config.yaml syntax and runtime constraints
sld v2 config show       # Print resolved .sld/config.yaml with applied defaults
sld v2 start             # Start isolated project containers in dependency order with health checks
sld v2 stop              # Stop & remove project containers (preserves volumes)
sld v2 destroy [--volumes] # Tear down containers, network, and optionally DB/Redis volumes
sld v2 restart           # Restart project containers
sld v2 status [--all] [--json] # Show container statuses for current project or all managed projects
sld v2 inspect           # Output detailed JSON Docker state for current project
sld v2 logs [service]    # Tail logs for a service (php, nginx, db, redis, mailpit, node) (-f, --tail)
sld v2 open              # Open project URL in host default browser

# Legacy V1 migration
sld v2 legacy status     # Inspect host for V1 daemon, state.json, and host binaries
sld v2 legacy migrate [--execute] # Preview (--dry-run) or execute V1 project & DB migration to V2
sld v2 legacy remove     # Archive V1 state and stop/disable V1 systemd daemon
sld v2 migrate-v1        # Backward-compatible alias for legacy migrate --execute

# Passthrough commands
sld v2 shell [--service] # Interactive shell inside project container (default: php)
sld v2 exec [--service] <cmd...> # Execute command inside project container (default: php)
sld v2 php <args...>     # Run PHP CLI inside project PHP container
sld v2 composer <args...># Run Composer inside project PHP container
sld v2 artisan <args...> # Run Laravel Artisan inside project PHP container
sld v2 npm <args...>     # Run npm via ephemeral Node container
```
