# Supreme Local Dev (SLD) 🚀

High-performance local development environment for PHP/Laravel, built with Go.

SLD provides a seamless experience for managing local web projects on Linux, offering automatic Nginx configuration, SSL management, and easy PHP version switching.

## Features

- **Project Parking**: Serve any directory of projects instantly.
- **Link System**: Create custom domains (e.g., `project.test`) for any path.
- **Automatic SSL**: Native HTTPS support via `mkcert`.
- **PHP Version Manager**: Switch between PHP versions (8.0, 8.1, 8.2, etc.) command.
- **Tools Integration**: Built-in support for phpMyAdmin.
- **GUI Dashboard**: Visual management of your sites and configurations.

## Installation

### Prerequisites

- Linux (Ubuntu/Debian recommended)
- Go 1.25+ (for building)

### Build & Install

```bash
# Clone the repository
git clone https://github.com/supreme-majesty/supreme-local-dev.git
cd supreme-local-dev

# Build the binary
go build -o sld cmd/sld/main.go

# Install binary (optional, or just add to PATH)
sudo mv sld /usr/local/bin/

# Initialize System Dependencies
sudo sld install
```

## Usage

### Basic Commands

- **`sld install`**: Install system dependencies (Nginx, PHP, Dnsmasq, etc.) and configure the environment.
- **`sld park [path]`**: Register a directory. All subdirectories will be served as `http://<dirname>.test`.
- **`sld link [name]`**: Link the current directory to `http://<name>.test`.
- **`sld secure`**: Generate SSL certificates and enable HTTPS for all `.test` domains.
- **`sld gui`**: Open the web-based dashboard.

### Managing Sites

```bash
# Park your projects folder
sld park ~/Developments

# Link a specific project
cd ~/dev/my-project
sld link my-project

# Remove a link
sld unlink my-project

# List all sites
sld paths   # Parked paths
sld links   # Linked sites
```

### PHP Management

Switch the global PHP version used by FPM:

```bash
sld php 8.2
sld php 8.1
```

### Services

```bash
# Check status of Nginx and PHP
sld status

# Start the background daemon (API)
sld daemon
```

## Docker

Runs the **SLD dashboard/API, MariaDB, phpMyAdmin, Redis, and Mailpit** (plus optional **PostgreSQL**) in project-scoped containers (`name: sld`, bridge network `sld-net`) with loopback-only host bindings (`127.0.0.1` by default) and named-volume persistence:

```bash
cp .env.example .env   # optional; override MYSQL_PORT / REDIS_PORT if 3306 or 6379 are already in use
docker compose config -q
docker compose up --build -d
```

If another local project already binds port `3306` or `6379` and you only want the SLD dashboard (which auto-discovers and connects to your running Docker databases):

```bash
docker compose up --build -d --no-deps app
```

### Services & Endpoints

All host ports bind to `${SLD_BIND_HOST:-127.0.0.1}` so local development services are not exposed on LAN/Wi-Fi.

| Service | Host Binding / URL | Credentials / Notes |
|---|---|---|
| **SLD Dashboard / API (`app`)** | [http://127.0.0.1:2025](http://127.0.0.1:2025) | Control plane, Docker DB discovery, SQL import/export, Web UI |
| **phpMyAdmin (`phpmyadmin`)** | [http://127.0.0.1:8080](http://127.0.0.1:8080) | System catalogs automatically filtered |
| **MariaDB (`mariadb`)** | `127.0.0.1:${MYSQL_PORT:-3306}` | User: `root`, Password: `${MYSQL_ROOT_PASSWORD:-sld}` |
| **Redis (`redis`)** | `127.0.0.1:${REDIS_PORT:-6379}` | Key-value store (persistent volume `sld-redis`) |
| **Mailpit Web UI (`mailpit`)** | [http://127.0.0.1:8025](http://127.0.0.1:8025) | Local email inbox (`axllent/mailpit:v1.31.2`) |
| **Mailpit SMTP (`mailpit`)** | `127.0.0.1:${MAILPIT_SMTP_PORT:-1025}` | SMTP capture server |
| **PostgreSQL (`postgres`, profile `postgres`)** | `127.0.0.1:${POSTGRES_PORT:-5432}` | User: `${POSTGRES_USER:-sld}`, DB: `${POSTGRES_DB:-app}` |

### Commands

```bash
# Validate Compose configuration
docker compose config -q

# Start full default stack (app, mariadb, phpmyadmin, redis, mailpit)
docker compose up --build -d

# Start stack including PostgreSQL
docker compose --profile postgres up --build -d

# Start dashboard only (skips bundled MariaDB/Redis if ports 3306/6379 are already taken)
docker compose up --build -d --no-deps app

# Check status & health of containers
docker compose ps

# View logs
docker compose logs -f --tail=100

# Open a shell inside the app container
docker compose exec app sh

# Stop containers safely (preserves all named volumes)
docker compose stop

# Stop & remove containers and network (preserves all named volumes)
docker compose down
```

> **Persistence & Data Safety**: `docker compose stop` and `docker compose down` retain `sld-state`, `sld-mariadb`, `sld-postgres`, and `sld-redis` volumes. Avoid `docker compose down -v` unless you intentionally want to permanently erase all local databases and SLD state.
>
> **Security Note**: The `app` container mounts `/var/run/docker.sock` to discover and connect to local project database containers. Keep `SLD_BIND_HOST=127.0.0.1` and run this Compose stack only on trusted local development workstations, never as an internet-facing service.

## phpMyAdmin

Access phpMyAdmin at:

- http://phpmyadmin.test (Preferred)
- http://localhost/phpmyadmin

## License

MIT
