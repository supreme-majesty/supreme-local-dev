# ADR 0009: Path Mapping and Workspace Mounts

## Status
Accepted (2026-11, Phase 15 M1/M4b)

## Context
When SLD runs in a container, it sees projects at `/workspace/shop`, but it asks the Docker daemon, which runs on the host (or in Docker Desktop's VM), to bind-mount them into app containers. The daemon resolves bind sources on **its** side, so `/workspace/shop` would mount an empty or wrong folder. Project identity (container names, volumes) must also stay the same whether a project is started from the image or from the CLI on the host.

## Decision
`internal/hostenv` describes where SLD runs and translates paths:

- **Self-inspection:** at start SLD inspects its own container and records its mounts. A path SLD can read is mapped to the mount that contains it.
  - Under a bind mount: becomes the host path (`/workspace/shop` → `C:\Users\me\Projects\shop`, or `/home/me/Projects/shop`).
  - Under a named volume (SLD's data: certificates, generated configs): mounted as a volume subpath.
  - Not under any mount: refused with a clear error (`ErrNotMounted`), never silently mounted empty.
- **Identity from the host path:** the project ID is derived from the canonical host path (ADR 0003), so the image and the CLI address the same containers.
- **Mirror mounts for tools that pass paths on:** `docker compose` resolves `./src:/var/www` relative to the compose file and hands the result to the daemon. The compose helper therefore mounts the project at the path the daemon itself uses for that host directory, and runs there:
  - Linux/macOS: the same path;
  - Docker Desktop on Windows: `/run/desktop/mnt/host/c/...`.

  Scaffolding helpers (new projects) use the same mirror mounts.
- **Ownership:** app processes run as the owner of the project folder as seen through the mount. On Docker Desktop, where files appear as root, a fixed UID (1000) is used.
- **Workspace boundary:** dashboard operations that take a path (hide, create, clone, discard, browse) only accept paths inside a workspace folder.

## Consequences
### Positive
- The same project gets the same containers from the dashboard and from the CLI.
- Docker Desktop on Windows and macOS works without special configuration beyond file sharing.
- Misconfigured mounts fail loudly instead of serving empty folders.

### Negative
- Projects must live under a mounted workspace; a folder elsewhere on the disk is invisible to the image until its parent is mounted.
- Bind mounts through Docker Desktop are slower than native Linux; dependency folders are kept in volumes, and Windows users get the best speed with projects in the WSL filesystem.
