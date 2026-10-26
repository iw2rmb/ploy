# internal/client

Shared control-plane API client adapters used by UI and command packages.

- `lists.go` owns migration and run list transport for CLI and TUI consumers.
- `jobs.go` owns paginated job-list transport and filters for CLI and TUI consumers.
- `waves.go` owns wave status and cancellation requests and context-aware polling.
  Callers render snapshots and select follow deadlines and cancellation policy.
- `tui/` contains TUI-only aggregate adapters.
