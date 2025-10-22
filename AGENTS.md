# AGENTS.md

Follow the global workflow rules in `~/.codex/AGENTS.md`. Repository-specific
addendum:

## Before You Start

- Commit to the RED → GREEN → REFACTOR cadence for the upcoming change.
- Plan local unit tests and coverage checks before touching code or docs.
- Verify required environment variables (see `docs/envs/README.md`) are
  discoverable or called out as TODOs for future slices.

## Documentation

1. Keep documentation synchronized with the codebase. After each change, update
   the affected docs, link or remove orphaned files, and note completion states
   where applicable.

## Local Development

### TDD Framework (CRITICAL)

- **LOCAL**: Unit tests and CLI builds (RED/GREEN phases).
- **GRID/VPS**: Reserved for integration/E2E tests once the workflow runner can
  talk to JetStream (REFACTOR phase).
- **Coverage**: Maintain ≥60% overall and ≥90% on critical workflow runner
  packages.
- **Cycle**: RED (write failing tests) → GREEN (minimal code) → REFACTOR
  (exercise Grid once available).
- For Codex execution details on the Mods Grid E2E harness, consult
  `tests/e2e/README.md`.

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
  static analysis for the workflow runner.

## Environment Variables

- Keep the canonical list of required variables in `docs/envs/README.md` and
  update that file whenever values change or new toggles are introduced.
