# TUI Navigation

This page describes current `ploy tui` navigation and list behavior.

## Layout

- Root screen (`ScreenPloyList`) shows a single `PLOY` list with 3 items: `Migrations`, `Runs`, `Jobs`.
- List screens are split into 2 columns:
  - left: `PLOY`
  - right: context list (`MIGRATIONS`, `RUNS`, or `JOBS`)
- Selected migration and selected run screens render only `PLOY` (no extra right pane).

## PLOY Item Naming

`PLOY` content changes as selection context is defined:

- After selecting a migration:
  - first item becomes `<mig-name>` / `<mig-id>`
  - second item becomes `Runs` / `total: <n>`
- After selecting a run:
  - first item becomes `<mig-name>` / `<mig-id>`
  - second item becomes `Run` / `<run-id>`
  - third item becomes `Jobs` / `total: <n>`
- After selecting a job: `Migration`, `Run`, `Job`

## List Widths

- `PLOY` width: `30`
- Standard list width: `24`
- `RUNS` width: `30`
- `JOBS` width: `30`

## JOBS Rows

Each job row uses two lines:

- Primary line: status glyph + job name + short duration (kept within row width, no ellipsis)
- Secondary line: `<job-id>`

## JobList Component

`JobList` is a reusable domain component (`internal/tui/joblist`) that owns:

- Job rows state and cursor.
- Confirmed selected job identity (set on Enter).
- Job detail payload cache.
- Jobs-specific view rendering (row format, status glyphs).

`JobList` is used in two screens:

- **`ScreenJobsList`** — renders `PLOY | JobList`. `JobList` may be the active list.
- **`ScreenPloyList`** — when the `Jobs` item (index 2) is selected in PLOY, renders `PLOY | JobList` in the right pane. PLOY remains the active list; focus does not transfer to `JobList`.

When the PLOY selection moves away from `Jobs`, the right pane is hidden and only `PLOY` is rendered.

## Data Access

`JobList` fetches job data through the unified CLI command layer:

- Job rows are populated from `internal/client/tui` job items.
- Job detail payloads are fetched via `internal/cli/runs` run-repo commands (`RepoJobEntry`).
- No TUI-only HTTP client is introduced; `internal/client/tui` acts as a thin adapter.
