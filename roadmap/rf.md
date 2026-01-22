# Refactors + Fixes (internal/)

Scope: Incremental refactors and hardening fixes inside `internal/` to reduce operational risk (API parsing, URL construction), improve performance (DB query shape), and simplify testing/maintenance (mocks, file decomposition). This is a plan only.

Documentation: `AGENTS.md`, `/Users/vk/@iw2rmb/auto/ROADMAP.md`, `GOLANG.md`, and the referenced code locations per step.

Legend: [ ] todo, [x] done.

## Code Quality
- [ ] Remove redundant `len(nil)` guards (S1009) — Unblocks `staticcheck`.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts`, `internal/workflow/stackdetect`
  - Scope: Update these checks (no behavior change intended):
    - `internal/workflow/contracts/stack_gate_spec_parse.go:16` (`parseStackGateSpec`): `if raw == nil || len(raw) == 0` → `if len(raw) == 0`
    - `internal/workflow/stackdetect/gradle.go:130` (`extractCompatibilityVersion`): `if matches == nil || len(matches) < 2` → `if len(matches) < 2`
    - `internal/workflow/stackdetect/gradle_test.go:60` (regex test): `if matches != nil && len(matches) > 1` → `if len(matches) > 1`
    - Blast radius: 3 files; estimate: ~5 minutes.
  - Snippets:
    - `if len(raw) == 0 { return nil, nil }`
    - `if len(matches) < 2 { return "" }`
  - Tests: `staticcheck ./internal/...` + `go test ./internal/...` — Expect clean `staticcheck` run.

## Server Request Parsing
- [ ] Enforce EOF after JSON decode in `DecodeJSON` — Prevents accepting `{...} trailing-garbage` payloads.
  - Repository: `ploy`
  - Component: `internal/server/handlers`
  - Scope:
    - Update `internal/server/handlers/ingest_common.go:59` (`DecodeJSON`) to require that the body contains exactly one JSON value.
    - Keep existing behavior: `http.MaxBytesReader` size cap + `dec.DisallowUnknownFields()`.
    - Add unit tests under `internal/server/handlers` to cover acceptance/rejection cases.
    - Blast radius: 1 implementation file + 1 test file; estimate: ~20–40 minutes.
  - Snippets:
    - `if err := dec.Decode(&struct{}{}); err != io.EOF { http.Error(..., http.StatusBadRequest); return err }`
  - Tests:
    - `go test ./internal/server/handlers -run DecodeJSON` — Expect rejection for trailing non-whitespace tokens.
    - `go test ./internal/...` + `go vet ./...` — Expect pass.

## Node Agent HTTP
- [ ] Reject absolute URLs passed as “path” to `BuildURL` — Prevents scheme/host override via `ResolveReference`.
  - Repository: `ploy`
  - Component: `internal/nodeagent`
  - Scope:
    - Update `internal/nodeagent/httputil.go:16` (`BuildURL`) to validate that `p` is path-only (no scheme/host).
    - Update `MustBuildURL` behavior only via `BuildURL` (still panics on error).
    - Extend tests in `internal/nodeagent/heartbeat_connection_test.go` (or a new focused test file) to cover rejection of absolute `p`.
    - Blast radius: 1 implementation file + tests; estimate: ~20–40 minutes.
  - Snippets:
    - `if pu.IsAbs() || pu.Scheme != "" || pu.Host != "" { return "", fmt.Errorf("invalid path reference") }`
  - Tests:
    - `go test ./internal/nodeagent -run BuildURL` — Expect absolute path URLs rejected.
    - `go test ./internal/...` — Expect pass.

## Control Plane API Performance
- [ ] Add a store query that lists run repos with `repo_url` in one roundtrip — Removes N+1 DB calls in `/v1/runs/{id}/repos`.
  - Repository: `ploy`
  - Component: `internal/store` (sqlc queries)
  - Scope:
    - Add/adjust a query in `internal/store/queries/run_repos.sql` that joins `run_repos rr` to `mod_repos mr` and returns the fields required by `runRepoToResponse` plus `mr.repo_url`.
    - Preserve ordering from `ListRunReposByRun` (`ORDER BY rr.created_at ASC, rr.repo_id ASC`).
    - Regenerate sqlc outputs (`internal/store/run_repos.sql.go`).
    - Blast radius: store SQL + generated file; estimate: ~30–60 minutes.
  - Snippets:
    - `SELECT rr.*, mr.repo_url FROM run_repos rr JOIN mod_repos mr ON rr.repo_id = mr.id WHERE rr.run_id = $1 ORDER BY rr.created_at ASC, rr.repo_id ASC;`
  - Tests: `go test ./internal/store -run RunRepos` — Expect new query returns URL and preserves ordering.

- [ ] Update `GET /v1/runs/{id}/repos` handler to use the joined query — Avoids per-row `GetModRepo` lookups.
  - Repository: `ploy`
  - Component: `internal/server/handlers`
  - Scope:
    - Update `internal/server/handlers/runs_batch_http.go:212` (`listRunReposHandler`) to call the new store query and fill `repo_url` without N+1 calls.
    - Keep response JSON shape unchanged.
    - Blast radius: 1 handler + tests; estimate: ~30–60 minutes.
  - Snippets:
    - Replace loop `GetModRepo(...)` with `repo_url` from joined query row.
  - Tests: `go test ./internal/server/handlers -run ListRunRepos` — Expect correct `repo_url` values and unchanged response shape.

## Cancellation Robustness (v1)
- [ ] Add bulk cancellation SQL queries for repos/jobs — Eliminates per-row loops and enables transactional cancel.
  - Repository: `ploy`
  - Component: `internal/store` (sqlc queries)
  - Scope:
    - Add queries in:
      - `internal/store/queries/run_repos.sql`: cancel Queued/Running repos by `run_id`
      - `internal/store/queries/jobs.sql`: cancel Created/Queued/Running jobs by `run_id` with consistent `finished_at`/`duration_ms`
    - Regenerate sqlc outputs (generated `*.sql.go` files).
    - Blast radius: store SQL + generated files; estimate: ~30–60 minutes.
  - Snippets:
    - `UPDATE run_repos SET status = 'Cancelled', finished_at = COALESCE(finished_at, now()) WHERE run_id = $1 AND status IN ('Queued','Running');`
  - Tests: `go test ./internal/store -run Cancel` — Expect only active rows are transitioned.

- [ ] Implement a transactional store method for cancel — Makes cancel atomic and error-checked.
  - Repository: `ploy`
  - Component: `internal/store`
  - Scope:
    - Add a method (e.g. `(*PgStore).CancelRunV1(ctx, runID)`) that runs:
      - run status update (if not terminal)
      - bulk repo cancel query
      - bulk job cancel query
    - Use a single DB transaction.
    - Blast radius: 1 store file + tests; estimate: ~30–60 minutes.
  - Snippets:
    - Transaction wrapper around the three update statements.
  - Tests: `go test ./internal/store -run CancelRunV1` — Expect all-or-nothing behavior on injected error.

- [ ] Update `cancelRunHandlerV1` to use the transactional store method — Stops silently ignoring update failures.
  - Repository: `ploy`
  - Component: `internal/server/handlers`
  - Scope:
    - Update `internal/server/handlers/runs_batch_http.go:19` (`cancelRunHandlerV1`) to call the new store method and propagate errors (500) instead of best-effort ignoring.
    - Preserve idempotence: if run is terminal, return current summary.
    - Blast radius: handler + tests; estimate: ~30–60 minutes.
  - Snippets:
    - Replace repo/job loops with `st.CancelRunV1(...)` (or equivalent interface).
  - Tests: `go test ./internal/server/handlers -run CancelRun` — Expect repos/jobs canceled consistently; handler returns 500 on DB failure.

## Handler Testability (Mock Size)
- [ ] Introduce narrow store interfaces for `runs_batch_http.go` handlers — Shrinks mocks and reduces test maintenance cost.
  - Repository: `ploy`
  - Component: `internal/server/handlers`
  - Scope:
    - Start with `internal/server/handlers/runs_batch_http.go` only (pilot slice).
    - Change handler constructors to accept minimal interfaces instead of `store.Store`.
    - Update `internal/server/handlers/register.go` wiring to pass `store.Store` where it satisfies the minimal interfaces.
    - Update tests to use small per-test mocks and reduce reliance on `internal/server/handlers/test_mock_store_test.go`.
    - Blast radius: handler signatures + register wiring + tests; estimate: ~1–2 hours for the pilot.
  - Snippets:
    - `type runRepoLister interface { ListRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) }`
  - Tests: `go test ./internal/server/handlers -run RunsBatch` — Expect identical handler behavior with smaller mocks.

## File Decomposition (Maintainability)
- [ ] Split `internal/workflow/runtime/step/gate_docker.go` into focused files — Improves readability and isolates concerns.
  - Repository: `ploy`
  - Component: `internal/workflow/runtime/step`
  - Scope:
    - Extract cohesive helpers into new files in the same package (keep exported surface stable):
      - image resolution (`BuildGateImageResolver` usage)
      - stack-gate evaluation/error mapping
      - metadata normalization (logs truncation + digest)
      - resource usage collection
    - Avoid semantic changes; keep tests as the oracle.
    - Blast radius: multiple files in one package; estimate: ~2–4 hours.
  - Snippets: n/a (refactor-only; keep diffs mechanical).
  - Tests: `go test ./internal/workflow/runtime/step` + `go test ./internal/...` + `staticcheck ./internal/...` — Expect no behavior change and clean checks.

- [ ] Split `internal/nodeagent/execution_healing.go` into focused files — Reduces complexity of the healing retry loop.
  - Repository: `ploy`
  - Component: `internal/nodeagent`
  - Scope:
    - Extract helpers into new files in the same package (keep existing behavior):
      - `/in` directory setup and `build-gate.log` persistence
      - workspace status calculation pre/post healing
      - session propagation read/write (`codex-session.txt` contract)
      - healing loop state machine
    - Keep `runController.runGateWithHealing` signature stable initially.
    - Blast radius: multiple files in one package; estimate: ~2–4 hours.
  - Snippets: n/a (refactor-only; keep diffs mechanical).
  - Tests: `go test ./internal/nodeagent` + `go test ./internal/...` + `staticcheck ./internal/...` — Expect no behavior change and clean checks.
