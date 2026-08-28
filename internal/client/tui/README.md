# internal/client/tui

TUI-only adapters for aggregate counters.

Migration, run, and job lists use the shared commands in `internal/client`.

- `mig_totals.go` — migration-level counters (repos/runs).
- `mig_totals_test.go` — tests for migration totals commands.
- `run_totals.go` — run-level aggregate counters for detail panels.
- `run_totals_test.go` — tests for run totals command behavior.
