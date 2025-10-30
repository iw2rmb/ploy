# Ploy — Stateless Workflow Runner

Ploy is a workstation‑first workflow runner that talks directly to the Ploy
control plane. It reconstructs the Mods/build/test DAG, streams checkpoints and
logs over SSE, and exits cleanly. The repository is focused on a CLI‑driven
experience; legacy service footprints have been removed.

## Operating Model

- The control plane exposes HTTP APIs and SSE streams for tickets, jobs, logs,
  and artifacts; IPFS Cluster stores artifacts.
- The CLI assembles the Mods/workflow DAG and submits work to the control plane;
  checkpoints and logs are streamed over SSE.
- Runs are stateless: decisions and artifacts are persisted; the CLI exits when
  work is dispatched or complete.

 

## Feature Highlights

- [x] Legacy teardown — repository scoped to the CLI-only stub and guardrail
      tests (Roadmap 00).
- [x] Event contracts — subject alignment and in‑memory stubs for offline work
      (Roadmap 01).
- [x] Workflow runner CLI — reconstructs DAGs, streams checkpoints, and exits
      cleanly after dispatching jobs (Roadmap 02).
- [x] Lane engine — deterministic lane specs bundled under `configs/lanes` with
      `ploy lanes describe` previews (Roadmap 03).
 
- [x] Integration manifests — manifest compiler enforcing topology, fixtures,
      and lane allowlists (Roadmap 05).
- [x] Commit environments — `ploy environment materialize` assembles
      `<sha>-<app>` builds with cache hydration (Roadmap 06).
- [x] Aster hook — exposes AST-pruned bundles and workflow toggles inside stage
      metadata (Roadmap 07).
- [x] Documentation refresh — doc set aligned around the CLI-first model and
      GRID hand-off (Roadmap 08).
- [x] Cache coordination — checkpoints carry lane cache keys for reuse
      (Roadmap 09).
 
- [x] Lane documentation hardening — schema enforcement and lane reference
      updates (Roadmap 11).
 
- [x] Integration manifest schema — JSON schema + CLI validation hook for
      manifests (Roadmap 13).
 
 
- [x] Checkpoint enrichment — stage metadata and artifact manifests embedded in
      workflow checkpoints (Roadmap 17).
 
- [x] Mods parallel planner — orchestrates orw/LLM/human stages with
      parallelism (Roadmap 19, see `docs/design/mods/README.md`).
- [x] Knowledge base remediation — classifies errors, surfaces CLI
      ingest/evaluate workflows, and seeds `llm-plan` with suggestions (Roadmap
      20, see `docs/design/knowledge-base/README.md`).
- [x] Build gate reboot — control‑plane integrated static checks and log parsing across
      languages (Roadmap 21, see `docs/design/build-gate/README.md`); sandbox
      runner, static check registry, log ingestion, metadata sanitisation, CLI
      knowledge base surfacing, and Java Error Prone coverage shipped (verified
      2025-09-29 via `cmd/ploy/mod_summaries.go` and
      `internal/workflow/buildgate/error_prone_adapter.go`).
 

Full design records live in `docs/design/README.md`.

## Removed Components

- Nomad/Consul/Traefik templates, wrappers, and deployment logic
  (`internal/orchestration`, embedded HCL, Ansible playbooks).
- Long-running API/service binaries, routing assumptions, and controller ingress
  paths.
- SeaweedFS-specific artifact plumbing now replaced by IPFS publishers.
- Legacy lane descriptors tied to Nomad job specs and system job scripts.
- Obsolete docs or runbooks referencing the retired controller deployment path.

## Data & Storage Expectations

- IPFS (or compatible object storage) stores build outputs, diff reports, and
  audit logs.
- Workspace metadata (hash IDs, eviction policies, ownership) enables cache
  reuse without bespoke scripting.

## Testing & Tooling Focus

- Unit and CLI tests exercise in‑memory stubs locally.
- Cadence and coverage thresholds stay governed by `AGENTS.md`.
- Workspace commands (`make build`, `make test`) remain workstation-first; no
  VPS/Grid state is required for the slices above.

## Success Criteria

- Mods workflows complete end‑to‑end with faster build/test cycles than the
  legacy runs.
- Developers request deterministic `<sha>-<app>` environments with lane caches
  and manifests applied automatically.
- No permanent services are required; when the CLI is idle, the control plane
  can queue work for the next invocation.

## Getting Started

1. **Clone & build**

   ```bash
   git clone https://github.com/iw2rmb/ploy
   cd ploy
   make build
   ```

2. **Inspect lane metadata**

   ```bash
   ./dist/ploy lanes describe --lane go-native --commit HEAD \
     --manifest smoke --aster plan,exec
   ```

   The command loads `go-native.toml` from the bundled catalogue, previews the
   composed cache key, and lists the build/test commands bound to that lane.

3. **Run the Mods CLI**

   ```bash
   ./dist/ploy mod run --ticket auto
   ```

   The CLI runs entirely against the local control plane over SSH in developer
   workflows.

4. **Dry-run a commit-scoped environment**

   ```bash
   ./dist/ploy environment materialize deadbeef --app commit-app --dry-run
   ```

   Dry-run mode compiles the `commit-app` manifest and previews cache keys for
   each required lane without mutating state.

7. **Tests**

   ```bash
   make test
   ```

   Unit tests assert that only the workflow CLI remains and that the event
   contract schema stays consistent.

8. **Manage knowledge base incidents**

   ```bash
   ./dist/ploy knowledge-base ingest --from ./fixtures/knowledge-base/new-incidents.json
   ```

   The ingest command merges incident fixtures into
   `configs/knowledge-base/catalog.json`, skipping duplicate IDs while keeping
   workstation runs deterministic.

## Environment Variables

- `PLOY_RUNTIME_ADAPTER` — Optional runtime adapter selector (default:
  `local-step`).
- `PLOY_ASTER_ENABLE` — Opt‑in switch for the experimental Aster bundle
  integration.

## Contributing

Follow the contributor workflow in `AGENTS.md` and keep docs aligned with
`docs/DOCS.md`.

## License

The project inherits its existing license terms; consult `LICENSE` if/when it is
reintroduced in a future slice.
