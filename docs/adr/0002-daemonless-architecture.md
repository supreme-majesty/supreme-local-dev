# ADR 0002: Daemonless Architecture

## Status
Accepted. Amended by [ADR 0008](0008-containerised-control-plane.md): SLD itself now also runs as a container (the control plane), still without changing the host.

## Context
SLD V1 required a background daemon running on port 2025 as a systemd unit (`sld-daemon.service`) with elevated privileges (`root`/`sudo`). The daemon maintained a global state file (`/var/lib/sld/state.json`) and hosted an unauthenticated REST API that allowed any local process to execute commands with root privileges.

## Decision
SLD V2 eliminates the background daemon, systemd unit, port 2025 HTTP API, and global `state.json` file. The `sld` CLI runs exclusively in user-space as a transient command-line tool. Docker Engine is the persistent background process.

## Consequences
### Positive
- Removes major security vulnerabilities (no unauthenticated local HTTP daemon, no root escalations).
- Eliminates brittle OS service management (`systemctl`, `sc.exe`, `launchctl`).
- Eliminates state synchronization bugs from global `state.json`.

### Negative
- Commands must query the Docker API to discover running project state rather than reading an in-memory cache. (Docker API latency is negligible for local CLI operations: < 20ms).
