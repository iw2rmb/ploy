# Ploy — Stateless Workflow Runner

Ploy operates as an on-demand workflow brain: it consumes Grid events, resolves
workflow DAGs, submits work back to Grid, and exits. The repository now focuses
entirely on that CLI-driven experience; the feature slices below replaced the
legacy API, Nomad, Consul, and SeaweedFS footprint.

## Operating Model

- Grid owns the control surface (webhooks, scheduling, cache promotion, node
  pools) and persists hot signals in JetStream plus cold artifacts in IPFS.
- Ploy consumes those streams, assembles the mods/workflow DAG, and submits
  follow-up jobs back to Grid via the workflow RPC client.
- Every checkpoint, artifact pointer, and decision is written to JetStream/IPFS
  so runs stay stateless and retries never depend on long-lived services.

### Related Grid References

- Workflow RPC contract and SDK expectations live in the Grid repository
  (`../grid/docs/design/workflow-rpc/README.md`).
- Webhook intake behaviour is defined in Grid's Webhook Gateway design
  (`../grid/docs/design/webhook-gateway/README.md`).
- Scheduler queue, quotas, and cache hints derive from the Grid Scheduler Core
  design (`../grid/docs/design/scheduler-core/README.md`).
- Workflow RPC helper usage, builders, and streaming retries are documented in
  `../grid/sdk/workflowrpc/README.md` and drive configuration parity with Grid.

## Feature Highlights

- [x] Legacy teardown — repository scoped to the CLI-only stub and guardrail
      tests (Roadmap 00).
- [x] Event contracts — JetStream subject map, schema enforcement, and in-memory
      stubs for offline work (Roadmap 01).
- [x] Workflow runner CLI — reconstructs DAGs, streams checkpoints, and exits
      cleanly after dispatching jobs (Roadmap 02).
- [x] Lane engine — deterministic lane specs published via the
      [`ploy-lanes-catalog`](https://github.com/iw2rmb/ploy-lanes-catalog)
      repository (mirrored into SHIFT) with
      `ploy lanes describe` previews (Roadmap 03).
- [x] Snapshot toolkit — `ploy snapshot plan` / `ploy snapshot capture` with
      strip/mask/synthetic rules baked in (Roadmap 04).
- [x] Integration manifests — manifest compiler enforcing topology, fixtures,
      and lane allowlists (Roadmap 05).
- [x] Commit environments — `ploy environment materialize` assembles
      `<sha>-<app>` builds with cache hydration (Roadmap 06).
- [x] Aster hook — exposes AST-pruned bundles and workflow toggles inside stage
      metadata (Roadmap 07).
- [x] Documentation refresh — doc set aligned around the CLI-first model and
      GRID hand-off (Roadmap 08).
- [x] Cache coordination — checkpoints carry lane cache keys for Grid reuse
      (Roadmap 09).
- [x] JetStream workflow client — live NATS connectivity with configuration
      discovered from Grid's cluster info endpoint (Roadmap 10).
- [x] Lane documentation hardening — schema enforcement and lane reference
      updates (Roadmap 11).
- [x] Snapshot validation — cross-engine verification with coverage guardrails
      (Roadmap 12).
- [x] Integration manifest schema — JSON schema + CLI validation hook for
      manifests (Roadmap 13).
- [x] Grid workflow client — workflow stages submit through the Grid RPC when
      `GRID_ENDPOINT` is set (Roadmap 14).
- [x] IPFS artifact publishing — snapshot captures stream artifacts through the
      Grid-discovered IPFS gateway (Roadmap 15).
- [x] Snapshot metadata streams — capture fingerprints and rule counts published
      to JetStream (Roadmap 16).
- [x] Checkpoint enrichment — stage metadata and artifact manifests embedded in
      workflow checkpoints (Roadmap 17).
- [x] Stage artifact streams — dedicated JetStream envelopes for stage artifacts
      to feed cache hydrators (Roadmap 18).
- [x] Mods parallel planner — orchestrates orw/LLM/human stages with Grid-aware
      parallelism (Roadmap 19, see `docs/design/mods/README.md`).
- [x] Knowledge base remediation — classifies errors, surfaces CLI
      ingest/evaluate workflows, and seeds `llm-plan` with suggestions (Roadmap
      20, see `docs/design/knowledge-base/README.md`).
- [x] Build gate reboot — Grid-integrated static checks and log parsing across
      languages (Roadmap 21, see `docs/design/build-gate/README.md`); sandbox
      runner, static check registry, log ingestion, metadata sanitisation, CLI
      knowledge base surfacing, and Java Error Prone coverage shipped (verified
      2025-09-29 via `cmd/ploy/mod_summaries.go` and
      `internal/workflow/buildgate/error_prone_adapter.go`).
- [x] Workflow RPC alignment — SDK/helper adoption, job spec schema enforcement,
      and subject alignment (Roadmap 22, see
      `docs/design/workflow-rpc-alignment/README.md`); SDK client, helper
      retries, subject realignment, and lane-driven job composition shipped
      (verified 2025-10-01 via `internal/workflow/grid/client.go`).

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

- JetStream carries events, run metadata, cache coordination signals, and
  artifact manifests.
- IPFS (or compatible object storage) stores build outputs, DB snapshot
  archives, diff reports, and audit logs.
- Workspace metadata (hash IDs, eviction policies, ownership) ensures Grid can
  claim/release caches without bespoke scripting.

## Testing & Tooling Focus

- Unit and CLI tests exercise the JetStream/Grid stubs locally; integration work
  against live Grid resumes once JetStream wiring completes.
- Cadence and coverage thresholds stay governed by `AGENTS.md`.
- Workspace commands (`make build`, `make test`) remain workstation-first; no
  VPS/Grid state is required for the slices above.

## Success Criteria

- Mods workflows complete end-to-end through Grid with faster build/test cycles
  than the legacy Nomad runs.
- Developers request deterministic `<sha>-<app>` environments with lane caches,
  manifests, and snapshots applied automatically.
- No permanent services are required; when the CLI is idle, Grid continues
  queuing work for the next invocation.

## Getting Started

1. **Clone & build**

   ```bash
   git clone https://github.com/iw2rmb/ploy
   cd ploy
   make build
   ```

2. **Clone the lane catalog (one-time)**

   ```bash
   cd ..
   git clone https://github.com/iw2rmb/ploy-lanes-catalog
   cd ploy
   ```

   Set `PLOY_LANES_DIR` to the checkout location (or place the checkout adjacent
   to the repo as shown) so the CLI can resolve lane definitions.

3. **Inspect lane metadata**

   ```bash
   ./dist/ploy lanes describe --lane go-native --commit HEAD --snapshot dev-db \
     --manifest smoke --aster plan,exec
   ```

   The command loads `go-native.toml` from the configured catalogue, previews the
   composed cache key, and lists the build/test commands bound to that lane.

4. **Run the Mods CLI**

   ```bash
   GRID_ENDPOINT=https://grid-dev.example \
     GRID_API_KEY=ghp_example \
     GRID_ID=dev-grid \
     ./dist/ploy mod run --tenant acme --ticket auto
   ```

   Ploy reads `/v1/cluster/info` from Grid to discover the API endpoint,
   JetStream route list, IPFS gateway, feature map, and Grid version before
   connecting. Omitting `GRID_ENDPOINT` keeps the CLI on the in-memory Grid and
   JetStream stubs.

5. **Preview snapshot rules**

   ```bash
   ./dist/ploy snapshot plan --snapshot dev-db
   ```

   The plan command loads `configs/snapshots/dev-db.toml`, summarises
   strip/mask/synthetic rules, and highlights which tables/columns are affected
   before a capture runs.

6. **Capture a snapshot (stub)**

   ```bash
   ./dist/ploy snapshot capture --snapshot dev-db --tenant acme \
     --ticket SNAPSHOT-1
   ```

   Capture applies the configured rules against `configs/snapshots/dev-db.json`,
   hashes the result, uploads the payload to the IPFS gateway reported by
   discovery (or the in-memory stub when discovery omits one), and publishes
   metadata through the current stub path.

7. **Dry-run a commit-scoped environment**

   ```bash
   ./dist/ploy environment materialize deadbeef --app commit-app --tenant acme --dry-run
   ```

   Dry-run mode compiles the `commit-app` manifest, verifies required snapshots
   (`commit-db`, `commit-cache`), and previews cache keys for each required lane
   without mutating state.

8. **Tests**

   ```bash
   make test
   ```

   Unit tests assert that only the workflow CLI remains and that the event
   contract schema stays consistent.

9. **Manage knowledge base incidents**

   ```bash
   ./dist/ploy knowledge-base ingest --from ./fixtures/knowledge-base/new-incidents.json
   ```

   The ingest command merges incident fixtures into
   `configs/knowledge-base/catalog.json`, skipping duplicate IDs while keeping
   workstation runs deterministic.

## Feature Roadmap

Per-feature write-ups live under `docs/tasks/shift/` (directory name retained for
historical context). Status checkboxes in this README mirror those roadmap
entries, and deeper design context is collected in `docs/design/README.md`.

## Environment Variables

Workstation builds rely on discovery to surface remote dependencies. The CLI
inspects the following environment variables:

- `GRID_ENDPOINT` — Workflow RPC host used to submit jobs back to Grid and
  query the discovery endpoint.
- `GRID_API_KEY` — Optional bearer token forwarded to Grid discovery and
  Workflow RPC requests.
- `GRID_ID` — Optional identifier scoping Grid state directories and
  emitted discovery headers.
- `PLOY_ASTER_ENABLE` — Opt-in switch for the experimental Aster bundle
  integration.

## Contributing

Follow the contributor workflow in `AGENTS.md` and keep docs aligned with
`docs/DOCS.md`.

## License

The project inherits its existing license terms; consult `LICENSE` if/when it is
reintroduced in a future slice.
