# Heal Error Contract: Structured Gate Errors and Automation Routing

## Summary
Improve heal effectiveness and reduce provider-side overload by making Build Gate failure input more actionable and less noisy. Introduce structured error payloads for known Gradle failure shapes, propagate them into healing context, and define a deterministic router task contract that remains limited to `code|deps|infra`.

## Scope
In scope:
- Structured Build Gate error payload generation for three modes:
  - `compileJava`
  - plugin apply failures (`An exception occurred applying plugin request`)
  - raw fallback
- Recovery context propagation of structured errors for `heal`/`re_gate` claims.
- `/in/errors.yaml` hydration for healing jobs while preserving `/in/build-gate.log`.
- Router task contract (`tasks[]`) with `error_kind` in `{code,deps,infra}` and index-based references to structured errors.
- Prompt contract updates that define when to use manual edits vs bash vs OpenRewrite, without project-specific clustering.
- Reuse existing ORW runtime images/contracts.

Out of scope:
- Budget or token-cap enforcement.
- New error kinds or project-specific sub-kinds.
- New ORW/healing runtime image family.
- Metrics/KPI instrumentation and dashboards.
- Copying/reattaching gate or sbom workspaces into heal runtime.
- Copying non-git-tracked generated files from gate/sbom containers into heal container.

## Why This Is Needed
Observed failures show that heal steps can consume high context and fail with provider retry exhaustion (`429`) while attempting wide manual edits. Existing trimmed logs are still represented as one large text message, which is insufficiently structured for deterministic mass-edit strategy selection.

Key pressure points:
- Trimmer emits a single text finding for Gradle failures and still carries broad content in some cases.
- Healing jobs currently rely on `/in/build-gate.log` text only.
- Router decisioning effectively collapses to one summary path and does not provide structured task items for selective prompting.
- ORW capabilities already exist but are not selected through a formal task-level contract.

## Observed Log Evidence
Evidence source directory:
- `/Users/v.v.kovalev/@scale/ploy-lib/research/3CAhnDZNSUr66d5mxmWUiThakpn`

Plugin-apply failure shape (`3CAhnXvC6L3ewDvUy33qOjRAkrw-build-gate.log`):
```text
FAILURE: Build failed with an exception.
* Where:
Build file '/workspace/build.gradle' line: 8
* What went wrong:
An exception occurred applying plugin request [id: 'org.springframework.boot', version: '3.0.5']
> Failed to apply plugin 'org.springframework.boot'.
   > Spring Boot plugin requires Gradle 7.x (7.4 or later).
```

CompileJava error shape with generated and non-generated paths (`3CAiv8IUHXNhmC9ppFsnlFSSbtB-build-gate-swagger-annotations.log`):
```text
Successfully generated code to /workspace/build/generated/sources/openapi
/workspace/build/generated/sources/openapi/.../GetIndexResponse.java:15: error: package io.swagger.v3.oas.annotations.media does not exist
/workspace/build/generated/sources/openapi/.../Indicator.java:21: error: cannot find symbol
@Schema(name = "Indicator", description = "...")
/workspace/src/main/java/.../GetIndexResponseBuilder.java:75: error: cannot find symbol
```

CompileJava error + stacktrace bloat shape (`build-gate.log` and `3CAk6tP7cnzKd8XlvX9Lwkz8Aq5-build-gate-open-telemetry.log`):
```text
67 errors
FAILURE: Build failed with an exception.
* What went wrong:
Execution failed for task ':compileJava'.
* Exception is:
org.gradle.api.tasks.TaskExecutionException: Execution failed for task ':compileJava'.
    at org.gradle.api.internal.tasks.execution...
```

These samples prove three target parser modes are required: `plugin_apply`, `compile_java`, and `raw`.

## Goals
- Provide compact, structured error input that preserves actionable compiler/plugin signals.
- Keep compatibility with existing healing flow by retaining `/in/build-gate.log`.
- Keep router semantics simple (`code|deps|infra`) while allowing multiple tasks with error references.
- Enable deterministic automation choice (`openrewrite|bash|manual`) based on error shape and transformation determinism.
- Reuse existing ORW contracts and images without introducing new runtime families.

## Non-goals
- No budget/cost guards in this phase.
- No project-specific domain clustering (for example Kafka-specific routing classes).
- No expansion of recovery kind taxonomy beyond existing values.
- No changes to universal gate/heal loop orchestration semantics.

## Current Baseline (Observed)
- Build Gate trimmer is stack-aware for Maven/Gradle and produces trimmed text findings:
  - `internal/workflow/step/build_gate_log_trimmer.go`
  - `internal/workflow/step/build_gate_log_trimmer_test.go`
- Gate metadata stores log findings as text messages and raw `LogsText` for local use:
  - `internal/workflow/contracts/build_gate_metadata.go`
  - `internal/workflow/step/gate_docker.go`
- Recovery claim context includes `build_gate_log` but no structured errors payload:
  - `internal/workflow/contracts/build_gate_metadata.go` (`RecoveryClaimContext`)
  - `internal/server/handlers/nodes_claim_recovery_context.go`
- Node agent hydrates `/in/build-gate.log` from recovery context and requires it for healing jobs:
  - `internal/nodeagent/execution_orchestrator_healing_runtime.go`
- Existing payload preference for healing log input is trimmed first finding text:
  - `internal/nodeagent/recovery_io.go`
- Gate/heal jobs use fresh rehydrated workspaces from base + ordered diffs; gate workspace is removed after execution:
  - `internal/nodeagent/execution_orchestrator_rehydrate.go`
  - `internal/nodeagent/execution_orchestrator_gate.go`
- ORW contracts and images already exist and are validated:
  - `internal/workflow/contracts/orw_cli_contract.go`
  - `images/orw/orw-cli-gradle`
  - `images/orw/orw-cli-maven`

## Target Contract or Target Architecture
### 1. Structured error payload (`errors.yaml`) v1
A new structured payload is produced from gate failure output for known Gradle failure classes.

Contract shape:
- Root keys:
  - `mode`: one of `compile_java`, `plugin_apply`, `raw`
  - `task`: optional (`compileJava` for `compile_java` mode)
  - `errors`: array
- `compile_java` mode:
  - Group errors by normalized signature from compiler output (message + symbol/package where present).
  - Keep per-entry `files[]` with `path:line` and optional short `snippet`.
  - Omit Gradle `:compileJava` stacktrace content from structured payload.
- `plugin_apply` mode:
  - Capture plugin id/version when extractable.
  - Capture concise multiline plugin-apply failure block.
- `raw` mode:
  - Preserve concise failure payload when specialized parsing does not match.

Rules:
- Structured payload is additive and does not replace existing text finding behavior.
- Parsing must be deterministic and tool-bounded; no project-specific heuristics.

### 2. Recovery context propagation
`RecoveryClaimContext` gains an optional structured error field (JSON-serializable payload representing `errors.yaml` content).

Rules:
- `build_gate_log` remains required for heal hydration.
- Structured errors are included when available and omitted otherwise.
- Backward path (no structured field) remains valid.

### 3. Healing `/in` hydration contract
For heal/re-gate jobs:
- Always hydrate `/in/build-gate.log` (existing behavior).
- Hydrate `/in/errors.yaml` when structured errors are present.

Rules:
- `/in/errors.yaml` is best-effort optional input, not a hard prerequisite.
- Missing `/in/errors.yaml` must not fail healing when `/in/build-gate.log` exists.
- Do not copy or reattach gate/sbom workspace content into heal runtime.
- Generated/non-git file paths from compiler output are diagnostic references in `errors.yaml`; fixes must target source-of-truth inputs and regeneration paths.

### 4. Router task contract
Router output becomes task-array oriented:
- `tasks[]` entries include:
  - `error_kind`: `code|deps|infra`
  - `bug_summary`: concise single-line summary
  - `items`: list of indexes into `errors.yaml.errors`

Rules:
- No sub-kinds beyond existing recovery kinds.
- No project-specific clustering keys.
- Router may emit multiple tasks, but kinds remain in existing taxonomy.

### 5. Automation choice contract
Prompt/router contract defines strategy selection per task:
- Prefer `openrewrite` or bash for deterministic, repeated transformations.
- Use manual edits when transformation is localized or semantic beyond deterministic rewrite/script.
- Selection criteria are based on transformation determinism and error shape, not project-specific rules.

## Implementation Notes
- Extend trimmer pipeline in gate execution metadata construction to emit structured payload for supported Gradle modes.
- Extend recovery metadata transport path from gate metadata -> claim response -> node hydration.
- Keep ownership boundaries:
  - parsing/normalization in workflow step layer,
  - transport in contracts/server claim handlers,
  - filesystem materialization in nodeagent healing runtime.
- Keep ORW reuse through existing images/contracts; do not add a new image class.

## Milestones
### Milestone 1: Structured Gradle error payload generation
Scope:
- Add structured payload generation for `compile_java`, `plugin_apply`, and `raw` fallback.
Expected Results:
- Gate failure metadata can produce deterministic structured payload for known Gradle classes.
Testable outcome:
- Unit tests cover all three modes and ensure compileJava stacktrace is excluded from structured payload.

### Milestone 2: Recovery context and `/in/errors.yaml` hydration
Scope:
- Add structured payload field to recovery context and hydrate `/in/errors.yaml` when present.
Expected Results:
- Heal/re-gate jobs receive structured errors alongside existing build-gate log input.
Testable outcome:
- Claim and nodeagent tests validate presence/absence paths and preserve `/in/build-gate.log` behavior.

### Milestone 3: Router/prompt automation contract alignment
Scope:
- Document and enforce task-array contract with `code|deps|infra` plus indexed `items`.
- Document strategy selection rules for `openrewrite|bash|manual`.
Expected Results:
- Router/prompt behavior is explicit and reproducible without project-specific clustering.
Testable outcome:
- Contract docs/examples are aligned with current runtime and e2e healing expectations.

## Acceptance Criteria
- Build Gate failure processing can produce structured errors in one of three modes: `compile_java`, `plugin_apply`, or `raw`.
- For heal/re-gate claims with structured errors, node agent writes `/in/errors.yaml` and `/in/build-gate.log`.
- For claims without structured errors, heal/re-gate execution remains unchanged and still requires `/in/build-gate.log`.
- Router task contract uses only `code|deps|infra` and index references into structured errors.
- ORW path remains on existing ORW images/contracts with no new runtime image introduced.

## Risks
- Overfitting compileJava/plugin parsers to one Gradle output variant can reduce coverage.
- Poor grouping quality in `compile_java` mode can mislead strategy selection.
- Contract drift between structured payload docs and claim/node runtime fields can break healing inputs.
- Prompt-level automation rules may still underperform if examples are ambiguous.
- Agents can still attempt direct edits in generated paths; prompts must explicitly bias toward source edits and regeneration.

## References
- `internal/workflow/step/build_gate_log_trimmer.go`
- `internal/workflow/step/build_gate_log_trimmer_test.go`
- `internal/workflow/step/gate_docker.go`
- `internal/workflow/contracts/build_gate_metadata.go`
- `internal/server/handlers/nodes_claim_recovery_context.go`
- `internal/nodeagent/execution_orchestrator_healing_runtime.go`
- `internal/nodeagent/recovery_io.go`
- `internal/workflow/contracts/orw_cli_contract.go`
- `docs/build-gate/trimmer.md`
- `/Users/v.v.kovalev/@scale/ploy-lib/research/3CAhnDZNSUr66d5mxmWUiThakpn/thoughts.md`
- `/Users/v.v.kovalev/@scale/ploy-lib/research/3CAhnDZNSUr66d5mxmWUiThakpn/3CAhnXvC6L3ewDvUy33qOjRAkrw-build-gate.log`
- `/Users/v.v.kovalev/@scale/ploy-lib/research/3CAhnDZNSUr66d5mxmWUiThakpn/3CAiv8IUHXNhmC9ppFsnlFSSbtB-build-gate-swagger-annotations.log`
- `/Users/v.v.kovalev/@scale/ploy-lib/research/3CAhnDZNSUr66d5mxmWUiThakpn/3CAk6tP7cnzKd8XlvX9Lwkz8Aq5-build-gate-open-telemetry.log`
- `/Users/v.v.kovalev/@scale/ploy-lib/research/3CAhnDZNSUr66d5mxmWUiThakpn/build-gate.log`
