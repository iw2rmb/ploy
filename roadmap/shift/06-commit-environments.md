# Commit-Scoped Environments
- [ ] Pending

## Why / What For
Let developers materialise `<sha>-<app>` environments on demand with deterministic caches, snapshots, and manifests.

## Required Changes
- Implement `ploy environment materialize` command with dry-run and execution modes.
- Stitch together lane cache hydration, snapshot attachment, and manifest dispatch to Grid.
- Document workflows for developers and CI.

## Definition of Done
- CLI can materialise a commit environment against stubs and report the resources touched.
- Dry-run mode surfaces cache/snapshot availability gaps.
- No lingering services remain after the command exits.

## Tests
- Unit tests for planning logic that determines required caches and snapshots.
- End-to-end stub test verifying materialisation flow and cleanup.
- CLI doc examples verified via automated snippet runner.
