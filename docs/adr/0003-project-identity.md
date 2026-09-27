# ADR 0003: Deterministic Project Identity

## Status
Accepted

## Context
In SLD V1, projects were identified solely by their folder basename (e.g. `api`). If a developer worked on multiple clients with projects sharing the same directory name (e.g. `/home/user/client-a/api` and `/home/user/client-b/api`), resource collisions occurred across container names, network bridges, domains, and database volumes.

## Decision
SLD V2 adopts a deterministic project identity algorithm based on the canonical project path:
```
canonical path ──> SHA-256 ──> first 8 hex characters ──> <slug>-<hash8>
```
1. Resolve symlinks and clean absolute path.
2. Extract directory basename and sanitize into a slug (alphanumeric, lowercase, max 20 chars).
3. Compute SHA-256 of canonical path and take first 8 hex characters.
4. Combine into `<slug>-<hash8>` (e.g., `api-3f7b49a1`).

## Consequences
### Positive
- 100% deterministic and reproducible across repeated `sld start` commands.
- Guaranteed collision avoidance between identically named directories across different paths.
- Clean, readable Docker resource names (`sld-api-3f7b49a1-nginx`).
- Avoids non-deterministic UUID randomness.

### Negative
- Renaming or moving the project directory on disk produces a new project ID, requiring an explicit migration if data volumes need to follow the renamed path.
