# Prep Track 1: Minimal E2E (Implemented Baseline)

Status: implemented.

## Objective (Delivered)

Deliver the first end-to-end prep flow for newly registered repos:
- detect repos that require prep
- run one non-interactive prep attempt
- validate and persist gate profile output
- gate run scheduling on `PrepReady`
- keep the existing unified jobs pipeline

## Implemented Scope

### 1. Data Model and Query Surface

Delivered:
- prep lifecycle fields on `mig_repos`
- `prep_runs` table for attempt-level evidence
- query surface for claim, retry-claim, state updates, and profile persistence

Primary files:
- `internal/store/schema.sql`
- `internal/store/queries/mig_repos.sql`
- `internal/store/queries/prep_runs.sql`
- generated sqlc outputs under `internal/store/*sql.go`

### 2. Prep Orchestrator Runtime

Delivered:
- scheduler task that claims prep work and persists transitions
- retry scheduling with configured delay and max attempts
- transactional success/failure persistence boundaries

Primary files:
- `internal/server/prep/task.go`
- `internal/server/prep/runner.go`
- `internal/server/prep/schema.go`

### 3. Codex Non-Interactive Runner

Delivered:
- prep runner backed by `codex exec --json-output --non-interactive`
- prompt loading from `design/prep-prompt.md` with builtin fallback
- profile JSON extraction and failure-code normalization

Primary files:
- `internal/server/prep/runner_codex.go`
- `internal/server/prep/runner_codex_test.go`

### 4. Trigger and Scheduling Gate Integration

Delivered:
- repos are initialized/reset to prep lifecycle states on creation/upsert flows
- run scheduling eligibility requires repo prep readiness (`PrepReady`)

Primary files:
- `internal/server/handlers/migs_repos.go`
- `internal/server/handlers/runs_submit.go`
- `internal/server/handlers/runs_batch_http.go`
- `internal/store/queries/run_repos.sql`
- `internal/store/batchscheduler/*`

### 5. Build Gate Consumption of Prep Profile

Delivered:
- claim-time merge of repo `gate_profile` into run spec gate prep overrides
- phase mapping:
  - `pre_gate <- targets.build`
  - `post_gate <- targets.unit`
  - `re_gate <- targets.unit`
- runtime docker hint mapping to gate env (`DOCKER_HOST`, `DOCKER_API_VERSION`)

Primary files:
- `internal/server/handlers/nodes_claim.go`
- `internal/workflow/contracts/gate_profile.go`
- `internal/nodeagent/execution_orchestrator_gate.go`
- `internal/workflow/step/gate_docker_stack_gate.go`

### 6. API and Documentation Surface

Delivered:
- prep state exposed in repo summary and repo prep endpoint
- OpenAPI path for `GET /v1/repos/{repo_id}/prep`

Primary files:
- `internal/server/handlers/repos.go`
- `docs/api/paths/repos_repo_id_prep.yaml`
- `docs/build-gate/README.md`
- `docs/migs-lifecycle.md`

## Runtime Behavior After Track 1

1. New repo enters `PrepPending`.
2. Prep task claims repo and transitions to `PrepRunning`.
3. Codex runner executes one attempt and returns JSON.
4. Schema validation gates success.
5. Success path persists profile/artifacts and sets `PrepReady`.
6. Failure path persists evidence and sets `PrepRetryScheduled` or `PrepFailed`.
7. Run scheduling queries materialize jobs only for repos in `PrepReady`.

## Verification Artifacts

Representative test suites:
- `go test ./internal/server/prep`
- `go test ./internal/server/handlers`
- `go test ./internal/store/...`
- `go test ./internal/workflow/contracts -run 'TestGateProfile'`

## Next Tracks (Post-Track-1)

Not delivered in this track:
- complex orchestration execution from gate profiles
- prompt/tactics feedback-loop adjudication and rollout
- broader healing/session continuity improvements in discrete build-gate healing jobs

These are tracked in prep design docs as future work.
