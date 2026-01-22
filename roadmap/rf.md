# Roadmap — Refactors + Fixes (internal/)

This document captures planned refactors and bug-fix hardening for `internal/`.
It is a planning artifact only (no implementation implied by its presence).

## 1) Unblock `staticcheck` (S1009)

- Current (HEAD): `staticcheck ./internal/...` fails with S1009 in:
  - `internal/workflow/contracts/stack_gate_spec_parse.go:16`
  - `internal/workflow/stackdetect/gradle.go:130`
  - `internal/workflow/stackdetect/gradle_test.go:60`
- Proposed: remove redundant nil checks (`len(nil)` is defined as 0 for maps/slices).
- Where:
  - `internal/workflow/contracts/parseStackGateSpec` (in `stack_gate_spec_parse.go`)
  - `internal/workflow/stackdetect/extractCompatibilityVersion` (in `gradle.go`)
  - `internal/workflow/stackdetect` tests (in `gradle_test.go`)
- Compatibility impact: none (no behavior change intended; mechanical simplification).
- Unchanged: parsing and detection semantics.

Implementation steps:

1. Update `internal/workflow/contracts/stack_gate_spec_parse.go`:
   - Replace `if raw == nil || len(raw) == 0` with `if len(raw) == 0`.
2. Update `internal/workflow/stackdetect/gradle.go`:
   - Replace `if matches == nil || len(matches) < 2` with `if len(matches) < 2`.
3. Update `internal/workflow/stackdetect/gradle_test.go`:
   - Replace `if matches != nil && len(matches) > 1` with `if len(matches) > 1`.
4. Run:
   - `go test ./internal/...`
   - `staticcheck ./internal/...`
   - `./scripts/validate-tdd-discipline.sh ./internal/...`

Blast radius:

- Files: 3
- Tests: existing tests only
- Time: ~5 minutes

Risks:

- Very low; changes rely on Go language guarantees for `len`.

Alternatives:

- Disable specific `staticcheck` rules (not recommended; increases future noise).

## 2) Strict JSON decode: reject trailing tokens (server handlers)

- Current (HEAD): `internal/server/handlers/DecodeJSON` decodes exactly one JSON value but does not enforce EOF; payloads like `{...} garbage` can be accepted. See `internal/server/handlers/ingest_common.go:59`.
- Proposed: enforce that the request body contains exactly one JSON value (after the object/array, only whitespace is allowed). This prevents silent contract drift and unexpected parse acceptance.
- Where:
  - `internal/server/handlers/DecodeJSON` in `internal/server/handlers/ingest_common.go:59`
  - Add/adjust tests under `internal/server/handlers/*_test.go` that exercise `DecodeJSON`.
- Compatibility impact: potentially breaking for clients sending trailing bytes; intended hardening.
- Unchanged: max body size cap and `DisallowUnknownFields()` behavior remain.

Implementation steps:

1. Update `DecodeJSON` to validate EOF after the initial decode:
   - After `dec.Decode(v)`, attempt `dec.Decode(&struct{}{})` and require `io.EOF`.
   - Keep existing `MaxBytesReader` and `DisallowUnknownFields()` behavior.
2. Add tests for `DecodeJSON`:
   - Accept: valid JSON object with whitespace.
   - Reject: valid JSON followed by non-whitespace token(s).
   - Preserve existing error mapping (400 for invalid JSON, 413 for size cap).
3. Run:
   - `go test ./internal/server/handlers -run DecodeJSON`
   - `go test ./internal/...`
   - `go vet ./...`

Blast radius:

- Files: `internal/server/handlers/ingest_common.go` + 1 test file
- Functions: `DecodeJSON`
- Time: ~20–40 minutes

Risks:

- Breaks callers that rely on permissive parsing. This is intentional hardening.

Alternatives:

- Keep permissive decode but add logging. (Lower safety; still accepts malformed payloads.)

## 3) Nodeagent URL building hardening: prevent host/scheme override

- Current (HEAD): `internal/nodeagent/BuildURL` uses `url.Parse(p)` + `ResolveReference`, so if `p` is an absolute URL it can override scheme/host. See `internal/nodeagent/httputil.go:16`.
- Proposed: enforce that `p` is a path-only reference (no scheme/host). Reject absolute URLs to prevent SSRF-style surprises if a “path” is constructed from untrusted input.
- Where:
  - `internal/nodeagent/BuildURL` + `MustBuildURL` in `internal/nodeagent/httputil.go`
  - Call sites that currently pass hard-coded `apiPath` strings (should remain unchanged).
  - Tests in `internal/nodeagent/*_test.go` (extend existing `heartbeat_connection_test.go`).
- Compatibility impact: potentially breaking if any call site currently passes absolute URLs as “path” (should be treated as a bug).
- Unchanged: for normal `/v1/...` paths, output URL is identical.

Implementation steps:

1. Update `BuildURL(base, p)`:
   - Parse base once (`bu`).
   - Parse `p` as a URL; if `pu.IsAbs()` or `pu.Host != ""` or `pu.Scheme != ""`, return an error like `invalid path reference`.
   - Ensure `p` is treated as a path reference (support both `"v1/foo"` and `"/v1/foo"` consistently).
2. Update/add tests:
   - Base with/without trailing slash + `p` with/without leading slash produces expected URL.
   - `p="https://evil.example/x"` is rejected.
3. Run:
   - `go test ./internal/nodeagent -run BuildURL`
   - `go test ./internal/...`

Blast radius:

- Files: `internal/nodeagent/httputil.go` + tests
- Functions: `BuildURL`, `MustBuildURL`
- Time: ~20–40 minutes

Risks:

- If any caller is (incorrectly) using absolute URLs as “paths”, this will surface as runtime error instead of silently overriding the base host.

Alternatives:

- Leave `BuildURL` as-is and rely on code review at call sites. (Less robust.)

## 4) Remove N+1 DB lookups: list run repos with URLs in one query

- Current (HEAD): `GET /v1/runs/{id}/repos` does:
  - `ListRunReposByRun` then per-repo `GetModRepo` to populate `repo_url`. See `internal/server/handlers/runs_batch_http.go:212`.
- Proposed: fetch run repos + repo_url via a single store query (join on `mod_repos`), eliminating N+1 queries.
- Where:
  - Handler: `internal/server/handlers/listRunReposHandler` in `internal/server/handlers/runs_batch_http.go`
  - Store query: prefer extending/using `internal/store/queries/run_repos.sql` (existing join query `ListRunReposWithURLByRun` at `run_repos.sql:109` is close but returns a different shape).
  - SQLC regen: `internal/store/run_repos.sql.go` (generated).
  - Tests: `internal/server/handlers/*runs_batch*_test.go`.
- Compatibility impact: none intended (response JSON shape unchanged).
- Unchanged: ordering (currently `ORDER BY created_at ASC, repo_id ASC`) and auth behavior.

Implementation steps:

1. Decide approach:
   - Option A (preferred): add a new query `ListRunReposWithURLByRun` that returns all fields currently used by `runRepoToResponse` plus `repo_url`.
   - Option B: modify existing `ListRunReposWithURLByRun` to return the needed fields and update its current callers.
2. Implement store query in `internal/store/queries/run_repos.sql`:
   - Join `run_repos rr` with `mod_repos mr` on `rr.repo_id = mr.id`.
   - Preserve current ordering.
3. Regenerate sqlc outputs (whatever the repo’s current workflow is) and update:
   - Handler uses the new query and no longer calls `GetModRepo` in a loop.
4. Tests:
   - Update handler tests to assert `repo_url` is present and correct.
   - Add a lightweight test that counts store calls if applicable (or validate via mock expectations).
5. Run:
   - `go test ./internal/server/handlers -run Runs`
   - `go test ./internal/...`

Blast radius:

- Files: `internal/server/handlers/runs_batch_http.go`, `internal/store/queries/run_repos.sql`, generated `internal/store/run_repos.sql.go`, handler tests
- Time: ~1–2 hours

Risks:

- SQLC regen churn and test expectation updates.

Alternatives:

- Cache `GetModRepo` results per request (reduces repeats but still does multiple queries).

## 5) Cancellation path robustness: bulk updates + transaction

- Current (HEAD): `POST /v1/runs/{id}/cancel`:
  - Updates run status, then best-effort loops over repos/jobs and ignores per-row update errors. See `internal/server/handlers/runs_batch_http.go:19`.
- Proposed: make cancellation updates atomic and consistently applied:
  - Wrap run + repo + job updates in a single transaction (or store-level methods that do the equivalent).
  - Prefer bulk update SQL (single statements) over per-row loops.
  - Decide and document error handling (fail request vs partial cancel) explicitly.
- Where:
  - Handler: `cancelRunHandlerV1` in `internal/server/handlers/runs_batch_http.go`
  - Store: new sqlc queries in `internal/store/queries/run_repos.sql` and `internal/store/queries/jobs.sql` (e.g. `CancelRunReposByRunID`, `CancelJobsByRunID`).
  - Tests: `internal/server/handlers/*cancel*test.go` or extend existing run handler tests.
- Compatibility impact: none intended (same endpoint/response shape), but operational semantics become stricter.
- Unchanged: v1 definition of “cancel”: Run status set to `Cancelled`, repo/job statuses moved to `Cancelled` when in active states.

Implementation steps:

1. Add store-level bulk queries:
   - `UpdateRunStatusCancelledIfNotTerminal` (optional to keep idempotence consistent).
   - `CancelActiveRunReposByRun` (Queued/Running → Cancelled).
   - `CancelActiveJobsByRun` (Created/Queued/Running → Cancelled; set `finished_at`, `duration_ms`).
2. Add a store method (non-sqlc) that runs those queries in a transaction:
   - `store.(*PgStore).CancelRunV1(ctx, runID)` or similar.
3. Update handler:
   - Use the transaction method.
   - On error: return 500 (do not silently ignore).
4. Tests:
   - Verify idempotence: cancelling an already-cancelled/finished run returns 200 with summary.
   - Verify state transitions: only active repos/jobs are cancelled; terminal states untouched.
5. Run:
   - `go test ./internal/server/handlers -run Cancel`
   - `go test ./internal/store -run Cancel`
   - `go test ./internal/...`

Blast radius:

- Files: handlers + store SQL + generated sqlc + tests
- Time: ~1–2 hours

Risks:

- Requires careful alignment with existing status derivation logic (e.g., repo status inference).

Alternatives:

- Keep handler loop but stop ignoring errors and collect/report failures. (Better than now, still not atomic.)

## 6) Shrink handler mocks: narrow interfaces per handler

- Current (HEAD): many handlers accept `store.Store` (which embeds sqlc `Querier`), forcing huge mocks in tests (e.g. `internal/server/handlers/test_mock_store_test.go`).
- Proposed: define narrow interfaces per handler (or per handler group) and adapt `register` wiring to pass `store.Store` where it satisfies those interfaces. This reduces test boilerplate and focuses handler tests on required DB behavior.
- Where:
  - `internal/server/handlers/*` function signatures (e.g., `cancelRunHandlerV1`, `listRunReposHandler`, etc.)
  - `internal/server/handlers/register.go` and call sites where handlers are built
  - Tests: remove/trim `test_mock_store_test.go` usage and add small local mocks per test file.
- Compatibility impact: internal-only (Go API); no HTTP surface change.
- Unchanged: handler logic and HTTP contracts.

Implementation steps:

1. Pick an initial slice (avoid “big bang”):
   - Start with `runs_batch_http.go` handlers only.
2. Define interfaces near the handlers:
   - Example: `type runRepoLister interface { ListRunReposByRun(ctx, runID) ([]store.RunRepo, error); GetModRepo(ctx, id) (store.ModRepo, error) }`
   - Keep interfaces minimal and file-scoped if possible.
3. Update handler constructors to accept those interfaces instead of `store.Store`.
4. Update `register.go` wiring to pass the existing `store.Store` (it should satisfy the interfaces).
5. Rewrite tests:
   - Replace large `mockStore` with small structs implementing only required methods.
6. Run:
   - `go test ./internal/server/handlers -run RunsBatch`
   - `go test ./internal/...`

Blast radius:

- Files: a few handlers + their tests + register wiring
- Time: ~4–8 hours if expanded broadly; ~1–2 hours for a single-file pilot.

Risks:

- Signature churn across handler construction; needs careful incremental rollout.

Alternatives:

- Keep `store.Store` but replace `test_mock_store_test.go` with generated mocks. (Still ties tests to huge interface.)

## 7) Split large “god files” for readability and reuse

- Current (HEAD):
  - `internal/workflow/runtime/step/gate_docker.go` is large and mixes concerns (image resolution, stack detection, log finding extraction, resource metrics, metadata normalization).
  - `internal/nodeagent/execution_healing.go` is large and combines retry loop state, manifest building, filesystem I/O, diff upload decisions, and logging.
- Proposed: split into smaller files with single responsibilities, keeping package-level API stable.
- Where:
  - `internal/workflow/runtime/step/*` (split `gate_docker.go` into focused files like `gate_images.go`, `gate_stack_gate.go`, `gate_logs.go`, `gate_resources.go`).
  - `internal/nodeagent/*` (split `execution_healing.go` into `healing_loop.go`, `healing_manifest.go`, `healing_workspace.go`, etc.).
- Compatibility impact: internal-only (Go file layout); no behavior change intended.
- Unchanged: gate semantics and healing loop semantics described in existing comments remain authoritative.

Implementation steps:

1. For `internal/workflow/runtime/step/gate_docker.go`:
   - Identify logical units:
     - Image resolution
     - Stack gate evaluation + error mapping
     - Gate execution + output capture
     - Metadata normalization + truncation + digest
     - Resource usage collection
   - Extract helpers into new files within same package (`package step`), keeping exported surface unchanged.
   - Keep tests passing; avoid changing behavior.
2. For `internal/nodeagent/execution_healing.go`:
   - Extract helpers for:
     - `/in` setup and `build-gate.log` persistence
     - Workspace status computation before/after healing
     - Session propagation file read/write
     - Retry loop core logic
   - Keep `runController.runGateWithHealing` signature unchanged initially.
3. Run:
   - `go test ./internal/workflow/runtime/step`
   - `go test ./internal/nodeagent`
   - `go test ./internal/...`
   - `staticcheck ./internal/...`

Blast radius:

- Files: multiple within two packages
- Time: ~2–4 hours (per package), depending on how aggressively split.

Risks:

- Refactor-only change risks subtle behavior drift; needs tight tests and minimal logic edits.

Alternatives:

- Leave files large but add internal helper types; smaller diff but less structural payoff.

