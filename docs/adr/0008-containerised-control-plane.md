# ADR 0008: Containerised Control Plane

## Status
Accepted (2026-11, Phase 15 M5–M9). Amends ADR 0001 and ADR 0002.

## Context
ADR 0002 removed the V1 daemon: a root service with an unauthenticated API on port 2025 that changed the host (nginx, PHP-FPM, dnsmasq, hosts file). V2 then worked from the CLI only. Users, however, want what the daemon gave them, a dashboard that shows and manages every project and its databases, without installing anything but Docker, on Windows, macOS and Linux alike.

## Decision
SLD ships as **one image** (`ghcr.io/supreme-majesty/sld`) whose default command is `sld serve`: the dashboard, the database manager and the project API in one long-running container.

- **The host is not changed.** SLD reaches Docker through the mounted socket and creates sibling containers: router, project apps, helpers. It installs no packages and edits neither the hosts file nor `/etc/resolver`. `*.localhost` resolves without DNS setup; for other TLDs SLD prints the hosts line to add.
- **Projects come from a workspace.** The user mounts a projects folder (`/workspace`). SLD finds the projects in it; there is no "park" or "link" step (ADR 0009).
- **Authenticated API.** Every route requires the admin token or a session, and the API binds to `127.0.0.1` (ADR 0010).
- **One engine, two front ends.** The dashboard calls `/api/v2` (projects, jobs with streamed output, logs, settings). The original pages call the same Docker-backed handlers under `/api` (services, plugins, share, sites, doctor). People who don't run the image use the same engine through the CLI (`sld start`, `sld scan`, ...).
- **Automation.** On start SLD can:
  - write configs for newly found projects;
  - restart the projects that were running;
  - start projects marked "autostart", or every set-up project.

  The defaults change nothing except bringing back what was running.
- **The V1 host layer is deleted** (OS adapters, nginx/PHP-FPM/hosts logic, systemd packaging, the unauthenticated listener).

## Consequences
### Positive
- `docker run` (or `docker compose up`) is the whole installation, identical on every OS.
- The dashboard survives terminal sessions and can restore projects after a reboot (`--restart unless-stopped`).
- Security improves on V1: no root service, no unauthenticated port, no host changes.

### Negative
- The container holds the Docker socket, which is root-equivalent on the machine. Mitigations: loopback binding, mandatory auth, Host/Origin/CSRF checks, and a non-root SLD user. A socket proxy that only allows the calls SLD needs is a possible future hardening.
- Paths seen inside the container differ from host paths; every mount must be translated (ADR 0009).
- Features that relied on host programs (an editor launched on the desktop, the host PHP behind the Artisan console, nginx logs for X-Ray) need container equivalents or are unavailable in the image.
