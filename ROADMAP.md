# Code Refactoring: Split Large Files for Maintainability

Scope: Systematically refactor large Go files (500+ LOC) into smaller, focused modules organized by domain responsibility. This improves code discoverability, reduces merge conflicts, and enhances testability by separating concerns into logical boundaries.

Documentation:
- [Go Project Layout Standards](https://github.com/golang-standards/project-layout)
- [Effective Go: Package organization](https://go.dev/doc/effective_go#names)

Legend: [ ] todo, [x] done.

## Node Agent: Execution Domain (705 LOC → 4 files)

- [x] Extract run orchestration into `execution_orchestrator.go` — Separate high-level run lifecycle from detail
  - Component: `internal/nodeagent/execution_orchestrator.go`
  - Change: Move `executeRun()` main flow, workspace setup, defer cleanup logic
  - Test: Run existing `execution_test.go` suite — Verify all run lifecycle tests pass

- [x] Extract healing loop into `execution_healing.go` — Isolate gate-heal-regate complexity
  - Component: `internal/nodeagent/execution_healing.go`
  - Change: Move `executeWithHealing()`, `buildHealingManifest()`, `executionResult` type, `gateRunMetadata`
  - Test: Run `execution_healing_test.go` — Verify healing retry logic and re-gate scenarios

- [x] Extract MR creation into `execution_mr.go` — Separate GitLab MR workflow from core execution
  - Component: `internal/nodeagent/execution_mr.go`
  - Change: Move `createMR()`, `shouldCreateMR()`, `extractProjectIDFromRepoURL()`
  - Test: Mock GitLab API calls in `execution_mr_test.go` — Verify branch push and MR creation

- [x] Extract artifact/status upload into `execution_upload.go` — Centralize upload operations
  - Component: `internal/nodeagent/execution_upload.go`
  - Change: Move diff upload, artifact bundling, `/out` upload, status upload logic (lines 128-367)
  - Test: Mock HTTP clients in `execution_upload_test.go` — Verify upload retry and error handling

## CLI: Mod Run Command (649 LOC → 4 files)

- [x] Extract spec parsing into `mod_run_spec.go` — Separate spec file handling from execution
  - Component: `cmd/ploy/mod_run_spec.go`
  - Change: Move `buildSpecPayload()`, `resolveEnvFromFileInPlace()`, `resolveEnvFromFile()` (lines 224-462)
  - Test: Run `mod_run_spec_test.go` — Verify YAML/JSON parsing and env_from_file resolution

- [x] Extract artifact download into `mod_run_artifact.go` — Isolate artifact fetching logic
  - Component: `cmd/ploy/mod_run_artifact.go`
  - Change: Move `downloadTicketArtifacts()`, `buildArtifactFilename()`, `fetchMRURL()` (lines 476-649)
  - Test: Add `mod_run_artifact_test.go` with mock HTTP — Verify manifest.json generation

- [x] Extract CLI flag handling into `mod_run_flags.go` — Separate flag definitions from execution
  - Component: `cmd/ploy/mod_run_flags.go`
  - Change: Move flag definitions and `printModRunUsage()` into reusable struct
  - Test: Run existing `mod_run_test.go` integration tests — Verify flag precedence

- [x] Refactor core execution into `mod_run_exec.go` — Keep only orchestration flow
  - Component: `cmd/ploy/mod_run_exec.go`
  - Change: Keep `executeModRun()` with calls to extracted functions; move `defaultStageDefinitions()`
  - Test: Run full CLI integration test suite — Verify end-to-end mod run flow
  - Fix (follow-up): Gate artifact download behind success only. In `executeModRun`, download artifacts only when final state is `succeeded`; skip when capped or failed to match README semantics and avoid erroneous fetches after `--cap`.

## Node Agent: Claim Manager (538 LOC → 3 files)

- [x] Extract buildgate claiming into `claimer_buildgate.go` — Separate buildgate job execution
  - Component: `internal/nodeagent/claimer_buildgate.go`
  - Change: Move `claimAndExecuteBuildGateJob()`, `completeBuildGateJob()`, `ackBuildGateJobStart()` (lines 183-326)
  - Test: Run `claimer_test.go` buildgate scenarios — Verify job claim/ack/complete flow
  - Fix (follow-up): On transient ACK failure, still execute and attempt `complete`; server accepts completion without `running` status. Current code returns early and can strand `claimed` jobs.

- [x] Extract spec parsing into `claimer_spec.go` — Isolate spec decoding from claim logic
  - Component: `internal/nodeagent/claimer_spec.go`
  - Change: Move `parseSpec()` and helper `stringValue()` from `claimer.go`; call site updated
  - Test: `claimer_spec_test.go`, `claimer_gitlab_config_test.go` — env merge, build_gate flattening, healing, MR flags
  - Commit: 79392032e0f5f67a0a795c49565d451cd3781be3
  - Review: Verified `claimer.go` uses `parseSpec` and `stringValue`; tests present and targeted

- [x] Refactor claim loop into `claimer_loop.go` — Keep only orchestration and backoff
  - Component: `internal/nodeagent/claimer_loop.go`
  - Change: Move `Start()`, `claimWork()`, `claimAndExecute()`, `ackRun()`, backoff methods to new file
  - Test: Run `claimer_test.go` suite — Verify backoff timing and work claiming priority
  - Note: `claimer.go` now contains only `ClaimManager` struct, `ClaimResponse` type, and `NewClaimManager()`
  - Commit: 7d34ba32e9fe34ac07f788cab40d83f98ce47496
  - Review: Confirmed loop/orchestration/backoff isolated in `claimer_loop.go`. Follow-up remains to execute buildgate job even if ACK fails (see item at line 62).

## Test Files: Server Tests

- [x] Split PKI endpoint tests into `handlers_pki_test.go` — Separate certificate management tests
  - Component: `internal/server/handlers/handlers_pki_test.go`
  - Change: Extract PKI-specific endpoint cases and fixtures from handlers scope
  - Test: Run `go test ./internal/server/handlers -run PKI` — Verify certificate lifecycle tests

- [x] Split run/claim endpoint tests into `server_runs_test.go` — Isolate run orchestration tests
  - Component: `internal/server/handlers/server_runs_test.go`
  - Change: Extract run submission, claim, status tests from handlers scope
  - Test: Run `go test ./internal/server/handlers -run Run` — Verify run state transitions

- [x] Keep server infra tests in `server_test.go` — Retain start/stop, TLS, mux API, and timeouts
  - Component: `internal/server/http/server_test.go`
  - Change: Keep infrastructure-only coverage; no endpoint behavior here
  - Test: Run `go test ./internal/server/http` — Infra tests remain green

## Test Files: GitLab MR Client Tests (772 LOC → 2 files)

- [x] Extract MR API tests into `mr_client_api_test.go` — Separate API interaction tests
  - Component: `internal/nodeagent/gitlab/mr_client_api_test.go`
  - Change: Extract MR creation tests with mock HTTP fixtures
  - Test: Run `go test ./internal/nodeagent/gitlab -run CreateMR` — Verify API contract coverage

- [x] Keep parsing tests in `mr_client_test.go` — Retain URL/project ID parsing tests
  - Component: `internal/nodeagent/gitlab/mr_client_test.go`
  - Change: Keep `ExtractProjectIDFromURL()` tests
  - Test: Run `go test ./internal/nodeagent/gitlab -run Extract` — Verify URL parsing edge cases

## Test Files: Node Agent Tests (696 LOC → 3 files)

- [x] Split execution tests into `agent_execution_test.go` — Focus on run execution scenarios
  - Component: `internal/nodeagent/agent_execution_test.go`
  - Change: Extract run execution, workspace hydration, container runtime tests
  - Test: Run `go test ./internal/nodeagent -run '(RunController|BuildManifestFromRequest|Workspace|EndToEndFlow)'` — Verify run lifecycle, manifest, workspace, and E2E flow

- [x] Split claim tests into `agent_claim_test.go` — Focus on work claiming scenarios
  - Component: `internal/nodeagent/agent_claim_test.go`
  - Change: Extract claim request, ack, backoff, and spec parsing tests
  - Test: Run `go test ./internal/nodeagent -run Claim` — Verify claim priority and retry logic

- [x] Keep integration tests in `agent_test.go` — Retain end-to-end agent lifecycle tests
  - Component: `internal/nodeagent/agent_test.go`
  - Change: Keep agent Start/Stop, heartbeat, and cross-component integration tests
  - Test: Run full agent test suite — Verify no test regressions

## Test Files: Server Events Service Tests (692 LOC → 2 files)

- [x] Split SSE stream tests into `service_stream_test.go` — Focus on streaming behavior
  - Component: `internal/server/events/service_stream_test.go`
  - Change: Extract SSE connection, reconnect, timeout tests
  - Test: Run `go test ./internal/server/events -run Stream` — Verify event delivery and reconnection

- [x] Keep event storage tests in `service_test.go` — Retain event persistence tests
  - Component: `internal/server/events/service_test.go`
  - Change: Keep event storage, filtering, retention tests
  - Test: Run `go test ./internal/server/events -run Storage` — Verify event ordering and cleanup

## Test Files: Execution Healing Tests (684 LOC → 2 files)

 - [x] Split retry tests into `execution_healing_retry_test.go` — Focus on retry boundaries and env/TLS injection
  - Component: `internal/nodeagent/execution_healing_retry_test.go`
  - Change: Extract retry exhaustion, env injection, error propagation tests
  - Test: Run `go test -count=1 ./internal/nodeagent -run 'ExecuteWithHealing_(Retries|Injects|HealingConfiguredNoMods|ModNonZeroExit)'` — Verify retry boundaries

 - [x] Keep gate tests in `execution_healing_test.go` — Retain gate validation logic tests
  - Component: `internal/nodeagent/execution_healing_test.go`
  - Change: Keep pre-gate, re-gate, healing mod execution tests
  - Test: Run `go test -count=1 ./internal/nodeagent -run 'ExecuteWithHealing_(Gate|NoHealing)'` — Verify gate pass/fail scenarios

## Test Files: PKI Handler Tests (677 LOC → 2 files)

- [x] Split admin PKI tests into `handlers_pki_admin_test.go` (already exists - 618 LOC)
  - Component: Already refactored
  - Change: N/A - file already split
  - Test: N/A

- [x] Keep node PKI tests in `handlers_pki_test.go` — Node certificate issuance/renewal
  - Component: `internal/server/handlers/handlers_pki_test.go`
  - Change: Renamed from `server_pki_test.go` to match naming convention
  - Test: Run `go test -count=1 ./internal/server/handlers -run PKI` — Verify node cert lifecycle

- [x] Keep client PKI tests in `handlers_pki_client_test.go` — Client issuance/renewal
  - Component: `internal/server/handlers/handlers_pki_client_test.go`
  - Change: No split; acknowledge existing client coverage
  - Test: Run `go test -count=1 ./internal/server/handlers -run ClientPKI`

## Additional Large Files: Priority Tier 2

- [x] Split `cmd/ploy/mod_run_spec_test.go` (677 LOC) — Extract env file and JSON/YAML tests
  - Component: `cmd/ploy/mod_run_env_file_test.go` (556 LOC already exists), create `mod_run_spec_parsing_test.go`
  - Change: Keep precedence tests in main file, extract format-specific tests
  - Test: Run `go test -count=1 ./cmd/ploy -run BuildSpecPayload` — Verify spec parsing coverage

- [x] Split `internal/store/claims_test.go` (625 LOC) — Separate by claim state transitions
  - Component: Created `claims_state_test.go` (599 LOC), reduced `claims_test.go` to 30 LOC
  - Change: Extracted all state transition tests to `claims_state_test.go`, kept basic infrastructure test in `claims_test.go`
  - Test: Run `go test ./internal/store -run Claims` — Verify claim lifecycle and querying

- [x] Split `internal/workflow/runtime/step/runner_test.go` (595 LOC) — Separate by step phase
  - Component: Created `runner_hydration_test.go` (52 LOC), `runner_gate_test.go` (422 LOC), `runner_exec_test.go` (85 LOC), reduced `runner_test.go` to 142 LOC
  - Change: Extracted hydration phase tests, gate phase tests (enabled/disabled/precedence/failure/timing), and execution timing tests; kept basic infrastructure and nil component tests in main file
  - Test: Run `go test ./internal/workflow/runtime/step -run Runner` — Verify phase isolation

- [x] Split `internal/nodeagent/heartbeat_test.go` (578 LOC) — Separate connection vs timing tests
  - Component: Create `heartbeat_connection_test.go`, `heartbeat_timing_test.go`
  - Change: Extract connection/retry vs interval/backoff tests
  - Test: Run `go test ./internal/nodeagent -run Heartbeat` — Verify heartbeat reliability

- [x] Split `internal/store/ttlworker/partition_dropper_test.go` (561 LOC) — Partition listing vs drop execution
  - Component: Create `partition_dropper_list_test.go`, `partition_dropper_drop_test.go`
  - Change: Separate list-partitions paths from drop/error paths
  - Test: Run `go test ./internal/store/ttlworker -run 'Partition|Worker.*Drop'` — Verify behavior parity

- [x] Split `cmd/ploy/config_command_test.go` (532 LOC) — File I/O vs flag/validation
  - Component: Create `config_command_files_test.go`, `config_command_flags_test.go`
  - Change: Isolate filesystem/env interactions from flag/validation logic
  - Test: Run `go test ./cmd/ploy -run Config` — Verify CLI behavior parity

## Validation Phase

- [x] Run full test suite after all splits — Ensure no regressions introduced
  - Component: All packages
  - Change: N/A
  - Test: `make test` or `go test ./...` — All tests pass with same coverage percentage

- [ ] Verify no circular dependencies introduced — Check import graph remains acyclic
  - Component: All packages
  - Change: N/A
  - Test: `go list -f '{{ .ImportPath }} {{ .Imports }}' ./...` — No import cycles detected

- [ ] Update package documentation — Add package-level godoc for new files
  - Component: All new files
  - Change: Add package comments explaining each file's domain responsibility
  - Test: `go doc <package>` — Readable documentation generated

- [ ] Run linters on refactored code — Ensure code quality standards maintained
  - Component: All modified packages
  - Change: N/A
  - Test: `golangci-lint run` — No new linting issues introduced

- [ ] Build CLI — Ensure binary output exists
  - Component: CLI
  - Change: N/A
  - Test: `make build` — Produces `dist/ploy`

- [ ] Enforce coverage thresholds (per AGENTS.md)
  - Component: All packages; critical workflow runner packages
  - Change: N/A
  - Test: `make test` — ≥60% overall, ≥90% on critical workflow runner packages
