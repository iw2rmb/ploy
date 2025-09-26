# Ploy Shift Design

## Purpose
Reboot Ploy as an on-demand workflow brain that evaluates mods DAGs, emits Grid jobs, and quits. No long-lived services, no Nomad ballast, and zero fallbacks to the legacy API.

## Context
- Source plan: `SHIFT.md` in this repo plus `../grid/SHIFT.md` for Grid’s responsibilities (no deep-dive doc exists at `../grid/docs/design/shift/SHIFT.md`).
- Ploy must stay workstation-first: run via CLI, hydrate state from JetStream/IPFS, and leave the execution/control plane to Grid.
- We explicitly drop Nomad/Consul/Traefik scaffolding, SeaweedFS-specific flows, and lane metadata tied to old job specs.

## Goals
- Stateless CLI (`ploy workflow run`) that retrieves one ticket, reconstructs the DAG (mods, build gate, lanes), publishes jobs to Grid, and exits fast.
- JetStream contracts capture every decision, artifact pointer, retry checkpoint, and cache coordination signal.
- IPFS (or compatible object store) stores long-lived artifacts (snapshots, diffs, audit logs) with content addressing Ploy can embed in events.
- Lane definitions become build profiles optimised for hot runtimes (WASM Node/C++, GraalVM incremental Java, etc.) with cache keys calculated in Ploy and respected by Grid.
- Snapshot tooling lives inside Ploy: connect to sources, apply strip/mask rules, generate synthetic data, and publish diff metadata back to JetStream/IPFS.
- Integration manifests (TOML/Markdown) describe topology, fixtures, lanes, and Aster toggles; Ploy validates them and passes compiled intent to Grid.
- Commit-scoped environments (`<sha>-<app>`) resolve deterministically from caches, manifests, and snapshots—no service stays resident.
- Aster hook surfaces AST-pruned code bundles as first-class cache strata and toggles per workflow step.

## Non-Goals
- No backups to Nomad, Consul, Traefik, or SeaweedFS.
- No compatibility layer for the legacy API or deployment flows.
- No bespoke Grid shims—rely on the contract described in `../grid/SHIFT.md`.

## Legacy Decommission Plan
- **Controller/Service binaries** – Delete `cmd/ployd`, API handlers under `internal/api`, HTTP routers, and Nomad job manager wrappers. Replace entrypoints with CLI-only commands.
- **Nomad/Consul/Traefik plumbing** – Remove `internal/orchestration`, embedded HCL specs, Consul KV accessors, `iac/dev` Ansible playbooks targeting Nomad, and any `bin/ployman` deploy flows tied to long-lived services.
- **SeaweedFS artifact pipeline** – Excise upload/download helpers, configuration flags, and docs that assume SeaweedFS. Substitute IPFS-compatible publishers/readers owned by the snapshot toolkit.
- **Legacy lane metadata** – Purge Nomad-specific lane descriptors, templates under `configs/lanes/nomad*`, and build/test scripts that rely on system jobs.
- **Build-gate service integrations** – Remove API-specific webhooks, Cron jobs, and background workers; ensure SBOM/vuln/static analysis hooks operate as Grid-submitted jobs only.
- **Obsolete docs/tooling** – Delete docs, runbooks, and scripts referencing the controller deployment path, Nomad admin commands, or Consul maintenance. Update onboarding checklists to point at the CLI.

## Architecture Outline
1. **JetStream Subjects**
   - `grid.webhook.<tenant>` (Grid-owned) delivers tickets that Ploy claims via pull consumer.
  - `ploy.workflow.<ticket>.checkpoints` stores DAG reconstructions, lane assignments, cache keys, stage metadata, artifact manifests, and retry markers. Each checkpoint now carries the computed lane cache key and stage context so Grid can reason about cache reuse and artifact availability without introspecting stage payloads.
   - `ploy.artifact.<ticket>` publishes IPFS hashes for build outputs, DB snapshot bundles, and diff reports.
   - `grid.status.<ticket>` (Grid-owned) streams job lifecycle events that the CLI consumes before exit.
2. **Workflow Runner CLI**
   - Single binary invoked by operators or Grid when work appears; default command `ploy workflow run --ticket auto`.
   - Uses NATS JS durable consumers, reconstructs DAG from mod definitions + integration manifests, emits Grid job specs through the Workflow RPC (HTTP client toggled via ``GRID_ENDPOINT`` with an in-memory fallback).
   - Persists minimal local state (ephemeral temp dirs) and wipes them post-run.
3. **Lane Engine**
   - Lanes defined in `configs/lanes/*.toml` referencing runtime families, cache namespaces, and build/test commands.
   - Cache keys incorporate lane, commit SHA, Aster toggle, snapshot fingerprint, and manifest version.
   - Expose `ploy lanes describe <lane>` for developers to inspect runtime assumptions.
4. **Snapshot Toolkit**
   - `ploy snapshot plan` to preview strip/mask/synthetic rules.
   - `ploy snapshot capture` produces IPFS artifacts and posts metadata to JetStream.
   - Replays snapshots locally using lightweight containers matching Grid runtime images.
5. **Integration Manifest Processing**
   - Validate schema (TOML + embedded Markdown) before a run; fail fast if topology or fixtures are inconsistent.
   - Compile allowlist flows (`A->B`, `B->C`) and required fixtures into JSON passed to Grid’s topology compiler.
6. **Commit-Scoped Environments**
   - `ploy environment materialize <sha> --app <id>` hydrates caches, snapshots, manifests, and kicks off required lanes through Grid.
   - Supports dry-run to show resources that would be touched.
7. **Aster Hook**
   - Discover Aster-generated AST-pruned bundles per workflow step.
   - Emit metadata into cache keys and attach bundle pointers to job submissions so Grid can select appropriate runtime accelerators.
   - Allow operators to toggle bundles per stage via CLI flags and surface bundle provenance after each workflow run.

## Testing Strategy
- All unit tests run locally against JetStream/Grid stubs (no Nomad fakes).
- Provide focused suites for DAG reconstruction, cache-key math, snapshot diffing, integration manifest parsing, and Aster hooks.
- Integration/E2E coverage moves to the VPS (Grid) once CLI + stubs land; these runs validate the Workflow RPC and Grid callbacks.
- Target coverage ≥60% overall, ≥90% on critical path packages (workflow runner, snapshot toolkit, manifest parser).

## Open Questions
- Finalise the Grid Workflow RPC schema once the upstream spec lands (current client uses a provisional JSON envelope).
- IPFS gateway availability for developers without direct cluster access.
- Versioning strategy for integration manifests when multiple teams share the same app but diverge on topology requirements.

## Next Steps
- ✅ Completed 2025-09-26: Harden lane-spec documentation (`docs/LANES.md`) and keep CLI examples (`ploy lanes describe`) in sync with TOML schema updates.
- ✅ Completed 2025-09-26: Validate snapshot tooling against representative databases (Postgres, MySQL, document store).
- ✅ Completed 2025-09-26: Draft integration manifest schema ahead of the workflow runner wiring slice.
- ✅ Completed 2025-09-26: Implement IPFS artifact publishing once the gateway (`IPFS_GATEWAY`) is provisioned for workstation slices (see `docs/design/ipfs-artifacts/README.md`).
- ✅ Completed 2025-09-26: Stream snapshot metadata to JetStream when `JETSTREAM_URL` is configured, keeping offline slices on the in-memory stub.
- ✅ Completed 2025-09-26: Mirror workflow stage artifacts to the JetStream artifact stream (see `docs/design/stage-artifacts/README.md`).
