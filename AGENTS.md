# AGENTS.md

Follow the global workflow rules in `~/.codex/AGENTS.md`. Repository-specific
addendum:

## Before You Start

- Commit to the RED → GREEN → REFACTOR cadence for the upcoming change.
- Plan local unit tests and coverage checks before touching code or docs.
- Verify required environment variables (see `docs/envs/README.md`) are
  discoverable or called out as TODOs for future slices.
- Confirm you understand the workstation-only scope for the current roadmap
  slice (VPS/Grid integration resumes once JetStream wiring lands).
- Skim `docs/DOCS.md` so AGENTS.md and scoped READMEs stay aligned with
 documentation conventions.

## Documentation

1. Keep documentation synchronized with the codebase. After each change, update
   the affected docs, link or remove orphaned files, and note completion states
   where applicable.
2. When documentation conventions change, reflect them across `docs/DOCS.md`,
   the design index, and related README files so repo guidance stays consistent.
3. Run `make lint-md` before submitting doc updates to enforce the Markdown
   formatting conventions codified in `.markdownlint.yaml`.

## Local Development

### TDD Framework (CRITICAL)

- **LOCAL**: Unit tests and CLI builds (RED/GREEN phases).
- **GRID/VPS**: Reserved for integration/E2E tests once the workflow runner can
  talk to JetStream (REFACTOR phase).
- **Coverage**: Maintain ≥60% overall and ≥90% on critical workflow runner
  packages.
- **Cycle**: RED (write failing tests) → GREEN (minimal code) → REFACTOR
  (exercise Grid once available).

### Go Tooling (MANDATORY)

- Prefer the MCP tools shipped with `mcp-golang` for common tasks:
  - `mcp_golang__format_source` (or `run_playbook` → `format-lint-test`) for
    formatting.
  - `mcp_golang__lint_package` for static analysis.
  - `mcp_golang__test_with_coverage` for running unit tests (fallback:
    `go test -cover ./...`).
- Run `go mod tidy` after removing or adding dependencies.

### CLI Build & Smoke Checks

- `make build` places the CLI in `dist/ploy`.
- `make test` executes `go test -cover ./...` including guardrail tests for
  legacy imports and command directories.
- Keep the CLI binary minimal; all orchestration logic belongs in
  `internal/workflow/...` packages.

### Test File Naming

- Use descriptive names (`workflow_runner_test.go`, `event_contract_test.go`).
- Avoid catch-all suffixes such as `_more_test.go` or `_extra_test.go`.

### Pre-commit Hooks (Optional for now)

- If you enable pre-commit hooks, keep them focused on `gofmt`, `go test`,
  static analysis for the workflow runner, and Markdown linting (`markdownlint`).

## Grid Integration (Future REFACTOR)

- JetStream consumers and Grid RPC clients will be introduced in roadmap items
  `01-event-contracts` and `02-workflow-runner-cli`.
- Until then, avoid stubbing external calls beyond interfaces defined inside
  `internal/workflow`.
- When Grid integration arrives, run integration tests from the workstation
  using the Dev API (`GRID_ENDPOINT`), keeping the CLI stateless.

## Documentation Discipline

- Root README describes the CLI-first model; update it whenever behaviour
  changes.
- Roadmap updates must mark the relevant checklist item
  (`roadmap/shift/<nn>-*.md`).
- When recording new behaviour in `CHANGELOG.md`, include the roadmap slice
  reference and a concrete YYYY-MM-DD date.

## Environment Variables

- Keep the canonical list of required variables in `docs/envs/README.md` and
  update that file whenever values change or new toggles are introduced.

## Delivery Checklist

1. RED: add failing unit tests capturing the target behaviour.
2. GREEN: implement the minimal code to satisfy tests.
3. go test -cover ./...
4. Update documentation, the design index, and the relevant roadmap entry.
5. (Once Grid integration lands) perform REFACTOR verification against the Dev
   API.
6. Commit with a clear message and ensure branch is ready for PR.
