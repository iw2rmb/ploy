# Library Reuse Plan

- **Goal**: Reduce bespoke infra by reusing maintained libraries where they fit existing behaviors.

- **Targets**
  - Exponential backoff: replace `cmd/ploy/rollout_backoff.go` and scattered bespoke loops with `https://github.com/cenkalti/backoff`.
  - GitLab MR client: replace `internal/nodeagent/gitlab/mr_client.go` with `https://gitlab.com/gitlab-org/api/client-go`.
  - SSE client: replace `internal/cli/stream/client.go` with a maintained SSE client `github.com/tmaxmax/go-sse`, keeping IdleTimeout logic.
  - CLI tree drift: generate command tree/completions from a framework (`spf13/cobra` + `pflag`) instead of maintaining `internal/clitree/tree.go` manually.

- **Backoff Unification**
  - Add dependency; centralize policy defaults and logging in `internal/workflow/backoff` (or similar).
  - Refactor call sites in rollout, nodeagent heartbeat/claimer/statusuploader, stream client, GitLab MR retry code to use a shared helper that accepts context, logger, metrics hooks.
  - Add unit tests for jitter, max cap, and context cancellation; update existing backoff tests to cover shared helper.

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
