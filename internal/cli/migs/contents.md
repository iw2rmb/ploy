[artifacts.go](artifacts.go) MIG artifact command helpers for listing and fetching run artifacts through the CLI.
[batch.go](batch.go) Batch-run command orchestration for submitting multiple MIG specs in one operation.
[batch_test.go](batch_test.go) Tests batch submission flow, validation failures, and request payload shaping.
[commands_test.go](commands_test.go) Coverage for MIG command tree wiring and top-level subcommand behavior.
[events.go](events.go) Event-stream helpers for MIG operations, including formatting and stream consumption.
[logs.go](logs.go) MIG logs command implementation for fetching and printing run logs.
[logs_test.go](logs_test.go) Tests log retrieval filtering, output handling, and error propagation paths.
[mig_management.go](mig_management.go) CLI client commands for MIG management operations (add/list/remove/archive/unarchive).
[mig_management_test.go](mig_management_test.go) Tests MIG management command behavior across create/list/delete and archive flows.
[mig_repos.go](mig_repos.go) CLI client commands for MIG repository set operations including add/list/remove and CSV bulk import.
[mig_repos_test.go](mig_repos_test.go) Tests MIG repo command request shaping, validation, and API error handling.
[mig_run.go](mig_run.go) CLI client command for creating MIG runs with all/explicit/failed repo selection modes.
[mig_run_test.go](mig_run_test.go) Tests MIG run selector validation and run-creation request/response handling.
[pull.go](pull.go) MIG pull command logic for syncing migration definitions to local workspace state.
[pull_test.go](pull_test.go) Tests MIG pull filtering, destination handling, and remote error cases.
[status.go](status.go) MIG status command implementation and status rendering utilities.
[status_test.go](status_test.go) Tests status conversion and CLI presentation across status response variants.
[submit.go](submit.go) Shared submit helpers used by run and batch commands to post jobs to the API.
