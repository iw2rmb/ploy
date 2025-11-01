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
3. How-to references:
   - `docs/how-to/deploy-a-cluster.md` — Deploy a cluster from scratch.
   - `docs/how-to/update-a-cluster.md` — Update `ployd` across VPS nodes.
   - `docs/envs/README.md` — Canonical environment variables.
 4. Consolidation policy: the former exploration docs (`ARCHITECTURE_DIAGRAM.md`,
    `CODEBASE_EXPLORATION.md`, `EXPLORATION_INDEX.md`, `EXPLORATION_README.md`) were
    removed on 2025‑11‑01. Keep architecture material in `README.md` (overview)
    and `SIMPLE.md` (deep dive). Do not reintroduce parallel sources.

## Development

### TDD Framework (CRITICAL)

- Unit tests and CLI builds (RED/GREEN phases) run locally.
- VPS lab is reserved for integration/E2E tests and manual smoke:
  - Nodes: 45.9.42.212 (A), 46.173.16.177 (B), 81.200.119.187 (C).
  - Reuse the current cluster descriptor from `~/.config/ploy/clusters/` to connect.
    If multiple descriptors exist, prefer the default marker (`~/.config/ploy/clusters/default`) or
    pick the one matching the lab’s cluster ID.
- Coverage: maintain ≥60% overall and ≥90% on critical workflow runner packages.
- Cycle: RED (failing tests) → GREEN (minimal code) → REFACTOR (exercise VPS when needed).
- For Codex execution details on the Mods E2E harness, consult `tests/e2e/README.md`.

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
