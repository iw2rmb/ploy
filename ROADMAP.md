# Mods Build Gate: Pre-Run Gate + Per-Mod Post Gates

> When following this template:
> - Align to the template structure
> - Include steps to update relevant docs

Scope: Run a single pre-mod Build Gate before any mods, then run a post-mod Build Gate after every mod in `mods[]` that exits with code 0. Every Build Gate (pre and post) uses the existing fail → heal → re-gate loop when healing mods are configured; if a gate cannot be healed, the run fails and no further mods are executed.

Documentation: `docs/mods-lifecycle.md`, `docs/build-gate/README.md`, `docs/envs/README.md`, `docs/schemas/mod.example.yaml`, `docs/how-to/publish-mods.md`, `internal/nodeagent/execution_healing.go`, `internal/nodeagent/execution_orchestrator.go`, `internal/workflow/runtime/step/stub.go`, `internal/workflow/runtime/step/runner_gate_test.go`, `internal/domain/types/runstats.go`, `internal/cli/mods/inspect.go`, `tests/integration/build-gate`, `tests/e2e/mods`.

Legend: [ ] todo, [x] done.

## Phase A — Clarify pre-gate vs post-gate semantics
- [x] Document gate sequence for single- and multi-mod runs — Make pre-/post-gate order explicit.
  - Component: `docs/mods-lifecycle.md`.
  - Scope:
    - Update the Build Gate section to describe:
      - Pre-mod gate: runs once on the initial workspace before any mods execute; on failure and when healing mods are configured, enter the fail → heal mods → re-gate loop; if still failing after retries, the run exits without executing mods.
      - Post-mod gates: run on the same workspace after each mod in `mods[]` that exits with code 0; on failure and when healing mods are configured, enter the same fail → heal mods → re-gate loop; if still failing after retries, the run fails and no further mods execute.
     - Extend the Mods execution diagrams or bullet list to show: `pre-gate(+healing) → mod[0] → post-gate[0](+healing) → mod[1] → post-gate[1](+healing) → ...`.
  - Test: `rg "Pre-mod Build Gate" docs/mods-lifecycle.md` — Confirm the sequence is described; run `make test`.
- [x] Document workspace/rehydration semantics for gates — Avoid ambiguity about which code version each gate sees.
  - Component: `docs/mods-lifecycle.md`.
  - Scope:
    - Add a short subsection referencing:
      - `internal/nodeagent/execution_orchestrator.go` — `executeRun` and `rehydrateWorkspaceForStep`.
      - Explain that:
        - The pre-mod gate runs on the initial hydrated workspace (step 0).
        - Each post-mod gate runs on the rehydrated workspace for that step (base clone + diffs from prior mods).
  - Test: `rg "rehydrateWorkspaceForStep" docs/mods-lifecycle.md` — Confirm workspace semantics are called out.

## Phase B — Keep step.Runner as pre-mod gate + mod executor
- [x] Confirm current Runner.Run behavior (pre-gate per call) — Establish the baseline before adding helpers.
  - Component: `internal/workflow/runtime/step/stub.go`.
  - Scope:
    - Read `Runner.Run` implementation to confirm stages:
      - Hydration.
      - Pre-mod Build Gate (when `Gate.Enabled`).
      - Container execution.
      - Diff generation.
    - Capture this in a short comment in `stub.go` to make the contract explicit.
  - Test:
    - Ensure existing tests in `internal/workflow/runtime/step/runner_gate_test.go` continue to pass (`TestRunner_Run_WithBuildGateEnabled`, `TestRunner_Run_PreModGateFailureWithoutHealing`).
    - Run `go test ./internal/workflow/runtime/step -run TestRunner_Run_`.
- [x] Add a gate-only helper in step package (no container execution) — Allow nodeagent to reuse gate logic without running a mod.
  - Component: `internal/workflow/runtime/step`.
  - Scope:
    - Add a new file, e.g. `internal/workflow/runtime/step/gate_only.go`, with a helper:
      ```go
      func RunGateOnly(ctx context.Context, r *Runner, req Request) (Result, error) {
          // Hydrate workspace, run gate (if enabled), populate Result.BuildGate and timings.
          // Do not create or start any containers.
      }
      ```
    - Internally, share as much logic as possible with `Runner.Run` for the gate stage to keep behavior identical.
  - Test:
    - Add `TestRunGateOnly_Enabled` / `TestRunGateOnly_Disabled` in `internal/workflow/runtime/step/runner_gate_test.go` to assert:
      - Gate executor is called when enabled.
      - No container runtime methods are invoked.
    - Run `go test ./internal/workflow/runtime/step -run TestRunGateOnly_`.

## Phase C — Orchestrate pre-mod gate + healing before any mods
- [x] Factor gate+healing orchestration into a reusable helper — Share logic between pre- and post-mod gates.
  - Component: `internal/nodeagent/execution_healing.go`.
  - Scope:
    - Extract the gate+healing portion of `executeWithHealing` into:
      ```go
      func (r *runController) runGateWithHealing(
          ctx context.Context,
          runner step.Runner,
          req StartRunRequest,
          manifest contracts.StepManifest,
          workspace, outDir string,
          inDir *string,
          gatePhase string, // "pre" or "post"
      ) (*contracts.BuildGateStageMetadata, []gateRunMetadata, error)
      ```
    - Move the following into this helper:
      - Gate execution (`runner.Gate.Execute`).
      - `/in` directory creation and `/in/build-gate.log` writing.
      - Healing mods loop and Codex session handling.
      - Re-gate loop and `ReGates` slice population.
      - Error wrapping on healing exhaustion, still rooted at `step.ErrBuildGateFailed`.
  - Test:
    - Refactor existing tests in `internal/nodeagent/execution_healing_test.go` to call `runGateWithHealing` via `executeWithHealing`, preserving expectations for:
      - Gate fail → heal → re-gate success.
      - Gate fail → healing exhausted → `ErrBuildGateFailed`.
    - Run `go test ./internal/nodeagent -run TestExecuteWithHealing`.
- [x] Run a single pre-mod gate with healing before executing mods — Ensure the baseline compiles before any changes.
  - Component: `internal/nodeagent/execution_orchestrator.go`, `internal/nodeagent/execution_healing.go`.
  - Scope:
    - In `executeRun`, before the loop over `stepIndex`:
      - Hydrate the base workspace for step 0 using existing rehydration logic.
      - Call `runGateWithHealing(..., gatePhase="pre")` once for the run.
      - Persist the resulting `PreGate` and `ReGates` into an `executionResult` accumulator.
    - Ensure that if the pre-mod gate cannot be healed:
      - The run terminates with `ErrBuildGateFailed` and `reason="build-gate"`.
      - No mods are executed.
  - Test:
    - Extend `internal/nodeagent/execution_healing_test.go` with focused scenarios:
      - Pre-mod gate fails, healing fixes it, and the run proceeds to mods.
      - Pre-mod gate fails, healing retries are exhausted, and the run exits without invoking any mod logic.
    - Run `go test ./internal/nodeagent -run TestExecuteWithHealing`.

## Phase D — Add per-mod post gates with healing
- [ ] Wire post-mod gates to reuse runGateWithHealing — Keep pre- and post-gate behavior consistent.
  - Component: `internal/nodeagent/execution_healing.go`.
  - Scope:
    - After each mod step completes with `ExitCode == 0` inside `executeWithHealing` or its successor:
      - Call `runGateWithHealing(..., gatePhase="post")` on the same workspace.
      - Append any returned `gateRunMetadata` entries to the existing `ReGates` slice for full history.
      - Set `result.BuildGate` to the final post-mod gate metadata.
  - Test:
    - Add tests in `internal/nodeagent/execution_healing_test.go` to cover:
      - Pre-mod gate passes, mod exits 0, post-mod gate passes without healing.
      - Pre-mod gate passes, mod exits 0, post-mod gate fails once, heals, then passes.
    - Run `go test ./internal/nodeagent -run TestExecuteWithHealing_PostGate`.
- [ ] Stop executing further mods when a post-mod gate cannot be healed — Ensure a failing post gate terminates the run.
  - Component: `internal/nodeagent/execution_orchestrator.go`.
  - Scope:
    - In `executeRun`, when the execution result for a given `stepIndex` contains an error rooted at `step.ErrBuildGateFailed` from a post gate:
      - Set `finalExecErr` and `finalExecResult` and break out of the steps loop.
      - Ensure no subsequent `stepIndex` is executed.
  - Test:
    - Add a multi-mod test in `internal/nodeagent/execution_healing_test.go` or a new `execution_multistep_postgate_test.go` that:
      - Simulates two mods where the first mod’s post gate passes, the second mod’s post gate fails after healing retries.
      - Asserts that only the first mod’s container is executed and the run terminates on the second post gate failure.
    - Run `go test ./internal/nodeagent -run TestExecuteRun_PostGateStopsFurtherMods`.

## Phase E — Preserve stats and CLI surface while switching to pre-run + per-mod gates
- [ ] Ensure stats capture pre-run gate and last post gate as final_gate — Keep CLI/API gate summaries correct with new semantics.
  - Component: `internal/nodeagent/execution_orchestrator.go`, `internal/domain/types/runstats.go`.
  - Scope:
    - In `buildExecutionStats`, keep existing logic but ensure:
      - `execResult.PreGate` is populated from the single pre-mod gate.
      - `execResult.ReGates` contains all healing re-gates from both pre- and post-mod phases in chronological order.
      - `result.BuildGate` reflects the last post-mod gate result (or the pre-mod gate when no mods executed).
    - In `buildGateStats`:
      - Continue to build `gate["pre_gate"]` from `execResult.PreGate`.
      - Continue to build `gate["re_gates"]` from `execResult.ReGates` in order (pre- and post-mod healing).
      - Keep `gate["final_gate"]` sourced from `result.BuildGate` and include uploaded logs as today.
    - In `RunStats.GateSummary()` (`internal/domain/types/runstats.go`):
      - Confirm the existing priority (final_gate → last re-gate → pre_gate) still holds.
      - Update comments to clarify that `final_gate` represents the latest post-mod gate, or pre-mod gate for runs with no mods.
  - Test:
    - Add/extend unit tests in `internal/domain/types/runstats_test.go` to cover:
      - A run with pre_gate + post-mod final_gate; expect summary `"passed duration=...ms"` based on final gate.
      - A run with failed final_gate after one or more mods; expect summary `"failed final-gate duration=...ms"`.
    - Extend `internal/nodeagent/statusuploader_test.go` or related tests to assert `stats["gate"]` contains `pre_gate`, `re_gates`, and `final_gate` keys when gate is enabled.

## Phase F — Update CLI, integration tests, and how-to docs
- [ ] Keep CLI gate summary aligned with final (post-mod) gate — Ensure users see the post-mod gate result for each ticket.
  - Component: `internal/cli/mods/inspect.go`.
  - Scope:
    - Confirm `ploy mod inspect <ticket-id>` uses `GateSummary()` from stats and does not assume gate is pre-mod only.
    - Update any inline help or output examples (if present) to mention that gate status reflects the final gate, which is typically post-mod.
  - Test:
    - Add/adjust unit tests in `internal/cli/mods/inspect_test.go` (or equivalent) to assert that:
      - A stats payload with `final_gate.failed` produces `Gate: failed final-gate ...`.
      - A stats payload with only pre_gate (no mods executed) still renders `Gate: failed pre-gate ...`.
    - Run `go test ./internal/cli/mods -run TestInspect`.

- [ ] Extend integration and E2E coverage for post-mod failures and healing — Guard against regressions in gate-heal-regate behavior.
  - Component: `tests/integration/build-gate`, `tests/e2e/mods`.
  - Scope:
    - In `tests/integration/build-gate`, add a scenario (e.g. `scenario-post-mod-fail.sh`) that:
      - Uses a Mods spec where the initial code passes pre-gate, the mod introduces a compile error, and healing plus post-gate restore a passing state.
      - Asserts that:
        - The run fails when healing cannot fix the post-mod break.
        - The run succeeds when healing does fix it, and `GateSummary()` reflects the final post-gate result.
    - In `tests/e2e/mods` (e.g., `scenario-multi-step` or `scenario-multi-node-rehydration`):
      - Extend or add a scenario where:
        - Each mods[] entry is followed by a post-gate check, with at least one step requiring healing after post-gate failure.
        - Final artifacts and SSE events show gate and healing events for each step.
  - Test:
    - Run the new integration scenario script(s) under `tests/integration/build-gate`.
    - Run the affected E2E scripts under `tests/e2e/mods` and verify:
      - For multi-step specs, Build Gate runs after every mod that exits with code 0.
      - Healing applies uniformly to any failing gate (pre or post), with consistent logs and stats.
