# Unified Run Reporting (Text + JSON)

Scope: Replace fragmented status outputs with one canonical run report exposed by `ploy run status <run-id>` in exactly two formats: human follow-style text (default) and machine JSON (`--json`). Remove overlapping status variants, especially `ploy mig run repo status`.

Documentation: `AGENTS.md`; `cmd/ploy/README.md`; `docs/migs-lifecycle.md`; `docs/testing-workflow.md`; `cmd/ploy/testdata/help.txt`

Legend: [ ] todo, [x] done.

## Phase 0: Contract and RED Gates
- [x] Define a single report contract used by both renderers — keeps text and JSON outputs structurally aligned.
  - Repository: `ploy`
  - Component: CLI reporting contract (`internal/cli/runs`), command flags (`cmd/ploy`)
  - Scope: Introduce a `RunReport` model with explicit fields for `mig_id`, `mig_name`, `spec_id`, repo rows (`repo_id`, `repo_url`, `base_ref`, `target_ref`, status, attempts, error), job/run graph entries (same rows currently shown by `--follow`), and link metadata for build logs + patches.
  - Snippets: `type RunReport struct { MigID string; MigName string; SpecID string; Repos []RepoReport; Runs []RunEntry }`
  - Tests: Add failing contract tests for both output formats derived from one fixture model — expect identical semantic data in text and JSON.

- [x] Add RED CLI behavior tests for the new status surface — prevents regressions while replacing old variants.
  - Repository: `ploy`
  - Component: `cmd/ploy/*_test.go`, help golden files
  - Scope: Add failing tests for: `ploy run status <id>` default human report, `ploy run status --json <id>` JSON report, and hard failure/removal path for `ploy mig run repo status`.
  - Snippets: `ploy run status <run-id>`; `ploy run status --json <run-id>`; `ploy mig run repo status <run-id>`
  - Tests: `go test ./cmd/ploy/...` — RED expected before implementation lands.

## Phase 1: Data Assembly (Canonical Report Builder)
- [x] Implement a report builder that aggregates all data once — centralizes collection logic for both renderers.
  - Repository: `ploy`
  - Component: `internal/cli/runs` (new report builder module), existing run/mig API clients
  - Scope: Build a `GetRunReportCommand` (or equivalent) that composes run summary, mig identity/name, spec id, repo base/target refs, per-repo job graph data, and artifact/log endpoints needed for OSC8 link rendering.
  - Snippets: `internal/cli/runs/report.go`; `internal/cli/runs/report_builder.go`
  - Tests: Unit tests with mocked HTTP responses to validate assembled contract, including missing optional fields.

- [x] Normalize link fields for build logs and patches in the report model — makes OSC8/text and JSON deterministic.
  - Repository: `ploy`
  - Component: report builder + URL helpers
  - Scope: Populate explicit URL fields per repo/job for build logs and patch downloads; keep relative/API path behavior consistent with existing `run logs` and `run diff` endpoints.
  - Snippets: `BuildLogURL`, `PatchURL`
  - Tests: Builder tests verify URL presence/absence and stable formatting.

## Phase 2: Renderer 1 (Human Follow-Style Snapshot)
- [x] Reuse follow visual language for `run status` output — provides one readable operator format.
  - Repository: `ploy`
  - Component: `cmd/ploy/run_commands.go`, `internal/cli/follow` (shared rendering helpers)
  - Scope: Render a static snapshot that matches follow job graph style; prepend run header with mig name and spec id; print repo header as `Repo: <repo> <base> -> <target>`; include statuses, durations, and errors.
  - Snippets: `Repo: github.com/org/repo main -> ploy/upgrade`
  - Tests: Golden output tests for representative runs (success, fail, partial, empty repos).

- [x] Add OSC8 hyperlinks for build logs and patches in text report — improves terminal navigation without changing JSON schema.
  - Repository: `ploy`
  - Component: status renderer utilities
  - Scope: Emit OSC8 links when terminal supports them; provide plain-text fallback labels when unsupported or disabled.
  - Snippets: `\x1b]8;;<url>\x1b\\build-log\x1b]8;;\x1b\\`
  - Tests: Renderer tests for OSC8-on and OSC8-off modes.

## Phase 3: Renderer 2 (JSON)
- [x] Implement `--json` on `ploy run status` with the same semantic payload as the human report — supports automation without parallel models.
  - Repository: `ploy`
  - Component: `cmd/ploy/run_commands.go`, `internal/cli/runs/report_json.go`
  - Scope: Add `--json` flag to `run status`; serialize canonical `RunReport` with top-level keys including `mig_id`, `mig_name`, `spec_id`, `repos`, `runs` and nested link fields.
  - Snippets: `{ "mig_id": "...", "mig_name": "...", "repos": [...], "runs": [...] }`
  - Tests: JSON snapshot tests and schema-shape tests; parity assertions against human renderer fixture input.

## Phase 4: Command Surface Consolidation (Remove Variants)
- [x] Remove `ploy mig run repo status` and route users to `ploy run status` — enforces a single status/report entrypoint.
  - Repository: `ploy`
  - Component: `cmd/ploy/mig_run_repo.go`, mig command routing/help
  - Scope: Delete repo status handler and usage lines; update command errors/help text to point to `ploy run status <run-id> [--json]`.
  - Snippets: error text `mig run repo status has been removed; use 'ploy run status <run-id>'`
  - Tests: Command routing tests updated for removed action.

- [x] Remove/replace other status-format variants in CLI docs/help/tests — completes de-duplication.
  - Repository: `ploy`
  - Component: `cmd/ploy/usage.go`, `cmd/ploy/README.md`, help goldens, docs references
  - Scope: Ensure docs and help expose only one report command with two formats; remove stale examples mentioning removed status variant.
  - Snippets: `ploy run status <run-id> [--json]`
  - Tests: `go test ./cmd/ploy/...` help golden checks pass.

## Phase 5: GREEN and REFACTOR Validation
- [x] Complete GREEN phase with local unit tests and build checks — validates behavior and command UX.
  - Repository: `ploy`
  - Component: CLI + internal report packages
  - Scope: Make all RED tests pass after implementation; keep scope limited to reporting/status surfaces.
  - Snippets: `make test`; `make build`
  - Tests: all relevant CLI/report tests green.

- [x] REFACTOR shared output code between follow and status report — removes duplication while preserving behavior.
  - Repository: `ploy`
  - Component: `internal/cli/follow`, `internal/cli/runs`
  - Scope: Extract shared row/column formatting primitives and keep follow dynamic redraw independent from status one-shot rendering.
  - Snippets: shared formatter package/function for repo/job rows
  - Tests: no golden drift except intentional header additions/links.

## Open Questions
- None.
