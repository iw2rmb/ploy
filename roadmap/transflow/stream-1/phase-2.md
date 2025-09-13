## Stream 1 · Phase 2 — Self-Healing From Build (LangGraph + Parallel Options) ⚠️ Partially Implemented

Goal: add a self-healing loop driven by build failures using LangGraph planner/reducer jobs and parallel healing branches (human, LLM-exec, ORW-generated).

Scope
- On build failure, capture stdout/stderr; persist as planner input.
- Planner job (LangGraph): inputs = repo metadata + last_error + KB snapshot + allowlists; output = `plan.json` with parallel options: human-step | llm-exec | orw-generated.
- Orchestrator fan-out: submit each option as a job/branch; first success wins; cancel others; collect outputs.
- Reducer job (LangGraph): inputs = branch results + winner; output = next actions (usually stop; else new `plan.json`).
- Keep CLI flow; no central service required.

 Parallel job types
 - human-step: wait for human MR/commit; success criteria = build passes after human push.
 - llm-exec: generate diff-only patch; apply and build-check.
 - orw-gen → openrewrite: generate ORW recipe (class/coords) via LLM; run OpenRewrite job; build-check.

Reuse
- Error-to-recipe mapping in `llm_error_analysis.go` for ORW-generated branch.
- ORW execution pipeline from Phase 1.
- LLM-exec runner from Stream 2 / Phase 1 for llm-exec branch.

Deliverables
- Planner/reducer job templates and CLI glue.
- Self-healing policy: `max_retries`, `cooldown` (0 in MVP), stop conditions.
- Runner emits a “healed” summary with winning branch + artifacts.
- Extend `mod.yaml` with optional `self_heal: {max_retries: 2}`; default to 1.
 - KB integration: implement case writes and summary read; add periodic compactor job spec (separate Nomad job) as future enhancement.

Implementation Steps
- Parse `self_heal.max_retries` from mod.yaml into runner config.
- On build failure, parse stdout/stderr; call `api/arf/llm_error_analysis.go` to map errors → ORW recipes.
- Apply chosen recipe(s) using existing ARF invocation; commit; re-run build; loop up to N.
- Unit tests: mock build failure → healing → passing path; verify retry count and summary content.

## Implementation Status

✅ **Completed:**
- Self-heal config parsing in `mod.yaml` (`self_heal.max_retries`, validation)
- Basic healing data structures (`TransflowHealingAttempt`, `TransflowHealingSummary`)
- Error analysis integration (uses `arf.ExtractErrorContext`)
- Job templates for planner, reducer, and healing options (`planner.hcl`, `reducer.hcl`, `llm_exec.hcl`, `orw_apply.hcl`)
- Diff validation and application utilities (`ValidateUnifiedDiff`, `ApplyUnifiedDiff`)
- Path allowlist validation for security
- Asset rendering methods for job submission

⚠️ **In Progress:**
- Orchestrator fan-out logic for parallel healing branches
- Job submission and result collection mechanisms

❌ **Pending:**
- Complete CLI integration in `cmd/ploy/main.go`
- KB read/write integration for learning from past fixes
- Full end-to-end testing with real build failures

Acceptance
- For a failing Java 11→17 migration, planner proposes parallel options; first successful branch (e.g., ORW fix or LLM patch) passes build; runner cancels others and finalizes.

Out of scope
- LLM-plan/exec, MCPs, MR.
