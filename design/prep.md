# Prep Stage Overview

## Goal

Prep establishes repo-specific build gate execution settings before the repo enters normal run scheduling.

Prep is mandatory for newly registered repos in current implementation: run scheduling is gated on `PrepReady`.

## What Prep Produces

Prep persists a repo-scoped `prep_profile` (schema v1) plus attempt evidence.

Profile carries:
- runner mode (`simple` or `complex`)
- per-target outcomes for `build`, `unit`, `all_tests`
- optional runtime hints
- orchestration declarations

Attempt evidence is stored per try in `prep_runs`.

## Current Runtime Model

Implemented now:
- scheduler claims repos in prep states and runs non-interactive Codex prep
- output is schema-validated before success persistence
- repo transitions to `PrepReady` only after valid profile persistence
- claim-time mapping injects simple prep target overrides into Build Gate phase prep config

Not implemented yet:
- complex orchestration execution engine
- feedback-loop rollout for prompt/tactics evolution

## Adopted Next-Step Recovery Design

The next gate-recovery design is fixed as:
- keep one recovery loop path (`agent -> re_gate`) for all gate failures
- keep `loop_kind` in metadata for future extensibility; current value is `healing`
- drive strategy by router `error_kind` (`infra|code|mixed|unknown|custom`)
- run router on every gate failure, including failed `re_gate`
- include gate phase as router input signal
- stop mig progression when router classification is `mixed` or `unknown`
- preserve per-attempt router/healer history for subsequent loop iterations
- introduce a dedicated `router/` folder for router prompt assets and classification strategies
- for `infra`, expect a typed profile candidate artifact (for example `prep-profile-candidate.json`) and persist as repo `prep_profile` only after validation + successful re-gate

## Integration Points

1. Control plane state and storage
- `mig_repos` prep fields
- `prep_runs` attempt records

2. Scheduling gate
- queued run repos are eligible only for repos in `PrepReady`

3. Build gate command/env derivation
- uses explicit `build_gate.<phase>.prep` first
- then mapped repo `prep_profile`
- then default tool-based gate command

4. API visibility
- `GET /v1/repos`
- `GET /v1/repos/{repo_id}/prep`

## Related Docs

- `design/prep-impl.md`
- `design/prep-simple.md`
- `design/prep-complex.md`
- `design/prep-states.md`
- `design/prep-prompt.md`
- `roadmap/prep/track-1-minimal-e2e.md`
- `docs/schemas/prep_profile.schema.json`
