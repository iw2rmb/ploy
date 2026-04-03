[build-gate/](build-gate) Shell integration scenarios that exercise build-gate failure artifacts and post-mig heal/re-gate behavior.
[build_test.go](build_test.go) Integration test ensuring `make build` creates an executable `dist/ploy` binary.
[coverage_guard_test.go](coverage_guard_test.go) Coverage-mode guard helper that skips DB-backed integration tests on shared DSNs.
[happy_path_test.go](happy_path_test.go) End-to-end store integration flow for creating v1 entities, runs, logs, events, and persisted state.
[hydra_contract_test.go](hydra_contract_test.go) Hydra contract integration tests for spec parsing precedence, mount validation, and legacy field rejection.
[lab_smoke_test.go](lab_smoke_test.go) Minimal workflow smoke test that persists run/job lifecycle data, logs, and diffs through the store.
[migs/](migs) Integration tests for migration executors and helpers across ORW CLI, shell runner, and codex paths.
[server_insecure_test.go](server_insecure_test.go) Integration test for HTTP server startup and protected route access in insecure-auth test mode.
[smoke_workflow_end_to_end_test.go](smoke_workflow_end_to_end_test.go) Multi-stage workflow smoke test covering job orchestration, logs, diffs, events, and completion status.
[smoke_workflow_fixture_test.go](smoke_workflow_fixture_test.go) Shared fixture builder that creates v1 spec/mig/repo/run entities for smoke workflow tests.
[smoke_workflow_healing_diffs_test.go](smoke_workflow_healing_diffs_test.go) Integration test validating ordered healing diff persistence and repo-scoped diff listing.
