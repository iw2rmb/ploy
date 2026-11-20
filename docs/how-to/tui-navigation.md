# TUI Navigation

`ploy tui` provides a terminal interface for runs and jobs.

## Layout And Navigation

The root screen (`ScreenPloyList`) shows `Runs`, `Jobs`, and `Users` in the `PLOY` column.
`Runs` is item 0. `Jobs` is item 1. `Users` is item 2.

- Enter on root `Runs` moves focus into the runs list.
- Enter on a run opens its details in `PLOY`: `Run` / `<run-id>` and `Jobs` / `total: <n>`.
- Enter on the run's `Jobs` item opens `PLOY | JOBS` for that run.
- Enter on root `Jobs` moves focus into the global jobs list.
- Enter on a job selects that row. `PLOY` shows its `Run` / `<run-id>` and `Job` / `<job-id>`.

Highlighting an active root item shows its list in the right pane.
The TUI checks access to Users before loading the Runs preview at startup.
Focus stays in `PLOY` until Enter moves into the preview.
Enter preserves the list's rows, cursor, filter, and pending request.
Esc returns focus to `PLOY` and keeps the list visible.
Changing the highlighted root item refreshes its preview while retaining loaded rows.
Revisit a root item to retry a failed load.

Only the focused column shows the `│` selection border. Other columns reserve a blank cell in its place.

`PLOY` uses 30 columns.
`RUNS`, `JOBS`, and `USERS` use the remaining terminal width after `PLOY` and the two-column separator.

## Keys

- `Ctrl+C` closes the TUI from every screen.
- `Esc` returns from a run's jobs to run details, then to the runs list, then to the root.
- `Esc` returns from global jobs or Users to the root.
- `Esc` at the root keeps the TUI open.
- `/` starts filtering in a list.
- While editing a filter, Enter accepts the filter and `Esc` cancels it.
- `q` is ordinary filter input.

After accepting a filter, Enter selects the highlighted result.

## RUNS Rows

Each run represents one repository execution. Each row uses three lines:

- First line: the CLI status icon and run ID.
- Second line: repository namespace and name, followed by the short source commit SHA.
- Third line: spec ID or named spec source.

Unselected rows show a bold green `✓` for success and a bold red `✗` for failure.
Selected rows use the list's selection color.

Named specs use `domain/namespace/repo:spec-name`. Anonymous specs use `spec_id`.
The repository label uses its namespace and repository name, followed by up to eight SHA characters.

The list loads up to 100 runs in server order, newest first.
For `cli-admin` tokens, the Users selection determines the creator filter.
For other tokens, the server filters runs by the authenticated token username, with `$USER` as the fallback.

## JOBS Rows

Each job row uses two lines:

- Primary line: status icon, aligned duration, and the step name shown by `ploy run status`.
- Secondary line: job ID.

The step name uses `display_name` when available, otherwise `job_type`.
The list sorts jobs by `started_at`, newest first. Jobs without a start time appear last.
Created and queued jobs without a start time show `-` as their status icon.
Running jobs show elapsed time from `started_at`.

Opening jobs from run details loads the selected run's current attempt, including start times and display names.
The root jobs list uses the Users selection for `cli-admin` tokens.
Other tokens retain the global jobs list.

Duration fields use spaces for leading zeros. Zero hours and minutes, including their units, occupy blank cells.
Seconds remain visible, including `0s`.

## Users

`Users` is active for tokens with role `cli-admin`.
Other tokens see `cli-admin only` below the inactive item.
A failed access check shows `unavailable`.

`All` is the default selection and the first entry in the Users list.
`All` shows runs and jobs for all users, within each list's existing page limit.
The remaining entries contain unique usernames from the API token registry, sorted alphabetically.
The list includes usernames from expired and revoked tokens.
Usernames have the label `User: <username>` to distinguish them from the `All` option.

Enter on root `Users` moves focus into the Users list.
Enter on a username selects that creator for both Runs and the root Jobs list.
Enter returns focus to `PLOY`, where `Users` shows the selection.
Selecting a different user clears the previous rows, filters, and confirmed job selection, then reloads both lists.
The selected user persists through navigation.

## Refresh

Visible lists refresh once per second while they contain nonterminal runs or jobs.
This includes the `RUNS` list, the `JOBS` list, and both root previews.
Each refresh updates status icons and elapsed durations while preserving the filter and selected identity.
Refresh stops when every row is terminal or the active list is hidden.

## Data Access And Components

The TUI passes its session context to every load command.
Exiting the TUI cancels pending requests, including global Jobs metadata requests.

The TUI uses shared client commands:

- `internal/client.ListTokensCommand` discovers admin access and supplies usernames.
- `internal/client.ListRunsCommand` returns `internal/domain/types.RunSummary` values.
- `internal/client.ListJobsCommand` returns global job rows.
  Job rows include `display_name`, `started_at`, and `finished_at` when available.
- `internal/cli/runs.ListRunJobsCommand` returns run jobs, totals, and job metadata.
- `internal/cli/job.GetStatusCommand` identifies the attempt for historical global jobs.
- `internal/cli/common` owns spec and repository labels shared with `ploy run ls`.

Global job names and timestamps come from each job's run attempt.
The TUI reuses loaded names and timestamps for terminal jobs while their status stays unchanged and a start time is available.
Running jobs show `--h --m --s` when their start time is unavailable.

`JobList` (`internal/tui/joblist`) owns job rows, the cursor, confirmed job identity, and job rendering.
Both the global and run-specific jobs lists use this component.
