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

1. Keep documentation synchronized with the codebase. After each change, update
   the affected docs, link or remove orphaned files, and note completion states
   where applicable.
2. Control-plane API changes must include updates to `docs/api/OpenAPI.yaml`
   alongside human-facing docs.
3. How-to references:
   - `docs/how-to/deploy-a-cluster.md` — Deploy a cluster from scratch.
   - `docs/how-to/update-a-cluster.md` — Update `ployd` across VPS nodes.
   - `docs/envs/README.md` — Canonical environment variables.

## Documentation Layout Policy

1. `docs/` is normative for the CURRENT IMPLEMENTED state at HEAD only.
   - Planned/unimplemented behavior MUST NOT live under `docs/`.
   - If behavior is not implemented, it MUST live under `roadmap/`.
2. Planned / next-version documentation MUST live under `roadmap/vN/` (e.g. `roadmap/v1/`).
   - Version folders under `docs/` are not allowed for planned work.
3. In documents in `roadmap/vN/`:
   - Every proposed change MUST be explained in-place where it is relevant (API/DB/CLI/etc).
   - Each proposed change MUST make it clear what remains unchanged by anchoring to the current behavior:
     - Prefer references to code (file paths + symbols where applicable).
     - Otherwise reference `docs/` headings (path + heading).
   - Each change entry MUST state:
     - what changes (vN behavior),
     - where it will be implemented (paths + likely symbols/handlers/sql),
     - compatibility impact (explicit; “none required” when applicable),
     - and what remains unchanged (usually a reference to the current behavior).
4. When a roadmap version is implemented:
   - All implemented behavior MUST be incorporated into `docs/` (and update any existing code comments that cite spec paths).
   - The corresponding `roadmap/vN/` folder MUST be deleted in the same change to avoid misinterpretation.

## Development

### TDD Framework (CRITICAL)

- Unit tests and CLI builds (RED/GREEN phases) run locally.
- VPS lab is reserved for integration/E2E tests and manual smoke:
  - Nodes: 45.9.42.212 (A), 193.242.109.13 (B), 45.130.213.91 (C).
  - Reuse the current cluster descriptor from `~/.config/ploy/clusters/` to connect.
    If multiple descriptors exist, prefer the default marker (`~/.config/ploy/clusters/default`) or
    pick the one matching the lab's cluster ID.
- Coverage: maintain ≥60% overall and ≥90% on critical workflow runner packages.
- Cycle: RED (failing tests) → GREEN (minimal code) → REFACTOR (exercise VPS when needed).
- For Codex execution details on the Mods E2E harness, consult `tests/e2e/README.md`.

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

- The VPS lab is the sole environment; no production migration or backward compatibility is required. Prior changes can assume fresh redeploys.
