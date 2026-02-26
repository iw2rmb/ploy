# Follow-Format Status Updates (`run status` + `mig status`)

Scope: keep `ploy run status` on follow-style repo/job frames, add build/heal exit one-liners, rename `Logs` column to `Artifacts` with tokenized OSC8 links, and change `ploy mig status` to accept `<mig-id>` with its own migration-level output.

Documentation: `AGENTS.md`; `roadmap/reporting.md`; `cmd/ploy/run_commands.go`; `cmd/ploy/mig_command.go`; `cmd/ploy/usage.go`; `internal/cli/follow/engine.go`; `internal/cli/runs/report_text.go`; `internal/cli/runs/report_builder.go`; `internal/server/auth/authorizer.go`; `internal/server/handlers/events.go`; `docs/api/paths/runs_id_logs.yaml`; `docs/api/paths/runs_run_id_repos_repo_id_logs.yaml`

Legend: [ ] todo, [x] done.

## Phase 0: Behavior Contract Tests
- [ ] Add/adjust CLI tests for `run status` frame contract.
  - Repository: `ploy`
  - Component: `cmd/ploy` status command tests
  - Scope: Extend `cmd/ploy/run_status_test.go` with assertions for:
    - header block:
      - `Mig:   <mig id>  | <mig_name>`
      - `Spec:  <spec id> | Download`
      - `Repos: <X>`
      - `Run:   <run_id>`
      - `Repo:  [1/1] <repo_ref>`
    - step table column rename:
      - `Step | Job ID | Node | Image | Duration | Artifacts`
    - artifacts cell values:
      - `Logs` (OSC8 link)
      - `Logs | Patch` (both OSC8 links) when patch exists
    - failed/crashed build one-liner:
      - `✗  pre_gate   <job-id>  <node> ...`
      - `└  Exit <ExitCode>: <Error-light-red-color>`
    - healing one-liner:
      - `✓  Heal       <job-id>  <node> ...`
      - `└  Exit <ExitCode>: <One-liner-from-healer>`
    - running jobs in one-shot status use spinner frame 0 (`⣾ `)
  - Snippets: `executeCmd([]string{"run", "status", runID.String()}, &buf)`
  - Tests: `go test ./cmd/ploy -run 'TestRunStatus'`.

- [ ] Add/adjust tests for `ploy mig status <mig-id>` output contract.
  - Repository: `ploy`
  - Component: `cmd/ploy` migration status tests
  - Scope: Add coverage for dedicated migration output:
    - `Mig:   <mig id>  | <mig_name>`
    - `Spec:  <spec id> | Download`
    - `Repos: <X>`
    - runs summary table:
      - header: `Run             Success        Fail`
      - row glyphs with run state (spinner/check)
      - row values: `<run id>`, success repo count, fail repo count
  - Snippets: `executeCmd([]string{"mig", "status", migID.String()}, &buf)`
  - Tests: `go test ./cmd/ploy -run 'TestMigStatus'`.

## Phase 1: Shared rendering primitives
- [ ] Keep shared follow frame rendering for `run status`, and extend row model for optional exit one-liner rows.
  - Repository: `ploy`
  - Component: `internal/cli/runs` + `internal/cli/follow`
  - Scope: Reuse one row/table renderer, add optional second line under a step for exit summaries (`Exit <code>: <message>`), and keep cursor/redraw control in follow engine.
  - Snippets: `func RenderFollowFrameText(frame FollowFrame, opts FollowFrameOptions) (string, int)`
  - Tests: update `internal/cli/follow/engine_test.go` and `internal/cli/runs` renderer tests.

- [ ] Keep spinner glyph logic centralized and deterministic.
  - Repository: `ploy`
  - Component: shared render helpers
  - Scope: Keep `spinnerFrames` + `statusGlyph` in shared helper; one-shot paths render frame index `0`; live follow increments frames.
  - Snippets: `glyph := StatusGlyph(status, 0)`
  - Tests: spinner tests in `internal/cli/follow/engine_test.go`.

## Phase 2: `run status` content changes
- [ ] Rename table column and wire artifacts link rendering.
  - Repository: `ploy`
  - Component: `internal/cli/runs/report_text.go`, `internal/cli/runs/report_builder.go`
  - Scope:
    - rename `Logs` column to `Artifacts`
    - value is `Logs` OSC8 link to repo logs
    - value is `Logs | Patch` when patch exists, with both OSC8 links
  - Snippets: `Step | Job ID | Node | Image | Duration | Artifacts`
  - Tests: update `internal/cli/runs/report_text_test.go` and `cmd/ploy/run_status_test.go`.

- [ ] Add exit one-liners for build failures/crashes and healing steps.
  - Repository: `ploy`
  - Component: `internal/cli/runs/report_builder.go`, `internal/cli/runs/report_text.go`
  - Scope:
    - when build step ends with error/crash, print:
      - `└  Exit <ExitCode>: <Error-light-red-color>`
    - for healing steps, print:
      - `└  Exit <ExitCode>: <One-liner-from-healer>`
  - Snippets: `line := fmt.Sprintf("Exit %d: %s", code, oneLiner)`
  - Tests: golden assertions for failed build and heal rows.

## Phase 3: Artifact links with `auth_token` query parameter
- [ ] Build tokenized `Logs` and `Patch` URLs in CLI output.
  - Repository: `ploy`
  - Component: `cmd/ploy/common_http.go`, `cmd/ploy/run_commands.go`, renderer options
  - Scope: Load cluster token once and append `?auth_token=<token>` to artifact links used by OSC8 rendering.
  - Snippets: `q.Set("auth_token", token)`
  - Tests: renderer/link tests in `internal/cli/runs/report_text_test.go` for OSC8 and plain modes.

- [ ] Keep server query-token auth behavior aligned for artifact endpoints.
  - Repository: `ploy`
  - Component: `internal/server/auth/authorizer.go`
  - Scope: Use query token fallback on allowed GET artifact endpoints when Authorization header is absent; validate token with existing bearer validation path.
  - Snippets: `token := strings.TrimSpace(r.URL.Query().Get("auth_token"))`
  - Tests: `authorizer_bearer_test.go` cases for allowed/disallowed path+method combinations.

## Phase 4: `mig status <mig-id>` with dedicated output
- [ ] Change command shape from run-scoped to migration-scoped.
  - Repository: `ploy`
  - Component: `cmd/ploy/mig_command.go`, `cmd/ploy/usage.go`
  - Scope: `ploy mig status <mig-id>` resolves migration and lists its runs.
  - Snippets: `ploy mig status <mig-id>`
  - Tests: command parsing tests and usage goldens.

- [ ] Implement dedicated migration status renderer (not run-status frame reuse).
  - Repository: `ploy`
  - Component: `internal/cli/runs/report_text.go` (or dedicated mig status text file)
  - Scope:
    - render header:
      - `Mig:   <mig id>  | <mig_name>`
      - `Spec:  <spec id> | Download`
      - `Repos: <X>`
    - render runs table:
      - header: `Run             Success        Fail`
      - rows: `<glyph>  <run-id> ...   <success-count>  <fail-count>`
  - Snippets: `fmt.Fprintf(w, "Mig:   %s  | %s\n", migID, migName)`
  - Tests: `cmd/ploy/mig_status_test.go` golden coverage for mixed run states.

## Phase 5: Help/docs and API docs sync
- [ ] Update CLI help text and goldens.
  - Repository: `ploy`
  - Component: `cmd/ploy/usage.go`, `cmd/ploy/testdata/help_mig.txt`, `cmd/ploy/help_flags_test.go`, `cmd/ploy/cli_test.go`
  - Scope: Replace `ploy mig status <run-id>` with `ploy mig status <mig-id>` and document dedicated output contract.
  - Snippets: `ploy mig status <mig-id>`
  - Tests: `go test ./cmd/ploy/...`.

- [ ] Document query-token behavior for artifact links.
  - Repository: `ploy`
  - Component: `docs/api/paths/runs_id_logs.yaml`, `docs/api/paths/runs_run_id_repos_repo_id_logs.yaml`, patch artifact path docs if separate
  - Scope: Document `auth_token` query parameter for browser/OSC8 artifact link flows.
  - Snippets: `GET .../logs?auth_token=<token>`
  - Tests: `go test ./docs/api/...`.

## Phase 6: Verification and cleanup
- [ ] Run focused local tests, then full suite.
  - Repository: `ploy`
  - Component: CLI, follow renderer, auth, API docs
  - Scope: Validate one-liners, artifact links, and migration-level status output; remove duplicate rendering code.
  - Snippets:
    - `go test ./cmd/ploy/...`
    - `go test ./internal/cli/follow/... ./internal/cli/runs/...`
    - `go test ./internal/server/auth/...`
    - `make test`
  - Tests: all above must pass; final smoke:
    - `ploy run status <run-id>`
    - `ploy run status --follow <run-id>`
    - `ploy mig status <mig-id>`

## Open Questions
- None.
