# Gate Profile Implementation (As-Built + Next Tracks)

## Purpose

This document describes the current gate profile implementation in Ploy.

Gate profile is no longer a separate scheduled lifecycle. It is a repo-scoped payload consumed by gate/healing execution and updated through validated infra recovery candidates.

## Scope

Implemented:
- repo-level gate profile payload persistence on `mig_repos`
- gate profile parsing and schema validation in shared contracts
- claim-time gate override mapping from persisted gate profile
- pre-gate auto-bootstrap when repo gate profile is missing (detected stack + resolved command)
- stack-bound gate_profile override propagation into gate phase config
- recovery candidate validation (`gate_profile_v1`) during healing flow
- candidate stack match enforcement before re-gate use
- promotion of validated candidate to repo gate profile on successful `re_gate`

Not implemented:
- runtime executor for complex orchestration primitives from `gate_profile.orchestration`
- automatic prompt/tactics adjudication and rollout

## Data Model

`mig_repos` gate profile fields:
- `gate_profile` (JSONB)
- `gate_profile_artifacts` (JSONB)
- `gate_profile_updated_at`

There is no `prep_status` lifecycle and no `prep_runs` table.

## Parsing and Validation

Gate profile contract and helpers live in `internal/workflow/contracts/gate_profile.go`:
- required: `schema_version`, `repo_id`, `runner_mode`, `stack`, `targets`, `orchestration`
- required stack fields: `stack.language`, `stack.tool`
- required target selector: `targets.active` (`build|unit|all_tests|unsupported`)
- runnable active target contract: selected target must exist with non-empty `command`
- terminal unsupported contract: `targets.build.status=failed` and `targets.build.failure_code=infra_support`
- simple-mode guard: no `orchestration.pre/post` steps

Schema validation for typed recovery candidate artifacts is in `internal/workflow/contracts/gate_profile_schema.go` and uses `docs/schemas/gate_profile.schema.json` (`title: Ploy Build Gate Profile`, with `$comment` guidance for infra-healing fields).

## Claim-Time Build Gate Integration

`internal/server/handlers/nodes_claim.go` merges repo gate profile into claimed specs.

Mapping:
- phase destination remains `pre` for `pre_gate`, `post` for `post_gate|re_gate`
- command/env source is always `targets.<targets.active>`

Injection guard:
- runtime mapping ignores target `status`/`failure_code`
- no runtime auto-fallback across targets
- `targets.active=unsupported` is terminal and injects no runnable override
- explicit `build_gate.<phase>.gate_profile` in run spec always wins

Pre-gate bootstrap guard:
- applies only to `pre_gate`
- requires missing persisted repo `gate_profile`
- skipped when explicit `build_gate.pre.gate_profile` is present
- generated profile is derived from stack detection + resolved gate command/env with `targets.active=all_tests` and command/env in `targets.all_tests`, then used in that same `pre_gate`

Claim-time gate_profile override includes:
- command
- env
- stack (`language`, `tool`, optional `release`)

Gate command resolution enforces stack compatibility (`internal/workflow/step/gate_docker*.go`): gate_profile override is rejected when gate_profile stack does not match gate stack context.

## Runtime Hints Mapping

Gate profile runtime hints mapped to gate env:
- `runtime.docker.mode=host_socket` -> `DOCKER_HOST=unix:///var/run/docker.sock`
- `runtime.docker.mode=tcp` -> `DOCKER_HOST=<runtime.docker.host>`
- `runtime.docker.api_version` -> `DOCKER_API_VERSION=<value>`

## Recovery Candidate Flow

For gate failure with `error_kind=infra` and artifact expectation `schema=gate_profile_v1`:
1. Healing strategy emits `/out/gate-profile-candidate.json`.
2. Healing insertion initializes `re_gate` candidate metadata from prior heal outputs when available.
3. On `heal` success, server refreshes linked `re_gate` candidate metadata from the just-finished heal artifact.
4. Candidate is validated against gate profile schema.
5. Candidate is parsed and stack-matched to failed-gate `detected_stack`
   expectation (`BuildGateStageMetadata.detected_stack`):
   - `language` and `tool` are strict matches
   - `release` is strict when detected release is present
   - empty detected release acts as wildcard
6. On valid candidate, re-gate spec receives candidate-derived gate_profile override.

Validation status is persisted in recovery metadata:
- `missing`
- `unavailable`
- `invalid`
- `valid`

Implementation path:
- validation/attach: `internal/server/handlers/nodes_complete_healing.go`
- heal-complete refresh: `internal/server/handlers/jobs_complete.go`
- claim-time candidate merge for `re_gate`: `internal/server/handlers/nodes_claim.go`

## Promotion on Successful Re-Gate

On successful `re_gate`, server promotes validated candidate into repo gate profile payload:
- writes `mig_repos.gate_profile` = candidate profile
- writes `mig_repos.gate_profile_artifacts` with recovery source metadata
- marks `candidate_promoted=true` in job recovery metadata

On successful `pre_gate`, server can also persist an auto-generated bootstrap profile:
- source metadata: `pre_gate_stack_detect`
- write is conditional on repo `gate_profile` still being NULL (no overwrite)

Implementation path:
- promotion trigger: `internal/server/handlers/jobs_complete.go`
- DB write: `PromoteReGateRecoveryCandidateGateProfile` (`internal/store/queries/mig_repos.sql`)

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

- `design/gate-profile.md`
- `design/gate-profile-simple.md`
- `design/gate-profile-complex.md`
- `design/gate-profile-states.md`
- `design/gate-profile-prompt.md`
- `docs/schemas/gate_profile.schema.json`
- `docs/build-gate/README.md`
- `docs/migs-lifecycle.md`
