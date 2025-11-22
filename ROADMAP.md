# Library Reuse Implementation Roadmap

> When following this template:
> - Align to the template structure
> - Include steps to update relevant docs

Scope: Implement the library reuse plan from `REUSE.md`: unify exponential backoff behavior using `github.com/cenkalti/backoff/v5`, replace bespoke SSE and GitLab MR clients with maintained libraries, and migrate CLI command tree/completions to a framework while preserving current CLI behavior and control-plane contracts.

Documentation: `REUSE.md`, `cmd/ploy/README.md`, `docs/how-to/deploy-a-cluster.md`, `docs/how-to/update-a-cluster.md`, `docs/envs/README.md`, `tests/e2e/README.md`.

Legend: [ ] todo, [x] done.

## Backoff Unification (github.com/cenkalti/backoff/v5)
- [x] Introduce shared backoff helper package using cenkalti/backoff/v5 — Centralize retry policy and prepare for call-site refactors
  - Component: `github.com/iw2rmb/ploy`; `go.mod`; new package `internal/workflow/backoff`
  - Scope: Add dependency `github.com/cenkalti/backoff/v5`; implement helpers (for example, `RunWithBackoff(ctx, logger, metrics, op func() error)` and policy constructors) that capture defaults for initial interval, max interval, jitter, and max elapsed time; wire structured logging and metrics hooks so call sites can emit consistent fields
  - Test: Add `internal/workflow/backoff/backoff_test.go` to cover jitter bounds, cap behavior, and context cancellation; run `go test ./internal/workflow/...` and `make test`; expect all tests passing and coverage ≥90% for `internal/workflow/backoff`

- [x] Refactor rollout backoff utilities to use the shared helper — Remove bespoke rollout retry loops while preserving behavior
  - Component: `cmd/ploy/rollout_backoff.go`, `cmd/ploy/rollout_backoff_test.go`, rollout callers under `cmd/ploy/rollout_*`
  - Scope: Replace `RetryPolicy`, `RetryWithBackoff`, and `PollWithBackoff` implementations with thin adapters around `internal/workflow/backoff`; keep effective defaults equivalent to current behavior; ensure `RolloutMetrics` recording and structured log keys (`poll_backoff_attempt`, `poll_backoff_exhausted`, etc.) remain unchanged
  - Test: Extend `cmd/ploy/rollout_backoff_test.go` to assert attempt counts, backoff intervals, and log fields remain stable; run `go test ./cmd/ploy/...`; expect rollout backoff tests to maintain ≥90% coverage of the new adapters

- [x] Apply shared backoff to nodeagent heartbeat retry logic — Align heartbeat backoff with shared policy while keeping 5xx-only semantics
  - Component: `internal/nodeagent/heartbeat.go`, `internal/nodeagent/heartbeat_timing_test.go`
  - Scope: Replace `HeartbeatManager.backoffDuration`, `maxBackoff`, `applyBackoff`, and `resetBackoff` with a shared backoff policy object; configure policy to start at 5s and cap at existing `maxBackoff`; keep 5xx-only triggering via `serverError`; ensure logs (`heartbeat backoff active`) still include backoff duration
  - Test: Update `heartbeat_timing_test.go` cases (e.g., `TestBackoffOn5xxErrors`, cap and reset tests) to exercise the shared helper through the public methods; run `go test ./internal/nodeagent/...`; expect backoff sequences to match current expectations within a small timing tolerance

- [x] Apply shared backoff to nodeagent claim loop — Use shared policy for polling intervals when no work is available
  - Component: `internal/nodeagent/claimer_loop.go`, `internal/nodeagent/agent_claim_test.go`
  - Scope: Replace `ClaimManager`’s `backoffDuration`, `minBackoff`, `maxBackoff`, `applyBackoff`, `resetBackoff`, and `getBackoffDuration` with a shared backoff policy; ensure the loop uses policy-derived intervals for ticker resets and that backoff resets when work is successfully claimed
  - Test: Adapt `TestClaimLoopBackoff` and related tests to verify interval growth and max-cap via the shared helper (with jitter tolerance); run `go test ./internal/nodeagent/...`; expect backoff to increase and respect the configured max interval within jitter bounds

- [x] Apply shared backoff to nodeagent status uploads — Unify status uploader retry logic with shared backoff
  - Component: `internal/nodeagent/statusuploader.go`, `internal/nodeagent/statusuploader_test.go`
  - Scope: Replace manual `maxRetries` and `backoff` doubling in `UploadStatus` with `internal/workflow/backoff` helpers configured for existing retry counts and base delay; preserve retry conditions for network errors and 5xx responses and keep slog messages unchanged
  - Test: Extend `TestStatusUploader_RetryBackoff` to assert the total retry duration and attempt count using the shared helper; run `go test ./internal/nodeagent/...`; expect behavior to remain equivalent while implementation complexity drops

- [x] Apply shared backoff to nodeagent certificate request retries — Remove bespoke exponential backoff in agent bootstrap
  - Component: `internal/nodeagent/agent.go`, `internal/nodeagent/agent_bootstrap_test.go`
  - Scope: Replace manual exponential backoff loop for certificate requests with a shared backoff wrapper; preserve the current number of attempts and log format (`retrying certificate request`, `backoff` fields); ensure context cancellation is honored and early-exit behavior is unchanged
  - Test: Add or extend tests (for example, `agent_backoff_test.go`) to validate retry count, backoff progression, and cancellation; run `go test ./internal/nodeagent/...`; expect certificate acquisition to still succeed under transient failures

- [x] Apply shared backoff to GitLab MR retries — Use shared policy for HTTP and API errors
  - Component: `internal/nodeagent/gitlab/mr_client.go`, `internal/nodeagent/gitlab/mr_client_api_retry_test.go`
  - Scope: Replace custom `shouldRetry` and `backoff` logic in `CreateMR` with `internal/workflow/backoff` helpers while maintaining retry conditions for 429 and 5xx responses; keep approximate delay pattern (1s, 2s, 4s) and ensure PAT redaction behavior remains intact
  - Test: Update `mr_client_api_retry_test.go` to assert retry count and approximate delays using the shared helper; run `go test ./internal/nodeagent/gitlab/...`; confirm fuzz tests still pass and no PAT values appear in error strings

- [x] Apply shared backoff to SSE stream reconnects — Align SSE reconnect behavior with shared backoff and IdleTimeout logic
  - Component: `internal/cli/stream/client.go`, SSE-related tests under `cmd/ploy` (for example, `mods_logs_test.go`, `runs_follow` tests)
  - Scope: Replace `Client.wait` and manual retry/backoff calculations with shared backoff helpers, preserving `MaxRetries`, `RetryBackoff` defaults, IdleTimeout semantics, and `ErrDone` handler behavior; integrate logging and metrics hooks where available
  - Test: Add or extend tests to cover reconnect attempts, IdleTimeout-triggered cancellation, and `MaxRetries` exhaustion; run `go test ./internal/cli/stream/... ./cmd/ploy/...`; expect streaming behavior to remain stable under transient failures

- [x] Document shared backoff usage — Make the shared helper the canonical retry mechanism
  - Component: `REUSE.md`, `GOLANG.md`, any internal developer docs referencing retry logic
  - Scope: Update `REUSE.md` to reference `internal/workflow/backoff` as the canonical retry package; add guidance in `GOLANG.md` on when and how to use the helper; remove or update references to bespoke backoff implementations in comments and docs
  - Test: Manual docs review; run `rg "backoff" .` to verify that all runtime retry loops either use or explicitly opt out of the shared helper

## SSE Client Replacement (github.com/tmaxmax/go-sse)
- [ ] Add go-sse dependency and adapter layer — Prepare to replace the custom SSE parser
  - Component: `go.mod`, `go.sum`, `internal/cli/stream`
  - Scope: Add `github.com/tmaxmax/go-sse` as a dependency; introduce an adapter (for example, `internal/cli/stream/sse_client.go`) that wraps the library and exposes a `Stream`-style API compatible with existing `Client`, `Event`, and `ErrDone` contracts
  - Test: Add unit tests in `internal/cli/stream` that exercise the adapter using an in-memory SSE source emitting `id`, `event`, `data`, `retry`, and comment lines; run `go test ./internal/cli/stream/...`; expect events to map correctly into existing `Event` fields

- [ ] Replace manual SSE parsing with go-sse — Delegate frame parsing while keeping behavior and flags
  - Component: `internal/cli/stream/client.go`
  - Scope: Remove `readEvent` and manual parsing loops; use go-sse’s event stream primitives to read events and map them into `Event`; ensure Last-Event-ID is propagated via headers and maintained across reconnects; keep IdleTimeout behavior by wrapping the connection context; integrate the shared backoff helper for reconnect delays
  - Test: Update existing SSE-related tests in `cmd/ploy` (for example, `mods_logs_test.go`, `runs_follow` tests) to verify Last-Event-ID replay, IdleTimeout cancellation, handler `ErrDone`, and malformed-frame handling; run `go test ./internal/cli/stream/... ./cmd/ploy/...`; expect unchanged public behavior

- [ ] Update streaming documentation — Reflect library-backed SSE semantics in CLI docs
  - Component: `cmd/ploy/README.md`, `tests/e2e/mods/README.md`
  - Scope: Ensure documentation for `mods logs` and `runs follow` describes IdleTimeout defaults, `--idle-timeout` and `--timeout` flags, reconnection semantics, and Last-Event-ID support; clarify that SSE streams use resilient reconnects backed by a shared backoff policy
  - Test: Manual docs review; run SSE-related e2e tests from `tests/e2e/mods` and confirm they still pass with the new implementation

## GitLab MR Client Swap (gitlab.com/gitlab-org/api/client-go)
- [ ] Add GitLab client-go dependency and configuration helper — Prepare to replace the bespoke HTTP MR client
  - Component: `go.mod`, `go.sum`, `internal/nodeagent/gitlab`
  - Scope: Add `gitlab.com/gitlab-org/api/client-go` as a dependency; implement a small configuration helper that constructs a typed client using domain, base URL (respecting localhost/127.0.0.1 HTTP scheme), and PAT; ensure headers preserve current behavior (Authorization bearer token plus `PRIVATE-TOKEN`) where required
  - Test: Add unit tests to validate that the configured client targets the expected base URL and carries the correct auth token; run `go test ./internal/nodeagent/gitlab/...`; expect no regressions in existing tests

- [ ] Refactor MR creation to use client-go types — Replace manual HTTP calls with typed API while preserving external contracts
  - Component: `internal/nodeagent/gitlab/mr_client.go`, `internal/nodeagent/gitlab/mr_client_api_create_test.go`
  - Scope: Replace manual HTTP request/response handling in `CreateMR` with client-go’s merge request creation API; maintain the external `MRCreateRequest` and `MRCreateResponse` contracts or introduce minimal new DTOs that keep the same call sites; preserve domain parsing, URL construction, and label/description handling
  - Test: Update `mr_client_api_create_test.go` to assert that requests sent via client-go hit the expected endpoint with the correct payload (golden JSON where applicable); run `go test ./internal/nodeagent/gitlab/...`; expect API behavior to match current tests

- [ ] Integrate shared backoff with client-go MR operations — Keep retry semantics while delegating delays to shared helpers
  - Component: `internal/nodeagent/gitlab/mr_client.go`, `internal/workflow/backoff`
  - Scope: Replace manual retry loop, `shouldRetry`, and `backoff` functions with shared backoff helpers; preserve retry conditions for 429 and 5xx statuses and the 3-attempt limit; ensure context cancellation is honored and error messages still pass through `redactError`
  - Test: Update `mr_client_api_retry_test.go` to validate retry count and approximate delay schedule via the shared helper; run `go test ./internal/nodeagent/gitlab/...`; confirm fuzz and validation tests continue to pass

- [ ] Clean up DTOs and redaction paths — Ensure no PAT leakage after the swap
  - Component: `internal/nodeagent/gitlab/mr_client.go`, `internal/nodeagent/gitlab/mr_client_api_redaction_test.go`
  - Scope: Remove unused request/response structs if fully replaced by client-go; ensure all errors flowing out of client-go-backed operations are wrapped with `redactError` and that URL-encoded PAT variants remain redacted
  - Test: Re-run `mr_client_api_redaction_test.go` and add cases for new error shapes if necessary; run `go test ./internal/nodeagent/gitlab/...`; confirm no PAT appears in error strings

- [ ] Document GitLab MR client behavior — Keep operator-facing docs in sync with implementation
  - Component: `cmd/ploy/README.md`, `docs/how-to/update-a-cluster.md`
  - Scope: Update any documentation describing GitLab MR creation and GitLab integration flags to note the client-go usage where relevant; ensure examples for MR-related flags and spec fields remain accurate
  - Test: Manual docs review; run GitLab-related integration/e2e tests (if present) that exercise MR creation flows

## CLI Tree Generation via Cobra/Pflag
- [ ] Introduce cobra-based root command and subcommands — Migrate from manual dispatch while preserving CLI surface
  - Component: `go.mod`, `go.sum`, `cmd/ploy/main.go`, new files under `cmd/ploy` (for example, `root.go`)
  - Scope: Add `github.com/spf13/cobra` and `github.com/spf13/pflag` as dependencies; construct a `rootCmd` with subcommands mirroring existing top-level commands (`mod`, `mods`, `runs`, `upload`, `cluster`, `config`, `manifest`, `knowledge-base`, `server`, `node`, `rollout`, `token`, `help`, `version`); change `main` to execute the cobra root while preserving error reporting and exit codes
  - Test: Update or add CLI tests (for example, `cmd/ploy/cli_test.go`) to assert `ploy --help`, `ploy help <command>`, and `ploy version` outputs match existing goldens (or intentionally updated ones); run `go test ./cmd/ploy/...`

- [ ] Wire existing handlers into cobra commands — Reuse current business logic behind cobra flags and args
  - Component: Command files under `cmd/ploy` (for example, `mod_command.go`, `server_deploy_cmd.go`, `config_command.go`, `node_command.go`)
  - Scope: For each command and subcommand, define a cobra `*cobra.Command` that parses flags with `pflag` and then invokes current handlers (such as `handleMod`, `handleServer`, `handleConfig`, `handleNode`, rollout helpers), preserving flag names, defaults, and error messages; deprecate the manual `execute` switch once coverage is in place
  - Test: Ensure existing command-specific tests (`mod_*_test.go`, `server_*_test.go`, `config_command_*_test.go`, `knowledge_base_command_test.go`, etc.) still pass using the cobra-based entrypoints; run `go test ./cmd/ploy/...`; adjust golden outputs only where cobra formatting requires

- [ ] Generate shell completions and command tree from cobra — Replace manual clitree-based completion maintenance
  - Component: `cmd/ploy/autocomplete/*`, `internal/clitree/tree.go`, new completion-generation helper under `cmd/ploy`
  - Scope: Use cobra’s completion support to generate bash, zsh, and fish completion scripts; add a small internal helper or command to regenerate `cmd/ploy/autocomplete/ploy.{bash,bash.new,zsh,fish}` from the cobra command tree; remove `internal/clitree/tree.go` and update any tests that depend on it
  - Test: Update `cmd/ploy/autocomplete_test.go` to validate that generated completions remain in sync with the cobra tree; run `go test ./cmd/ploy/...`; ensure no remaining references to `internal/clitree.Tree`

- [ ] Keep CLI documentation aligned with cobra-based behavior — Ensure help and examples stay accurate
  - Component: `cmd/ploy/README.md`, `docs/how-to/deploy-a-cluster.md`, `docs/how-to/update-a-cluster.md`
  - Scope: Review CLI usage examples and help snippets, updating them where cobra changes formatting (for example, help headers or subcommand listings); document any new convenience commands (such as `ploy completion <shell>`) added for completion generation
  - Test: Manual docs review; run CLI help snapshot tests under `cmd/ploy` to confirm consistency between docs and actual output

- [ ] Add binary size guardrail for CLI changes — Protect against excessive growth from new dependencies
  - Component: `Makefile`, CI/test harness, `dist/ploy`
  - Scope: Introduce a guardrail that measures `dist/ploy` after `make build` (for example, a simple size check script) and fails the build if the binary exceeds an agreed threshold; wire this into `make test` or a dedicated CI job
  - Test: Manually verify the guardrail by building and checking size locally; ensure regular builds pass and CI enforces the threshold

## Cross-cutting Validation and Risk Mitigation
- [ ] Capture baseline test and coverage metrics — Establish a starting point before refactors
  - Component: Repository-wide tests and coverage tools
  - Scope: Run `make test` and capture coverage summary for key packages (`internal/workflow/...`, `internal/nodeagent/...`, `internal/cli/stream`, `cmd/ploy`); record baseline in `CHECKPOINT.md` or `CHECKPOINT_MODS.md` to ensure overall coverage stays ≥60% and critical workflow/runner packages remain ≥90%
  - Test: None (documentation step); ensure coverage reports are committed or referenced where appropriate

- [ ] Follow RED→GREEN→REFACTOR for each slice — Maintain TDD discipline across backoff, SSE, GitLab, and CLI changes
  - Component: All affected packages and tests
  - Scope: For each roadmap slice (backoff, SSE, GitLab client, CLI), first add or tighten tests to pin current behavior (RED), then implement the minimal change to make tests pass (GREEN), and finally clean up implementations, removing duplication and legacy helpers (REFACTOR)
  - Test: Enforced via CI: `make test` must pass at each stage; manual review of diffs should show tests leading code changes

- [ ] Run targeted integration and e2e smoke tests — Validate end-to-end behavior for critical workflows
  - Component: `tests/e2e` harness, especially Mods workflows, SSE log streaming, and GitLab MR flows
  - Scope: After finishing each major slice (backoff, SSE, GitLab client, CLI), run the relevant e2e suites described in `tests/e2e/README.md` to ensure no regressions in ticket execution, log streaming, MR creation, or cluster updates
  - Test: Execute the documented e2e commands; expect all smoke tests to pass before marking the slice complete
