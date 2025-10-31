# Ploy Next Roadmap

## 1. Backbone Foundation

- [x] 1.1 Align the etcd layout with `docs/next/etcd.md`: extend `internal/controlplane/scheduler` to persist `expires_at`, retention bundles, lease metadata, and node capacity snapshots; add watchers for `leases/jobs`, `gc/jobs`, and node status prefixes; document the schema contract in the package tests.
- [x] 1.2 Build a Mods orchestrator in the control plane: create a service that persists Mod tickets under `mods/<ticket>` (status, stage graph, artifact references) and translates submissions into scheduler jobs; wire optimistic concurrency so nodes cannot double-claim stages.
- [x] 1.3 Expand the HTTP surface in `internal/api/httpserver/controlplane.go` to match `docs/next/api.md`: add `/v1/mods` (submit, resume, cancel, status, events, logs), `/v1/artifacts` CRUD, `/v1/jobs/{id}/events`, `/v1/config`, `/v1/status`, `/v1/version` (registry endpoints removed in favor of Docker Hub).
- [x] 1.4 Consolidate on artifacts backend; OCI registry removed. Reuse `internal/controlplane/artifacts` for IPFS Cluster pins and surface pin status for CLI queries.
- [x] 1.5 Harden configuration discovery: update `internal/api/config` and `cmd/ploy/config_gitlab.go` to merge cluster descriptors, SSH tunnel metadata, and local overrides so every CLI call can rely on descriptors without separate CA bundles. (See `.archive/config-discovery/README.md`.)
- [x] 1.6 Persist SSH transfer slots and enforcement: move the new `/v1/transfers/*` state out of process into etcd, add the `ployd` SFTP guard/cleanup units, and emit structured audit logs so uploads/downloads survive restarts.

## 2. Worker Runtime & Job Execution

- [x] 2.1 Replace the assignment polling client (`internal/api/controlplane.Client`) with a job-claim loop that hits `/v1/jobs/claim`, `/v1/jobs/{id}/heartbeat`, and `/v1/jobs/{id}/complete`; integrate the logstream hub so every container run emits SSE frames keyed by job ID.
 - [x] 2.2 Wire the step runner: assemble the workspace hydrator, Docker runtime, diff capture, IPFS Cluster publisher, and Build Gate client inside a worker executor package; persist retention hints to etcd and stream them via SSE (`internal/node/logstream`).
 - [x] 2.3 Implement embedded Build Gate per `docs/workflow/README.md`: replace legacy shell-out with an in-process Java executor and record structured reports in IPFS; add failure mapping to `buildgate` metadata.
- [x] 2.4 Support repository hydration and snapshot reuse on nodes: teach the workspace hydrator to pull base snapshots/diff CIDs from IPFS Cluster, materialise ordered diffs, and fall back to GitLab clones using credentials stored in etcd.
 - [x] 2.5 Add node lifecycle controllers: expose `/v1/node/status`, `/v1/node/jobs`, `/v1/node/logs` in the worker HTTP surface; periodically publish capacity heartbeats (`nodes/<node-id>/capacity`), Docker/Build Gate health, and IPFS health; persist node status snapshots via `PATCH /v1/nodes/{node}` to etcd (`nodes/<node-id>/status`) for scheduler consumption.

## 3. CLI & Operator Workflow

 - [x] 3.1 Rebuild `ploy mod run` (see `cmd/ploy/mod_run.go`) to submit Mods over `/v1/mods`, stream checkpoints/events, and fetch final artifact bundles instead of invoking `internal/workflow/runner` locally; remove direct dependencies on legacy clients and event stubs.
  - Derivables
    - CLI behavior: `ploy mod run <path|repo> [--param k=v] [--ticket T] [--follow] [--quiet] [--json] [--artifact-dir DIR]` with clear help text and examples; supports stdin-driven config and param overrides.
    - Control-plane submission: build a `POST /v1/mods` payload that includes repo/ref, workspace hydration hints (base snapshot, diffs), Build Gate policy selector, and operator notes; map `--param` to submission `params` and persist a returned ticket ID.
    - Event/Checkpoint streaming: when `--follow` is set, subscribe to `GET /v1/mods/{ticket}/events` (SSE) and render progress with stage names, durations, retry counts, and warnings; optionally `--json` emits machine-readable lines.
    - Logs access: on demand `--logs` or at completion, stream `GET /v1/mods/{ticket}/logs` with tailing and archive handoff; support `--since` and `--grep` client-side filters.
    - Artifact retrieval: on success fetch artifact bundle metadata from `GET /v1/mods/{ticket}` and download referenced items via `GET /v1/artifacts/{id}` or IPFS/OCI targets; write to `--artifact-dir` with digest-prefixed filenames and a manifest JSON.
    - Exit codes & failure mapping: non-zero on terminal Mod failure or policy violation; map severities (policy soft fail vs hard fail) to distinct exit codes; print remediation hint when available.
    - Config/env integration: respect control-plane URL/token discovery from `docs/envs/README.md` (e.g., `PLOY_API_URL`, `PLOY_TOKEN`, IPFS Cluster toggles); surface effective config with `--debug`.
    - Telemetry hooks: increment CLI metrics (submissions, follow durations, cancel counts) and include `X-Ploy-Client` header with version/platform; guard behind opt-in env toggle.
    - Legacy removal: drop direct calls to `internal/workflow/runner` and old stubs; depend only on the HTTP client in `internal/api/controlplane` and the logstream reader.
    - Docs refresh: update `docs/next/api.md` and `docs/api/OpenAPI.yaml` examples for `/v1/mods` submission, events schema, and artifact listing; add a quickstart snippet to the root README.
  - How to test
    - Unit tests (LOCAL): flag/args parsing, param merging, and payload construction with table-driven tests in `cmd/ploy/mod_run_test.go`; verify exit code mapping and quiet/json output modes. Target ≥90% coverage for the command package slice.
    - Client contract tests (LOCAL): mock the control-plane client to emit a golden SSE transcript (events, retries, warnings) and assert rendered output; add a JSON mode golden to ensure machine-readability stability.
    - Artifact handling tests (LOCAL): fake artifact metadata and downloads, assert pathing under `--artifact-dir`, digest-prefixed names, and manifest structure; include collision handling and partial failure cases.
    - Resilience tests (LOCAL): inject network timeouts, 5xx retries, and SSE disconnects; ensure exponential backoff, resume behavior, and user-facing warnings are correct without duplicate events.
    - Cancel/Resume flows (LOCAL): simulate `SIGINT` while `--follow` is active and assert the CLI calls `POST /v1/mods/{ticket}/cancel`; verify resume path via `ploy mod resume` is suggested with the correct ticket.
    - Help/UX snapshots (LOCAL): snapshot `ploy mod run --help` and typical success/failure outputs; protect from regressions with minimal golden maintenance.
    - OpenAPI alignment (LOCAL): validate request/response structs against `docs/api/OpenAPI.yaml` using a schema check; fail tests on drift of fields/types.
    - Integration (VPS): use the VPS lab to run against a seeded repo and exercise submission → events/logs → artifact retrieval; assert deterministic diff CIDs, Build Gate enforcement presence, and final manifest matches.
    - E2E smoke (VPS): drive `ploy mod run` end-to-end with `--json` and verify a stable event stream contract and final artifacts are pinned in IPFS Cluster; collect timings for SLA baselines.
    - Docs guard (LOCAL): extend `tests/guards/docs_guard_test.go` to assert roadmap/API examples reference existing flags/endpoints and that `docs/next/api.md` stays in sync after changes.
- [x] 3.2 Add CLI commands for control-plane parity: `ploy mod resume`, `ploy mod cancel`, `ploy mod inspect`, `ploy mod artifacts`, `ploy jobs ls`, `ploy jobs inspect`, `ploy jobs retry`, `ploy artifact push/pull/status/rm` wired to the new HTTP API; update the command tree (`internal/clitree/tree.go`) and completions.
- [ ] 3.3 Update cluster and node administration flows: ensure the unified `ploy cluster add` command (primary + worker modes), `ploy node rm`, and GitLab signer commands hit the new endpoints, reuse the SSH descriptor format, and surface tunnel status where operators need it.
- [x] 3.4 Refresh configuration/environment handling: purge legacy-specific env vars from `docs/envs/README.md`, introduce the Ploy Next variables (IPFS Cluster, control plane URL, token paths), and update `cmd/ploy/config_*` helpers to honour them.
- [x] 3.5 Remove the legacy workflow runner path: delete `internal/workflow/grid`, the local `runner.Run` invocation path, and associated tests once the control-plane submission round-trips are covered.
- [ ] 3.6 Finish SSH transfer UX: add chunked/resumable SFTP support, progress reporting, and CLI resume flows (`ploy upload --resume`, partial downloads) so large artifacts do not require re-sending on flaky links.

## 4. Observability, Retention & Operations

- [x] 4.1 Implement Mod/job event streaming: aggregate job-level SSE streams into Mod-level streams, broadcast retention hints, and back `/v1/mods/{ticket}/logs` with archived IPFS bundles and tails (`internal/node/logstream`, `docs/next/logs.md`).
 - [ ] 4.2 Expose Prometheus metrics per `docs/next/observability.md`: register queue depth, claim latency, retry counts, Build Gate duration, IPFS pin metrics, GC activity, and log upload status; serve them at `/metrics` with tests covering label cardinality.
- [ ] 4.3 Ship the GC controller and CLI: implement a background reconciler that walks `gc/jobs/**`, unpins artifacts via IPFS Cluster, and deletes expired records; add `ploy gc` with dry-run and filtering options, wiring it to the same logic.
- [ ] 4.4 Build node administration routes: `/v1/nodes` should report health, running jobs, IPFS peer lag, and support drain/heal actions, all reachable through the SSH tunnel manager.
- [ ] 4.5 Document and automate operations: update `docs/how-to/deploy-a-cluster.md` and `docs/next/observability.md` with the new commands, required ports, and smoke tests; ensure bootstrap scripts drop the right resolver entries and verify systemd units.
- [ ] 4.6 Wire transfer ingestion to jobs/artifacts: have committed slots trigger IPFS publishes, update job metadata with report CIDs/digests, and expose metrics/alerts when uploads stall or digests mismatch.

## 5. Migration & Cleanup

- [x] 5.1 Remove legacy runtime dependencies: drop the old modules from `go.mod`, delete retired packages, and migrate any reusable helpers into the new control-plane or runtime packages as needed.
- [ ] 5.2 Normalise docs: replace the root `README.md` with the Ploy Next narrative, rm stale design docs, and ensure `CHANGELOG.md` records the migration milestones.
- [ ] 5.3 Audit environment variables, configs, and system dependencies to confirm they match the versions listed in `docs/next/README.md` and `docs/how-to/deploy-a-cluster.md`; call out TODOs in `docs/envs/README.md` if a value cannot be finalised yet.

## 6. Testing & Release Readiness

- [ ] 6.1 Establish TDD coverage for each slice: write unit tests for the new control-plane handlers, scheduler edge cases (leases, retries, retention), node executors, and CLI commands; maintain ≥60% overall coverage and ≥90% on critical workflow packages.
- [ ] 6.2 Add integration suites: stand up multi-node scenarios in the VPS lab that exercise job submission, log streaming, artifact pinning, GC, GitLab credential rotation, and SSH tunnel failover; gate them behind `make test` targets.
 - [ ] 6.3 Introduce end-to-end smoke tests for `ploy mod run` against a seeded repository, verifying deterministic diffs, Build Gate enforcement, and artifact retrieval.
- [ ] 6.4 Wire CI to the new workflow: ensure `make build`, `make test`, and formatting pass; add IPFS Cluster and etcd fixtures to CI if required.
- [ ] 6.5 Prepare release artefacts: update build pipelines to ship `ploy` and `ployd` binaries, embed the bootstrap script, and document version tagging/upgrade steps for operators.
