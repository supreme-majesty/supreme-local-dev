# ADR 0005: Local Secret Management & Random Credentials

## Status
Accepted

## Context
Early designs considered generating database credentials deterministically from project paths or hashes, or using static default credentials like `sld_secret`. Deterministic or static credentials weaken local security, making passwords predictable across projects and environments.

## Decision
SLD V2 adopts a random, persistent local secret model:
- Passwords for project databases and auxiliary services are generated using cryptographically random strings (`crypto/rand`).
- Credentials are saved to `.sld/secrets.json` (or `.sld/.env`) within the project root.
- The `.sld/secrets.json` file is gitignored with `0600` file permissions.
- Containers receive credentials via environment variables injected at container creation.
- Secrets survive container restarts and normal `sld destroy`. They are removed only during explicit `sld destroy --volumes`.

## Consequences
### Positive
- High security standards with zero predictable passwords.
- No secrets committed to `.sld/config.yaml`.
- Simple local file-based storage with no external secret management daemons needed.

### Negative
- Developers must use `sld status` or inspect `.sld/secrets.json` if connecting an external database GUI (e.g. TablePlus) to the local database.
