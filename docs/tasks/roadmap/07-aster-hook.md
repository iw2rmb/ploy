# Aster Hook Integration

- [x] Done (2025-09-26)

## Why / What For

Wire Aster’s AST-pruned bundles into Ploy’s workflow so Grid can route to
accelerator runtimes and cache appropriately.

## Required Changes

- Detect and ingest Aster bundle metadata per workflow step.
- Extend cache key composer and job submission payloads with Aster toggles.
- Provide CLI controls to enable/disable Aster per step and surface bundle
  provenance.

## Current Status (2025-09-26)

- Filesystem-backed Aster locator under `configs/aster/` attaches bundle
  metadata to every stage submission.
- Cache keys incorporate manifest and Aster toggle data, differentiating
  Aster-on versus Aster-off runs.
- CLI exposes `--aster` and `--aster-step` flags and prints bundle summaries so
  operators can confirm toggles pre-Grid wiring.

## Definition of Done

- Workflow runner attaches Aster metadata to every relevant job submission.
- Cache keys differentiate between Aster-on and Aster-off runs.
- Documentation covers enabling/disabling Aster and interpreting cache hits.

## Tests

- Unit tests for bundle detection and metadata propagation.
- Workflow stub tests verifying Grid receives Aster hints and supports per-stage
  disablement.
- CLI tests covering the new Aster flags and locator wiring, plus regression
  coverage for disabling stages.
- Keep RED → GREEN → REFACTOR in focus: add failing bundle-detection tests, wire
  minimal metadata plumbing, then refactor once CLI and stub coverage succeed.
