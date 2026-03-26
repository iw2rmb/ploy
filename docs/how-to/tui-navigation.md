# TUI Navigation

This page describes current `ploy tui` navigation and list behavior.

## Layout

- Root screen shows a single `PLOY` list with 3 items: `Migrations`, `Runs`, `Jobs`.
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
