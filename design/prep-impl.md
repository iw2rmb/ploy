# Prep Profile Implementation (As-Built + Next Tracks)

## Purpose

This document describes the current prep profile implementation in Ploy.

Prep profile is no longer a separate scheduled lifecycle. It is a repo-scoped payload consumed by gate/healing execution and updated through validated infra recovery candidates.

## Scope

Implemented:
- repo-level prep payload persistence on `mig_repos`
- prep profile parsing and schema validation in shared contracts
- claim-time gate override mapping from persisted prep profile
- stack-bound prep override propagation into gate phase config
- recovery candidate validation (`prep_profile_v1`) during healing flow
- candidate stack match enforcement before re-gate use
- promotion of validated candidate to repo prep profile on successful `re_gate`

Not implemented:
- runtime executor for complex orchestration primitives from `prep_profile.orchestration`
- automatic prompt/tactics adjudication and rollout

## Data Model

`mig_repos` prep fields:
- `prep_profile` (JSONB)
- `prep_artifacts` (JSONB)
- `prep_updated_at`

There is no `prep_status` lifecycle and no `prep_runs` table.

## Parsing and Validation

Prep profile contract and helpers live in `internal/workflow/contracts/prep_profile.go`:
- required: `schema_version`, `repo_id`, `runner_mode`, `stack`, `targets`, `orchestration`
- required stack fields: `stack.language`, `stack.tool`
- simple-mode guard: no `orchestration.pre/post` steps

Schema validation for typed recovery candidate artifacts is in `internal/workflow/contracts/prep_profile_schema.go` and uses `docs/schemas/prep_profile.schema.json`.

## Claim-Time Build Gate Integration

`internal/server/handlers/nodes_claim.go` merges repo prep profile into claimed specs.

Mapping:
- `pre_gate` <- `targets.build`
- `post_gate` <- `targets.unit`
- `re_gate` <- `targets.unit`

Injection guard:
- target status must be `passed`
- target command must be non-empty
- explicit `build_gate.<phase>.prep` in run spec always wins

Claim-time prep override includes:
- command
- env
- stack (`language`, `tool`, optional `release`)

Gate command resolution enforces stack compatibility (`internal/workflow/step/gate_docker*.go`): prep override is rejected when prep stack does not match gate stack context.

## Runtime Hints Mapping

Prep runtime hints mapped to gate env:
- `runtime.docker.mode=host_socket` -> `DOCKER_HOST=unix:///var/run/docker.sock`
- `runtime.docker.mode=tcp` -> `DOCKER_HOST=<runtime.docker.host>`
- `runtime.docker.api_version` -> `DOCKER_API_VERSION=<value>`

## Recovery Candidate Flow

For gate failure with `error_kind=infra` and artifact expectation `schema=prep_profile_v1`:
1. Healing strategy emits `/out/prep-profile-candidate.json`.
2. Server loads candidate artifact from prior heal job outputs.
3. Candidate is validated against prep profile schema.
4. Candidate is parsed and stack-matched to detected gate stack.
5. On valid candidate, re-gate spec receives candidate-derived prep override.

Validation status is persisted in recovery metadata:
- `missing`
- `unavailable`
- `invalid`
- `valid`

Implementation path:
- validation/attach: `internal/server/handlers/nodes_complete_healing.go`
- claim-time candidate merge for `re_gate`: `internal/server/handlers/nodes_claim.go`

## Promotion on Successful Re-Gate

On successful `re_gate`, server promotes validated candidate into repo prep payload:
- writes `mig_repos.prep_profile` = candidate profile
- writes `mig_repos.prep_artifacts` with recovery source metadata
- marks `candidate_promoted=true` in job recovery metadata

Implementation path:
- promotion trigger: `internal/server/handlers/jobs_complete.go`
- DB write: `PromoteReGateRecoveryCandidatePrepProfile` (`internal/store/queries/mig_repos.sql`)

## Recovery Contract (As-Built)

- single loop path: `gate -> router -> healing -> re_gate`
- router always runs before healing on each failed gate iteration
- router output drives `error_kind` strategy selection
- terminal router outcomes: `mixed`, `unknown`
- non-terminal outcomes: `infra`, `code`
- selected strategy is injected as `build_gate.healing.selected_error_kind`

## Known Gaps and Next Tracks

### Recovery History Input

Target:
- pass structured loop history to router and healing actions each iteration

### Router Prompt Packaging

Target:
- version router prompts/contracts under dedicated `healing/` assets

### Complex Execution

Target:
- execute `runner_mode=complex` orchestration declaratively with deterministic cleanup semantics

### Prompt/Tactics Feedback Loop

Target:
- controlled collection and promotion of reusable prompt deltas

## References

- `design/prep.md`
- `design/prep-simple.md`
- `design/prep-complex.md`
- `design/prep-states.md`
- `design/prep-prompt.md`
- `docs/schemas/prep_profile.schema.json`
- `docs/build-gate/README.md`
- `docs/migs-lifecycle.md`
