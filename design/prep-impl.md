# Prep Implementation (As-Built + Next Tracks)

## Purpose

This document describes the current prep implementation in Ploy and the next tracks after Track 1.

Prep is a control-plane stage that discovers and persists repo-specific build/test execution settings before run scheduling is allowed for that repo.

Track 1 from `roadmap/prep/track-1-minimal-e2e.md` is implemented.

## Scope

In scope (implemented):
- repo-level prep lifecycle state machine
- scheduler-driven prep attempts
- non-interactive Codex runner execution
- schema validation of produced prep profile JSON
- persistence of prep attempts and repo prep profile
- run scheduling gate on `PrepReady`
- build-gate claim-time prep-profile mapping for simple mode

Out of scope (not implemented yet):
- execution of complex orchestration steps from prep profile
- automatic feedback-loop promotion of prompt/tactics deltas

## Implemented Architecture

### Data Model

Prep state is persisted on `mig_repos` and prep attempts are persisted in `prep_runs`.

`mig_repos` prep fields:
- `prep_status`
- `prep_attempts`
- `prep_last_error`
- `prep_failure_code`
- `prep_updated_at`
- `prep_profile`
- `prep_artifacts`

`prep_runs` captures per-attempt evidence:
- `repo_id`
- `attempt`
- `status`
- `started_at`
- `finished_at`
- `result_json`
- `logs_ref`

### Scheduler Task

Prep runs in `internal/server/prep/task.go` as `prep-orchestrator`.

Cycle behavior:
1. Claim next `PrepPending` repo (`ClaimNextPrepRepo`).
2. If none, claim eligible retry repo (`ClaimNextPrepRetryRepo`) using `prep_retry_delay` cutoff.
3. Create `prep_runs` row with status `PrepRunning`.
4. Execute runner.
5. Validate output against `docs/schemas/prep_profile.schema.json`.
6. Persist success (`PrepReady`) or failure (`PrepRetryScheduled` / `PrepFailed`).

### Runner

Current runner is `internal/server/prep/runner_codex.go`.

Execution model:
- clone repo into temp workspace
- load prompt from `design/prep-prompt.md` (fallback to builtin prompt if file missing)
- run `codex exec --json-output --non-interactive`
- extract JSON object from output
- return profile/result/log reference to task

Runner errors are normalized to prep failure taxonomy in `internal/server/prep/runner.go`.

### Schema Validation

`internal/server/prep/schema.go` validates runner JSON output against `docs/schemas/prep_profile.schema.json` before profile persistence.

Validation is mandatory for success transition.

## Repo Lifecycle Integration

### Initial State

Repo creation/upsert paths initialize prep state as `PrepPending`.

### Run Scheduling Gate

Queued run_repos are eligible for job materialization only when repo prep status is `PrepReady`.

Eligibility is enforced in store queries (`ListQueuedRunReposByRun`, `ListRunsWithQueuedRepos`) via `mig_repos.prep_status = 'PrepReady'`.

### Visibility

Prep status and evidence are exposed via:
- `GET /v1/repos`
- `GET /v1/repos/{repo_id}/prep`

## Build Gate Integration From Prep Profile

Prep profile is merged at claim time in `internal/server/handlers/nodes_claim.go`.

Mapping for simple mode gate override:
- `pre_gate` uses `targets.build`
- `post_gate` uses `targets.unit`
- `re_gate` uses `targets.unit`

Mapping is injected only when target status is `passed` and command is non-empty.

Command precedence:
1. explicit `build_gate.<phase>.prep` in run spec
2. prep-profile mapped override (claim-time)
3. Build Gate fallback command by detected tool

Runtime hint env mapping from prep profile:
- `runtime.docker.mode=host_socket` -> `DOCKER_HOST=unix:///var/run/docker.sock`
- `runtime.docker.mode=tcp` -> `DOCKER_HOST=<runtime.docker.host>`
- `runtime.docker.api_version` -> `DOCKER_API_VERSION=<value>`

## Operational Configuration

Prep task settings are in server scheduler config:
- `scheduler.prep_interval` (0 disables prep task)
- `scheduler.prep_max_attempts`
- `scheduler.prep_retry_delay`

Task is wired in `cmd/ployd/server.go`.

## Failure Handling

Failure paths:
- runner failure
- schema validation failure
- persistence failure

State outcomes:
- retryable: `PrepRetryScheduled` when attempt < `prep_max_attempts`
- terminal: `PrepFailed` when retries exhausted

Attempt evidence (`prep_runs.result_json`, `logs_ref`) is persisted on failure and success paths where available.

## Recovery Contract (Track 2 As-Built)

Implemented behavior:
- one recovery loop mechanism (`agent -> re_gate`) is used for all gate failures
- loop metadata carries `loop_kind=healing` and router classification context
- router runs after every failed gate, including failed `re_gate`
- router receives gate phase signal (`pre_gate|post_gate|re_gate`)
- router emits `error_kind` in `infra|code|mixed|unknown` with optional `strategy_id`, `confidence`, `reason`, `expectations`
- healing strategy is configured by `error_kind` under `build_gate.healing.by_error_kind`
- server healing insertion selects action by persisted `job_meta.gate.recovery.error_kind`
- server injects `build_gate.healing.selected_error_kind` into claimed heal spec
- `mixed` and `unknown` are terminal classifications; remaining chain is cancelled

Selector contract:
- `build_gate.healing.by_error_kind.infra|code` define healing actions
- `build_gate.healing.by_error_kind.mixed|unknown` are forbidden as action keys
- action fields:
  - `retries`
  - `image`
  - optional `command`
  - optional `env`
  - optional `expectations.artifacts[]`

Infra artifact contract boundary:
- expected artifact path: `/out/prep-profile-candidate.json`
- expected schema id: `prep_profile_v1`
- schema validation hook is available through prep schema boundary; promotion workflow remains a subsequent track

## Known Gaps and Next Tracks

### Next Track: Recovery History Input

Adopted design:
- provide router and healer with loop history on each attempt (for example `/in/recovery-history.json`)
- include:
  - gate phase and attempt number
  - current failure summary/fingerprint
  - router classification/confidence/reason
  - previous healing action summaries
  - re-gate outcomes

### Next Track: Router Prompt Packaging

Adopted design:
- introduce a dedicated `healing/` folder for recovery prompt assets and strategy variants
- keep classification and prompt selection templates versioned together in that folder

### Next Track: Complex Execution

Goal:
- execute `runner_mode=complex` orchestration declaratively from profile

Current status:
- schema contract exists
- runtime executor for orchestration primitives is not implemented

### Next Track: Prompt/Tactics Feedback Loop

Goal:
- collect, review, and safely promote reusable prompt/tactic improvements

Current status:
- profile schema contains `prompt_delta_suggestion`
- no automatic adjudication/rollout pipeline yet

### Related Gate-Healing Continuity Gap (Outside Prep Track 1)

Build-gate healing currently runs as discrete jobs. Session-resume mechanics present in inline healing code paths are not used by the active discrete healing flow.

This is a build-gate/healing workflow concern, not a prep orchestrator concern, but it affects end-to-end Codex continuity expectations.

## References

- `roadmap/prep/track-1-minimal-e2e.md`
- `design/prep.md`
- `design/prep-simple.md`
- `design/prep-complex.md`
- `design/prep-states.md`
- `design/prep-prompt.md`
- `docs/schemas/prep_profile.schema.json`
- `docs/build-gate/README.md`
- `docs/migs-lifecycle.md`
