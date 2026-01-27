# Healing Log (router bug summary + healing action summary)

Scope: Implement structured one-line summaries for (1) build-gate failures and (2) healing actions, plus per-iteration `/in` artifacts, using a Codex-based router + healer flow. Update the spec schema to add `build_gate.router` and flatten `build_gate.healing` (remove `build_gate.healing.mod`).

Documentation: `design/healing-log.md`, `design/healing-log.yaml`, `tests/e2e/mods/scenario-orw-fail/mod.yaml`, `cmd/ploy/mod_run_spec.go`

Legend: [ ] todo, [x] done.

## Spec Schema
- [ ] Flatten `build_gate.healing` (remove `mod`) — Makes healing config consistent with step-like specs and matches `design/healing-log.yaml`.
  - Repository: ploy
  - Component: `internal/workflow/contracts`
  - Scope:
    - `internal/workflow/contracts/build_gate_config.go`: replace `HealingSpec.Mod *HealingModSpec` with direct mod-like fields on `HealingSpec` (`image`, `command`, `env`, `retain_container`)
    - `internal/workflow/contracts/mods_spec.go`: update validation paths and required fields (e.g. `build_gate.healing.image`)
    - `internal/workflow/contracts/mods_spec_parse.go`: parse `build_gate.healing.{image,command,env,retain_container}`
    - `internal/workflow/contracts/mods_spec_wire.go`: serialize `build_gate.healing.*` (no nested `mod`)
  - Snippets: `build_gate.healing.image: ghcr.io/iw2rmb/mods-codex:latest`
  - Tests: `go test ./internal/workflow/contracts -run ModsSpec` — parsing + validation accepts the new shape

- [ ] Add `build_gate.router` config — Enables a dedicated coding-agent call to summarize `/in/build-gate.log` before healing.
  - Repository: ploy
  - Component: `internal/workflow/contracts`
  - Scope:
    - `internal/workflow/contracts/build_gate_config.go`: add `Router *RouterSpec` to `BuildGateConfig` (router spec is mod-like: `image`, `command`, `env`, `retain_container`)
    - `internal/workflow/contracts/mods_spec_parse.go`: parse `build_gate.router.*`
    - `internal/workflow/contracts/mods_spec_wire.go`: serialize `build_gate.router.*`
    - `internal/workflow/contracts/mods_spec.go`: validate that when healing is configured, router must also be configured
  - Snippets: `build_gate.router.env.CODEX_PROMPT: "Summarize /in/build-gate.log …"`
  - Tests: `go test ./internal/workflow/contracts -run ModsSpec` — invalid/missing router config is rejected

## CLI Spec Preprocessing (`env_from_file`)
- [ ] Update `env_from_file` resolution for new paths — Keeps e2e specs working with secrets for router + healing.
  - Repository: ploy
  - Component: `cmd/ploy`
  - Scope:
    - `cmd/ploy/mod_run_spec.go`: change resolution targets:
      - old: `build_gate.healing.mod.env_from_file`
      - new: `build_gate.healing.env_from_file`
      - add: `build_gate.router.env_from_file`
    - Ensure `env_from_file` is removed after resolution (spec-cleaning invariant).
  - Snippets: N/A
  - Tests: `go test ./cmd/ploy -run EnvFromFile` — router + healing sections inline file contents and drop `env_from_file`

## Job Metadata (store one-liners)
- [ ] Add `bug_summary` to gate metadata — Persists the router-derived one-liner onto the failing gate job.
  - Repository: ploy
  - Component: `internal/workflow/contracts`
  - Scope:
    - `internal/workflow/contracts/build_gate_metadata.go`: add `BugSummary string \`json:"bug_summary,omitempty"\``
    - `internal/workflow/contracts/build_gate_metadata.go`: validate `bug_summary` (single-line; max 200 chars; optional)
  - Snippets: `{"kind":"gate","gate":{"bug_summary":"…"}}`
  - Tests: `go test ./internal/workflow/contracts -run BuildGateStageMetadata` — invalid summaries rejected

- [ ] Add `action_summary` to mod job metadata — Persists the healer-derived one-liner onto the healing job.
  - Repository: ploy
  - Component: `internal/workflow/contracts`
  - Scope:
    - `internal/workflow/contracts/job_meta.go`: add `ActionSummary string \`json:"action_summary,omitempty"\`` to `JobMeta`
    - `internal/workflow/contracts/job_meta.go`: validate `action_summary` (single-line; max 200 chars; only allowed for `kind="mod"`)
  - Snippets: `{"kind":"mod","action_summary":"…"}`
  - Tests: `go test ./internal/workflow/contracts -run JobMeta` — rejects action_summary on non-mod jobs

## Router Execution (before healing)
- [ ] Execute router agent on gate failure and persist `bug_summary` — Ensures the bug one-liner is stored on the *gate job* before healing runs.
  - Repository: ploy
  - Component: `internal/nodeagent`
  - Scope:
    - Identify the gate-job execution path (pre_gate/post_gate/re_gate) and, on failure when healing is configured, run `build_gate.router`:
      - hydrate a temporary `/in` with `build-gate.log`
      - execute router image with its configured env/prompt
      - parse final JSON line for `bug_summary`
      - attach `bug_summary` to the gate job’s completion payload (`stats.job_meta`), so it lands in `jobs.meta.gate.bug_summary`
    - Reuse existing container execution plumbing used for healing mods (no new runtime).
  - Snippets: router final message: `{"bug_summary":"…"}`
  - Tests: `go test ./internal/nodeagent -run Router` — failing gate triggers router run and job_meta includes bug_summary

## Healing Execution (action summary + `/in` artifacts)
- [ ] Capture `action_summary` from the healing step — Makes the healer responsible for a one-line “what I did”.
  - Repository: ploy
  - Component: `internal/nodeagent`, `docker/mods/mod-codex`
  - Scope:
    - `docker/mods/mod-codex/mod-codex.sh`: enforce a reliable “final message capture” artifact (e.g. always write `/out/codex-last.txt` or a dedicated `/out/summary.json`)
    - `internal/nodeagent`: after healing container completes, parse `{"action_summary":"…"}` and include it in the healing job’s completion payload (`stats.job_meta.action_summary`)
  - Snippets: healer final message: `{"action_summary":"…"}`
  - Tests: `go test ./internal/nodeagent -run HealingSummary` — healing completion persists action_summary

- [ ] Write per-iteration `/in` artifacts and `/in/healing-log.md` — Provides the required iteration-indexed logs and markdown summary.
  - Repository: ploy
  - Component: `internal/nodeagent`
  - Scope:
    - On each healing iteration `N`:
      - write `/in/build-gate-iteration-N.log` (copy of current failing gate log)
      - write `/in/healing-iteration-N.log` (copy of healing agent logs, e.g. from `/out/codex.log`)
      - write/update `/in/healing-log.md` exactly per `design/healing-log.md` format
    - Ensure iteration count matches the build-gate ↔ healing cycle (`retries` loop).
  - Snippets: N/A
  - Tests: `go test ./internal/nodeagent -run HealingLog` — files exist with correct names and markdown formatting

## Server Persistence Path
- [ ] Ensure `jobs_complete` persists the new job_meta fields — Makes sure the server accepts and stores `bug_summary`/`action_summary`.
  - Repository: ploy
  - Component: `internal/server/handlers`, `internal/store`
  - Scope:
    - If needed, extend validation to accept the new fields (should work once `contracts.UnmarshalJobMeta` understands them)
    - Add/adjust handler tests that persist `jobs.meta` from `stats.job_meta`
  - Snippets: N/A
  - Tests: `go test ./internal/server/handlers -run CompleteJob_.*JobMeta` — verifies stored `jobs.meta` includes the new fields

## E2E + Docs
- [ ] Update e2e scenario spec to new schema — Locks in the router + flattened healing shape.
  - Repository: ploy
  - Component: `tests/e2e/mods`
  - Scope:
    - `tests/e2e/mods/scenario-orw-fail/mod.yaml`: replace `build_gate.healing.mod` with flattened `build_gate.healing.*` and add `build_gate.router.*`
  - Snippets: N/A
  - Tests: `tests/e2e/mods/scenario-orw-fail/run.sh` (or equivalent) — gate fail triggers router, healing runs, re-gate passes/fails as expected, and `/in/healing-log.md` is present in the healing container context

- [ ] Update user-facing docs and schema example — Prevents drift and documents the new required contract for Codex output.
  - Repository: ploy
  - Component: `docs/`
  - Scope:
    - `docs/schemas/mod.example.yaml`: add `build_gate.router` and flattened `build_gate.healing`
    - `docs/mods-lifecycle.md`: document bug/action summary contract + `/in/healing-log.md` artifact
  - Snippets: N/A
  - Tests: doc-only — verify examples match `design/healing-log.yaml`
