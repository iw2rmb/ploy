# Commit-Scoped Environments
- [x] Done (2025-09-26)

## Why / What For
Let developers materialise `<sha>-<app>` environments on demand with deterministic caches, snapshots, and manifests.

## Required Changes
- Implement `ploy environment materialize` command with dry-run and execution modes.
- Stitch together lane cache hydration, snapshot attachment, and manifest dispatch to Grid.
- Document workflows for developers and CI.

Status: `ploy environment materialize` now accepts commit/app/tenant inputs, loads/validates the `commit-app` manifest, and either dry-runs or executes hydration. Dry-run mode lists required snapshots and cache keys without side effects; execution mode captures snapshots via the stub registry and records cache hydration in the in-memory hydrator. Snapshot specs (`commit-db`, `commit-cache`) and the `gpu-ml` lane profile back the manifest fixtures, and docs were updated to guide developers.

## Definition of Done
- CLI can materialise a commit environment against stubs and report the resources touched.
- Dry-run mode surfaces cache/snapshot availability gaps.
- No lingering services remain after the command exits.

## Tests
- Unit tests for planning logic that determines required caches and snapshots.
- End-to-end stub test verifying materialisation flow and cleanup.
- CLI doc examples verified via automated snippet runner.
