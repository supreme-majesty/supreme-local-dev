# SLD V2 Dockerization — Progress Tracker

## ✅ Completed Components

| Component | Files | Description & Status |
|-----------|-------|----------------------|
| **PHP Runtime Image** | [`docker/php/Dockerfile`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/docker/php/Dockerfile) | Multi-arch (`linux/amd64`, `linux/arm64`), PHP 8.1–8.4, Xdebug toggle, Composer, non-root `WWWUSER`/`WWWGROUP` dynamic UID/GID mapping |
| **Nginx Sidecar Image** | [`docker/nginx/Dockerfile`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/docker/nginx/Dockerfile) | Template-based config, health check, `WEB_ROOT`/`PHP_HOST` env substitution, zero-config FastCGI routing |
| **Router Image (Traefik)** | [`docker/router/Dockerfile`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/docker/router/Dockerfile), [`docker/router/traefik.yml`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/docker/router/traefik.yml) | Traefik v3.1, Docker label auto-discovery on `sld-global` network, dynamic TLS configuration watching `/etc/traefik/dynamic` |
| **Docker Engine Client** | [`internal/docker/client.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/docker/client.go) | Official Docker SDK client wrapper: ping, network creation/connection, volume management, container lifecycle, logs streaming, image pull |
| **Label System** | [`internal/docker/labels.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/docker/labels.go) | Standardized `com.supreme.sld.*` labels for project containers, networks, volumes |
| **Config Schema & Defaults** | [`internal/config/schema.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/config/schema.go) | `.sld/config.yaml` schema: runtime, domain, services (DB, Redis, Mailpit), Node dev server, Xdebug, custom env |
| **Config Loader & Resolver** | [`internal/config/loader.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/config/loader.go) | Parse, validate, defaults, composer PHP constraint resolver, deterministic SHA-256 config hashing |
| **Project Identity** | [`internal/project/identity.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/project/identity.go) | Deterministic `<slug>-<hash8>` project IDs, resource naming helpers (`PHPContainerName`, `NetworkName`, etc.) |
| **Reconciliation Engine** | [`internal/reconcile/diff.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/reconcile/diff.go) | Declarative desired-vs-actual state reconciliation: CREATE, START, RECREATE (on config change), REMOVE, NOOP |
| **Secret Manager** | [`internal/secret/secret.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/secret/secret.go) | `.sld/secrets.json`, cryptographic random generator, idempotent `GetOrGenerate` |
| **TLS & Certificate Manager** | [`internal/tls/cert.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/tls/cert.go) | Automatic `mkcert` wildcard SSL generation with Go `crypto/x509` self-signed fallback, Traefik dynamic TLS configuration generator mounted into router |
| **DNS Resolution Helper** | [`internal/dns/resolver.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/dns/resolver.go) | Cross-platform DNS check (`net.LookupHost`), hosts file parsing/entry helper, macOS `/etc/resolver/test` configuration |
| **Core Orchestrator** | [`internal/orchestrate/orchestrator.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/orchestrate/orchestrator.go) | Builds `[]DesiredContainer` from `.sld/config.yaml`, reconciles against Docker Engine, mounts volumes, configures Traefik router labels and networks |
| **V2 CLI Commands & Runner** | [`internal/v2/commands.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/v2/commands.go), [`internal/v2/runner.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/v2/runner.go) | Cobra command tree under `sld v2`: `init`, `start`, `stop`, `restart`, `status`, `doctor`, `init-project`, `logs`, `open`, `update`, `migrate-v1` |
| **CLI Exec & Passthrough** | [`internal/v2/exec.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/v2/exec.go) | TTY-attached interactive exec: `sld v2 shell`, `sld v2 exec`, `sld v2 composer`, `sld v2 artisan`, `sld v2 npm` (ephemeral container) |
| **Framework Auto-Detector** | [`internal/v2/detect.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/internal/v2/detect.go) | Auto-detects Laravel, Symfony, WordPress, PHP versions from `composer.json`, Node dev servers from `package.json`, and databases from `.env` |
| **Root CLI Integration** | [`cmd/sld/main.go`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/cmd/sld/main.go) | Wired `v2.RegisterCommands(rootCmd)` into root CLI, coexisting with V1 commands during migration |
| **CI/CD Pipeline** | [`.github/workflows/docker-images.yml`](file:///c:/Users/Supreme%20Majesty/Documents/Development/supreme-local-dev/.github/workflows/docker-images.yml) | Multi-arch Docker Buildx builds pushed to GHCR for router, nginx, php 8.1–8.4 |
| **Unit Test Suite** | 9 test suites across all packages | `config_test.go`, `labels_test.go`, `identity_test.go`, `diff_test.go`, `secret_test.go`, `orchestrator_test.go`, `cert_test.go`, `resolver_test.go`, `detect_test.go`, `runner_test.go` |

## 🚀 Available V2 Commands

```bash
# Global management
sld v2 init              # Initialize sld-global network, wildcard certs, and Traefik router
sld v2 doctor            # Validate Docker Engine, global router, TLS certs, and *.test DNS
sld v2 update            # Pull latest GHCR images (router, nginx, php 8.1-8.4)

# Project management (run inside project directory)
sld v2 init-project      # Auto-detect framework & generate .sld/config.yaml
sld v2 start             # Start isolated project containers (PHP, Nginx, DB, Redis, Mailpit)
sld v2 stop              # Stop & remove project containers
sld v2 restart           # Restart project containers
sld v2 status            # Show container statuses and services
sld v2 logs [service]    # Tail logs for a service (php, nginx, db, redis, mailpit) (-f, --tail)
sld v2 open              # Open project URL in host default browser
sld v2 migrate-v1        # Migrate legacy V1 host project to V2 Docker setup

# Passthrough commands
sld v2 shell             # Interactive bash shell inside project PHP container
sld v2 exec <cmd...>     # Execute command inside project PHP container
sld v2 composer <args...># Run Composer inside project PHP container
sld v2 artisan <args...> # Run Laravel Artisan inside project PHP container
sld v2 npm <args...>     # Run npm via ephemeral Node container
```
