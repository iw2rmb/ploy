[cancel.go](cancel.go) CLI command that sends run cancellation requests with optional reason payload.
[commands_test.go](commands_test.go) Integration-style coverage for command delegation to run status API endpoints.
[diffs.go](diffs.go) Repo diff list/download commands and result contracts for run-scoped diff artifacts.
[follow.go](follow.go) SSE follow command that streams job logs and retention events to terminal output.
[follow_frame_text.go](follow_frame_text.go) Follow-frame text renderer that builds aligned multi-repo live sections with metadata.
[follow_frame_text_test.go](follow_frame_text_test.go) Verifies follow-frame layout, line counting, alignment, and dynamic section rendering.
[follow_test.go](follow_test.go) Tests follow streaming behavior including reconnects, event parsing, and completion handling.
[jobs.go](jobs.go) Repo-job listing command with optional attempt filtering and chain-order reconstruction.
[jobs_test.go](jobs_test.go) Tests repo-job command decoding, ordering logic, and request contract behavior.
[printing_test.go](printing_test.go) Tests shared log printer output formats and retention summary rendering behavior.
[render_shared.go](render_shared.go) Shared rendering helpers for statuses, durations, spinners, links, and compact one-liners.
[render_shared_test.go](render_shared_test.go) Unit tests for shared run rendering helpers and status-formatting edge cases.
[report_builder.go](report_builder.go) Aggregates run, repo, job, and artifact APIs into canonical run report payloads.
[report_builder_test.go](report_builder_test.go) End-to-end report assembly tests across run summary, repo, job, and artifact sources.
[report_contract.go](report_contract.go) Canonical run report data contract shared by text and JSON renderers.
[report_json.go](report_json.go) JSON renderer for canonical run reports with writer validation and encoding errors.
[report_json_test.go](report_json_test.go) Validates run report JSON shape, required keys, and serialization behavior.
[report_text.go](report_text.go) Text renderer for follow-style run reports with optional links and live durations.
[report_text_test.go](report_text_test.go) Tests run report text rendering for headers, rows, artifacts, and formatting variants.
[start.go](start.go) CLI command that starts pending repos in a run via start endpoint.
[start_test.go](start_test.go) Tests run-start command success cases, validation, and API error handling.
[status.go](status.go) CLI command that fetches single-run summary status from control-plane API.
