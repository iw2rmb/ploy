# Goal-Bounded Dependency Bump Controller

## Summary
Define a deterministic algorithm for mass dependency bumps where the full impacted library set is unknown upfront.  
The controller is goal-bounded (example: Spring Boot `2.7 -> 3.0`) and expands changes only from observed compile/runtime evidence.

## Scope
In scope:
- Objective-driven dependency bump loop for Java stacks.
- Bridge-version handling for API migrations requiring typed OpenRewrite.
- Integration with existing `sbom -> hook -> gate -> heal -> re_gate` flow.
- Deterministic stop conditions and reporting.

Out of scope:
- New orchestration model or job types.
- Unbounded autonomous “fix anything” editing.
- Backward compatibility with legacy contracts.

## Why This Is Needed
- Mass migrations have unknown transitive impact; static dependency allowlists are insufficient.
- Typed OpenRewrite requires resolvable symbols; direct jumps to versions that removed old APIs can block recipe matching.
- Current system already has SBOM snapshots, hook matching on version changes, ORW runtime contracts, and healing loops. We need one algorithm that composes these parts deterministically.

## Goals
- Keep dependency expansion objective-bounded.
- Prefer minimal, monotonic version movement.
- Use bridge versions when API rename/removal requires `A` and `B` overlap.
- Keep retries bounded and auditable.

## Non-goals
- Perfect automatic remediation for all builds.
- Per-project custom logic hardcoded in control-plane.
- Introducing legacy-shape validation guards.

## Current Baseline (Observed)
- Unified job pipeline executes gate-adjacent phases in deterministic order (`sbom`, optional `hook`, then gate): `/Users/v.v.kovalev/@iw2rmb/ploy/docs/build-gate/README.md`.
- Hook specs support SBOM predicates including `on_change {name, from, to}`: `/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/hook/spec.go`.
- Hook matcher already compares versions and detects transitions: `/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/hook/matcher.go`.
- Claim-time runtime hook decision exposes matched package and prev/current versions: `/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/contracts/hook_runtime.go`.
- Runtime hook chain insertion after `sbom` exists and is deterministic: `/Users/v.v.kovalev/@iw2rmb/ploy/internal/server/handlers/jobs_complete_service_runtime_hooks.go`.
- SBOM compatibility endpoint exists (`/v1/sboms/compat`) and returns minimal compatible versions from observed evidence: `/Users/v.v.kovalev/@iw2rmb/ploy/internal/server/handlers/sboms_compat.go`.
- ORW runtime contract exists, including unsupported reason `type-attribution-unavailable`: `/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/contracts/orw_cli_contract.go`.

## Target Contract or Target Architecture
### 1. Inputs
- `objective_id` is the migration identity (`mig` name/id).
- `root_bumps[]` (initial explicit bumps, may be small).
- `budget`:
  - max iterations,
  - max files changed,
  - max dependency edits per iteration.

### 1.1 Execution Placement
- Bump cycles execute inside the existing run execution as child chains of `mig` and `heal`.
- No standalone top-level bump phase is introduced.
- Child-chain progression remains `next_id`-driven and reuses current job lifecycle semantics.

### 2. Deterministic Bump Loop (Core Algorithm)
1. Apply `root_bumps[]`.
2. Run compile gate.
3. If success: stop.
4. Classify failures:
   - `deps/api-mismatch` (missing symbols/imports/signatures),
   - `infra`,
   - `non-actionable`.
5. For `deps/api-mismatch`, derive candidate dependencies from evidence:
   - current SBOM snapshot,
   - error signatures,
   - `/v1/sboms/compat` floors for target stack.
6. For each candidate dependency, choose action:
   - direct bump, or
   - bridge flow when API migration needs overlap (`A` and `B` both resolvable).
7. Bridge flow:
   1. bump to overlap version,
   2. run pinned ORW recipe,
   3. bump to target version.
8. Re-run compile gate.
9. Repeat from step 3 until success or budget exhausted.

### 3. Expansion Rules
- Expansion is allowed only when tied to current failure evidence.
- New dependency edits must be monotonic toward objective target.
- No unrelated dependency drift.

### 4. Stop Rules
- Stop on:
  - success,
  - exhausted budget,
  - failure class outside `deps/api-mismatch`.
- Emit terminal report with:
  - proven cause,
  - attempted dependency transitions,
  - bridge+ORW operations,
  - unresolved blockers.

## Implementation Notes
- Implement controller in migration/hook runtime (no new control-plane job types).
- Treat the active `mig` as the objective authority; do not add a separate objective registry.
- Use existing hook lifecycle:
  - `sbom` snapshot as dependency evidence,
  - runtime hook decisions for package transition context,
  - gate/heal/re-gate loop for validation.
- Add an objective manifest in migration repository (not control-plane) containing:
  - target stack baseline,
  - known bridge mappings (`dep`, `from range`, `bridge`, `target`, `orw recipe`),
  - per-objective budgets.
- Keep ORW use strict:
  - run only with pinned recipes and explicit dependency target,
  - if ORW reports `type-attribution-unavailable`, require bridge step or fail fast.

## Milestones
### Milestone 1: Objective Contract
Scope:
- Define `objective_id`, budgets, and report schema.
Expected Results:
- One deterministic controller config per migration objective.
Testable outcome:
- Contract validation tests pass.

### Milestone 2: Evidence-Driven Expansion
Scope:
- Candidate selection from compile errors + SBOM + `/v1/sboms/compat`.
Expected Results:
- Additional dependency edits are only evidence-linked.
Testable outcome:
- Integration tests show no unrelated dependency edits.

### Milestone 3: Bridge + ORW Execution
Scope:
- Bridge detection and three-step bridge flow.
Expected Results:
- API rename/removal migrations succeed when overlap exists.
Testable outcome:
- Failing fixture reproduces success only with bridge+ORW path.

### Milestone 4: Bounded Retry + Reporting
Scope:
- Stop rules and terminal report.
Expected Results:
- Predictable termination and actionable diagnostics.
Testable outcome:
- Exhaustion scenarios produce deterministic failure reports.

## Acceptance Criteria
- Controller never performs unbounded edit loops.
- Every non-root dependency change is traceable to observed failure evidence.
- Bridge+ORW flow is used when overlap is required and defined.
- Final output always includes a machine-readable attempt report.
- Existing job orchestration (`sbom`, `hook`, gate, heal, `re_gate`) remains unchanged.

## Risks
- Error-to-dependency attribution can be noisy for highly coupled projects.
- Missing bridge mappings can still block typed ORW migrations.
- SBOM compatibility evidence may be sparse for uncommon stacks/libs.
- Objective budget may be too strict or too loose for specific repositories.

## References
- `/Users/v.v.kovalev/@iw2rmb/ploy/docs/build-gate/README.md`
- `/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/hook/spec.go`
- `/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/hook/matcher.go`
- `/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/contracts/hook_runtime.go`
- `/Users/v.v.kovalev/@iw2rmb/ploy/internal/server/handlers/jobs_complete_service_runtime_hooks.go`
- `/Users/v.v.kovalev/@iw2rmb/ploy/internal/server/handlers/sboms_compat.go`
- `/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/contracts/orw_cli_contract.go`
