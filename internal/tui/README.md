# internal/tui

Bubble Tea TUI state machine, rendering, and screen navigation for `ploy tui`.

- `commands.go` — async load commands wiring to client adapter packages.
- `joblist/` — reusable JobList domain component (rows, selection, rendering).
- `model_core.go` — top-level model construction and update dispatch.
- `model_jobs_test.go` — jobs screen behavior and rendering tests.
- `model_lists.go` — shared list constructors and list sizing helpers.
- `model_migration_details_test.go` — migration details screen contract tests.
- `model_migrations_test.go` — migrations list navigation/state tests.
- `model_navigation.go` — Enter/Esc transition logic across screens.
- `model_root_test.go` — root (PLOY list) navigation and composition tests.
- `model_run_details_test.go` — run details screen contract tests.
- `model_runs_test.go` — runs list navigation/state tests.
- `model_test.go` — model initialization and global navigation tests.
- `model_types.go` — model types, screen enum, and message payload structs.
- `model_window_size_test.go` — window sizing propagation tests.
- `view.go` — screen view composition and split-pane rendering.
- `view_test.go` — view-level screen rendering assertions.
