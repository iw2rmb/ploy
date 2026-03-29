[artifacts.go](artifacts.go) MIG artifact command helpers for listing and fetching run artifacts through the CLI.
[batch.go](batch.go) Batch-run command orchestration for submitting multiple MIG specs in one operation.
[batch_test.go](batch_test.go) Tests batch submission flow, validation failures, and request payload shaping.
[commands_test.go](commands_test.go) Coverage for MIG command tree wiring and top-level subcommand behavior.
[events.go](events.go) Event-stream helpers for MIG operations, including formatting and stream consumption.
[logs.go](logs.go) MIG logs command implementation for fetching and printing run logs.
[logs_test.go](logs_test.go) Tests log retrieval filtering, output handling, and error propagation paths.
[mod_management.go](mod_management.go) Management-mode MIG commands for lifecycle operations such as archive and remove.
[mod_management_test.go](mod_management_test.go) Tests management-mode MIG command behavior and mutation guardrails.
[mod_repos.go](mod_repos.go) Repository-scoped MIG command module for listing and selecting migration repositories.
[mod_repos_test.go](mod_repos_test.go) Tests repository module command behavior and repository selector parsing.
[mod_run.go](mod_run.go) Run-mode MIG command module that builds and submits run-related requests.
[mod_run_test.go](mod_run_test.go) Tests run-module command validation, defaults, and request generation.
[pull.go](pull.go) MIG pull command logic for syncing migration definitions to local workspace state.
[pull_test.go](pull_test.go) Tests MIG pull filtering, destination handling, and remote error cases.
[status.go](status.go) MIG status command implementation and status rendering utilities.
[status_test.go](status_test.go) Tests status conversion and CLI presentation across status response variants.
[submit.go](submit.go) Shared submit helpers used by run and batch commands to post jobs to the API.
