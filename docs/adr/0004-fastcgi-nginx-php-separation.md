# ADR 0004: FastCGI Separation (Traefik → Nginx → PHP-FPM)

## Status
Accepted

## Context
Initial draft architecture diagrams considered routing HTTP traffic directly from Traefik to PHP-FPM containers. However, PHP-FPM speaks FastCGI, not HTTP. Direct routing would either fail or require custom HTTP-to-FastCGI middleware inside Traefik, while also forcing PHP-FPM to serve static assets (.css, .js, images), which it does poorly.

## Decision
SLD V2 introduces a dedicated, lightweight Nginx container (`sld-nginx:1`) for each project:
- Traefik routes HTTP/HTTPS to project Nginx over `sld-global`.
- Project Nginx serves static assets directly from `/app/public` without waking PHP-FPM.
- Project Nginx proxies dynamic / `.php` requests over FastCGI (TCP port 9000) to `sld-php` on `sld-{id}-net`.
- PHP-FPM remains an internal service connected only to the project network.

## Consequences
### Positive
- Strict web server / application tier separation adhering to production PHP hosting patterns.
- High performance static asset delivery.
- PHP-FPM is not exposed to the global routing network.

### Negative
- Adds one lightweight Nginx container per active project (~15MB RAM per project).
