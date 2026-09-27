# ADR 0006: Declarative Reconciliation Model

## Status
Accepted

## Context
A naive reconciler might destroy and recreate all containers whenever any configuration hash changes. This causes unnecessary container downtime, flushes in-memory caches, and risks data disruption if volumes are misconfigured.

## Decision
SLD V2 implements a declarative reconciler that computes fine-grained diffs between Desired State (`.sld/config.yaml`) and Actual State (Docker labels `com.supreme.sld.*`):
- **Mutable changes** (e.g. labels, network connect/disconnect) are applied in-place without container recreation.
- **Immutable changes** (e.g. base image version change, bind mount path changes) trigger graceful container replacement.
- **Volume persistence**: Persistent data volumes (`sld-{id}-db-data`) are permanently preserved across container recreations and normal `sld destroy` operations.
- State checks are completely idempotent: running `sld start` against an unchanged configuration is a zero-cost `NoOp`.

## Consequences
### Positive
- Fast, minimal-mutation lifecycle.
- Maximum protection against accidental database data loss.
- Predictable and testable state machine.

### Negative
- Requires a more sophisticated diffing engine than simple container recreation.
