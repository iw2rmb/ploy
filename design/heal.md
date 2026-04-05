# Build Gate Healing via Single `build_gate.heal` Workflow

## Summary

Replace router-driven healing selection with a single healing workflow contract under `build_gate.heal`.

Expected outcome:
- Build Gate failures trigger direct `heal -> re_gate` orchestration without a separate router phase.
- Error classification and prompt selection happen inside the healing workflow (Amata), not in control-plane routing logic.
- The spec surface removes `build_gate.router` and `build_gate.healing.by_error_kind`.

## Scope

In scope:
- Introduce `build_gate.heal` as the canonical healing step spec (job type remains `heal`).
- Remove router from Build Gate recovery orchestration and runtime contracts.
- Remove server-side healing selection by `error_kind` (`healing.by_error_kind`), moving that decision into the healing workflow itself.
- Define deterministic retry/cancel behavior for the direct `heal -> re_gate` loop.

Out of scope:
- Changing base gate execution semantics for successful gates.
- Changing diff/rehydration behavior for regular `mig` steps.
- Adding parallel healing branches.

## Why This Is Needed

Current recovery behavior spreads decision-making across two layers:
- Router classification produced by one container, then
- Server-side action selection via `healing.by_error_kind`.

This creates unnecessary orchestration complexity and extra contracts (`router` + classifier schema + `by_error_kind` map) for one recovery loop.

A single healing workflow can read `/in/build-gate.log`, classify internally, and choose the right prompt/tool path in one place. This keeps control plane logic simpler and keeps recovery strategy logic with the workflow that executes it.

## Goals

- Make `build_gate.heal` the only healing spec surface for Build Gate recovery.
- Keep chain progression deterministic (`gate fail -> heal -> re_gate`, retries bounded).
- Keep orchestration server-owned and simple (no classifier-based branch table).
- Keep healing strategy flexibility inside Amata workflow logic.

## Non-goals

- Backward compatibility for `build_gate.router` or `build_gate.healing.by_error_kind`.
- Preserving router-specific metadata contracts in job APIs.
- Introducing new job types.

## Current Baseline (Observed)

1. Contracts currently require router + classifier selector:
- [build_gate_config.go](../internal/workflow/contracts/build_gate_config.go) defines `BuildGateConfig.Router` and `HealingSpec.ByErrorKind`.

2. Node currently executes router inline on gate failure:
- [execution_orchestrator_router_runtime.go](../internal/nodeagent/execution_orchestrator_router_runtime.go) runs router, parses classification, and writes recovery metadata.

3. Server currently selects healing action by classifier kind:
- [orchestrator.go](../internal/workflow/lifecycle/orchestrator.go) `EvaluateGateFailureTransition(...)` chooses action from `healing.by_error_kind[error_kind]`.
- [nodes_complete_healing.go](../internal/server/handlers/nodes_complete_healing.go) inserts `heal -> re_gate` from failed gate completion using classifier-driven decision.

4. Public docs and examples currently document router/by_error_kind model:
- [migs-lifecycle.md](../docs/migs-lifecycle.md)
- [mig.example.yaml](../docs/schemas/mig.example.yaml)

## Target Contract Or Target Architecture

### 1. Single Healing Spec

`build_gate.heal` is the only recovery step contract for gate failures.

Canonical shape (conceptual):
- mig-like execution fields: `image`, `command`, `envs`, `ca`, `in`, `out`, `home`, optional `amata`.
- `retries` controls max heal attempts for the failing gate loop.

Invariant:
- No `build_gate.router` field.
- No `build_gate.healing.by_error_kind` field.
- `heal` is always executed as a queued `jobs` row (`job_type=heal`); inline healing execution is forbidden.

### 2. Recovery Lifecycle

For gate failure with healing configured:
1. Gate job completes with `status=Fail`.
2. Server inserts/continues `heal -> re_gate` chain directly.
3. Heal executes once per attempt using `build_gate.heal` spec.
4. Re-gate validates healed workspace.
5. Loop repeats until re-gate success or retries exhausted.

Invariant:
- No router job/phase in chain.
- No control-plane branch by `error_kind`.

### 3. Decision Ownership

- Control plane owns retry counting, chain insertion, and cancellation.
- Healing workflow owns internal classification and prompt/tool path selection.
- Classifier output is optional observability data, not orchestration input.

### 4. Failure Semantics

- Heal runtime/container failures are terminal heal job outcomes (`Fail`/`Error`) via existing status mapping.
- Re-gate failures consume retry budget until exhaustion.
- Exhaustion cancels remaining chain and finalizes repo/run as failed.

### 5. Gate Output Contract

- Build Gate orchestration does not require router-style one-line failure summaries.
- Gate/re-gate user-facing continuation output is generic (`Exit <code>: Error`), not classifier- or summary-derived.
- Healing keeps one-line human summary ownership via heal job `action_summary`.

## Implementation Notes

### A. Contracts

- Replace `BuildGateConfig.Healing` + `BuildGateConfig.Router` with a single `BuildGateConfig.Heal` pointer in:
  - [build_gate_config.go](../internal/workflow/contracts/build_gate_config.go)
- Remove `HealingSpec.ByErrorKind` and router-specific structs.
- Keep heal job type as existing `heal` (no new `job_type`).

### B. Server Lifecycle

- Simplify gate-failure transition evaluator to consume one heal spec + retries, without classifier action lookup.
- Keep `heal -> re_gate` insertion path in completion flow, but remove router/classifier branch points.

### C. Node Execution

- Remove inline router execution from gate paths.
- Execute heal job only from claim-provided `build_gate.heal` spec.
- Keep existing `/in/build-gate.log` hydration and healing artifacts contract.

### D. API/Metadata

- Remove router-specific metadata fields from contracts where they exist only for routing decisions.
- Keep minimal healing metadata for observability (`action_summary`, retry/attempt context).

### E. Docs and Examples

- Update lifecycle/spec docs to describe single heal workflow contract and direct gate-fail loop.
- Remove router and `by_error_kind` examples from schemas/docs.

## Milestones

### Milestone 1: Contract Surface Rewrite

Scope:
- Introduce `build_gate.heal`; remove router/by_error_kind config types and parser paths.

Expected result:
- Run specs accept one healing step contract only.

Testable outcome:
- Contract parsing tests pass for `build_gate.heal` and reject removed fields.

### Milestone 2: Lifecycle Simplification

Scope:
- Remove classifier-based selection from gate-failure orchestration; keep direct `heal -> re_gate` retries.

Expected result:
- Gate failure transitions are independent of router/classifier output.

Testable outcome:
- Completion/lifecycle tests validate deterministic retry and cancel behavior.

### Milestone 3: Runtime and Docs Alignment

Scope:
- Remove router runtime path and align docs/examples with single-heal architecture.

Expected result:
- Node runtime no longer runs router; docs present only `build_gate.heal`.

Testable outcome:
- Node execution tests and docs/schema checks pass without router references.

## Acceptance Criteria

- Spec contract exposes `build_gate.heal` as the only healing config entry.
- Failed gates with healing configured always enter direct `heal -> re_gate` loop.
- Retry exhaustion is deterministic and independent of classifier branches.
- Gate/re-gate output no longer depends on `error_kind` or `bug_summary` one-liners.
- No router phase appears in lifecycle docs, API enums, or job-chain expectations.

## Risks

- Migration risk for existing specs that still use router/by_error_kind.
- Hidden coupling risk where runtime/tests assume router-produced metadata.
- Docs drift risk if lifecycle/spec examples are not updated atomically.

## References

- Existing router/healing contracts:
  - [build_gate_config.go](../internal/workflow/contracts/build_gate_config.go)
- Existing router runtime path:
  - [execution_orchestrator_router_runtime.go](../internal/nodeagent/execution_orchestrator_router_runtime.go)
- Existing healing insertion/evaluation:
  - [nodes_complete_healing.go](../internal/server/handlers/nodes_complete_healing.go)
  - [orchestrator.go](../internal/workflow/lifecycle/orchestrator.go)
- Current lifecycle/spec docs to align:
  - [migs-lifecycle.md](../docs/migs-lifecycle.md)
  - [mig.example.yaml](../docs/schemas/mig.example.yaml)
