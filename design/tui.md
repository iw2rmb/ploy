# Ploy CLI TUI (Bubble Tea v2)

## Summary
Implement an interactive TUI entrypoint for `ploy` that covers six required screens:
1) root selector (`PLOY`), 2) migrations list, 3) migration details, 4) runs list, 5) run details, 6) jobs list.

The TUI must use `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, and `charm.land/lipgloss/v2`, with default list components and no style overrides.

## Scope
In scope:
- Add a top-level CLI command `ploy tui`.
- Add a Bubble Tea TUI state machine matching the exact screen/key behavior requested.
- Use 24-character width for every visible list.
- Disable help on all lists.
- Disable search/filter for the `PLOY` root list.
- Ensure migrations and runs are shown newest-to-oldest.
- Provide data for jobs screen (`job`, `mig name`, `run id`, `repo id`) and run-details job totals.

Out of scope:
- Any styling/theme customization beyond default component rendering.
- New remote deployment flows or non-local runtime modes.
- Backward-compatibility paths for alternate/legacy TUI contracts.
- Pagination UX design beyond the required six screens.

## Why This Is Needed
The current CLI is command-based only, which forces users to manually chain multiple commands to inspect migrations/runs/jobs and drill into related summaries. The requested behavior is a compact navigation workflow with deterministic `Enter`/`Esc` transitions and fixed-width side-by-side lists.

## Goals
- Match the six requested screens exactly.
- Keep navigation deterministic and testable as a state machine.
- Use Bubble Tea v2/Bubbles v2/Lip Gloss v2 only.
- Keep all list rendering on default components (no custom style/theme mutations).
- Keep data ordering deterministic (newest first where required).

## Non-goals
- Rebuild existing text commands (`ploy mig ...`, `ploy run ...`) around TUI.
- Add rich keyboard shortcuts outside requested behavior.
- Add extra visual indicators, colors, borders, or custom delegates for style.
- Add migration/data-compatibility logic for older API response shapes.

## Current Baseline (Observed)
- Top-level CLI command wiring is in Cobra root command builder: `cmd/ploy/root.go`.
- Existing top-level commands are `mig`, `run`, `pull`, `cluster`, `config`, `manifest`; no TUI command exists.
- No TUI package currently exists under `internal/`.
- No Bubble Tea/Bubbles/Lip Gloss dependencies currently exist in `go.mod`.
- Existing list commands are text-oriented:
  - migrations: `cmd/ploy/mig_list.go` -> `internal/cli/migs.ListMigsCommand`.
  - runs: `cmd/ploy/run_list.go` -> `internal/cli/migs.ListBatchesCommand`.
- Existing server list ordering already satisfies newest-to-oldest for migrations/runs:
  - `internal/store/queries/migs.sql`: `ORDER BY created_at DESC, id DESC`.
  - `internal/store/queries/runs.sql`: `ORDER BY created_at DESC, id DESC`.
- There is no control-plane endpoint for global jobs listing suitable for the requested Jobs screen (`job + mig name + run id + repo id`); only repo-scoped jobs endpoint exists:
  - `GET /v1/runs/{run_id}/repos/{repo_id}/jobs` in `internal/server/handlers/runs_repo_jobs.go`.
- Run details currently use run summary/report commands (`internal/cli/runs/status.go`, `internal/cli/runs/report_builder.go`) but there is no direct run-scoped jobs total endpoint.
- Required env var documentation is already centralized in `docs/envs/README.md`; this feature should not introduce new env vars.

## Target Contract Or Target Architecture

### Command surface
- Add `ploy tui` as a top-level command.
- Update usage/help output to include `tui` in:
  - `cmd/ploy/main.go` (`printUsage`)
  - command wiring tests/help tests under `cmd/ploy/*test.go`
  - `cmd/ploy/README.md`

### Package boundaries
- `cmd/ploy/tui_command.go` (command entrypoint and control-plane bootstrap reuse).
- `internal/tui/*` (Bubble Tea model, update loop, view composition, list setup, state transitions).
- `internal/cli/tui/*` (small HTTP client commands used by TUI only).
- `internal/server/handlers/*` + `internal/store/queries/*` for missing jobs-list contract.

### UI state machine contract
States:
- `S1 Root`
- `S2 MigrationsList`
- `S3 MigrationDetails`
- `S4 RunsList`
- `S5 RunDetails`
- `S6 JobsList`

Transitions:
- `S1 --Enter(Migrations)--> S2`
- `S2 --Enter(migration item)--> S3`
- `S3 --Esc--> S2`
- `S2 --Esc--> S1`
- `S1 --Enter(Runs)--> S4`
- `S4 --Enter(run item)--> S5`
- `S5 --Esc--> S4`
- `S4 --Esc--> S1`
- `S1 --Enter(Jobs)--> S6`
- `S6 --Esc--> S1`

### Screen contracts
S1 Root:
- Single list title: `PLOY`.
- Items:
  - `Migrations` / `select migration`
  - `Runs` / `select run`
  - `Jobs` / `select job`
- Filtering/search disabled for this list.

S2 Migrations list:
- Two side-by-side lists: `PLOY` and `MIGRATIONS`.
- `MIGRATIONS` items show migration name and migration id.
- Order: newest -> oldest.
- `Esc` returns to `S1`.

S3 Migration details:
- Single list title: selected migration heading (`MIGRATION <name or id>`).
- Items:
  - `repositories` / `total: <total-repos>`
  - `runs` / `total: <total-runs>`
- `Esc` returns to `S2`.

S4 Runs list:
- Two side-by-side lists: `PLOY` and `RUNS`.
- `RUNS` items show:
  - run label/id
  - migration name
  - `DD MM HH:mm` timestamp (from run creation time)
- Order: newest -> oldest.
- `Esc` returns to `S1`.

S5 Run details:
- Single list title: `RUN`.
- Items:
  - `Repositories` / `total: <total-repos>`
  - `Jobs` / `total: <total-jobs>`
- `Esc` returns to `S4`.

S6 Jobs list:
- Two side-by-side lists: `PLOY` and `JOBS`.
- `JOBS` items show:
  - job label/name
  - migration name
  - run id
  - repo id
- `Esc` returns to `S1`.

### List configuration invariants
- Every list width is fixed at `24` characters.
- Every list has help disabled (`SetShowHelp(false)`).
- No custom style/theme overrides for list/title/items.
- Default delegates/components are used.

### Data contracts
- Migrations source: existing mig list API/client (`GET /v1/migs`).
- Runs source: existing runs list API/client (`GET /v1/runs`).
- Jobs source: add `GET /v1/jobs` for TUI.

`GET /v1/jobs` response contract (new):
- Supports `limit`/`offset` pagination and optional `run_id` filter.
- Returns deterministic newest-first order.
- Row fields required by TUI:
  - `job_id`
  - `name`
  - `mig_name`
  - `run_id`
  - `repo_id`
- Includes `total` for filtered set to support detail counts.

Example response:
```json
{
  "jobs": [
    {
      "job_id": "2fM...",
      "name": "mig-1",
      "mig_name": "java17-upgrade",
      "run_id": "2fL...",
      "repo_id": "a1b2c3d4"
    }
  ],
  "total": 1
}
```

## Implementation Notes
- Reuse existing control-plane resolution path used by other commands (`resolveControlPlaneHTTP`, `resolveControlPlaneToken`).
- Keep TUI behavior in `internal/tui` as a pure state machine around typed messages and async commands.
- Fetch list payloads through dedicated `internal/cli/tui` clients; do not embed raw HTTP logic in model code.
- For run details job totals, query `GET /v1/jobs?run_id=<id>&limit=1` and read `total`.
- For migration details repo totals, reuse `ListModReposCommand` and count rows.
- For migration details run totals, count runs from list client paging (or add optional server-side filtering in same slice if implemented).
- Keep `Esc` handling explicit per state rather than implicit component defaults.
- Follow default list rendering paths only; avoid title/item style mutation calls.

## Milestones

### Milestone 1: Data contract completion
Scope:
- Add missing jobs list API/server/store/client contract used by TUI Jobs and Run-details screens.

Expected results:
- `GET /v1/jobs` exists with deterministic order and `total`.
- TUI client can request jobs list globally and by `run_id`.

testable outcome:
- handler/store tests validate ordering, filters, and payload shape.

### Milestone 2: TUI command and model skeleton
Scope:
- Add `ploy tui` command and base TUI model with all six states and transitions.

Expected results:
- Enter/Esc transitions match required state graph.
- Base lists render with fixed width and required titles.

testable outcome:
- model tests for transitions and key behavior pass.

### Milestone 3: Screen data binding
Scope:
- Bind migrations/runs/jobs list data and migration/run detail totals.

Expected results:
- Required text fields are rendered exactly per screen contract.
- sorting requirements are satisfied.

testable outcome:
- model/data tests assert row ordering and formatted detail lines.

### Milestone 4: CLI docs and hygiene
Scope:
- Update CLI docs/help and execute standard validation.

Expected results:
- `ploy --help` and CLI docs mention `tui`.
- No new env vars introduced for this feature.

testable outcome:
- command tests, unit tests, vet/staticcheck, and docs-link checks pass.

## Acceptance Criteria
- `ploy tui` starts a Bubble Tea v2 interface and uses Bubbles v2/Lip Gloss v2 modules.
- S1..S6 navigation and layouts behave exactly as specified.
- `PLOY` list search is disabled on root screen.
- Help is disabled for all lists.
- Every list width is `24`.
- Migrations and runs are shown newest-to-oldest.
- Run details show repository and job totals.
- Jobs screen shows `job`, `mig name`, `run id`, `repo id` per entry.
- No styling modifications are applied beyond default component output.

## Risks
- Without a dedicated jobs listing API, Jobs screen would require expensive N+1 fan-out.
- Fixed width (`24`) can truncate long identifiers; defaults must remain readable without custom styles.
- If list sizes grow large, synchronous loading can block UI; async command usage is required.
- Timestamp formatting (`DD MM HH:mm`) must be deterministic across local timezones.

## References
- `cmd/ploy/root.go`
- `cmd/ploy/main.go`
- `cmd/ploy/mig_list.go`
- `cmd/ploy/run_list.go`
- `internal/cli/migs/mod_management.go`
- `internal/cli/migs/batch.go`
- `internal/cli/runs/status.go`
- `internal/cli/runs/jobs.go`
- `internal/server/handlers/runs.go`
- `internal/server/handlers/runs_repo_jobs.go`
- `internal/store/queries/migs.sql`
- `internal/store/queries/runs.sql`
- `internal/store/queries/jobs.sql`
- `docs/envs/README.md`
- `docs/testing-workflow.md`
- `../ord/internal/tui/model_init.go`
- `../ord/internal/tui/model_update.go`
- `roadmap/tui.md`
