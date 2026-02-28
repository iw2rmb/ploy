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

## Known Gaps and Next Tracks

### Next Track: Universal Recovery Loop Contract

Adopted design:
- keep one recovery loop mechanism (`agent -> re_gate`) for all gate failures
- keep `loop_kind` in metadata as an extension point; current runtime value is `healing`
- do not split runtime flow into separate preparing/healing loops yet
- router `error_kind` selects strategy and artifact expectations

### Next Track: Router-Driven Recovery Policy

Adopted design:
- run router after every gate failure, including every failed `re_gate`
- pass phase signal to router (`pre_gate`, `post_gate`, `re_gate`) so it can use phase priors
- persist router classification and confidence in loop history

Classification contract:
- `infra`
- `code`
- `mixed`
- `unknown`
- `custom` (user-defined kinds via strategy registry)

Current decision for conservative stopping:
- if router returns `mixed` or `unknown`, stop the loop and stop remaining mig progression

### Next Track: Error-Kind Strategy Registry

Adopted design:
- define strategy by `error_kind` in config, not by branching code paths
- strategy fields include:
  - prompt template id
  - allowed tool/capability set
  - expected output artifacts and schemas
  - retry and stop policy
  - whether output can be promoted to repo defaults

`infra` strategy expectation:
- agent emits typed artifact (for example `/out/prep-profile-candidate.json`)
- control plane validates candidate against prep profile schema
- candidate is persisted as repo `prep_profile` only if subsequent re-gate succeeds

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
- introduce a dedicated `router/` folder for router prompt assets and strategy variants
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
