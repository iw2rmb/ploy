# Library Reuse Plan

- **Goal**: Reduce bespoke infra by reusing maintained libraries where they fit existing behaviors.

- **Targets**
  - Exponential backoff: replace `cmd/ploy/rollout_backoff.go` and scattered bespoke loops with `https://github.com/cenkalti/backoff`.
  - GitLab MR client: replace `internal/nodeagent/gitlab/mr_client.go` with `https://gitlab.com/gitlab-org/api/client-go`.
  - SSE client: replace `internal/cli/stream/client.go` with a maintained SSE client `github.com/tmaxmax/go-sse`, keeping IdleTimeout logic.
  - CLI tree drift: generate command tree/completions from a framework (`spf13/cobra` + `pflag`) instead of maintaining `internal/clitree/tree.go` manually.

- **Backoff Unification**
  - **Status**: ✅ Complete. All backoff logic migrated to `internal/workflow/backoff`.
  - **Package**: `internal/workflow/backoff` is the **canonical retry mechanism** using `github.com/cenkalti/backoff/v5`.
  - **Rule**: Do not implement bespoke retry loops. Use the shared helper or document the opt-out rationale.

  - **Usage Guide**:
    1. **One-off retries**: Use `RunWithBackoff(ctx, policy, logger, op)` where `op` returns `error`.
    2. **Polling**: Use `PollWithBackoff(ctx, policy, logger, condition)` where `condition` returns `(bool, error)`.
    3. **Long-running loops**: Use `NewStatefulBackoff(policy)` and call `Apply()` on errors, `Reset()` on success, `GetDuration()` for current interval.
    4. **Non-retryable errors**: Wrap validation errors, 4xx HTTP status with `Permanent(err)` to prevent retries.

  - **Predefined Policies**: Use appropriate policy for your use case:
    - `DefaultPolicy()` — General retry operations (2s–30s, 10 attempts, 5m max elapsed).
    - `RolloutPolicy()` — Rollout operations (2s–30s, 10 attempts, 5m max elapsed).
    - `HeartbeatPolicy()` — Nodeagent heartbeat (5s–5m, unlimited attempts, no time limit).
    - `ClaimLoopPolicy()` — Nodeagent claim polling (250ms–5s, unlimited attempts).
    - `StatusUploaderPolicy()` — Status upload retries (100ms–400ms, 4 attempts).
    - `CertificateBootstrapPolicy()` — Certificate requests (1s–16s, 5 attempts).
    - `GitLabMRPolicy()` — GitLab MR API calls (1s–4s, 4 attempts).
    - `SSEStreamPolicy()` — SSE reconnects (250ms–30s, unlimited by default).

  - **Custom Policies**: Create custom `Policy` struct with `InitialInterval`, `MaxInterval`, `Multiplier`, `MaxElapsedTime`, and `MaxAttempts`.

  - **Features**:
    - Exponential backoff with 50% jitter for robustness.
    - Context cancellation honored (returns early when `ctx` is done).
    - Structured logging via `slog` with stable keys (`attempt`, `backoff_duration`, `status`, `error`).
    - Metrics hooks available for emitting retry telemetry.

  - **Migration**: All call sites in rollout, nodeagent (heartbeat, claimer, status uploader, certificate bootstrap), SSE stream client, and GitLab MR client now use the shared helper.
  - **Tests**: Unit tests cover jitter bounds, max cap, context cancellation, and policy behavior in `internal/workflow/backoff/backoff_test.go` with ≥90% coverage.

- **GitLab Client Swap**
  - Add `go-gitlab`; wire MR creation via typed client with retry/backoff wrapper above.
  - Map auth headers, request/response fields, and redaction behavior; remove duplicate DTOs.
  - Tests: golden JSON for request body, retry logic on 429/5xx using go-gitlab test server, token redaction.

- **SSE Client Replacement**
  - Evaluate library fit for Last-Event-ID, comment lines, reconnection hooks; wrap with local IdleTimeout and handler contract.
  - Replace parser loop; keep RetryBackoff/MaxRetries semantics by delegating to shared backoff helper.
  - Tests: idle-timeout cancellation, Retry field override, handler ErrDone, malformed frames.

- **CLI Tree Generation**
  - Adopt `cobra`/`pflag` for flag parsing; generate completion tree from command definitions; keep binary size minimal by isolating framework usage to CLI layer.
  - Regenerate `cmd/ploy/autocomplete/*` and remove manual `internal/clitree/tree.go`; ensure `cmd/ploy/README.md` stays in sync.
  - Tests: CLI help/usage snapshots, flag parsers, completion generation smoke.

- **Risks / Mitigations**
  - Binary size creep (cobra + go-gitlab): measure `make build` size; gate with threshold test.
  - Behavior drift: keep existing defaults (retry counts, headers, log keys) by codifying them in helpers and tests before refactor.
  - Dependency surface: review licenses and CVE posture before adding.

- **Order / Effort**
  1) Backoff helper (low blast, many users) — ~0.5d.  
  2) SSE client swap — ~0.5d after helper.  
  3) GitLab client swap — ~0.5–1d including tests.  
  4) CLI framework migration — ~1–2d; highest churn, do last.

- **Coverage & Checks**
  - Expand unit tests per module; run `make test` and size check after each step.
  - No control-plane API changes; docs touch `cmd/ploy/README.md` and any completion/how-to notes.
