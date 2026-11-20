# SLD V2 Dockerization — Closed

This tracker is closed (2026-11-20). The dockerization it followed is complete:

- **Phases 1–13:** the V2 engine: images, Docker client, labels, config, identity, reconciliation, secrets, TLS, orchestrator, CLI, detection, CI images.
- **Phase 14:** end-to-end tests in container mode: `scripts/e2e.sh`, `make e2e`, `.github/workflows/e2e.yml`.
- **Phase 15:** the cutover:
  - SLD runs as one container with an authenticated dashboard;
  - multi-runtime projects and existing compose stacks;
  - the V1 host layer deleted;
  - the project commands are the top-level CLI (`sld start`, `sld scan`, ...).

Where to look now:
- Current architecture: [`docs/architecture/SLD_V2_ARCHITECTURE.md`](docs/architecture/SLD_V2_ARCHITECTURE.md)
- Decisions: [`docs/adr`](docs/adr) (0007–0010 cover the cutover)
- Milestone notes and what is still open: [`docs/architecture/SLD_V2_PHASE15_CUTOVER.md`](docs/architecture/SLD_V2_PHASE15_CUTOVER.md)
