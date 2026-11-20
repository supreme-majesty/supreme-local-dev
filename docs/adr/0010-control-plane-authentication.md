# ADR 0010: Control Plane Authentication

## Status
Accepted (2026-11, Phase 15 M5)

## Context
The control plane (ADR 0008) can create containers with the Docker socket, so anyone who can call its API effectively has root on the machine. V1's daemon had no authentication and allowed every origin (`Access-Control-Allow-Origin: *`): any web page open in the browser could call it. A local web UI is also exposed to DNS rebinding (a hostile domain resolving to 127.0.0.1) and to cross-site request forgery.

## Decision
- **Admin token.** On first start SLD generates a random token and prints it once. Only its hash is stored (`auth.json` in the data directory). `sld serve --reset-token` issues a new one; `SLD_ADMIN_TOKEN` can provide it (used by CI).
- **Every API route requires authentication**, except login, the auth status and `/api/health`:
  - scripts send the token as `Authorization: Bearer <token>`;
  - the dashboard exchanges it once for an `HttpOnly`, `SameSite=Strict` session cookie, so the token never sits in page storage.
- **Browser writes need more than the cookie:**
  - a custom `X-SLD-Request` header, which other sites cannot send without a CORS preflight that SLD does not grant;
  - an `Origin`, when present, on the same local host.
- **Host allow-list:** requests are refused unless the `Host` header is `localhost`, `127.0.0.1`, `[::1]` or a `*.localhost` name (plus names added with `SLD_ALLOWED_HOSTS`). This defeats DNS rebinding.
- **Loopback by default:** the image and compose file publish the API on `127.0.0.1:2025` only, and project ports on `SLD_BIND_HOST` (default `127.0.0.1`).
- **No CORS.** The unauthenticated V1 listener is deleted (M9).

## Consequences
### Positive
- Web pages and other users on the network cannot drive Docker through SLD.
- The token is not recoverable from disk; losing it only requires a restart with `--reset-token`.

### Negative
- One more step on first use (copy the token from the logs).
- Single user, single role: anyone with the token has full control. Acceptable for a developer's own machine, which is the only supported deployment.
