# TUI Navigation

This page describes current `ploy tui` navigation and list behavior.

## Layout

- Root screen shows a single `PLOY` list with 3 items: `Migrations`, `Runs`, `Jobs`.
- Non-root screens are always split into 2 columns:
  - left: `PLOY`
  - right: context list (`MIGRATIONS`, `MIGRATION <name>`, `RUNS`, `RUN`, or `JOBS`)

## PLOY Item Naming

`PLOY` item titles change from plural to singular as selection context is defined:

- After selecting a migration: `Migration`, `Runs`, `Jobs`
- After selecting a run: `Migration`, `Run`, `Jobs`
- After selecting a job: `Migration`, `Run`, `Job`

## List Widths

- Standard list width: `24`
- `RUNS` width: `30`
- `JOBS` width: `48`

## JOBS Rows

Each job row uses two lines:

- Primary line: status glyph + job name + right-aligned compact duration
- Secondary line: `<job-id>`
