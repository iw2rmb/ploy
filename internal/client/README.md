# internal/client

Shared control-plane API client adapters used by UI and command packages.

- `lists.go` owns migration and run list transport for CLI and TUI consumers.
- `jobs.go` owns paginated job-list transport and filters for CLI and TUI consumers.
- `tui/` contains TUI-only aggregate adapters.
