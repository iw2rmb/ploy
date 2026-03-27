# JobList Component Integration

Scope: Extract and integrate a reusable JobList component across PLOY root and JOBS screen, with CLI-unified API access.

Documentation: `design/job.md`

- [x] 1.1 Extract JobList domain component (`determined`)
  - Component: `internal/tui` (new `joblist` domain module), existing model/view wiring in `internal/tui/model_core.go`, `internal/tui/model_navigation.go`, `internal/tui/view.go`
  - Implementation:
    1. Introduce a standalone `JobList` model with explicit state/update/view contract for job rows, selected job, and details payload.
    2. Move existing job-row rendering and selection helpers into the `JobList` domain module.
    3. Replace direct jobs state mutations in root model with delegation to `JobList` APIs.
  - Verification:
    1. `go test ./internal/tui -run Job`
    2. Verify existing JOBS list rendering tests still pass after extraction.
  - Reasoning: medium (CFP_delta: 6)

- [x] 1.2 Compose JobList in both required screens (`determined`)
  - Component: `internal/tui/model_types.go`, `internal/tui/model_navigation.go`, `internal/tui/model_core.go`, `internal/tui/view.go`
  - Implementation:
    1. Normalize root-screen naming/contract to `ScreenPloyList` semantics in code and comments.
    2. Render `JobList` in `ScreenJobsList` as the jobs pane owned by the component.
    3. Render `JobList` in `ScreenPloyList` when PLOY is active and selected item is `Job`, without changing active list focus.
  - Verification:
    1. `go test ./internal/tui -run "Root|Jobs|View"`
    2. Add/adjust test cases for root-with-job-selected composition and focus behavior.
  - Reasoning: medium (CFP_delta: 7)

- [x] 1.3 Unify JobList data access with CLI command layer (`determined`)
  - Component: `internal/cli/runs`, `internal/cli/tui`, `internal/tui/commands.go`
  - Implementation:
    1. Reuse existing `internal/cli/runs` run-repo commands for JobList details and extend them with machine-readable helpers where current command surface is print-oriented.
    2. Keep `internal/cli/tui` as thin adapters only, avoiding duplicate endpoint wiring in TUI.
    3. Wire JobList command calls through TUI commands so both screens share the same data-fetch path.
  - Verification:
    1. `go test ./internal/cli/runs ./internal/cli/tui ./internal/tui`
    2. Confirm no new TUI-only HTTP command package is introduced.
  - Reasoning: medium (CFP_delta: 8)

- [x] 1.4 Extend current run-repo payloads for required JobList details (`determined`)
  - Component: `internal/migs/api/run_repo_jobs.go`, `internal/server/handlers/runs_repo_jobs.go`, `internal/domain/types/diffsummary.go`, `internal/server/handlers/diffs.go`, `docs/api/components/schemas/controlplane.yaml`
  - Implementation:
    1. Extend run-repo jobs response with missing structured detail fields required by JobList while keeping existing endpoint path.
    2. Extend diff summary fields required for `Patch total +N -N` and project them via existing repo-diffs endpoint.
    3. Update command decoders and OpenAPI schemas to match extended response shapes.
  - Verification:
    1. `go test ./internal/server/handlers ./internal/migs/api ./internal/domain/types ./internal/cli/runs`
    2. Validate OpenAPI schema references for modified fields.
  - Reasoning: high (CFP_delta: 10)

- [x] 1.5 Finalize docs and behavioral acceptance (`determined`)
  - Component: `docs/how-to/tui-navigation.md`, `design/job.md`, `roadmap/job.md`
  - Implementation:
    1. Update TUI navigation docs to include JobList reuse in both screens and root conditional composition behavior.
    2. Ensure implemented behavior matches the DD acceptance criteria exactly.
    3. Run docs link validation.
  - Verification:
    1. `~/@iw2rmb/amata/scripts/check_docs_links.sh`
    2. Manual terminal smoke check: root PLOY with Job selected shows JobList; jobs screen shows same component behavior.
  - Smoke check record (2026-03-27):
    - `TestPloyListJobsSelectedShowsJobListPanel`: `ScreenPloyList` cursor on Jobs (index 2) — view contains both `PLOY` and `JOBS` panel. PASS.
    - `TestPloyListFocusRemainsOnPloy`: `'k'` key on `ScreenPloyList` moves `m.ploy` cursor (2→1), `m.jobList` index unchanged, screen remains `ScreenPloyList`. PASS.
    - `TestSplitScreensRenderColumns/jobs`: `ScreenJobsList` renders `PLOY | JOBS` split columns — same `JobList` component behavior as root composition. PASS.
    - All three pass: `go test ./internal/tui -run "PloyListJobsSelected|PloyListFocus|SplitScreens"` → ok.
  - Reasoning: low (CFP_delta: 3)
