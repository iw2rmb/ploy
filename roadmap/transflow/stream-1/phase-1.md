# Stream 1 / Phase 1 — MVP Transflow: OpenRewrite + Self‑Healing from Build

Objective: Run an OpenRewrite recipe against a repo, build the app, and if the build fails, trigger a self‑healing pass, then re‑build. Ship a runnable path ASAP by composing existing capabilities.

## Reuse First

- Transform: `internal/arf` engine + recipes (storage‑backed registry available).
- Orchestration: `internal/orchestration` Nomad SDK adapter for dispatch/health.
- Build: `internal/build` endpoints and unified storage resolution (via config service).
- Self‑healing: `api/arf` healing manager and internal healing utilities (already tested); wire as a remediation step.
- Config + Storage: `internal/config.Service` + unified `internal/storage`.
- Errors + Routing: `internal/errors` envelope; `internal/routing` tags + KV.

## Scope

- HTTP entrypoint: `/v1/transflow/orw` (controller) accepting:
  - repo URL (or archive), branch, recipe ID, app name, lane (optional), build main (optional)
- Flow:
  1) Resolve recipe → download source → apply ORW recipe
  2) Build via existing build handlers (lane auto‑select if omitted)
  3) If build fails → invoke healing step (use ARF healing primitives) → re‑build once
  4) Persist outputs (diffs, logs, artifact) to unified storage; return JSON with links + status

## Acceptance Criteria

- Provide a minimal end‑to‑end run on VPS: recipe applied and a build artifact produced or a clear failure with error envelope.
- On initial build failure, a single healing attempt is made; second attempt outcome is reflected in response.
- All new code uses config service and unified storage; no raw HTTP orchestration.
- Unit tests cover request validation, happy path, and failing build → healing path.

## TDD Plan

1) RED: Unit tests for controller handler stubs (validate input, call sequence)
2) GREEN: Implement minimal handler composing existing modules; mock ARF/build/orchestration in tests
3) REFACTOR: Extract minimal helper struct for orchestration of steps

## Milestones

- M1 (day 1–2): Handler skeleton + request validation + mocked tests
- M2 (day 3–4): Integration with ARF apply → build; storage of outputs
- M3 (day 5): Healing hook + re‑build + final response schema

