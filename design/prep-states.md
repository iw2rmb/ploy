# Prep State Machine (As-Built)

## Repo-Level States

Implemented repo prep states:
- `PrepPending`
- `PrepRunning`
- `PrepRetryScheduled`
- `PrepReady`
- `PrepFailed`

## Transition Rules

### Claim transitions
- `PrepPending -> PrepRunning` when claimed by prep task.
- `PrepRetryScheduled -> PrepRunning` when retry delay cutoff is reached and repo is claimed.

Both claim transitions:
- increment `prep_attempts`
- clear `prep_last_error`
- clear `prep_failure_code`
- set `prep_updated_at=now()`

### Success transition
- `PrepRunning -> PrepReady` when:
  - runner returns profile JSON
  - profile validates against prep schema
  - prep run is finalized and profile/artifacts are persisted

### Failure transitions
- `PrepRunning -> PrepRetryScheduled` when attempt failed and attempts remain.
- `PrepRunning -> PrepFailed` when attempt failed and max attempts are exhausted.

Failure transitions persist:
- `prep_last_error`
- `prep_failure_code`
- `prep_updated_at`

## Attempt Records (`prep_runs`)

Each claimed attempt creates a `prep_runs` row with `status=PrepRunning`.

Attempt completion updates the same attempt row to terminal status:
- success path writes `PrepReady`
- failure path writes `PrepFailed`

`prep_runs.status` is attempt-local evidence, while `mig_repos.prep_status` is the repo lifecycle state.

## Retry Policy (As-Built)

Configured by scheduler settings:
- `prep_max_attempts`
- `prep_retry_delay`

Retry selection rule:
- only repos in `PrepRetryScheduled` with `prep_updated_at <= now()-prep_retry_delay` are eligible.

## Scheduling Gate Dependency

Run scheduling queries require `mig_repos.prep_status='PrepReady'`.

Repos not in `PrepReady` remain queued at run-repo level and do not materialize job chains.

## API Visibility

State and evidence are exposed via:
- `GET /v1/repos` (status summary)
- `GET /v1/repos/{repo_id}/prep` (full prep status, profile, artifacts, attempt history)

## Planned Gate-Recovery State Context

In the next recovery track, gate loops will carry explicit context:
- `loop_kind`: current value `healing` (reserved as extension point for future loop families)
- `error_kind`: `infra|code|mixed|unknown|custom` (router output per failed gate)
- `history`: per-iteration router + healer summaries

Routing and stopping policy:
- any gate fail enters the same loop mechanism (`agent -> re_gate`)
- `error_kind` selects strategy contract (prompt/tools/expected outputs)
- `re_gate` fail continues using stored loop context
- `mixed` or `unknown` classification stops further mig progression for the repo attempt

## Cross References

- `design/prep-impl.md`
- `design/prep.md`
- `roadmap/prep/track-1-minimal-e2e.md`
