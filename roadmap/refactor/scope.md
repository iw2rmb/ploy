# Refactor Scope (`roadmap/refactor`)

This folder now tracks only *remaining* refactor work. Implemented behavior lives under `docs/` (and code).

## Remaining Priorities

- Fix stream hub safety risks (see `roadmap/refactor/stream.md`).
- Make spec-merge behavior strict (reject invalid/non-object JSON) (see `roadmap/refactor/contracts.md`, `roadmap/refactor/server.md`).
- Update heartbeat/resource units to integer + unit-explicit fields end-to-end (see `roadmap/refactor/contracts.md`, `roadmap/refactor/server.md`, `roadmap/refactor/worker.md`).
- Standardize CLI HTTP boundary behavior (preserve `BaseURL.Path`, strict JSON decode) (see `roadmap/refactor/contracts.md`, `roadmap/refactor/cli-runs.md`, `roadmap/refactor/cli-mods.md`).
