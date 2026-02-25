# AGENTS.md

## Backward Compatibility and Deprecation Policy
Unless otherwise specified, the following policies apply when planning changes:
- **DO NOT** provide fallbacks or compatibility layers.
- **DO NOT** keep old versions/algorithms.
- **REMOVE** code and docs of replaced solutions.
- **DO NOT** plan or develop solutions for data migration.

## Before You Start

- Plan local unit tests before touching code or docs.
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

### Testing Framework (CRITICAL)

- Unit tests and CLI builds run locally.
- E2E tests run against the local Docker cluster (see `deploy/local/run.sh`).
  - Default CLI config: `PLOY_CONFIG_HOME=$PWD/deploy/local/cli`.
- For Mods E2E details, consult `tests/e2e/migs/README.md`.

#### Validation Commands

Use Make targets and standard Go tooling:

```bash
# Unit tests
make test

# Hygiene
make vet
make staticcheck
```

Reference: `docs/testing-workflow.md`.

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
