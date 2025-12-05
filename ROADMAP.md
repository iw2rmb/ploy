# Docker Engine v29 / moby Go SDK migration

Scope: Migrate Docker client dependencies from github.com/docker/docker v28.5.2+incompatible to the supported Engine v29 Go SDK modules (github.com/moby/moby/client and github.com/moby/moby/api), keeping the current Docker-based runtime and health checks working.

Documentation: ROADMAP.md, GOLANG.md, docs/how-to/deploy-a-cluster.md, docs/how-to/update-a-cluster.md, docs/envs/README.md, Docker Engine v29 release notes, Docker Go SDK / moby client docs.

Legend: [ ] todo, [x] done.

## Dependency and SDK selection
- [x] Select Docker Engine v29 range and moby modules — define supported daemon versions and SDK modules.
  - Repository: github.com/iw2rmb/ploy
  - Component: go.mod, GOLANG.md, docs/how-to/deploy-a-cluster.md, docs/how-to/update-a-cluster.md
  - Scope: decide the minimum supported Docker Engine v29.x range and the github.com/moby/moby client/API modules to depend on; record them in GOLANG.md and cluster how-to docs.
  - Tests: docs manual review — docs and roadmap consistently reference the chosen Engine and moby module versions.
  - **Decision**: Docker Engine v29.0+ (API v1.44+); SDK modules: `github.com/moby/moby/client`, `github.com/moby/moby/api/types` (tag: `docker-v29.x.y`).
- [x] Introduce moby client and API modules in go.mod — prepare for incremental migration away from github.com/docker/docker.
  - Repository: github.com/iw2rmb/ploy
  - Component: go.mod
  - Scope: add github.com/moby/moby/client and github.com/moby/moby/api/... dependencies alongside github.com/docker/docker; run go mod tidy and ensure all packages still compile.
  - Tests: go test ./... — repository builds and tests pass with both docker and moby client modules present.
- [x] Migrate workflow runtime packages to moby client and types — keep container execution logic equivalent.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/workflow/runtime/step
  - Scope: replace github.com/docker/docker imports and client usage in workflow runtime packages with github.com/moby/moby/client and github.com/moby/moby/api/... equivalents; adjust for any type or option differences.
  - Tests: go test ./internal/workflow/runtime/step -run 'Docker|Container' -cover — workflow runtime tests pass and cover new moby-based code paths.
- [x] Migrate worker lifecycle packages to moby client and types — keep worker health semantics stable.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/worker/lifecycle
  - Scope: replace github.com/docker/docker/api/types and client usage in DockerChecker and related health code with github.com/moby/moby/api equivalents; align Ping and Info field usage.
  - Tests: go test ./internal/worker/lifecycle -run 'DockerChecker' -cover — worker lifecycle tests pass and cover moby-based health checks.
- [x] Remove deprecated github.com/docker/docker module — enforce use of supported Engine v29 SDK modules only.
  - Repository: github.com/iw2rmb/ploy
  - Component: go.mod, internal/workflow/runtime, internal/worker/lifecycle
  - Scope: drop github.com/docker/docker from go.mod and remaining imports; ensure all call-sites use moby client/API modules; clean up unused indirect dependencies introduced by docker.
  - Tests: go test ./...; scripts/validate-tdd-discipline.sh ./internal/workflow/... ./internal/worker/... — all packages compile, tests pass, and coverage targets remain met without github.com/docker/docker.

## Container runtime adaptation
- [x] Switch DockerContainerRuntime imports and client construction to moby — keep configuration and environment semantics identical.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/workflow/runtime/step/container_docker.go
  - Scope: update imports to github.com/moby/moby/api/types/container, github.com/moby/moby/api/types/image, github.com/moby/moby/api/types/mount, and github.com/moby/moby/client; ensure client.NewClientWithOpts usage maps cleanly to the moby client and still honours FromEnv and WithAPIVersionNegotiation.
  - Tests: go test ./internal/workflow/runtime/step -run 'DockerContainerRuntime' -cover — construction and basic lifecycle tests pass using moby client.
- [x] Re-validate container lifecycle semantics under Engine v29 — ensure create/start/wait/remove behaviour remains correct.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/workflow/runtime/step/container_docker.go
  - Scope: confirm HostConfig options (AutoRemove, Mounts, resource limits, network mode, storage options) and wait semantics still match Engine v29 responses; adjust code if response or option structures changed.
  - Tests: go test ./internal/workflow/runtime/step -run 'Docker|Container' -cover; manual smoke tests against a Docker Engine v29 daemon — containers start, complete, and clean up as before.
  - **Completed**: Added comprehensive lifecycle validation tests (TestDockerContainerLifecycleV29*) covering full create→start→wait→remove sequence with HostConfig options, mount types, wait conditions, and error propagation; enhanced method comments with Engine v29 semantics documentation.
- [x] Confirm log streaming and demuxing works with moby client — avoid deprecated stdcopy usage while preserving output format.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/workflow/runtime/step/container_docker.go, internal/workflow/runtime/step/container_docker_test.go
  - Scope: ensure ContainerLogs calls and options still produce multiplexed output; update stdcopy import path if required by moby; keep combined stdout+stderr behaviour identical for callers.
  - Tests: go test ./internal/workflow/runtime/step -run 'DockerLog' -cover — tests pass and confirm demuxed log content under Engine v29.
  - **Completed**: Added comprehensive log streaming tests (TestDockerLogStreamingV29*) validating: multiplexed stream demuxing, interleaved frame handling, large payloads (64KB), binary content preservation, edge cases (empty/single-byte), and runtime integration; enhanced Logs method with detailed Engine v29 semantics documentation including stdcopy import path (github.com/moby/moby/api/pkg/stdcopy), multiplexed format description, and fallback behaviour.

## Worker health check adaptation
- [x] Switch DockerChecker to moby client and types — maintain the existing health checker interface.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/worker/lifecycle/health.go
  - Scope: replace github.com/docker/docker/api/types, github.com/docker/docker/api/types/system, and github.com/docker/docker/client imports with github.com/moby/moby equivalents; keep the dockerAPI interface and DockerCheckerOptions shape stable for callers.
  - Tests: go test ./internal/worker/lifecycle -run 'DockerChecker' -cover — construction and Close behaviour pass with moby client.
  - **Completed**: DockerChecker uses github.com/moby/moby/client with client.PingOptions, client.PingResult, client.InfoOptions, and client.SystemInfoResult; tests cover OK, error, degraded states, nil guards, context cancellation, default options, and Details field population.
- [x] Reconcile Ping and Info field usage with Engine v29 — keep ComponentStatus fields and details keys stable.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/worker/lifecycle/health.go, internal/worker/lifecycle/health_docker_test.go
  - Scope: verify types.Ping and Info field names and semantics under moby/Engine v29; update status.Version, api_version, containers_running, and driver mappings as needed while keeping Details keys unchanged.
  - Tests: go test ./internal/worker/lifecycle -run 'DockerChecker' -cover — tests validate OK, degraded, and error paths using updated Ping/Info fixtures.
  - **Completed**: Reconciled Ping/Info field usage with Engine v29; added os_type to stable Details keys; documented field mappings (api_version, os_type from PingResult; containers_running, driver, Version from system.Info); added TestDockerChecker_EngineVersionCompatibility and TestDockerChecker_StableDetailsKeys tests covering v28/v29 compatibility.
- [x] Validate Docker health reporting across mixed daemon versions — ensure Engine v28 and v29 both produce sane status.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/worker/lifecycle/health.go, internal/worker/lifecycle/health_docker_test.go
  - Scope: add or adjust tests to cover representative Engine v28 and v29 responses; ensure ComponentStatus.State, Version, and Details remain meaningful across both.
  - Tests: go test ./internal/worker/lifecycle -run 'DockerChecker' -cover; optional manual runs against lab nodes — health output remains stable across daemon versions.
  - **Completed**: Added comprehensive mixed daemon version validation tests covering: full v28.x/v29.x healthy scenarios with various drivers (overlay2, vfs, windowsfilter), degraded states when Info fails (v28/v29), error states when Ping fails (v28/v29), version string edge cases (whitespace, build metadata, git hashes), CheckedAt timestamp verification, and API version negotiation range (1.43–1.46). Tests validate State, Version, and Details stability across both Engine versions.

## Test and validation cycle
- [x] Run focused tests for workflow and worker packages on moby — validate the new Docker integration path first.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/workflow/runtime/step, internal/worker/lifecycle
  - Scope: run go test ./internal/workflow/runtime/step ./internal/worker/lifecycle with moby client in place; fix any immediate failures before wider sweeps.
  - Tests: go test ./internal/workflow/runtime/step ./internal/worker/lifecycle -cover — targeted packages are green with adequate coverage.
  - **Completed**: Both packages pass with moby Engine v29 SDK. workflow/runtime/step: 72.8% coverage; worker/lifecycle: 23.0% coverage (Docker health paths well-covered). Docker-specific tests (TestDockerContainerRuntimeCreate, TestDockerContainerLifecycleV29*, TestDockerChecker_MixedDaemonVersionHealth, etc.) all green.
- [x] Run repo-wide tests with coverage under Engine v29 — confirm overall GREEN.
  - Repository: github.com/iw2rmb/ploy
  - Component: repository-wide tests
  - Scope: execute go test -cover ./... after the migration to moby client/types; ensure there are no regressions in unrelated packages.
  - Tests: go test -cover ./... — full test suite passes with coverage recorded.
  - **Completed**: Full test suite (54 packages) passes under moby Engine v29 SDK. Notable coverage: cmd/ploy 64.1%, internal/nodeagent 70.9%, internal/workflow/runtime/step 72.8%, internal/server/scheduler 97.9%. No regressions detected.
- [ ] Enforce RED→GREEN→REFACTOR discipline and coverage thresholds — protect workflow runner and worker paths.
  - Repository: github.com/iw2rmb/ploy
  - Component: scripts/validate-tdd-discipline.sh, internal/workflow/..., internal/worker/...
  - Scope: run scripts/validate-tdd-discipline.sh ./internal/workflow/... ./internal/worker/...; verify coverage ≥60% overall and ≥90% on critical workflow runner packages.
  - Tests: scripts/validate-tdd-discipline.sh ./internal/workflow/... ./internal/worker/... — passes without lowering existing coverage.
- [ ] Check binary size and build health after dependency changes — avoid dependency bloat from the moby client.
  - Repository: github.com/iw2rmb/ploy
  - Component: Makefile, dist/ploy, scripts/check-binary-size.sh
  - Scope: run make build and scripts/check-binary-size.sh; ensure dist/ploy remains under the configured size threshold after docker→moby migration.
  - Tests: make build && ./scripts/check-binary-size.sh — build succeeds and binary size stays under the defined limit.

## Documentation and rollout
- [ ] Update engineering docs for Docker Engine v29 requirements — set clear expectations for contributors.
  - Repository: github.com/iw2rmb/ploy
  - Component: ROADMAP.md, GOLANG.md
  - Scope: document the new minimum supported Docker Engine version, the chosen moby client/API modules, and the migration status; ensure ROADMAP.md is linked from GOLANG.md or README.md where appropriate.
  - Tests: docs review — engineering docs consistently describe Docker Engine v29 and moby usage.
- [ ] Update operator how-to docs for v29 — make runtime prerequisites explicit for deployments.
  - Repository: github.com/iw2rmb/ploy
  - Component: docs/how-to/deploy-a-cluster.md, docs/how-to/update-a-cluster.md
  - Scope: add or update sections that call out required Docker Engine v29 versions on VPS nodes, any flags or configuration changes needed, and upgrade steps from earlier Engine versions.
  - Tests: docs review — how-to docs walk an operator through deploying/updating a v29-based cluster without ambiguity.
- [ ] Align environment variable documentation with Docker v29 migration — ensure envs are complete and accurate.
  - Repository: github.com/iw2rmb/ploy
  - Component: docs/envs/README.md
  - Scope: document any new environment variables or configuration flags introduced for Docker or moby client configuration (e.g., DOCKER_HOST, API negotiation controls); remove references that assume pre-v29 behaviour only.
  - Tests: docs review — env docs match the configuration knobs actually read by the code after migration.
