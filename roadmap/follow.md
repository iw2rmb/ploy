# Follow-Format Status Unification (`run status` + `mig status`)

Scope: Make `ploy run status` and new `ploy mig status` print the same follow-style frame, add live updates via `--follow`, and add per-step Logs links (`Download`) with `?auth_token=` support for browser/OSC8 flows.

Documentation: `AGENTS.md`; `roadmap/reporting.md`; `cmd/ploy/run_commands.go`; `cmd/ploy/mig_command.go`; `cmd/ploy/usage.go`; `internal/cli/follow/engine.go`; `internal/cli/runs/report_text.go`; `internal/cli/runs/report_builder.go`; `internal/server/auth/authorizer.go`; `internal/server/handlers/events.go`; `docs/api/paths/runs_id_logs.yaml`; `docs/api/paths/runs_run_id_repos_repo_id_logs.yaml`

Legend: [ ] todo, [x] done.

## Phase 0: Behavior Contract Tests
- [ ] Add/adjust CLI tests for the new status frame contract.
  - Repository: `ploy`
  - Component: `cmd/ploy` status command tests
  - Scope: Extend `cmd/ploy/run_status_test.go` with assertions for the exact header block and step table contract:
    - `Mig:   <mig id>  | <mig_name>`
    - `Spec:  <spec id> | Download`
    - `Repos: <X>`
    - `Run:   <run_id>`
    - `Repo:  [1/1] <repo_ref>`
    - step table includes `Logs` column with `Download`
    - running jobs in one-shot status use spinner frame 0 (`⣾ `)
  - Snippets: `executeCmd([]string{"run", "status", runID.String()}, &buf)`
  - Tests: `go test ./cmd/ploy -run 'TestRunStatus'` — must pass.

- [ ] Add/adjust tests for `--follow` on status commands (live update path).
  - Repository: `ploy`
  - Component: `cmd/ploy` integration-style command tests with SSE test server
  - Scope: Add coverage for `ploy run status --follow <run-id>` that verifies frame refresh and terminal stop, and same behavior for `ploy mig status --follow <run-id>`.
  - Snippets: `executeCmd([]string{"run", "status", "--follow", runID.String()}, &buf)`
  - Tests: `go test ./cmd/ploy -run 'Status.*Follow|MigStatus'`.

## Phase 1: Shared frame renderer (single output implementation)
- [ ] Extract follow frame text rendering into reusable code and have both engines call it.
  - Repository: `ploy`
  - Component: `internal/cli/runs` + `internal/cli/follow`
  - Scope: Move row/header/table rendering primitives out of `internal/cli/follow/engine.go` into a shared renderer (for example `internal/cli/runs/follow_frame_text.go`) and keep cursor/redraw control inside the follow engine. Reuse this same renderer from status commands.
  - Snippets: `func RenderFollowFrameText(frame FollowFrame, opts FollowFrameOptions) (string, int)`
  - Tests: Update `internal/cli/follow/engine_test.go` and add renderer-focused tests in `internal/cli/runs`.

- [ ] Centralize spinner glyph logic and explicitly support static frame rendering.
  - Repository: `ploy`
  - Component: shared render helpers
  - Scope: Move `spinnerFrames` + `statusGlyph` into shared helper; status one-shot path calls with frame index `0`; live follow path increments frames as now.
  - Snippets: `glyph := StatusGlyph(job.Status, 0)`
  - Tests: keep/update spinner tests (frame 0 = `⣾ `) in `internal/cli/follow/engine_test.go`.

## Phase 2: Status frame content and header contract
- [ ] Rebuild `run status` text rendering from the shared follow frame + required header block.
  - Repository: `ploy`
  - Component: `internal/cli/runs/report_text.go`, `internal/cli/runs/report_builder.go`
  - Scope: Replace current `Run:/Status:/Mig Name:/...` report header with requested lines and switch step table to follow base columns plus `Logs`:
    - `Step | Job ID | Node | Image | Duration | Logs`
    - `Logs` value: `Download` (OSC8 link to repo logs URL)
  - Snippets: `fmt.Fprintf(w, "Mig:   %s  | %s\n", report.MigID, report.MigName)`
  - Tests: update `internal/cli/runs/report_text_test.go` and `cmd/ploy/run_status_test.go` goldens/assertions.

- [ ] Keep output deterministic for multi-repo runs.
  - Repository: `ploy`
  - Component: shared status/follow frame model
  - Scope: For each repo block, print `Repo:  [i/N] <repo_ref_as_osc8_url>` and render that repo’s ordered jobs (`jobchain` order already produced by `ListRepoJobsCommand`).
  - Snippets: `Repo:  [2/5] <link>`
  - Tests: add multi-repo status rendering test in `internal/cli/runs/report_text_test.go`.

## Phase 3: Logs links with `auth_token` query parameter
- [ ] Build tokenized Logs URLs in CLI output without duplicating URL logic.
  - Repository: `ploy`
  - Component: `cmd/ploy/common_http.go`, `cmd/ploy/run_commands.go`, shared renderer options
  - Scope: Load cluster token once (same descriptor used by `resolveControlPlaneHTTP`) and pass it into renderer options so Logs links use:
    - `/v1/runs/{run_id}/repos/{repo_id}/logs?auth_token=<token>`
  - Snippets: `q.Set("auth_token", token)`
  - Tests: renderer/link tests in `internal/cli/runs/report_text_test.go` for OSC8 and plain modes.

- [ ] Accept `auth_token` on server for logs SSE endpoints when Authorization header is absent.
  - Repository: `ploy`
  - Component: `internal/server/auth/authorizer.go`
  - Scope: In `identityFromRequest`, add token fallback from query for `GET` logs paths only:
    - `/v1/runs/{id}/logs`
    - `/v1/runs/{run_id}/repos/{repo_id}/logs`
    Then validate exactly as bearer tokens (`identityFromBearerToken`).
  - Snippets: `token := strings.TrimSpace(r.URL.Query().Get("auth_token"))`
  - Tests: add `authorizer_bearer_test.go` cases:
    - accepts query token on allowed GET logs paths
    - rejects query token on non-logs paths
    - rejects query token on non-GET methods

## Phase 4: `--follow` on status commands + `mig status`
- [ ] Add `--follow` to `ploy run status` and wire to live refresh loop.
  - Repository: `ploy`
  - Component: `cmd/ploy/run_commands.go`, `internal/cli/follow/engine.go`
  - Scope: Extend `handleRunStatus` flags with `--follow` (and reuse existing max-retry/cap knobs if already available in shared follow config). Non-follow prints one frame (spinner frame 0). Follow mode streams and redraws until terminal.
  - Snippets: `if *followFlag { return followStatusRun(...) }`
  - Tests: `cmd/ploy/run_status_test.go` follow mode coverage.

- [ ] Add `ploy mig status` as a first-class command that reuses the same status implementation.
  - Repository: `ploy`
  - Component: `cmd/ploy/mig_command.go`, `cmd/ploy/usage.go`, help tests/goldens
  - Scope: Add `case "status":` in `handleMig` and delegate to shared status handler (same args/flags/output as `run status`). No separate renderer/transport logic.
  - Snippets: `case "status": return handleMigStatus(args[1:], stderr)`
  - Tests: add command routing/help tests and a behavioral parity test vs `run status`.

## Phase 5: Help/docs and API docs sync
- [ ] Update CLI help text and goldens for new status surfaces.
  - Repository: `ploy`
  - Component: `cmd/ploy/usage.go`, `cmd/ploy/testdata/help_mig.txt`, `cmd/ploy/help_flags_test.go`, `cmd/ploy/cli_test.go`
  - Scope: Document `mig status`; show `--follow` on both `run status` and `mig status`; remove stale wording that implies status lives only under `run`.
  - Snippets: `ploy mig status <run-id> [--follow]`
  - Tests: `go test ./cmd/ploy/...`.

- [ ] Document `auth_token` query auth for logs endpoints.
  - Repository: `ploy`
  - Component: `docs/api/paths/runs_id_logs.yaml`, `docs/api/paths/runs_run_id_repos_repo_id_logs.yaml`, related contract tests
  - Scope: Add optional `auth_token` query parameter docs and note header/query auth behavior for browser/OSC8 links.
  - Snippets: `GET .../logs?auth_token=<token>`
  - Tests: `go test ./docs/api/...`.

## Phase 6: Verification and cleanup
- [ ] Run focused local tests, then full suite.
  - Repository: `ploy`
  - Component: CLI, follow renderer, auth, API docs
  - Scope: Validate command behavior, renderer parity, and auth fallback; ensure no duplicate output code remains.
  - Snippets:
    - `go test ./cmd/ploy/...`
    - `go test ./internal/cli/follow/... ./internal/cli/runs/...`
    - `go test ./internal/server/auth/...`
    - `make test`
  - Tests: all above must pass; final check with manual smoke:
    - `ploy run status <run-id>`
    - `ploy run status --follow <run-id>`
    - `ploy mig status <run-id>`
    - `ploy mig status --follow <run-id>`

## Open Questions
- None.
