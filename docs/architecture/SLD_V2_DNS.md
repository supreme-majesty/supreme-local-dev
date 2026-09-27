# SLD V2 DNS Architecture & Resolution Strategy

This document defines the separation of concerns between DNS resolution and HTTP routing, evaluates host port 53 conflict constraints, and outlines the platform-specific resolution strategies for SLD V2.

---

## 1. Separation of Concerns: DNS vs. HTTP Routing

A common source of confusion in local development orchestrators is conflating DNS resolution with application routing. SLD V2 enforces a strict separation:

```
┌──────────────────────────────────────────────────────────────┐
│                    1. DNS Resolution Layer                   │
│                                                              │
│  Browser: "What IP address hosts the domain `my-app.test`?"   │
│  DNS Answer: "127.0.0.1"                                     │
└──────────────────────────────┬───────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────┐
│                    2. HTTP Routing Layer                     │
│                                                              │
│  Browser sends HTTP request to 127.0.0.1:80 with:            │
│  Host: my-app.test                                           │
│  Traefik (sld-router) receives request, inspects Host header,│
│  and routes to container: sld-my-app-3f7b49a1-nginx          │
└──────────────────────────────────────────────────────────────┘
```

**DNS does NOT perform application routing.** DNS only resolves the hostname to the loopback IP `127.0.0.1`. Traefik is responsible for routing to the appropriate project container.

---

## 2. Port 53 Constraint & The Nonexistent Resolver Fallacy

### The Flawed Assumption
Earlier designs proposed adding the following to Linux `systemd-resolved`:
```ini
[Resolve]
DNS=127.0.0.1
Domains=~test
```
**This is fundamentally invalid unless an actual DNS server (such as dnsmasq or CoreDNS) is actively bound and listening on `127.0.0.1:53`.**
Pointing systemd-resolved to `127.0.0.1:53` when no daemon is listening causes DNS queries for `*.test` to hang and timeout, breaking all local development.

### Port 53 Binding Conflicts
On modern Linux distributions (Ubuntu 22.04+, Debian 12, Fedora):
- `systemd-resolved` binds to `127.0.0.53:53` by default.
- Trying to bind a Docker container to host `0.0.0.0:53` or `127.0.0.1:53` will often fail with `bind: address already in use`.
- Running a Dockerized DNS server on port 53 requires elevated privileges and complex host resolver reconfigurations.

---

## 3. Platform Resolution Strategies (MVP)

SLD V2 avoids fragile port 53 container bindings for MVP, selecting the cleanest native mechanism per platform:

### 1. macOS (Darwin)
- **Mechanism**: Native resolver configuration file at `/etc/resolver/test`.
- **Configuration**:
  ```text
  # /etc/resolver/test
  nameserver 127.0.0.1
  ```
- **Behavior**: macOS includes native support for per-TLD resolvers. When a local dnsmasq or resolver runs, macOS queries it. For MVP, SLD can sync `/etc/hosts` or write `/etc/resolver/test` when a local forwarder is present.
- **Privilege**: One-time sudo during `sld init`.

### 2. Linux (Ubuntu, Debian, Arch)
- **Primary MVP Strategy**: Local Hosts Synchronization.
  - When `sld start` brings up a project with domain `my-app.test`, SLD ensures an entry exists in `/etc/hosts`:
    ```text
    127.0.0.1 my-app.test
    ```
  - SLD provides a clean, user-space helper or one-time host integration that manages a dedicated SLD block in `/etc/hosts`:
    ```text
    ### BEGIN SLD MANAGED DOMAINS ###
    127.0.0.1 my-app.test
    127.0.0.1 api-client.test
    ### END SLD MANAGED DOMAINS ###
    ```
  - Zero port 53 conflicts. Works on 100% of Linux distributions regardless of whether `systemd-resolved`, `NetworkManager`, or plain `resolv.conf` is in use.

### 3. Windows / WSL2
- **Architecture**: In WSL2, web browsers run on the Windows host, while Docker Engine runs inside WSL2.
- **Strategy**:
  - The Windows host resolver handles browser queries.
  - Port 53 bindings in WSL2 do not reliably intercept Windows host DNS queries.
  - The recommended MVP approach is synchronizing active `.test` domains to `C:\Windows\System32\drivers\etc\hosts` via an automated WSL2 helper script during `sld start`, or providing clear guidance.

---

## 4. Future Hardened Strategy (V2.x)

In a future hardening phase, SLD may offer an opt-in containerized DNS forwarder (`sld-dns` running CoreDNS or dnsmasq):
- Binds to an isolated loopback alias (e.g., `127.0.0.54:53`) rather than `127.0.0.1:53`.
- Integrates with systemd-resolved using:
  ```ini
  [Resolve]
  DNS=127.0.0.54
  Domains=~test
  ```
- This will only be enabled after verifying that the DNS daemon is actively running and responding to DNS queries.
