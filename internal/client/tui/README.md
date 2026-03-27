# internal/client/tui

Thin API adapter commands used by `internal/tui` to fetch list data and counters.

- `jobs.go` — lists jobs and maps response rows to TUI-friendly items.
- `jobs_test.go` — tests for jobs listing command behavior and decoding.
- `mig_totals.go` — migration-level counters (repos/runs).
- `mig_totals_test.go` — tests for migration totals commands.
- `migs.go` — lists migrations for the TUI.
- `migs_test.go` — tests for migrations listing command.
- `run_totals.go` — run-level aggregate counters for detail panels.
- `run_totals_test.go` — tests for run totals command behavior.
- `runs.go` — lists runs for the TUI.
- `runs_test.go` — tests for runs listing command behavior.
