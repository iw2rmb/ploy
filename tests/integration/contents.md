[build-gate](build-gate) Shell-driven build-gate integration scenarios that simulate compile failures, healing, and re-gate validation.
[build_test.go](build_test.go) Verifies `make build` produces an executable `dist/ploy` CLI binary.
[happy_path_test.go](happy_path_test.go) Integration flow that creates v1 entities, appends events/logs, and validates persisted happy-path data.
[lab_smoke_test.go](lab_smoke_test.go) Minimal end-to-end store smoke test covering run/job creation plus log and diff ingestion.
[migs](migs) Integration tests for migration executors (ORW, shell, codex) and related CLI/path behaviors.
[server_insecure_test.go](server_insecure_test.go) Validates HTTP server startup and protected endpoint access when insecure auth is enabled for tests.
[smoke_workflow_end_to_end_test.go](smoke_workflow_end_to_end_test.go) Multi-stage workflow smoke test that exercises jobs, logs, diffs, events, and run completion state.
[smoke_workflow_fixture_test.go](smoke_workflow_fixture_test.go) Shared fixture builder for creating v1 spec/mig/repo/run entities in smoke workflow tests.
[smoke_workflow_healing_diffs_test.go](smoke_workflow_healing_diffs_test.go) Verifies healing diff creation and ordered repo-scoped diff retrieval across workflow steps.
