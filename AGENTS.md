# AGENTS.md

## Backward Compatibility and Deprecation Policy
Unless otherwise specified, the following policies apply when planning changes:
- **DO NOT** provide fallbacks or compatibility layers.
- **DO NOT** keep old versions/algorithms.
- **REMOVE** code and docs of replaced solutions.
- **DO NOT** plan or develop solutions for data migration.

## Before You Start

- Commit to the RED → GREEN → REFACTOR cadence for the upcoming change.
- Plan local unit tests and coverage checks before touching code or docs.
- Verify required environment variables (see `docs/envs/README.md`) are
  discoverable or called out as TODOs for future slices.

## Documentation 

### Folders Structure

- `design/` — design docs (how to implement).
- `research/` — research docs (what are options and how cool feature can possibly be).
- `roadmap/` — decomposed plans/implementation notes (in what order what to implement).
- `docs/` — actual state docs (how it is working right now).

### Policy

- With every commit of any diff it is mandatory to ensure that `docs/` reflect changes in that commit.
- When updating `docs/`, ensure that every file in that folder is limited within it's specific domain.
- Keep all documents cross-refereced.
- When creating file in `roadmap/`, use template from `/Users/vk/@iw2rmb/auto/ROADMAP.md` if not specified otherwise.

## Development

### TDD Framework (CRITICAL)

- Unit tests and CLI builds (RED/GREEN phases) run locally.
- E2E tests run against the local Docker cluster (see `scripts/deploy-locally.sh`).
  - Default CLI config: `PLOY_CONFIG_HOME=$PWD/local/cli`.
- Coverage: maintain ≥60% overall and ≥90% on critical workflow runner packages.
- Cycle: RED (failing tests) → GREEN (minimal code) → REFACTOR (exercise E2E when needed).
- For Mods E2E details, consult `tests/e2e/mods/README.md`.

#### TDD Discipline Validation

Use `scripts/validate-tdd-discipline.sh` to enforce RED→GREEN→REFACTOR discipline:

```bash
# Validate entire repository (recommended before commits)
./scripts/validate-tdd-discipline.sh

# Validate specific package during development
./scripts/validate-tdd-discipline.sh ./internal/workflow/...
```

The validation script checks:
1. **RED phase**: Test files exist for all packages with implementation code.
2. **GREEN phase**: All tests pass with `go test -cover ./...`.
3. **GREEN validation**: Coverage thresholds met (≥60% overall, ≥90% critical paths).
4. **REFACTOR validation**: Binary size remains under threshold (detects dependency bloat).
5. **Code quality**: `go vet` and `staticcheck` pass.

Reference: `GOLANG.md` (line 135-141).

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

- The local Docker cluster is the sole environment; no remote/VPS deployment or backward compatibility is required.
