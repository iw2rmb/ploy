# Ploy — Stateless Workflow Runner

Ploy operates as an on-demand workflow brain: it consumes Grid events, resolves workflow DAGs, submits work back to Grid, and exits. The repository now focuses entirely on that CLI-driven experience; the feature slices below replaced the legacy API, Nomad, Consul, and SeaweedFS footprint.

## Operating Model
- Grid owns the control surface (webhooks, scheduling, cache promotion, node pools) and persists hot signals in JetStream plus cold artifacts in IPFS.
- Ploy consumes those streams, assembles the mods/workflow DAG, and submits follow-up jobs back to Grid via the workflow RPC client.
- Every checkpoint, artifact pointer, and decision is written to JetStream/IPFS so runs stay stateless and retries never depend on long-lived services.

## Feature Highlights
- [x] Legacy teardown — repository scoped to the CLI-only stub and guardrail tests (Roadmap 00).
- [x] Event contracts — JetStream subject map, schema enforcement, and in-memory stubs for offline work (Roadmap 01).
- [x] Workflow runner CLI — reconstructs DAGs, streams checkpoints, and exits cleanly after dispatching jobs (Roadmap 02).
- [x] Lane engine — deterministic lane specs under `configs/lanes/*.toml` with `ploy lanes describe` previews (Roadmap 03).
- [x] Snapshot toolkit — `ploy snapshot plan` / `ploy snapshot capture` with strip/mask/synthetic rules baked in (Roadmap 04).
- [x] Integration manifests — manifest compiler enforcing topology, fixtures, and lane allowlists (Roadmap 05).
- [x] Commit environments — `ploy environment materialize` assembles `<sha>-<app>` builds with cache hydration (Roadmap 06).
- [x] Aster hook — exposes AST-pruned bundles and workflow toggles inside stage metadata (Roadmap 07).
- [x] Documentation refresh — doc set aligned around the CLI-first model and GRID hand-off (Roadmap 08).
- [x] Cache coordination — checkpoints carry lane cache keys for Grid reuse (Roadmap 09).
- [x] JetStream workflow client — live NATS connectivity with stub fallback toggled by ``JETSTREAM_URL`` (Roadmap 10).
- [x] Lane documentation hardening — schema enforcement and lane reference updates (Roadmap 11).
- [x] Snapshot validation — cross-engine verification with coverage guardrails (Roadmap 12).
- [x] Integration manifest schema — JSON schema + CLI validation hook for manifests (Roadmap 13).
- [x] Grid workflow client — workflow stages submit through the Grid RPC when ``GRID_ENDPOINT`` is set (Roadmap 14).
- [x] IPFS artifact publishing — snapshot captures stream artifacts through ``IPFS_GATEWAY`` when available (Roadmap 15).
- [x] Snapshot metadata streams — capture fingerprints and rule counts published to JetStream (Roadmap 16).
- [x] Checkpoint enrichment — stage metadata and artifact manifests embedded in workflow checkpoints (Roadmap 17).
- [x] Stage artifact streams — dedicated JetStream envelopes for stage artifacts to feed cache hydrators (Roadmap 18).
- [x] Mods parallel planner — orchestrates orw/LLM/human stages with Grid-aware parallelism (Roadmap 19, see `docs/design/mods/README.md`).
- [x] Knowledge base remediation — classifies errors, surfaces CLI ingest/evaluate workflows, and seeds `llm-plan` with suggestions (Roadmap 20, see `docs/design/knowledge-base/README.md`).
- [ ] Build gate reboot — Grid-integrated static checks and log parsing across languages (Roadmap 21, see `docs/design/build-gate/README.md`).

Full design records live in `docs/design/README.md`.

## Removed Components
- Nomad/Consul/Traefik templates, wrappers, and deployment logic (`internal/orchestration`, embedded HCL, Ansible playbooks).
- Long-running API/service binaries, routing assumptions, and controller ingress paths.
- SeaweedFS-specific artifact plumbing now replaced by IPFS publishers.
- Legacy lane descriptors tied to Nomad job specs and system job scripts.
- Obsolete docs or runbooks referencing the retired controller deployment path.

## Data & Storage Expectations
- JetStream carries events, run metadata, cache coordination signals, and artifact manifests.
- IPFS (or compatible object storage) stores build outputs, DB snapshot archives, diff reports, and audit logs.
- Workspace metadata (hash IDs, eviction policies, ownership) ensures Grid can claim/release caches without bespoke scripting.

## Testing & Tooling Focus
- Unit and CLI tests exercise the JetStream/Grid stubs locally; integration work against live Grid resumes once JetStream wiring completes.
- Cadence and coverage thresholds stay governed by `AGENTS.md`.
- Workspace commands (`make build`, `make test`) remain workstation-first; no VPS/Grid state is required for the slices above.

## Success Criteria
- Mods workflows complete end-to-end through Grid with faster build/test cycles than the legacy Nomad runs.
- Developers request deterministic `<sha>-<app>` environments with lane caches, manifests, and snapshots applied automatically.
- No permanent services are required; when the CLI is idle, Grid continues queuing work for the next invocation.

## Getting Started
1. **Clone & build**
   ```bash
   git clone https://github.com/iw2rmb/ploy
   cd ploy
   make build
   ```
2. **Inspect lane metadata**
   ```bash
   ./dist/ploy lanes describe --lane go-native --commit HEAD --snapshot dev-db --manifest smoke --aster plan,exec
   ```
   The command parses `configs/lanes/go-native.toml`, previews the composed cache key, and lists the build/test commands bound to that lane.
3. **Run the workflow CLI**
   ```bash
   JETSTREAM_URL=nats://127.0.0.1:4222 ./dist/ploy workflow run --tenant acme --ticket auto
   ```
   With ``JETSTREAM_URL`` set the CLI connects to JetStream, claims the next ticket, and publishes checkpoints on the real stream. When omitted it boots the in-memory stub for offline development.
4. **Preview snapshot rules**
   ```bash
   ./dist/ploy snapshot plan --snapshot dev-db
   ```
   The plan command loads `configs/snapshots/dev-db.toml`, summarises strip/mask/synthetic rules, and highlights which tables/columns are affected before a capture runs.
5. **Capture a snapshot (stub)**
   ```bash
   ./dist/ploy snapshot capture --snapshot dev-db --tenant acme --ticket SNAPSHOT-1
   ```
   Capture applies the configured rules against `configs/snapshots/dev-db.json`, hashes the result, uploads the payload to the configured IPFS gateway (or the in-memory stub when unset), and publishes metadata through the current stub path.
6. **Dry-run a commit-scoped environment**
   ```bash
   ./dist/ploy environment materialize deadbeef --app commit-app --tenant acme --dry-run
   ```
   Dry-run mode compiles the `commit-app` manifest, verifies required snapshots (`commit-db`, `commit-cache`), and previews cache keys for each required lane without mutating state.
7. **Tests**
   ```bash
   make test
   ```
   Unit tests assert that only the workflow CLI remains and that the event contract schema stays consistent.

8. **Manage knowledge base incidents**
   ```bash
   ./dist/ploy knowledge-base ingest --from ./fixtures/knowledge-base/new-incidents.json
   ```
   The ingest command merges incident fixtures into `configs/knowledge-base/catalog.json`, skipping duplicate IDs while keeping workstation runs deterministic.

## Feature Roadmap
Per-feature write-ups live under `roadmap/shift/` (directory name retained for historical context). Status checkboxes in this README mirror those roadmap entries, and deeper design context is collected in `docs/design/README.md`.

## Environment Placeholders
Workstation builds still rely on the in-memory Grid stub; JetStream is optional. Keep the following environment variables handy:
- ``JETSTREAM_URL`` — JetStream endpoint used by the workflow runner and snapshot publisher (optional; falls back to stub).
- ``GRID_ENDPOINT`` — Workflow RPC host used to submit jobs back to Grid.
- ``IPFS_GATEWAY`` — Gateway for retrieving snapshot artifacts published by `ploy snapshot capture`.

## Contributing
Follow the contributor workflow in `AGENTS.md` and keep docs aligned with `docs/DOCS.md`.

## License
The project inherits its existing license terms; consult `LICENSE` if/when it is reintroduced in a future slice.
