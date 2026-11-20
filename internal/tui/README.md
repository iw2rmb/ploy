# internal/tui

Bubble Tea TUI state machine, rendering, and screen navigation for `ploy tui`.

- `commands.go` — async load commands using shared API clients.
- `commands_jobs.go` — global job metadata from run attempts.
- `commands_users.go` — admin capability and token usernames.
- `joblist/` — reusable JobList domain component (rows, selection, rendering).
- `model_core.go` — top-level model construction and update dispatch.
- `model_jobs_test.go` — jobs screen behavior and rendering tests.
- `model_lists.go` — shared list constructors and list sizing helpers.
- `model_navigation.go` — Enter/Esc transition logic across screens.
- `model_refresh.go` — refresh scheduling for visible runs and jobs.
- `model_root_test.go` — root (PLOY list) navigation and composition tests.
- `model_run_details_test.go` — run details screen contract tests.
- `model_runs_test.go` — runs list navigation/state tests.
- `model_test.go` — model initialization and global navigation tests.
- `model_types.go` — model types, screen enum, and message payload structs.
- `model_users.go` — Users selection and creator scope for Runs and Jobs.
- `model_window_size_test.go` — window sizing propagation tests.
- `run_delegate.go` — run row rendering and status icon styling.
- `run_delegate_test.go` — run row rendering tests.
- `view.go` — screen view composition and split-pane rendering.
- `view_test.go` — view-level screen rendering assertions.

See [TUI Navigation](../../docs/how-to/tui-navigation.md) for the user-facing behavior.
