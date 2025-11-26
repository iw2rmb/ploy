# Build Gate Decouple: Repo+Diff + Remote Execution

> When following this template:
> - Align to the template structure
> - Include steps to update relevant docs

Scope: Decouple Build Gate execution from the local node workspace so gate runs use the HTTP Build Gate API and repo+diff model. Enable gate jobs to run on any eligible worker (multi‑VPS) while Mods and healing operate on per-step workspaces. Preserve current docker-based gate behavior as an implementation detail of the Build Gate workers, not the Mods node agent.

Documentation: `docs/build-gate/README.md`, `docs/mods-lifecycle.md`, `docs/envs/README.md`, `cmd/ploy/README.md`, `tests/README.md`, `internal/workflow/runtime/step/gate_docker.go`, `internal/nodeagent/execution_healing.go`, `internal/server/handlers/handlers_buildgate.go`, `internal/store/buildgate_jobs.sql.go`.

Legend: [ ] todo, [x] done.

## Phase A — Clarify current Build Gate execution paths
- [ ] Document local vs remote gate flows — Establish the baseline before decoupling.
  - Component: `docs/build-gate/README.md`, `docs/mods-lifecycle.md`.
  - Scope:
    - Add a short section explaining the two existing paths:
      - Local docker gate: `internal/workflow/runtime/step/gate_docker.go` mounted on the node agent; used for Mods pre‑gate and re‑gate (`runner.Gate.Execute`).
      - HTTP Build Gate API: `POST /v1/buildgate/validate` + job queue in `buildgate_jobs`; executed by workers claiming jobs via `/v1/nodes/{id}/buildgate/claim`.
    - Make explicit that the **target state** is: Mods and healing call the HTTP Build Gate API (repo+diff), and Build Gate workers encapsulate docker execution.
  - Test: `rg "Build Gate execution paths" docs -n` — Confirm the new section exists and is consistent; run `make test` to ensure no doc-related tests break.

## Phase B — Introduce a Build Gate HTTP client adapter in workflow layer
- [ ] Add a GateExecutor implementation that calls the HTTP Build Gate API — Bridge `step.GateExecutor` to the `/v1/buildgate/validate` repo+diff API.
  - Component: `internal/workflow/runtime/step`.
  - Scope:
    - Add a new file, e.g. `internal/workflow/runtime/step/gate_http.go`, implementing `GateExecutor` with:
      - Config for `BuildGateServerURL` and optional `PLOY_API_TOKEN` / TLS paths (reuse envs documented in `docs/envs/README.md`).
      - `Execute(ctx, spec, workspace)` that:
        - Builds a `contracts.BuildGateValidateRequest` with:
          - `repo_url`, `ref` from the step manifest or run options (must be threaded through from `StartRunRequest`).
          - `profile` and `timeout` from `spec.Profile` and a default (e.g., `PLOY_BUILDGATE_TIMEOUT`).
          - `diff_patch` derived from the current workspace diff vs baseline (see Phase C).
        - Issues `POST /v1/buildgate/validate` to `BuildGateServerURL`.
        - Handles both sync and async responses:
          - If `result` is present, decode into `contracts.BuildGateStageMetadata`.
          - If only `job_id` + `status=pending` is returned, poll `GET /v1/buildgate/jobs/{id}` until terminal.
      - Returns `BuildGateStageMetadata` and error semantics matching `dockerGateExecutor`.
    - Keep `NewDockerGateExecutor` unchanged for now; `gate_http.go` is added but not yet wired.
  - Test:
    - Add unit tests in `internal/workflow/runtime/step/gate_http_test.go`:
      - Mock HTTP client responding with:
        - Immediate completion (`status=completed`, `result` present).
        - Async completion via job polling.
        - Error cases (400 validation error, 500 store error).
      - Assert that `Execute` returns a populated `BuildGateStageMetadata` and propagates failure when `passed=false`.
    - Run `GOFLAGS=${GOFLAGS:-} go test ./internal/workflow/runtime/step -run TestHTTPGate*`.

- [ ] Make GateExecutor pluggable between docker and HTTP modes — Allow configuration-driven selection.
  - Component: workflow runtime initialization (`internal/workflow/runtime/...`), node agent config.
  - Scope:
    - In the runtime initialization (currently where `NewDockerGateExecutor` is wired into `step.Runner`), add configuration:
      - `PLOY_BUILDGATE_MODE` env or config value with allowed values: `local-docker`, `remote-http`.
    - When `PLOY_BUILDGATE_MODE=remote-http`:
      - Construct `gate := NewHTTPGateExecutor(...)` instead of `NewDockerGateExecutor`.
    - Ensure fallback:
      - If `PLOY_BUILDGATE_MODE` is unset or invalid, default to the current docker gate behavior to avoid regressing existing clusters.
  - Test:
    - Extend `internal/workflow/runtime/step/runner_gate_test.go` to cover:
      - A runner constructed with HTTP gate mode calls the new executor.
      - A runner constructed with docker gate mode behaves exactly as before.

## Phase C — Define repo+diff inputs for gate from Mods runs
- [ ] Thread repo metadata into step manifests for gate — Ensure GateExecutor has `repo_url` and `ref`.
  - Component: `internal/nodeagent/manifest.go`, `internal/nodeagent/run_options.go`, `internal/workflow/contracts`.
  - Scope:
    - Verify that `StartRunRequest` already carries `RepoURL`, `BaseRef`, `TargetRef`, `CommitSHA` (used by healing). Confirm these fields are accessible where the runtime is initialized.
    - Extend `contracts.StepManifest` and/or gate spec options to carry:
      - `RepoURL` and `BuildGateRef` (choose `BaseRef` or `CommitSHA` per docs in `docs/build-gate/README.md`).
    - When building the main step manifest and gate spec in the node agent:
      - Populate these fields from `StartRunRequest`.
    - In `gate_http.go`, read this data to construct `BuildGateValidateRequest.repo_url` and `.ref`.
  - Test:
    - Add tests in `internal/nodeagent/manifest_test.go` to assert:
      - Step manifests and gate spec contain the expected repo metadata for a Mods run.
    - Run `go test ./internal/nodeagent -run TestManifestBuildWithGateRepoMeta`.

- [ ] Define how to compute `diff_patch` for gate from workspace state — Align in-process gate with HTTP repo+diff semantics.
  - Component: diff generator (`internal/workflow/runtime/step/diff_*`), node agent `executeWithHealing`.
  - Scope:
    - Reuse the existing diff generator used by `uploadHealingModDiff` (`r.createDiffGenerator()` in `internal/nodeagent/execution_healing.go`) to produce a unified diff of the workspace vs the baseline ref.
    - Introduce a helper, e.g. `buildGateDiffForWorkspace(ctx, workspace, baseline)` that:
      - Generates a diff in unified format.
      - Gzips + base64 encodes it as expected by `BuildGateValidateRequest.diff_patch` (see `docker/mods/mod-codex/buildgate-validate.sh` for reference).
    - Wire this helper into `gate_http.go`:
      - Before sending the HTTP request, compute `diff_patch` for:
        - Initial gate (if any uncommitted changes already exist in workspace).
        - Re‑gates after healing (healing changes are in workspace; diff captures them).
  - Test:
    - Add tests in `internal/nodeagent/execution_healing_test.go`:
      - Simulate a workspace with healing changes and assert `buildGateDiffForWorkspace` returns non‑empty `diff_patch`.
    - Add unit tests around encoding in a small helper package or in `gate_http_test.go`, comparing the encoded `diff_patch` against a known diff+gzip+base64 pipeline.

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
- [ ] Implement Build Gate worker loop on nodes — Let nodes claim and execute HTTP Build Gate jobs via docker.
  - Component: `internal/nodeagent` (new worker), `internal/workflow/runtime/step/gate_docker.go`.
  - Scope:
    - Add a new node-side controller or worker loop that:
      - Periodically calls `POST /v1/nodes/{id}/buildgate/claim` to claim jobs (`handlers_buildgate.go` → `ClaimBuildGateJob`).
      - For each claimed job:
        - ACK start via `POST /v1/nodes/{id}/buildgate/{job_id}/ack`.
        - Execute gate locally using `dockerGateExecutor`:
          - Clone `repo_url@ref` into a fresh temp `/workspace`.
          - If `diff_patch` is present, decode, decompress, and apply via `git apply`.
          - Run the profile-specific commands as in `gate_docker.go`.
        - Submit completion via `POST /v1/nodes/{id}/buildgate/{job_id}/complete` with `status` and `result`.
    - Ensure this worker loop is optional and controlled by config:
      - e.g., `PLOY_BUILDGATE_WORKER_ENABLED=true` for nodes designated as gate workers.
  - Test:
    - Add tests for the worker loop wiring in `internal/nodeagent` (using a fake store or HTTP client) to verify:
      - Jobs transition through `pending` → `claimed` → `running` → `completed`/`failed`.
      - Errors in docker execution are surfaced as `BuildgateJobStatusFailed` with an error message.
    - Manual/local: run a small stack with one control plane and two workers, one designated as a Build Gate worker, and verify jobs can be executed on either.

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

