# SLD V2 Security Model & Boundaries

> **See also** [ADR 0010](../adr/0010-control-plane-authentication.md) for the control plane's authentication (token, session cookie, Host allow-list, CSRF), added when SLD started running as a container with a dashboard.

This document defines the security architecture, trust boundaries, privilege separation, and credential handling for Supreme Local Dev (SLD) V2.

---

## 1. Privileges & User-Space Operation

### Elimination of Elevated Daemon
V1 operated an elevated background daemon (`pkg/daemon/api/server.go`) running as `root` with `sudo` execution wrappers (`elevate()` in `cmd/sld/main.go`). This introduced critical security risks:
- An unauthenticated HTTP API listening on `localhost:2025` with full root execution capability.
- Root-owned files created in developer workspace directories (`/var/lib/sld`).
- OS package manager mutations running with full system privileges.

### V2 Principle: Zero Host Daemon
- The `sld` CLI runs entirely in user-space under the unprivileged developer account.
- The user interacts with Docker via standard Docker group membership (Linux) or Docker Desktop / OrbStack socket permissions (macOS/WSL2).
- Routine commands (`sld start`, `sld stop`, `sld artisan`, `sld composer`, `sld logs`) require **zero** sudo privileges.
- Sudo is strictly isolated to one-time host integration during `sld init` (e.g., adding `/etc/resolver/test` on macOS). Sudo is never invoked silently.

---

## 2. Docker Socket Security Boundary

### Accurate Threat Modeling of the Docker Socket
Mounting the Docker socket (`/var/run/docker.sock`) grants root-equivalent control over the host operating system. A read-only mount (`:ro`) prevents writing directly to the socket file itself, but **does not constitute a least-privilege security boundary** against Docker API calls. Through read-only API access, a process can:
- Inspect all containers running on the host.
- Extract cleartext environment variables and secrets from container inspect metadata.
- Monitor all network attachments, volumes, and events across unrelated projects.

### Two-Stage Docker Socket Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                           MVP Phase                             │
│                                                                 │
│  SLD CLI (Host) ─────────────► /var/run/docker.sock             │
│  sld-router (Traefik) ───────► /var/run/docker.sock:ro          │
│                                                                 │
│  * Trust Boundary: Docker socket access is trusted by SLD.      │
│  * Project Containers: ZERO Docker socket access permitted.     │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Hardened V2.x Phase                         │
│                                                                 │
│  sld-router (Traefik) ───────► sld-socket-proxy (Socket Proxy)  │
│                                           │                     │
│                                   Filters API Calls:            │
│                                   ALLOW: GET /v1.*/containers/* │
│                                   DENY:  POST, DELETE, secrets  │
│                                           │                     │
│                                           ▼                     │
│                                 /var/run/docker.sock            │
└─────────────────────────────────────────────────────────────────┘
```

### Strict Container Isolation Rule
**Project containers (`sld-php`, `sld-nginx`, `sld-db`, `sld-redis`, `sld-node`) MUST NEVER receive the Docker socket under any circumstance.** Application vulnerabilities (e.g. Remote Code Execution in a PHP package) cannot escalate to host container control.

---

## 3. Secret Management Architecture

### Prohibition of Deterministic Secrets
In previous iterations, local database passwords were sometimes derived deterministically (e.g. hashing the project path or using a fixed string like `sld_secret`). This is insecure because:
- Anyone with knowledge of the path algorithm can reconstruct database credentials.
- Compromising one local environment exposes credentials across related local environments.

### V2 Local Secret Specification
1. **Cryptographically Secure Random Generation**:
   When a service requiring credentials (e.g., MySQL root or application user) is first provisioned for a project, SLD generates a random 32-character string using `crypto/rand`.
2. **Local Storage in Gitignored Directory**:
   Credentials are saved to `.sld/secrets.json` (or `.sld/.env`) within the project root.
   - The `.sld/secrets.json` file is explicitly added to `.gitignore`.
   - File permissions are set to `0600` (readable/writable only by the owner).
3. **Environment Injection**:
   The reconciler reads the local secret store and injects credentials directly into the container environment (`MYSQL_PASSWORD`, `DB_PASSWORD`) at container creation time.
4. **Lifecycle**:
   - Secrets persist across `sld stop`, `sld start`, and `sld restart`.
   - Secrets are deleted only when the project is destructively removed via `sld destroy --volumes`.

---

## 4. Host Filesystem & UID/GID Parity

### Preventing Root File Hijacking
When Docker containers run as `root` (UID 0), files created by commands like `composer install`, `php artisan make:model`, or `npm install` become owned by root on the host filesystem. This prevents developers from editing or deleting their own files without `sudo`.

### V2 Parity Rules
1. **Dynamic UID/GID Mapping**:
   The CLI discovers the host user's UID and GID:
   ```go
   uid := os.Getuid()
   gid := os.Getgid()
   ```
   These values are passed to container creation as environment variables:
   ```ini
   WWWUSER=1000
   WWWGROUP=1000
   ```
2. **Entrypoint User Switching**:
   Container entrypoints configure the internal service user (`www-data` or `node`) to match `WWWUSER` and `WWWGROUP` before executing the service process.
3. **No Recursive Chown on Startup**:
   Containers MUST NOT execute `chown -R` across the entire `/app` mount on startup. Recursive ownership changes on large codebases cause severe I/O lag (especially on macOS and WSL2). Only container-internal runtime directories (e.g. `/var/log/nginx`, `/var/run/php-fpm`) are adjusted.

---

## 5. Network Boundary Isolation

- **Default Zero Host Port Exposure**: Project databases (MySQL, Postgres, MariaDB) and caching layers (Redis) are private to `sld-{project-id}-net`. They do not publish ports on `0.0.0.0` or `127.0.0.1` on the host by default, preventing unauthorized access across local area networks.
- **Cross-Project Isolation**: Bridge networks are isolated per project ID. Container `sld-app1-php` cannot reach `sld-app2-db` over the network.
