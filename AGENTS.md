# AGENTS.md

## Before You Start

- Commit to the RED → GREEN → REFACTOR cadence for the upcoming change.
- Plan local unit tests and coverage checks before touching code or docs.
- Verify required environment variables (see `docs/envs/README.md`) are
  discoverable or called out as TODOs for future slices.

## Documentation

1. Keep documentation synchronized with the codebase. After each change, update
   the affected docs, link or remove orphaned files, and note completion states
   where applicable.
2. Control-plane API changes must include updates to `docs/api/OpenAPI.yaml`
   alongside human-facing docs.

## Local Development

### TDD Framework (CRITICAL)

- **LOCAL**: Unit tests and CLI builds (RED/GREEN phases).
- **VPS**: Reserved for integration/E2E tests once the workflow runner can
  talk to JetStream (REFACTOR phase).
- **Coverage**: Maintain ≥60% overall and ≥90% on critical workflow runner
  packages.
- **Cycle**: RED (write failing tests) → GREEN (minimal code) → REFACTOR
  (exercise VPS once available).
- For Codex execution details on the Mods Grid E2E harness, consult
  `tests/e2e/README.md`.
- Use the VPS lab control plane for integration/E2E verification. Connection and
  environment expectations are documented in `docs/next/vps-lab.md`; plan test
  runs there when validating hydration or other control-plane interactions.

### CLI Build & Smoke Checks

- `make build` places the CLI in `dist/ploy`.
- `make test` executes `go test -cover ./...` including guardrail tests for
  legacy imports and command directories.
- Keep the CLI binary minimal; all orchestration logic belongs in
  `internal/workflow/...` packages.

### Test File Naming

- Use descriptive names (`workflow_runner_test.go`, `event_contract_test.go`).
- Avoid catch-all suffixes such as `_more_test.go` or `_extra_test.go`.

### Deployment Scope

- The VPS lab is the sole environment; no production migration or backward compatibility is required. Prior changes can assume fresh redeploys.
- Environment variables are managed centrally in `docs/envs/README.md`.
