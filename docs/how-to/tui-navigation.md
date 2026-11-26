# TUI Navigation

`ploy tui` provides a terminal interface for runs and jobs.

## Layout And Navigation

The root screen (`ScreenPloyList`) shows `Runs`, `Jobs`, and `Users` in the `PLOY` column.
`Runs` is item 0. `Jobs` is item 1. `Users` is item 2.

- Enter on root `Runs` moves focus into the runs list.
- Enter on a run confirms it in `PLOY`, shows its Jobs total, and replaces `RUNS` with `RUN`.
- Enter on the run's `Jobs` item opens `PLOY | JOBS` for that run.
- Enter on root `Jobs` moves focus into the global jobs list.
- Enter on a job shows its three-line row in `PLOY`. For global jobs, the Run entry shows the run ID and an empty third line.

Highlighting an active root item shows its list in the right pane.
The TUI checks access to Users before loading the Runs preview at startup.
Focus stays in `PLOY` until Enter moves into the preview.
Enter preserves the list's rows, cursor, filter, and pending request.
Esc returns focus to `PLOY` and keeps the list visible.
Changing the highlighted root item refreshes its preview while retaining loaded rows.
Revisit a root item to retry a failed load.

The focused column shows a colored title background and the `│` selection border. Each unfocused list uses a transparent title background and reserves a blank cell for the border.

`PLOY` uses 30 columns.
When all three lists appear, `RUNS` uses up to 30 columns and `RUN` uses the remaining width.
In a two-list layout, the second list uses the remaining width. Adjacent lists have a two-column separator.

## Keys

- `Ctrl+C` closes the TUI from every screen.
- Backspace clears the confirmed run when its row is highlighted in `PLOY`. Focus stays on `Runs` in `PLOY`.
- `Esc` returns from a run's jobs to run details, then to the runs list, then to the root.
- `Esc` returns from global jobs or Users to the root.
- `Esc` at the root keeps the TUI open.
- `/` starts filtering in a list.
- While editing a filter, Enter accepts the filter and `Esc` cancels it.
- `q` is ordinary filter input.

After accepting a filter, Enter selects the highlighted result.

## RUNS Rows

Each run represents one repository execution. Each row uses three lines.

1. The status icon and the spec name from `ploy spec ls`, or the spec ID for anonymous specs.
2. The repository as `<namespace>/<repo>`.
3. The creator as `@<username>`. This line is empty when the run has no creator.

Unselected rows show a bold green `✓` for success and a bold red `✗` for failure.
Selected rows use the list's selection color.

The list loads up to 100 runs in server order, newest first.
For `cli-admin` tokens, the Users selection determines the creator filter.
For other tokens, the server filters runs by the authenticated token username, with `$USER` as the fallback.

## RUN Details

The read-only `RUN` list follows the highlighted row in `RUNS`, even when `PLOY` has focus.
An empty or filtered list with no matching run hides `RUN`.
Highlighting the confirmed run in `PLOY` shows its details beside `PLOY`.

| Field | Value |
|---|---|
| ID | Full run ID |
| STATUS | Run status text |
| SPEC | Source domain and repository followed by the spec name or ID. Specs without a source show the name or ID. |
| REPO | Repository namespace, repository name, and short source commit SHA. The terminal link opens the source commit. |
| NODE | Name of the node assigned to the current attempt, or `-` until assignment |

Commit links use the GitHub path on `github.com` and the GitLab path on other remote hosts.

## JOBS Rows

Each job row uses three lines.

1. The status icon and the step name from `ploy run status`.
2. The repository as `<namespace>/<repo>`.
3. The duration.

Job status icons use the same colors as `RUNS`.
The step name uses `display_name` when available, otherwise `job_type`.
The list shows running jobs first. Within each group, jobs with a `started_at` appear newest first, followed by jobs without a start time.
Created and queued jobs without a start time show `-` as their status icon.
Running jobs show elapsed time from `started_at`.

Opening jobs from run details loads the selected run's current attempt, including start times and display names.
The root jobs list reserves space for running jobs within its 100-row limit.
The root jobs list uses the Users selection for `cli-admin` tokens.
Other tokens retain the global jobs list.

The duration starts with the first nonzero unit.
Seconds remain visible, including `0s`.

## Users

`Users` is active for tokens with role `cli-admin`.
Other tokens see `cli-admin only` below the inactive item.
A failed access check shows `unavailable`.

`All` is the default selection and the first entry in the Users list.
`All` shows runs and jobs for all users, within each list's existing page limit.
The remaining entries contain unique usernames from the API token registry, sorted alphabetically.
The list includes usernames from expired and revoked tokens.
Usernames have the label `@<username>` to distinguish them from the `All` option.

Enter on root `Users` moves focus into the Users list.
Enter on a username selects that creator for both Runs and the root Jobs list.
Enter returns focus to `PLOY`, where `Users` shows the selection.
Selecting a different user clears the previous rows, filters, and confirmed job selection, then reloads both lists.
The selected user persists through navigation.

## Refresh

Visible lists refresh once per second while they contain nonterminal runs or jobs.
This includes `RUNS`, `JOBS`, their root previews, and the confirmed run in `RUN`.
Each refresh updates status icons and elapsed durations while preserving the filter and selection.
Refresh stops when every row is terminal or the active list is hidden.

## Data Access And Components

The TUI passes its session context to every load command.
Exiting the TUI cancels pending requests, including global Jobs metadata requests.

The TUI uses shared client commands:

- `internal/client.ListTokensCommand` discovers admin access and supplies usernames.
- `internal/client.ListRunsCommand` returns `internal/domain/types.RunSummary` values.
- `internal/client.ListJobsCommand` returns global job rows.
  Job rows include `display_name`, `started_at`, and `finished_at` when available.
- `internal/cli/runs.ListRunJobsCommand` returns jobs for the current attempt, including the assigned node ID.
- `internal/client.ListNodesCommand` resolves the assigned node ID to the name shown in `RUN`.
- `internal/cli/job.GetStatusCommand` identifies the attempt for historical global jobs.
- `internal/cli/common` formats repository labels.

The TUI uses the run summary for the repository label when job responses omit the repository URL.
Global job names and timestamps come from each job's run attempt.
The TUI reuses loaded names and timestamps for terminal jobs while their status stays unchanged and a start time is available.
Running jobs show `--h --m --s` when their start time is unavailable.

`JobList` (`internal/tui/joblist`) owns job rows, the cursor, confirmed job identity, and job rendering.
Both the global and run-specific jobs lists use this component.
