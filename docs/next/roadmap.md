# Ploy Next Roadmap

## 1. Backbone Foundation

- [x] 1.1 Align the etcd layout with `docs/next/etcd.md`: extend `internal/controlplane/scheduler` to persist `expires_at`, retention bundles, lease metadata, and node capacity snapshots; add watchers for `leases/jobs`, `gc/jobs`, and node status prefixes; document the schema contract in the package tests.
- [ ] 1.2 Build a Mods orchestrator in the control plane: create a service that persists Mod tickets under `mods/<ticket>` (status, stage graph, artifact references) and translates submissions into scheduler jobs; wire optimistic concurrency so nodes cannot double-claim stages.
- [ ] 1.3 Expand the HTTP surface in `internal/api/httpserver/controlplane.go` to match `docs/next/api.md`: add `/v1/mods` (submit, resume, cancel, status, events, logs), `/v1/artifacts` CRUD, `/v1/jobs/{id}/events`, `/v1/config`, `/v1/status`, `/v1/version`, `/v1/beacon/*`, `/v1/registry/*`, and ensure every route enforces mutual TLS/token auth once the security middleware lands.
- [ ] 1.4 Introduce artifact/registry backends behind the new endpoints: reuse `internal/workflow/artifacts` for IPFS Cluster pins, implement OCI manifest/blob storage (etcd metadata + IPFS payload), and surface pin status for CLI queries.
- [ ] 1.5 Harden configuration discovery: update `internal/api/config` and `cmd/ploy/config_gitlab.go` to merge cluster descriptors, beacon discovery, and local overrides; ensure CA bundle rotation flows propagate to control-plane clients.

## 2. Worker Runtime & Job Execution

- [ ] 2.1 Replace the assignment polling client (`internal/api/controlplane.Client`) with a job-claim loop that hits `/v1/jobs/claim`, `/v1/jobs/{id}/heartbeat`, and `/v1/jobs/{id}/complete`; integrate the logstream hub so every container run emits SSE frames keyed by job ID.
- [ ] 2.2 Wire the step runner: assemble the workspace hydrator, Docker runtime, diff capture, IPFS Cluster publisher, and SHIFT client inside a worker executor package; persist retention hints to etcd and stream them via SSE (`internal/node/logstream`).
- [ ] 2.3 Implement real SHIFT integration per `docs/next/shift.md`: replace `noopSandboxExecutor` in `cmd/ploy/dependencies_runtime_local.go` with a sandbox executor that shells out to the standalone SHIFT binary/library, plumbs static-check adapters, and records structured reports in IPFS; add failure mapping to `buildgate` metadata.
- [ ] 2.4 Support repository hydration and snapshot reuse on nodes: teach the workspace hydrator to pull base snapshots/diff CIDs from IPFS Cluster, materialise ordered diffs, and fall back to GitLab clones using credentials stored in etcd.
- [ ] 2.5 Add node lifecycle controllers: expose `/v1/node/status`, `/v1/node/jobs`, `/v1/node/logs` in the worker HTTP surface; periodically publish capacity heartbeats (`nodes/<node-id>/capacity`), Docker/SHIFT health, and IPFS peer info so the control plane can schedule intelligently.

## 3. CLI & Operator Workflow

- [ ] 3.1 Rebuild `ploy mod run` (see `cmd/ploy/mod_run.go`) to submit Mods over `/v1/mods`, stream checkpoints/events, and fetch final artifact bundles instead of invoking `internal/workflow/runner` locally; remove direct dependencies on Grid clients and event stubs.
- [ ] 3.2 Add CLI commands for control-plane parity: `ploy mods resume`, `ploy mods cancel`, `ploy mods inspect`, `ploy mods artifacts`, `ploy jobs ls`, `ploy jobs inspect`, `ploy jobs retry`, `ploy artifact push/pull/status/rm` wired to the new HTTP API; update the command tree (`internal/clitree/tree.go`) and completions.
- [ ] 3.3 Update cluster and node administration flows: ensure `ploy deploy bootstrap`, `ploy node add`, `ploy node rm`, and GitLab signer commands hit the new endpoints, use the refreshed descriptor format, and surface beacon CA rotation data.
- [ ] 3.4 Refresh configuration/environment handling: purge Grid-specific env vars from `docs/envs/README.md`, introduce the Ploy Next variables (IPFS Cluster, control plane URL, token paths), and update `cmd/ploy/config_*` helpers to honour them.
- [ ] 3.5 Remove the legacy workflow runner path: delete `internal/workflow/grid`, the local `runner.Run` invocation path, and associated tests once the control-plane submission round-trips are covered.

## 4. Observability, Retention & Operations

- [ ] 4.1 Implement Mod/job event streaming: aggregate job-level SSE streams into Mod-level streams, broadcast retention hints, and back `/v1/mods/{ticket}/logs` with archived IPFS bundles and tails (`internal/node/logstream`, `docs/next/logs.md`).
- [ ] 4.2 Expose Prometheus metrics per `docs/next/observability.md`: register queue depth, claim latency, retry counts, SHIFT duration, IPFS pin metrics, GC activity, and log upload status; serve them at `/metrics` with tests covering label cardinality.
- [ ] 4.3 Ship the GC controller and CLI: implement a background reconciler (likely in beacon mode) that walks `gc/jobs/**`, unpins artifacts via IPFS Cluster, and deletes expired records; add `ploy gc` with dry-run and filtering options, wiring it to the same logic.
- [ ] 4.4 Build node/beacon administration routes: `/v1/nodes` should report health, running jobs, IPFS peer lag, and support drain/promote/heal actions; `/v1/beacon/*` must publish discovery metadata, CA bundles, and accept rotations.
- [ ] 4.5 Document and automate operations: update `docs/next/devops.md`, `docs/next/observability.md`, and `docs/next/vps-lab.md` with the new commands, required ports, and smoke tests; ensure bootstrap scripts drop the right resolver entries and verify systemd units.

## 5. Migration & Cleanup

- [ ] 5.1 Remove Grid dependencies: drop `github.com/iw2rmb/grid` from `go.mod`, delete Grid-specific packages, and migrate any reusable helpers into the new control-plane or runtime packages as needed.
- [ ] 5.2 Provide migration tooling: add scripts to drain legacy Grid workflows, backfill etcd with existing Mod tickets/artifacts, and document the cut-over sequence in `docs/next/migration.md`.
- [ ] 5.3 Normalise docs: replace the root `README.md` with the Ploy Next narrative, retire stale design docs to `.archive/`, and ensure `CHANGELOG.md` records the migration milestones.
- [ ] 5.4 Audit environment variables, configs, and system dependencies to confirm they match the versions listed in `docs/next/README.md` and `docs/next/devops.md`; call out TODOs in `docs/envs/README.md` if a value cannot be finalised yet.

## 6. Testing & Release Readiness

- [ ] 6.1 Establish TDD coverage for each slice: write unit tests for the new control-plane handlers, scheduler edge cases (leases, retries, retention), node executors, and CLI commands; maintain ≥60% overall coverage and ≥90% on critical workflow packages.
- [ ] 6.2 Add integration suites: stand up multi-node scenarios in the VPS lab (`docs/next/vps-lab.md`) that exercise job submission, log streaming, artifact pinning, GC, GitLab credential rotation, and beacon failover; gate them behind `make test` targets.
- [ ] 6.3 Introduce end-to-end smoke tests for `ploy mod run` against a seeded repository, verifying deterministic diffs, SHIFT enforcement, and artifact retrieval.
- [ ] 6.4 Wire CI to the new workflow: ensure `make build`, `make test`, static checks, and formatting pass; add IPFS Cluster and etcd fixtures to CI if required.
- [ ] 6.5 Prepare release artefacts: update build pipelines to ship `ploy` and `ployd` binaries, embed the bootstrap script, and document version tagging/upgrade steps for operators.
