# SLD V2 Declarative Reconciliation Engine

This document defines the architecture, diffing logic, state transitions, and execution flow of the SLD V2 declarative reconciliation engine.

---

## 1. Principles

1. **Declarative Target**: `.sld/config.yaml` is the single source of truth for the desired project state.
2. **Docker as Runtime Source of Truth**: Container, network, and volume labels (`com.supreme.sld.*`) reflect the actual state.
3. **Idempotence**: Executing `sld start` repeatedly against an unchanged configuration results in a `NoOp` (no container restarts, no network changes).
4. **Volume Preservation**: Persistent data volumes (`sld-{id}-db-data`) are NEVER destroyed during reconciliation. They survive container recreation, image upgrades, and standard `sld destroy`.
5. **Surgical Diffing (Mutable vs. Immutable)**: Recreating every container whenever any configuration hash changes is inefficient and unnecessary. The engine differentiates between mutable and immutable changes.

---

## 2. Reconciliation Pipeline

```
┌─────────────────────────┐
│    .sld/config.yaml     │
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│     Config Loader       │  ── Validate schema, apply defaults, resolve PHP version
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│      Desired State      │  ── Struct containing required Networks, Volumes, Containers
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│      Actual State       │  ── Docker API query filtered by com.supreme.sld.project={id}
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│       Diff Engine       │  ── Categorize deltas into: NoOp, Start, Update, Recreate, Delete
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│       Apply Plan        │  ── Execute minimal Docker API actions in dependency order
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│   Health Verification   │  ── Await service health checks (exponential backoff, 30s max)
└─────────────────────────┘
```

---

## 3. Mutable vs. Immutable Properties

When the configuration changes, the reconciler inspects each altered field to determine whether a resource must be recreated or can be updated in-place:

| Resource Property | Category | Action Required |
|---|---|---|
| **Container Image Tag** (`php:8.2` → `php:8.3`) | Immutable | Gracefully stop old container, remove, create new container, start. |
| **Port Bindings / Host Mappings** | Immutable | Recreate container. |
| **Volume Mount Bind Paths** | Immutable | Recreate container. Persistent data volume is re-attached. |
| **Network Topology** (adding/removing networks) | Mutable | Docker API `NetworkConnect` or `NetworkDisconnect` in-place without restarting container. |
| **Traefik Labels** (domain name changed) | Mutable | In-place label update (or recreation if Docker API requires it for label reload). |
| **Environment Variables** | Immutable (Docker) | Stop old container, create new with updated env, start. |
| **Container State (Stopped → Running)** | State Change | Call `ContainerStart` directly (no recreation). |

---

## 4. State Transition Matrix

| Desired State | Actual Docker State | Evaluated Action | Resulting Execution |
|---|---|---|---|
| Resource required | Does not exist | **Create** | Create resource, attach networks/labels, start. |
| Resource required | Exists, Running, Hash matches | **NoOp** | Instant completion. Zero interruption. |
| Resource required | Exists, Stopped, Hash matches | **Start** | Call `docker container start`. |
| Resource required | Exists, Mutable property changed | **Update** | Apply update in-place (e.g. network connect). |
| Resource required | Exists, Immutable property changed | **Recreate** | Graceful stop (10s timeout), remove container, create new container, attach existing volumes, start. |
| Resource NOT required | Exists (e.g. Redis disabled) | **Remove** | Stop and remove container. Retain any associated volumes. |
| Network required | Does not exist | **CreateNetwork** | Create bridge network `sld-{id}-net`. |
| Volume required | Does not exist | **CreateVolume** | Create persistent volume `sld-{id}-db-data`. |
| Volume required | Exists | **NoOp** | Volumes are permanent data anchors. |

---

## 5. Execution Dependency Order

When applying changes, the reconciler enforces strict topological ordering:

1. **Global Gateway**: Ensure `sld-global` network and `sld-router` (Traefik) exist and are healthy.
2. **Project Network**: Ensure `sld-{project-id}-net` exists.
3. **Data Volumes**: Ensure persistent volumes exist before creating containers that mount them.
4. **Auxiliary Services**: Reconcile Database and Redis containers; await database readiness.
5. **Application Backend**: Reconcile PHP-FPM container with database/redis environment injected; connect to project network.
6. **Project Ingress Web Server**: Reconcile Nginx container; connect to `sld-global` and `sld-{project-id}-net`; apply Traefik labels.
7. **Node Services**: Reconcile managed Node container if `node.dev_server.enabled: true`.

---

## 6. Failure Recovery & Degraded Modes

- **Sibling Independence**: If the Database container fails to start, the reconciler reports the failure, streams the database container's error logs, and marks the project as **Degraded**. It DOES NOT tear down or delete healthy sibling containers (Nginx, PHP).
- **Subsequent Run Recovery**: A subsequent `sld start` re-evaluates the actual Docker state and attempts to heal only the failed or missing components.
