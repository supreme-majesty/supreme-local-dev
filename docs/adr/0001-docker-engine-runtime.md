# ADR 0001: Docker Engine as Execution Runtime

## Status
Accepted. Amended by [ADR 0008](0008-containerised-control-plane.md): SLD itself now also runs as a container (the control plane), still without changing the host.

## Context
SLD V1 implemented local development by directly installing software packages (`nginx`, `php-fpm`, `mariadb-server`, `dnsmasq`) onto the host operating system using package managers (`apt`, `brew`, `winget`). This caused dependency hell across projects needing different PHP versions, polluted the host system, and broke easily across host OS upgrades.

## Decision
SLD V2 adopts the Docker Engine as its sole execution runtime. All services (Traefik router, Nginx web servers, PHP-FPM runtimes, databases, Redis caches, Node tooling) run as isolated Docker containers. The host machine hosts only the Go CLI, Docker Engine, and developer project source code.

## Consequences
### Positive
- Strict version isolation between projects (e.g. Project A on PHP 8.1, Project B on PHP 8.4).
- Zero host operating system pollution; no `apt` or `brew` package conflicts.
- Consistent, reproducible environments across Linux, macOS, and Windows WSL2.

### Negative
- Requires Docker Engine / Docker Desktop to be installed and running.
- Minor filesystem overhead on macOS bind mounts (mitigated by VirtioFS).
