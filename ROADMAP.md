# Build Gate Decouple: Repo+Diff + Remote Execution

> When following this template:
> - Align to the template structure
> - Include steps to update relevant docs

Scope: Decouple Build Gate execution from the local node workspace so gate runs use the HTTP Build Gate API and repo+diff model. Enable gate jobs to run on any eligible worker (multi‑VPS) while Mods and healing operate on per-step workspaces. Preserve current docker-based gate behavior as an implementation detail of the Build Gate workers, not the Mods node agent.

Documentation: `docs/build-gate/README.md`, `docs/mods-lifecycle.md`, `docs/envs/README.md`, `cmd/ploy/README.md`, `tests/README.md`, `internal/workflow/runtime/step/gate_docker.go`, `internal/nodeagent/execution_healing.go`, `internal/server/handlers/handlers_buildgate.go`, `internal/store/buildgate_jobs.sql.go`.

Legend: [ ] todo, [x] done.

## Phase A — Clarify current Build Gate execution paths
- [x] Document local vs remote gate flows — Establish the baseline before decoupling.
  - Component: `docs/build-gate/README.md`, `docs/mods-lifecycle.md`.
  - Scope:
    - Add a short section explaining the two existing paths:
      - Local docker gate: `internal/workflow/runtime/step/gate_docker.go` mounted on the node agent; used for Mods pre‑gate and re‑gate (`runner.Gate.Execute`).
      - HTTP Build Gate API: `POST /v1/buildgate/validate` + job queue in `buildgate_jobs`; executed by workers claiming jobs via `/v1/nodes/{id}/buildgate/claim`.
    - Make explicit that the **target state** is: Mods and healing call the HTTP Build Gate API (repo+diff), and Build Gate workers encapsulate docker execution.
  - Test: `rg "Build Gate execution paths" docs -n` — Confirm the new section exists and is consistent; run `make test` to ensure no doc-related tests break.

## Phase B — Introduce a Build Gate HTTP client adapter in workflow layer
- [x] B1 — HTTP client wrapper for Build Gate API — Typed client for `/v1/buildgate/validate` + job polling.
  - Component: `internal/workflow/runtime/step/gate_http_client.go`, `internal/workflow/runtime/step/gate_iface.go`.
  - Scope:
    - In `gate_iface.go`, add:
      - `type BuildGateHTTPClient interface {`
      - `  Validate(ctx context.Context, req contracts.BuildGateValidateRequest) (*contracts.BuildGateValidateResponse, error)`
      - `  GetJob(ctx context.Context, jobID string) (*contracts.BuildGateJobStatusResponse, error)`
      - `}`.
    - Implement `BuildGateHTTPClient` in `gate_http_client.go` using `*http.Client`, `PLOY_SERVER_URL`, `PLOY_API_TOKEN`, TLS envs (reuse `createHTTPClient` patterns from node agent).
    - Encode/decode `contracts.BuildGateValidateRequest` / `BuildGateValidateResponse` and `BuildGateJobStatusResponse`.
  - Test:
    - Add `internal/workflow/runtime/step/gate_http_client_test.go`:
      - Fake `http.RoundTripper` to assert `POST /v1/buildgate/validate` and `GET /v1/buildgate/jobs/{id}`.
    - Run: `go test ./internal/workflow/runtime/step -run TestBuildGateHTTPClient`.

- [x] B2 — HTTPGateExecutor skeleton wired to client — Minimal GateExecutor that calls the HTTP client (sync only).
  - Component: `internal/workflow/runtime/step/gate_http.go`.
  - Scope:
    - Define:
      - `type HTTPGateExecutor struct { client BuildGateHTTPClient }`.
      - `func NewHTTPGateExecutor(client BuildGateHTTPClient) GateExecutor`.
    - Implement `Execute(ctx, spec, workspace)` with:
      - Early return `nil, nil` when `spec==nil` or `!spec.Enabled` (mirror `dockerGateExecutor`).
      - Build a minimal `contracts.BuildGateValidateRequest` (temporary: only `Profile`/`Timeout`, repo+diff wired in Phase C).
      - Call `client.Validate`.
      - If `resp.Result != nil`, return it; if `resp.Status==BuildGateJobStatusPending`, return a TODO error (`"async jobs not supported yet"`).
  - Test:
    - Add `internal/workflow/runtime/step/gate_http_test.go`:
      - Fake `BuildGateHTTPClient` returning immediate completion and pending status.
      - Assert disabled spec returns `nil,nil`.
    - Run: `go test ./internal/workflow/runtime/step -run TestHTTPGateExecutor_Sync`.

- [ ] B3 — Async job polling for HTTPGateExecutor — Support pending jobs via `GetJob` polling.
  - Component: `internal/workflow/runtime/step/gate_http.go`.
  - Scope:
    - Extend `Execute` so when `Validate` returns `Status=BuildGateJobStatusPending`:
      - Poll `GetJob` until `Status` is `BuildGateJobStatusCompleted`/`Failed` or context timeout.
      - Use timeout from `spec` or `PLOY_BUILDGATE_TIMEOUT` env.
      - Return `Result` on completed; return error on failed or timeout.
  - Test:
    - Extend `gate_http_test.go`:
      - Fake client that returns `Pending` then `Completed` with `Result`.
      - Fake client returning `Failed` with error string.
      - Context timeout case.
    - Run: `go test ./internal/workflow/runtime/step -run TestHTTPGateExecutor_Async`.

- [ ] B4 — Make GateExecutor pluggable between docker and HTTP modes — Configuration-driven selection.
  - Component: workflow runtime initialization (`internal/workflow/runtime/...`), node agent runtime wiring.
  - Scope:
    - Add factory:
      - `func NewGateExecutor(mode string, rt ContainerRuntime, httpClient BuildGateHTTPClient) GateExecutor`.
      - `mode==""` or `"local-docker"` → `NewDockerGateExecutor(rt)`.
      - `mode=="remote-http"` → `NewHTTPGateExecutor(httpClient)`.
    - In runtime initialization (where `step.Runner` is built in `internal/nodeagent/execution_orchestrator.go`), read `PLOY_BUILDGATE_MODE` (or config) and call `NewGateExecutor`.
    - Keep default behavior (`local-docker`) when mode is unset/invalid.
  - Test:
    - Extend `internal/workflow/runtime/step/runner_gate_test.go`:
      - Runner with `mode="remote-http"` uses HTTP executor (fake client).
      - Runner with `mode=""` or `"local-docker"` uses `dockerGateExecutor`.
    - Run: `go test ./internal/workflow/runtime/step -run TestNewGateExecutor`.

## Phase C — Define repo+diff inputs for gate from Mods runs
- [ ] C1 — Thread repo metadata into step manifests for gate — Ensure GateExecutor has `repo_url` and `ref`.
  - Component: `internal/nodeagent/manifest.go`, `internal/nodeagent/run_options.go`, `internal/workflow/contracts`.
  - Scope:
    - Verify `StartRunRequest` exposes `RepoURL`, `BaseRef`, `TargetRef`, `CommitSHA` where runtime is initialized.
    - Extend `contracts.StepManifest` and/or gate options to carry:
      - `RepoURL` and `BuildGateRef` (decide `BaseRef` vs `CommitSHA` per `docs/build-gate/README.md`).
    - When building manifests and gate spec in node agent, populate these fields from `StartRunRequest`.
    - In `gate_http.go`, read this data to set `BuildGateValidateRequest.RepoURL` and `.Ref`.
  - Test:
    - Extend `internal/nodeagent/manifest_test.go`:
      - Asserts step manifests and gate spec contain expected repo metadata.
    - Run: `go test ./internal/nodeagent -run TestManifestBuildWithGateRepoMeta`.

- [ ] C2 — Treat every execution step as a stage + diff — Unify Mods, healing, and Build Gate around stages and diffs.
  - Component: `SCHEMA.sql`, `internal/store/diffs.sql.go`, `internal/server/handlers/handlers_diffs.go`, `internal/nodeagent/execution_orchestrator.go`, `internal/nodeagent/difffetcher.go`.
  - Scope:
    - Use `stages` as canonical “step/node” table:
      - Each execution unit (pre-run gate, healing, mod, post-gate, future nodes) has a `stages` row with:
        - `run_id`, `id` (stage_id), `name`, `meta.type` (`"pre_gate"`, `"mod"`, `"post_gate"`, `"healing"`), `meta.step_index`.
    - Ensure every diff emitted during execution:
      - Stores non-null `diffs.stage_id` and `diffs.run_id`.
      - Tags `diffs.summary` with `step_index` and `mod_type` (`"mod"`, `"healing"`, `"pre_gate"`, `"post_gate"`).
    - Update rehydration logic so `rehydrateWorkspaceForStep(stepIndex=k)`:
      - Fetches all diffs where `summary.step_index <= k` (mods + healing) and applies them in `created_at` order.
      - Notes future DAG mode will replace `step_index` filter with “all ancestor stages”.
    - Keep this model as the single source of truth for Mods rehydration and repo+diff inputs to Build Gate.
  - Test:
    - Extend `internal/store/diffs_step_index_test.go`:
      - Asserts ordering by `step_index` then `created_at`, includes healing diffs.
    - Extend `internal/nodeagent/difffetcher_test.go`, `tests/integration/smoke_workflow_test.go`:
      - `FetchDiffsForStep` includes all diffs (mods + healing) up to target step.
      - Rehydrated workspaces match single-node execution.
    - Run:
      - `go test ./internal/store -run TestDiffsStepIndex`.
      - `go test ./internal/nodeagent -run TestDiffFetcher`.
      - `go test ./tests/integration -run TestSmokeWorkflow`.

## Phase D — Switch Mods pre‑gate and re‑gate to HTTP Build Gate API
- [ ] Route main pre-mod Build Gate calls through the HTTP adapter — Allow gate to run on any Build Gate worker.
  - Component: `internal/nodeagent/execution_orchestrator.go` (or equivalent runner), `internal/workflow/runtime/step`.
  - Scope:
    - In the runner that executes a Mods step:
      - Ensure `runner.Gate` is set to the HTTP-based `GateExecutor` when `PLOY_BUILDGATE_MODE=remote-http`.
    - For pre‑mod gate (before main mod container runs):
      - No change to caller logic: still invokes `runner.Gate.Execute(ctx, spec, workspace)`.
      - Under HTTP mode:
        - Workspace content is _not_ used directly by the gate; instead, gate uses `repo_url` + `ref` + `diff_patch` from workspace via the HTTP API.
      - Under docker mode:
        - Behavior remains unchanged, for backward compatibility.
  - Test:
    - Extend `internal/workflow/runtime/step/runner_gate_test.go` to:
      - Assert that in HTTP mode, the gate executor makes at least one HTTP call to `/v1/buildgate/validate`.
      - Assert that the returned `BuildGateStageMetadata` is wired into step results as before.

- [ ] Route re‑gates after healing through the HTTP adapter — Decouple healing node from gate node.
  - Component: `internal/nodeagent/execution_healing.go`.
  - Scope:
    - In `executeWithHealing`, where re‑gate currently uses `runner.Gate.Execute(ctx, gateSpec, workspace)`:
      - Ensure this call invokes the HTTP gate when `PLOY_BUILDGATE_MODE=remote-http`.
      - For re‑gates:
        - Use the same `diff_patch` computation helper so the HTTP Build Gate API sees base clone + accumulated healing diffs.
      - Keep semantics identical:
        - Gate metadata appended to `ReGates`.
        - Start/finish timing recorded as today.
    - Confirm that the local workspace is **not** treated as the only source of truth for gate anymore; the gate is free to run wherever the Build Gate job is scheduled.
  - Test:
    - Extend `internal/nodeagent/execution_healing_test.go` to:
      - Use a fake `GateExecutor` that records that it is HTTP-based and receives diff-bearing requests.
      - Verify that after a healing attempt, the re‑gate path uses the HTTP-based gate rather than direct docker execution.

## Phase E — Node workers that execute Build Gate jobs
- [ ] E1 — Gate worker enable flag in node config — Control which nodes run Build Gate jobs.
  - Component: `internal/nodeagent/config.go`, node config YAML/docs.
  - Scope:
    - Extend `Config` with `BuildGateWorkerEnabled bool "yaml:\"buildgate_worker_enabled\""` (or similar).
    - In `LoadConfig`, default `BuildGateWorkerEnabled` when unset.
    - Document mapping from `buildgate_worker_enabled` and/or `PLOY_BUILDGATE_WORKER_ENABLED` env in `docs/envs/README.md`.
  - Test:
    - Add `internal/nodeagent/config_test.go`:
      - Parse YAML with and without `buildgate_worker_enabled` and assert defaults.
    - Run: `go test ./internal/nodeagent -run TestLoadConfig`.

- [ ] E2 — Condition claim loop on worker flag — Only enabled nodes claim Build Gate jobs.
  - Component: `internal/nodeagent/claimer_loop.go`.
  - Scope:
    - In `claimWork`, guard Build Gate claim:
      - When `cfg.BuildGateWorkerEnabled` is true, call `claimAndExecuteBuildGateJob`.
      - When false, skip straight to `claimAndExecute` (runs only).
  - Test:
    - Add/extend `internal/nodeagent/claimer_loop_test.go`:
      - Fake `ClaimManager` with `BuildGateWorkerEnabled=false` and injected fakes for claim methods; assert only run-claim path executes.
    - Run: `go test ./internal/nodeagent -run TestClaimWork`.

- [ ] E3 — Worker loop wiring and smoke coverage — Ensure nodes execute claimed Build Gate jobs end-to-end.
  - Component: `internal/nodeagent/claimer_buildgate.go`, node agent entrypoint, docs.
  - Scope:
    - Confirm `ClaimManager.Start` is used for all nodes and Build Gate jobs flow:
      - `claimAndExecuteBuildGateJob` → `BuildGateExecutor.Execute` → `/buildgate/{job_id}/complete`.
    - Update `docs/build-gate/README.md` and `docs/envs/README.md`:
      - Explain how to designate Build Gate worker nodes via config/env.
      - Note that non-worker nodes will not claim Build Gate jobs.
  - Test:
    - Manual/local:
      - Run two-node lab:
        - Node A: `buildgate_worker_enabled=true`.
        - Node B: `buildgate_worker_enabled=false`.
      - Submit Build Gate job; verify only Node A logs `claimed buildgate job`.
      - Submit Mods run; verify either node can claim it.

## Phase F — Update specs, docs, and E2E tests
- [ ] Update Mods and Build Gate documentation to reflect the new architecture — Make repo+diff and remote execution the primary story.
  - Component: `docs/build-gate/README.md`, `docs/mods-lifecycle.md`, `docs/envs/README.md`, `cmd/ploy/README.md`, `tests/README.md`.
  - Scope:
    - In `docs/build-gate/README.md`, add a “Remote execution” section:
      - Build Gate jobs are queued via `/v1/buildgate/validate`.
      - Workers claim jobs via `/v1/nodes/{id}/buildgate/claim`.
      - Jobs encapsulate repo+diff payloads; workers handle docker execution and report `BuildGateStageMetadata`.
    - In `docs/mods-lifecycle.md`, update the Mods execution diagram:
      - Pre‑gate and re‑gate call the HTTP Build Gate API through the new GateExecutor adapter.
      - Healing flows still use repo+diff semantics, but gate now runs on dedicated Build Gate workers when configured.
    - In `cmd/ploy/README.md`, describe how CLI-visible gate summaries are unaffected, but the execution location is now decoupled.
  - Test:
    - Run `make test` to ensure doc-sensitive tests still pass.
    - Run `rg "local docker gate" docs -n` to confirm no stale descriptions remain.

- [ ] Adjust E2E scenarios to cover remote Build Gate mode — Validate multi‑VPS gate behavior.
  - Component: `tests/e2e/mods`, cluster configuration for tests.
  - Scope:
    - Extend or add E2E scenarios (e.g., `tests/e2e/mods/scenario-multi-node-rehydration`) to run with:
      - `PLOY_BUILDGATE_MODE=remote-http`.
      - At least two worker nodes, one designated as a Build Gate worker.
    - Assertions:
      - Gate jobs are executed on any eligible worker node (check node IDs in `buildgate_jobs.node_id` or logs).
      - Mods steps and healing may run on different nodes than the gate.
      - Repo+diff semantics remain correct: gate results match expectations with accumulated healing diffs.
  - Test:
    - Run E2E scripts (e.g., `bash tests/e2e/mods/scenario-orw-fail/run.sh` and `scenario-multi-node-rehydration/run.sh`) in a multi-node test cluster with `remote-http` mode enabled.
    - Verify via logs and DB inspection that:
      - Build Gate jobs move through the new job workflow.
      - Mods steps and gates can land on different VPS nodes while producing the same observable behavior as the local-docker baseline.
