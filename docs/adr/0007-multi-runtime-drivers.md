# ADR 0007: Multi-Runtime Drivers

## Status
Accepted (2026-11, Phase 15 M3/M4)

## Context
SLD V2 started as a PHP/Laravel tool: one project = nginx + PHP-FPM (+ database, Redis, Mailpit). Developers also work on Next.js, Vite, Django, FastAPI, Rails, Go services, static sites, projects that ship their own Dockerfile, and existing Docker Compose stacks. Often several of these live in one repository (`backend/` + `frontend/`).

## Decision
A project config (`.sld/config.yaml`, `version: 2`) is an ordered list of **apps**, each with a `runtime`:

| Runtime | What runs |
|---|---|
| `php` | `sld-php:<version>` (FPM) + `sld-nginx` (ADR 0004) |
| `static` | `nginx:1.27-alpine` with an SLD config that refuses hidden files |
| `node`, `python`, `ruby`, `go` | the official image (`node:<v>-bookworm-slim`, `python:<v>-slim`, ...) running the app's dev command |
| `image` | any image, with a port |
| `dockerfile` | an image built from the project's Dockerfile |
| `proxy` | no container: a router entry to a process on the host |
| `compose` | the project's own `compose.yaml`, run as-is (see below) |

Each runtime has a **driver** (`internal/runtime`) that turns an app into a plan: containers, image builds, dependency volumes, router entries. The orchestrator runs every plan the same way (reconcile, health, routing), so adding a runtime does not touch the orchestrator.

Rules every driver follows:
- **Routing:** the first app owns `<name>.localhost`; the others get `<app>.<name>.localhost`.
- **Your user:** processes run as the folder owner's UID/GID (`setpriv`), so files created by an app belong to the developer.
- **Commands as data:** user commands are passed through environment variables, never spliced into shell scripts.
- **Dependencies inside the container:** `node_modules`, `vendor`, virtualenvs and gems live in a per-app volume, reinstalled when the lockfile hash changes.
- **Recreate only what changed:** each container carries a hash of its spec, so changing one app recreates only that app.

**Compose projects** keep their own stack. SLD runs `docker compose` with the project's files plus a generated override that:
- labels every service as part of the SLD project;
- removes published host ports (unless `keep_ports`);
- connects the routed service to the router network with routing labels.

Compose runs in a `docker:cli` helper container, so neither the host nor the SLD image needs the Compose plugin.

**Detection** (`internal/detect`) writes the config: it recognises frameworks, monorepo sub-folders (mobile apps skipped) and compose stacks. Generic folder names (`backend`, `app`) take the parent's name, and suffixes such as `-apps` are dropped (`sunport-apps/backend` → `sunport.localhost`).

## Consequences
### Positive
- Any web stack works without writing Docker files; existing compose projects work unchanged.
- One project can mix runtimes (Laravel API + Next.js frontend) under one name.
- V1 PHP configs are upgraded in memory and keep their containers (no recreation).

### Negative
- Official language images are larger than a tuned base image; first starts download them.
- Compose projects are only as reproducible as their own compose files; SLD does not rewrite them beyond the override.
