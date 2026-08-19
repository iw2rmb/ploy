# internal/client/tui

TUI-only adapters for job data and aggregate counters.

Migration and run lists use the shared commands in `internal/client`.

- `jobs.go` — lists jobs and maps response rows to TUI-friendly items.
- `jobs_test.go` — tests for jobs listing command behavior and decoding.
- `mig_totals.go` — migration-level counters (repos/runs).
- `mig_totals_test.go` — tests for migration totals commands.
- `run_totals.go` — run-level aggregate counters for detail panels.
- `run_totals_test.go` — tests for run totals command behavior.
