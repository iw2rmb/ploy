# Prep Profile Overview

## Goal

Prep profile defines repo-scoped Build Gate command/env overrides and runtime hints.

Current runtime does not run a standalone prep scheduler. Prep profile is consumed in the gate/healing flow at claim-time and re-gate time.

## Persisted Prep Payload

`mig_repos` stores:
- `prep_profile`
- `prep_artifacts`
- `prep_updated_at`

There is no repo prep lifecycle state machine and no `prep_runs` attempt table.

## Current Runtime Model

Implemented now:
- claim-time mapping injects repo `prep_profile` into `build_gate.<phase>.prep` when target mapping is eligible
- gate failures enter one recovery loop (`gate -> router -> healing -> re_gate`)
- router classification selects healing strategy via `error_kind`
- infra healing can produce typed candidate artifact `/out/prep-profile-candidate.json` (`schema=prep_profile_v1`)
- validated candidate is used for re-gate override
- candidate is promoted to persistent repo `prep_profile` only when re-gate succeeds

Not implemented yet:
- runtime execution of complex lifecycle orchestration primitives from profile
- automated prompt/tactics promotion pipeline

## Recovery Contract (As-Built)

- one loop path for all gate failures
- router runs on every gate failure, including failed `re_gate`
- router emits `error_kind`: `infra|code|mixed|unknown`
- healing action is selected from `build_gate.healing.by_error_kind.<error_kind>`
- server injects `build_gate.healing.selected_error_kind` on heal claims
- `mixed` and `unknown` stop progression (no healing branch)
- infra candidate is schema-validated and stack-matched before use
- promotion to repo default prep profile is gated by successful follow-up `re_gate`

## Integration Points

1. Control plane storage
- `mig_repos.prep_profile`, `mig_repos.prep_artifacts`, `mig_repos.prep_updated_at`

2. Build Gate command/env derivation
- explicit `build_gate.<phase>.prep`
- then mapped repo `prep_profile`
- then default tool-based gate command

3. Healing and promotion
- infra candidate validation + stack compatibility checks
- successful `re_gate` writes promoted candidate into repo `prep_profile`

4. API visibility
- `GET /v1/repos`
- `GET /v1/repos/{repo_id}/runs`

## Related Docs

- `design/prep-impl.md`
- `design/prep-simple.md`
- `design/prep-complex.md`
- `design/prep-states.md`
- `design/prep-prompt.md`
- `docs/schemas/prep_profile.schema.json`
- `docs/build-gate/README.md`
- `docs/migs-lifecycle.md`
