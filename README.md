# Supreme Local Dev (SLD) 🚀

**A local development control plane that runs as one Docker container.** Point it at your projects folder and manage every project from a dashboard: each one gets a `*.localhost` domain, the right runtime, its databases, logs and HTTPS — on Windows, macOS and Linux, with nothing installed on your machine except Docker.

- **Any web stack.** PHP (Laravel, Symfony, WordPress), Node (Next.js, Nuxt, Vite, Astro, SvelteKit, NestJS, Express…), Python (Django, FastAPI, Flask), Ruby (Rails), Go, static sites, a Dockerfile, any image — or a process already running on your machine.
- **Your existing Docker Compose projects, as they are.** SLD runs a project's own `compose.yaml` and gives its web service a domain. It removes clashing host ports and keeps your containers, images and data volumes.
- **Monorepos.** `backend/` + `frontend/` folders become separate apps of one project; mobile apps (React Native, Expo, Flutter) are skipped.
- **Dashboard.** Sign in, see every project with its status and URLs, set up / start / stop / restart, follow output live, read logs, and manage databases.

## Quick start

You need Docker (Docker Desktop on Windows/macOS).

```bash
docker run -d --name sld --restart unless-stopped \
  -p 127.0.0.1:2025:2025 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --group-add 0 \
  -v sld-data:/home/sld/.sld \
  -v "$HOME/Projects:/workspace" \
  ghcr.io/supreme-majesty/sld:latest

docker logs sld        # copy the admin token printed on first start
```

Open **http://localhost:2025**, paste the token, and you will see the dockerized projects found in `~/Projects` (see Projects below for which ones).

- **Windows (PowerShell):** use your folder path, e.g. `-v "C:\Users\me\Projects:/workspace"`. For speed, keep projects in the WSL filesystem (`\\wsl$\Ubuntu\home\me\Projects`) and run the command from WSL; projects on `C:\` work but file access is slower.
- **macOS:** the projects folder must be shared with Docker Desktop (*Settings → Resources → File sharing*; your home folder is shared by default). VirtioFS (the default file-sharing mode) is the fastest.
- **Linux:** SLD runs as a non-root user and needs the Docker socket's group: replace `--group-add 0` with `--group-add $(stat -c %g /var/run/docker.sock)`.
- **Several project folders:** mount each one and list them: `-v ~/work:/workspace/work -v ~/oss:/workspace/oss`, or set `SLD_WORKSPACES=/a,/b`.

> **Lost the token?** Only its hash is stored, so it can't be shown again, but `docker exec sld sld token` (with Compose: `docker exec sld-app-1 sld token`, or `make token`; the sign-in page shows the exact command) prints a new one with a link that signs you straight in. The running dashboard accepts it at once; the old token stops working.

### With this repository (Docker Compose)

```bash
cp .env.example .env    # optional: change ports, projects folder
docker compose up -d --build
docker compose logs app # admin token (later: make token)
```

`compose.yaml` runs the SLD control plane plus MariaDB, Redis, Mailpit and phpMyAdmin that SLD's database manager uses. By default the folder **containing** this repository is mounted as your projects workspace (set `SLD_WORKSPACE` to change it).

| Service | Address | Notes |
|---|---|---|
| SLD dashboard + API | http://127.0.0.1:2025 | sign in with the admin token |
| phpMyAdmin | http://127.0.0.1:8080 | |
| MariaDB | `127.0.0.1:${MYSQL_PORT:-3306}` | user `root`, password `${MYSQL_ROOT_PASSWORD:-sld}` |
| Redis | `127.0.0.1:${REDIS_PORT:-6379}` | |
| Mailpit | http://127.0.0.1:8025, SMTP `127.0.0.1:1025` | |
| PostgreSQL (`--profile postgres`) | `127.0.0.1:${POSTGRES_PORT:-5432}` | |

If another project already uses 3306 or 6379, set `MYSQL_PORT` / `REDIS_PORT` in `.env` (e.g. 3316 / 6380).

## Using the dashboard

- **Projects** — the dockerized projects in the workspace (a compose file or Dockerfile of their own, also in a sub-folder such as `backend/`, or containers SLD already runs for them) plus the folders you park or link with **Park / link folder**. Other folders are not detected: they are not listed and auto setup never writes a config into them. Group folders such as `laravel-builds/` are searched inside; parking a group folder detects every project in it.
  - **Set up** writes the project's `.sld/config.yaml` (detected automatically; review it in the repo).
  - **Start / Stop / Restart** run in the background; their output streams into a console.
  - **Logs** follows any container of the project.
  - **Remove** deletes containers but keeps data unless you explicitly choose to delete the data volumes.
- **Setup** — Docker and router status, ports, workspace, the HTTPS certificate download, and automation (set up new projects, restart what was running, autostart).
- **Dashboard**, **Sites**, **Domains** — overview, metrics and every site with its URL; tag and categorise sites. *Remove* hides a project (it writes `.sldignore` in its folder).
- **Services** — SLD's own containers (router, the bundled databases and mail, plugins) with start / stop / restart. **Doctor** checks Docker, the router, workspaces, HTTPS and domains.
- **Plugins** — shared Redis, Mailpit and PostgreSQL containers, reachable from every project.
- **Share** — a public `trycloudflare.com` URL for a project, through a Cloudflare quick tunnel.
- **New project** — Laravel, WordPress, React, Vue, Next.js, Node, static or a Git clone, created in your workspace (the tools run in containers). **Ghost Mode** clones a project, and optionally its database, to experiment.
- **Database**, **Logs**, **X-Ray**, **Settings** — database manager, log viewer, request inspector and preferences.

### Domains and ports

Projects are served on `http://<name>.localhost`. Browsers resolve `*.localhost` to your machine automatically — no hosts file, no admin rights.

SLD's router uses ports 80/443. If another app already uses them, choose others with `SLD_HTTP_PORT` / `SLD_HTTPS_PORT` (e.g. 8888 / 8443); URLs then include the port: `http://shop.localhost:8888`.

### HTTPS

SLD creates a local certificate authority and a certificate per project. Download the CA from **Setup → Download CA certificate** and trust it once (Windows: *Install Certificate → Local Machine → Trusted Root Certification Authorities*; macOS: Keychain Access → *Always Trust*; Firefox: *Settings → Certificates → Import*). Plain `http://` works without it. Enable HTTPS per project with `domain.tls: true`.

## Project configuration

`sld` writes `.sld/config.yaml` when you set a project up. Examples:

```yaml
# A Laravel app with a Vite dev server, MySQL, Redis and Mailpit
version: 2
name: shop
apps:
  web:  { runtime: php, version: "8.3", web_root: public }
  vite: { runtime: node, command: "npm run dev -- --host 0.0.0.0", port: 5173 }
services:
  database: { type: mysql }
  redis:    { enabled: true }
  mailpit:  { enabled: true }
```

```yaml
# An existing Docker Compose project in a sub-folder (sunport-apps/backend)
version: 2
name: sunport                 # domain: sunport.localhost
apps:
  backend: { runtime: compose, path: backend, service: nginx, port: 80 }
```

```yaml
# A Next.js frontend and a FastAPI backend
version: 2
name: tailorly
apps:
  web: { runtime: node, command: "npm run dev -- -H 0.0.0.0 -p 3000", port: 3000 }
  api: { runtime: python, command: "uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload", port: 8000 }
services:
  database: { type: postgres }
```

Runtimes: `php`, `static`, `node`, `python`, `ruby`, `go`, `image`, `dockerfile`, `proxy`, `compose`. The first app owns the root domain; others get `<app>.<name>.localhost`. Dependencies are installed inside the containers (in a per-app volume) and reinstalled when the lockfile changes. Version 1 files keep working; `sld config migrate` rewrites them.

Add a `.sldignore` file to a folder to keep SLD from treating it as a project.

### Databases

- **Per project:** `services.database: { type: mysql | mariadb | postgres }` gives the project its own database container. Credentials are generated into `.sld/secrets.json` (keep it out of git) and passed to the app as `DB_HOST`, `DB_DATABASE`, `DATABASE_URL`, … The data lives in a Docker volume that survives stop, restart and remove; only *Remove* with *Also delete data volumes* ticked (`sld destroy --volumes`) deletes it.
- **Shared:** the compose setup includes MariaDB (and PostgreSQL with `--profile postgres`). The PostgreSQL plugin adds one more shared server. Point a project at one in its own `.env` and manage it from **Database**.
- **Compose projects** keep their own database services and volumes.

## Command line (without the dashboard)

The same engine is available as the `sld` CLI for people who don't run the image. From a project folder:

```bash
sld init                  # start the router, create the local CA
sld scan [dir]            # list projects and the domain each gets (--write creates configs)
sld init-project          # detect the stack and write .sld/config.yaml (--dry-run to preview)
sld start | stop | restart | status | logs [app] | destroy [--volumes]
sld shell [app]           # shell in an app container (runs as your user)
sld npm|php|composer|artisan|python|pip|rails|go …   # tools inside the project
sld serve                 # run the dashboard + API yourself
```

Scripts written for the earlier `sld v2 …` commands keep working: `v2` is an alias.

Binaries for Linux, macOS and Windows are attached to each GitHub release.

## Security

The SLD container controls Docker through its socket, which is equivalent to root access on the machine. SLD therefore binds to `127.0.0.1` only, requires the admin token for every API call, rejects requests for unknown host names (DNS rebinding), and requires a CSRF header plus a local `Origin` on browser writes. Run it only on your own development machine, never as an internet-facing service.

## Development

```bash
go test ./...                      # engine, server and services tests
go build -o sld ./cmd/sld          # CLI / server
cd sld-dashboard && npm ci && npm run dev   # dashboard on :5173 (proxies /api to :2025)
make e2e SLD_IMAGE=sld:local       # end-to-end test of an image (own router and network; safe next to a running SLD)
```

Architecture and decisions: [`docs/architecture`](docs/architecture) and [`docs/adr`](docs/adr); the V2 plan is [`docs/architecture/SLD_V2_PHASE15_CUTOVER.md`](docs/architecture/SLD_V2_PHASE15_CUTOVER.md).

## License

MIT
