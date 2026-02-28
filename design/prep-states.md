# Prep Profile State Model (As-Built)

## Scope

This document describes prep-related state that still exists after prep scheduler removal.

There is no repo prep lifecycle (`PrepPending/PrepReady/...`) and no `prep_runs` attempt state machine.

## Persisted Repo Prep Fields

Stored on `mig_repos`:
- `prep_profile`
- `prep_artifacts`
- `prep_updated_at`

These fields represent the current default prep payload for a repo.

## Recovery Candidate Validation States

During infra healing flows, candidate validation status is recorded in gate recovery metadata:
- `missing` — expected candidate artifact not found
- `unavailable` — candidate artifact exists but cannot be read
- `invalid` — schema parse/validation/stack-match failure
- `valid` — schema-valid and stack-compatible candidate payload

Candidate metadata fields:
- `candidate_schema_id`
- `candidate_artifact_path`
- `candidate_validation_status`
- `candidate_validation_error`
- `candidate_prep_profile` (set only when status is `valid`)
- `candidate_promoted` (set `true` only after successful promotion)

## Candidate Lifecycle Transitions

1. Failed gate classified as `infra` with candidate expectation.
2. Heal job emits candidate artifact.
3. Server validates candidate and records validation status.
4. Valid candidate is merged into re-gate prep override.
5. If re-gate succeeds, candidate is promoted to repo default prep profile and `candidate_promoted=true`.

If re-gate fails or candidate is not valid, promotion does not occur.

## Recovery Loop Context

Shared loop metadata fields:
- `loop_kind` (`healing`)
- `error_kind` (`infra|code|mixed|unknown`)
- optional router details: `strategy_id`, `confidence`, `reason`, `expectations`

Stopping policy:
- `mixed` and `unknown` stop progression
- `infra` and `code` continue through configured healing actions

## Scheduling Dependency

Run scheduling and job materialization are not gated by prep lifecycle status.

Prep profile is consumed opportunistically at job claim/re-gate time when available.

## Visibility

Repo-level visibility is through:
- `GET /v1/repos`
- `GET /v1/repos/{repo_id}/runs`

There is no dedicated prep state endpoint.

## Cross References

- `design/prep.md`
- `design/prep-impl.md`
- `design/prep-simple.md`
- `docs/migs-lifecycle.md`
