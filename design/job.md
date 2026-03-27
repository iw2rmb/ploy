# JobList Component DD

## Summary

Introduce a dedicated `JobList` TUI component and use it in two places:
- `ScreenJobsList` (existing jobs screen).
- `ScreenPloyList` (the PLOY root screen) when the active PLOY item is `Job` while PLOY remains the active list.

The component must render the same JOB list/details contract in both contexts and fetch data through CLI-unified API commands by reusing/extending current command implementations.

## Scope

In scope:
- New reusable `JobList` component for TUI state/update/view.
- Root-screen composition update so selecting `Job` in PLOY shows `JobList` panel without leaving PLOY focus.
- Jobs-screen composition update so `ScreenJobsList` delegates list/details behavior to `JobList`.
- API/client unification: TUI reads job details through existing CLI command paths (`internal/cli/runs` and/or thin adapters in `internal/cli/tui`) rather than bespoke HTTP wiring.
- Extension of current run-repo job payload where required for detail fields.

Out of scope:
- New navigation domains beyond PLOY/JOBS.
- New transport stack outside current HTTP endpoints.
- Visual redesign of non-job screens.

## Why This Is Needed

Current behavior duplicates jobs concerns in root/navigation/model code and has no reusable jobs domain component.

Concrete issues:
- Job list behavior is embedded in `internal/tui/model_core.go`, `internal/tui/model_navigation.go`, and `internal/tui/view.go`.
- Root (`ScreenRoot`) does not support a reusable jobs panel while PLOY remains active.
- TUI uses `/v1/jobs` list only (`internal/cli/tui/jobs.go`), while richer job context already exists in run-repo CLI/API surfaces (`internal/cli/runs/jobs.go`, `internal/server/handlers/runs_repo_jobs.go`).

This creates domain mixing and makes jobs UX changes expensive and inconsistent.

## Goals

- Isolate jobs UI state and rendering into one component with a clear contract.
- Ensure one source of truth for jobs API access that is shared with CLI command logic.
- Keep deterministic behavior between `ScreenJobsList` and `ScreenPloyList` (PLOY active + Job selected).
- Keep implementation modular and testable by domain.

## Non-goals

- Rewriting full TUI architecture to nested programs.
- Replacing existing endpoints with a new API family.
- Adding backward-compat shims beyond what current code requires.

## Current Baseline (Observed)

- Screen model has six states and root is `ScreenRoot` (`internal/tui/model_types.go`).
- Jobs list rows are rendered from `/v1/jobs` payload only (`internal/cli/tui/jobs.go`, `internal/tui/model_jobs.go`).
- Jobs screen currently renders `PLOY | JOBS` only (`internal/tui/view.go`).
- Enter on JOBS item in root navigates away to `ScreenJobsList` (`internal/tui/model_navigation.go`).
- Repo-scoped jobs API already projects structured metadata (`display_name`, `action_summary`, `bug_summary`, `recovery`) (`internal/server/handlers/runs_repo_jobs.go`, `internal/migs/api/run_repo_jobs.go`).
- Repo-scoped CLI command already exists for jobs and diffs (`internal/cli/runs/jobs.go`, `internal/cli/runs/diffs.go`).

## Target Contract / Target Architecture

### Screen contract

- Rename/alias current root screen to `ScreenPloyList` (domain-accurate name).
- `ScreenPloyList` behavior:
  - PLOY stays active.
  - When selected PLOY item is `Job`, render `JobList` in the right pane.
  - Other PLOY items keep current root behavior.
- `ScreenJobsList` behavior:
  - Render `PLOY | JobList`.
  - `JobList` may be active list in this screen.

### Component contract: `JobList`

- `JobList` owns:
  - Job rows state.
  - Selected job identity.
  - Job detail payload/cache.
  - Jobs-specific view rendering.
- Parent model owns:
  - Screen transitions.
  - High-level focus (PLOY vs JobList active).
  - Shared run/mig context wiring.

### API/command contract (CLI-unified)

- TUI `JobList` must consume command-layer abstractions reused by CLI:
  - Base list from existing jobs command.
  - Repo-scoped detail data from existing run-repo command surfaces.
- Required extensions must be added to existing commands/endpoints, not parallel TUI-only HTTP clients.
- Preferred shape:
  - Extend `internal/cli/runs` commands to expose machine-readable detail getters.
  - Keep `internal/cli/tui` as thin adapters where needed.

### Data contract for details

`JobList` detail rendering requires the following normalized fields:
- Job identity/status/image/exit.
- Build-gate fields: `lang`, `version`, `tooling`, `router_kind` (`recovery.error_kind`) and summary.
- Non-build fields: `/out`, `/in`, `/tmp` file/size aggregates.
- Patch totals: `+N/-N`.

Current endpoints are reused and extended where needed:
- `GET /v1/runs/{run_id}/repos/{repo_id}/jobs` remains primary details source and gains missing structured fields.
- `GET /v1/runs/{run_id}/repos/{repo_id}/diffs` remains patch source; summary extended if line deltas are missing.

## Implementation Notes

- Introduce a new domain module for the component (for example `internal/tui/joblist/...`) and remove jobs-specific rendering/state mutations from generic model files.
- Replace ad-hoc jobs handling in `model_core`/`model_navigation` with delegation calls into `JobList`.
- Update root view composition to conditionally mount `JobList` when PLOY item `Job` is selected and PLOY is active.
- Keep API logic in command packages:
  - Reuse `internal/cli/runs` commands.
  - Extend those commands and corresponding API payloads when missing detail fields are required.
- Update OpenAPI/CLI contracts only where endpoint payloads are extended.

## Milestones

### Milestone 1: Component extraction and screen integration

Scope:
- Create standalone `JobList` component.
- Integrate into `ScreenJobsList` and `ScreenPloyList` composition.

Expected Results:
- Jobs UI behavior lives in one component.
- Root screen can show JobList panel while PLOY remains active.

Testable outcome:
- TUI tests prove both screens render `JobList` and selection/focus behavior is deterministic.

### Milestone 2: API/command unification and payload extension

Scope:
- Route all JobList data access through reused/extended CLI command surfaces.
- Extend run-repo job/diff payloads for missing detail fields.

Expected Results:
- No TUI-only duplicate HTTP command stack.
- Job details required by UI are available from unified command APIs.

Testable outcome:
- Command tests and handler tests cover extended fields and backward-safe decoding.

### Milestone 3: Details rendering contract completion

Scope:
- Finalize details formatter for build-gate vs non-build sections.

Expected Results:
- `ScreenJobsList` and root-embedded JobList render same details contract.

Testable outcome:
- Snapshot/behavior tests verify required lines and conditional sections by job domain.

## Acceptance Criteria

- `design/job.md` contract is implemented with a standalone `JobList` component.
- `JobList` is used in both:
  - `ScreenJobsList`.
  - `ScreenPloyList` when PLOY is active and Job item is selected.
- TUI jobs data fetching reuses/extends current CLI command implementation; no parallel bespoke TUI HTTP layer.
- Required details fields are rendered with deterministic build-gate/non-build conditional sections.
- Automated tests cover:
  - component behavior,
  - screen composition,
  - command/handler payload extensions.

## Risks

- Component extraction can regress key handling/focus transitions between PLOY and JobList.
- Extending run-repo payloads can desync CLI, TUI, and OpenAPI if not updated atomically.
- Partial metadata availability can produce empty detail sections; formatter must handle missing fields deterministically.

## References

- Current navigation docs: `docs/how-to/tui-navigation.md`
- TUI model/view/navigation: `internal/tui/model_types.go`, `internal/tui/model_core.go`, `internal/tui/model_navigation.go`, `internal/tui/view.go`
- Current jobs command: `internal/cli/tui/jobs.go`
- Reusable run-repo commands: `internal/cli/runs/jobs.go`, `internal/cli/runs/diffs.go`
- Run-repo jobs API projection: `internal/server/handlers/runs_repo_jobs.go`, `internal/migs/api/run_repo_jobs.go`
