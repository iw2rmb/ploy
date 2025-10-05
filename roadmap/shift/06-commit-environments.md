# Commit-Scoped Environments

- [x] Done (2025-09-26)

## Why / What For

Let developers materialise `<sha>-<app>` environments on demand with
deterministic caches, snapshots, and manifests.

## Required Changes

- Implement `ploy environment materialize` command with dry-run and execution
  modes.
- Stitch together lane cache hydration, snapshot attachment, and manifest
  dispatch to Grid.
- Document workflows for developers and CI.

## Current Status (2025-09-26)

- `ploy environment materialize` accepts commit/app/tenant inputs, validates the
  `commit-app` manifest, and supports dry-run plus execution modes.
- Dry-run mode lists required snapshots and cache keys; execution mode captures
  snapshots via the stub registry and records cache hydration with the in-memory
  hydrator.
- Snapshot specs (`commit-db`, `commit-cache`) and the `gpu-ml` lane profile
  underpin the fixtures and updated developer docs.

## Definition of Done

- CLI can materialise a commit environment against stubs and report the
  resources touched.
- Dry-run mode surfaces cache/snapshot availability gaps.
- No lingering services remain after the command exits.

## Tests

- Unit tests for planning logic that determines required caches and snapshots.
- End-to-end stub test verifying materialisation flow and cleanup.
- CLI doc examples verified via automated snippet runner.
- Uphold RED → GREEN → REFACTOR: write failing materialisation tests, add
  minimal CLI wiring, then refactor after dry-run/execution parity passes.
