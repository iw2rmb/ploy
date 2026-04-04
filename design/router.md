# Router As First-Class Job

## Summary

Promote Build Gate router execution from an inline node-side sub-step into a first-class queued job (`job_type=router`) with its own lifecycle, status, logs, artifacts, and failure semantics.

Expected outcome:
- Router execution is visible as a separate job in run/job APIs and CLI surfaces.
- Docker/node/runtime failures during router execution are reported as job `Error` for the router job (not collapsed into gate `Fail`).
- Healing insertion (`heal -> re_gate`) is triggered from router completion, not from failed gate completion.

## Scope

In scope:
- Add `router` as a canonical job type across domain types, DB schema constraints, lifecycle logic, server handlers, API schema, and node execution paths.
- Replace inline router call in gate execution with server-orchestrated router job creation and completion handling.
- Move healing insertion trigger point from failed gate completion to router completion.
- Ensure infra/runtime router failures propagate as terminal router job `Error` with persisted error message in run-repo diagnostics.
- Update lifecycle docs and tests for new chain shape.

Out of scope:
- Changing healing strategy semantics (`error_kind`, retries, candidate promotion).
- Changing router prompt format, image contract, or Codex entrypoint behavior.
- Introducing parallel router/heal branches in this change.

## Why This Is Needed

Current behavior mixes two concerns:
- Build outcome (`post_gate` compile/test failure), and
- Router infrastructure outcome (container create/start/mount/runtime).

Observed failure mode in current implementation:
- Gate job fails (`status=Fail`, `exit_code=1`),
- Inline router container fails to start (Docker mount/runtime issue),
- Router error is logged but swallowed,
- Recovery metadata defaults to `error_kind=unknown`,
- Final user-visible status is `unknown fail` under gate job, without a dedicated infra-error job.

This hides actionable ownership boundaries (build vs ploy infra) and prevents correct use of the existing job `Error` channel for router infra failures.

## Goals

- Represent router execution as an explicit workflow unit (`router` job).
- Preserve deterministic chain semantics and single-successor `next_id` model.
- Ensure router runtime failures map to router job `Error` (via existing `JobStatusFromRunError` behavior).
- Keep healing decisions deterministic and server-owned.
- Keep run/repo terminal reconciliation unchanged in principle (still based on terminal jobs in one chain).

## Non-goals

- Backward compatibility for old inline-router chain shape in persistence.
- Keeping old API enum set without `router`.
- Introducing a second recovery orchestrator path.

## Current Baseline (Observed)

1. Router is executed inline inside gate execution on node:
- [execution_orchestrator_gate.go](../internal/nodeagent/execution_orchestrator_gate.go) calls `runRouterForGateFailure(...)` before gate status upload.
- [execution_orchestrator_router_runtime.go](../internal/nodeagent/execution_orchestrator_router_runtime.go) runs router container with the same `run_id/job_id` as failing gate.

2. Router runtime error is swallowed (not returned as job runtime error):
- [execution_orchestrator_router_runtime.go](../internal/nodeagent/execution_orchestrator_router_runtime.go) logs `"router execution failed"` and `return nil`.
- Gate job then continues and uploads `Fail` based on gate result.

3. Healing insertion currently happens on failed gate completion:
- [jobs_complete_service_post_actions.go](../internal/server/handlers/jobs_complete_service_post_actions.go) triggers `maybeCreateHealingJobs(...)` on `CompletionChainEvaluateGateFailure`.
- [nodes_complete_healing.go](../internal/server/handlers/nodes_complete_healing.go) rewires failed gate `next_id` to `heal` and inserts `re_gate`.

4. Job type contract excludes router:
- [migs.go](../internal/domain/types/migs.go) has `pre_gate|mig|post_gate|heal|re_gate|mr`.
- [schema.sql](../internal/store/schema.sql) enforces the same set in jobs table check.
- [controlplane.yaml](../docs/api/components/schemas/controlplane.yaml) documents the same set.

## Target Contract Or Target Architecture

### 1. New Job Type

Add `router` as a canonical job type across domain/API/store contracts.

Invariant:
- Router runs as a normal queued job with its own `job_id`, status transitions, logs, stats, and metadata.

### 2. Chain Semantics

For any failing gate with healing configured:
1. Failed gate completes with `status=Fail` (build failure).
2. Server inserts `router` job and rewires `failed_gate.next_id -> router`.
3. Router completion decides next action:
   - `router=Success` + non-terminal classification with matching healing action: insert `heal -> re_gate` chain and set `router.next_id -> heal`.
   - `router=Success` + terminal classification or no matching action: cancel remainder from `router`.
   - `router=Fail|Error|Cancelled`: cancel remainder from `router`.

Invariant:
- Healing insertion is never performed directly from failed gate completion.
- Router is the only entrypoint to healing insertion after gate failure.

### 3. Error Propagation

- Any router container infra/runtime failure (Docker create/start/mount/network/runtime error) must result in router job `Error` via existing run-controller terminal-status logic.
- `run_repos.last_error` must include router error details when router ends in `Error`.

### 4. Recovery Context Ownership

- Gate job metadata remains source of gate findings (`build_gate_log`, detected stack).
- Router output metadata becomes authoritative source for recovery classification consumed by healing insertion.
- Healing claim context remains server-built and independent from node-local cache.

### 5. No Inline Router Execution in Gate Job

- Node gate execution does not run router.
- Node router execution is handled by dedicated router job executor path.

## Implementation Notes

### A. Contracts and Schema

- Extend `JobType` enum and validation in:
  - [migs.go](../internal/domain/types/migs.go)
- Extend jobs table `job_type` check constraint in:
  - [schema.sql](../internal/store/schema.sql)
- Extend API enum/docs for `job_type=router` in:
  - [controlplane.yaml](../docs/api/components/schemas/controlplane.yaml)

### B. Server Orchestration

- Refactor gate-failure completion flow:
  - In [jobs_complete_service_post_actions.go](../internal/server/handlers/jobs_complete_service_post_actions.go), replace direct `maybeCreateHealingJobs(...)` call with router insertion path.
- Introduce router insertion helper (new server handler module) that:
  - Validates failed gate eligibility (gate type + healing configured + router configured).
  - Creates `router` job with queued status.
  - Rewires failed gate successor link.
  - Stores router bootstrap metadata (gate source job ID, phase, loop kind).
- Add router completion post-action branch in completion service:
  - Parse router classification metadata.
  - Reuse existing transition evaluator (`EvaluateGateFailureTransition`) with classification input.
  - Insert `heal -> re_gate` or cancel chain accordingly.

### C. Node Execution

- Remove inline router call from gate path in:
  - [execution_orchestrator_gate.go](../internal/nodeagent/execution_orchestrator_gate.go)
- Add dedicated `executeRouterJob(...)` branch in node run controller dispatch.
- Reuse existing router manifest builder/runtime execution logic from:
  - [manifest.go](../internal/nodeagent/manifest.go)
  - [execution_orchestrator_router_runtime.go](../internal/nodeagent/execution_orchestrator_router_runtime.go)
  but wire it as primary job execution path that returns runtime errors to terminal status reporting.
- Router executor should populate `/in/build-gate.log` from recovery claim context (server-provided), not from node-local run cache assumptions.

### D. Recovery Claim Context

- Extend claim context builder to serve router jobs:
  - [nodes_claim_recovery_context.go](../internal/server/handlers/nodes_claim_recovery_context.go)
- Ensure `build_gate_log` is present for router jobs based on failed gate metadata.

### E. Metadata Shape

- Define a dedicated job meta kind for router output (`kind="router"`) that includes:
  - `bug_summary`
  - `recovery` (`loop_kind`, `error_kind`, `strategy_id`, `confidence`, `reason`, `expectations`, etc.)
- Keep existing gate metadata schema unchanged for gate jobs.
- Update projection/mapping in run-repo jobs API handler as needed.

### F. Lifecycle Rules

- Update lifecycle decision helpers to recognize `router` as a non-gate execution phase with explicit post-action handling.
- Keep `IsGateJobType(...)` semantics unchanged (`pre_gate|post_gate|re_gate` only).

### G. Testing

Add/adjust tests for:
- Router job insertion after failed gate.
- Router runtime Docker failure -> router `Error` and chain cancellation.
- Router success with terminal `error_kind` -> cancellation path.
- Router success with non-terminal `error_kind` -> heal/re-gate insertion.
- API/CLI job ordering and phase labels include router.
- Claim context for router includes `build_gate_log` and selected phase metadata.

## Milestones

### Milestone 1: Contracts + Schema + API Surface

Scope:
- Add `router` job type across type validation, schema constraints, and API schemas.

Expected results:
- System accepts/stores/serves `job_type=router` without ad-hoc casting.

Testable outcome:
- Unit tests for `JobType.Validate()` pass with `router`.
- Store integration tests can insert/list router jobs.
- API schema tests include router enum.

### Milestone 2: Server-Side Router Job Insertion

Scope:
- Replace direct healing insertion on failed gate completion with router job insertion and chain rewiring.

Expected results:
- Failed gate creates router successor job instead of immediate heal/re-gate chain.

Testable outcome:
- Completion-service tests show `failed_gate -> router` chain after gate failure.

### Milestone 3: Router Completion Drives Recovery

Scope:
- Add router completion post-actions for cancel/insert-healing decisions.

Expected results:
- Healing insertion occurs only after successful router classification.

Testable outcome:
- Tests cover all router terminal statuses and classification branches.

### Milestone 4: Node Router Executor + Error Propagation

Scope:
- Execute router as dedicated job in node agent and propagate runtime errors as `Error`.

Expected results:
- Docker/router infra issues are reflected as router job `Error` with persisted diagnostics.

Testable outcome:
- Node execution tests: injected runtime error leads to `status=Error`, `exit_code=-1`, and `run_repos.last_error` update.

## Acceptance Criteria

- A failed gate with healing configured always produces a persisted `router` job before any heal job.
- Router job appears in run repo job listing and follow stream with `job_type=router`.
- Router Docker/runtime failures result in router job `Error` (not hidden under gate `Fail`).
- Healing insertion (`heal -> re_gate`) is initiated only from router completion path.
- Existing non-router chains (`pre_gate -> mig -> post_gate`, MR behavior, run completion) remain deterministic.

## Risks

- Schema evolution risk: adding `router` to check constraints and enum surfaces requires consistent rollout across server/node binaries.
- Orchestration regression risk: moving healing insertion trigger point can break existing recovery tests if not fully migrated.
- Metadata migration risk: introducing router meta kind may require projection updates in CLI/API consumers.
- Scheduling race risk: incorrect link rewiring could leave orphan `Created` jobs; must keep one authoritative rewire transaction pattern.

## References

- Current inline router execution:
  - [execution_orchestrator_gate.go](../internal/nodeagent/execution_orchestrator_gate.go)
  - [execution_orchestrator_router_runtime.go](../internal/nodeagent/execution_orchestrator_router_runtime.go)
- Current gate-failure healing insertion:
  - [jobs_complete_service_post_actions.go](../internal/server/handlers/jobs_complete_service_post_actions.go)
  - [nodes_complete_healing.go](../internal/server/handlers/nodes_complete_healing.go)
- Lifecycle transition helpers:
  - [orchestrator.go](../internal/workflow/lifecycle/orchestrator.go)
- Job type and schema contracts:
  - [migs.go](../internal/domain/types/migs.go)
  - [schema.sql](../internal/store/schema.sql)
  - [controlplane.yaml](../docs/api/components/schemas/controlplane.yaml)
- Existing lifecycle docs:
  - [migs-lifecycle.md](../docs/migs-lifecycle.md)
