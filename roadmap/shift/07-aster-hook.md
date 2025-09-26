# Aster Hook Integration
- [x] Done (2025-09-26)

## Why / What For
Wire Aster’s AST-pruned bundles into Ploy’s workflow so Grid can route to accelerator runtimes and cache appropriately.

## Required Changes
- Detect and ingest Aster bundle metadata per workflow step.
- Extend cache key composer and job submission payloads with Aster toggles.
- Provide CLI controls to enable/disable Aster per step and surface bundle provenance.

Status: A filesystem-backed Aster locator now reads bundle metadata under `configs/aster/` and the workflow runner attaches that provenance to every stage submission. Stage cache keys continue to incorporate manifest + toggle data, and per-stage toggles can be enabled or disabled through the new `--aster`/`--aster-step` flags exposed on `ploy workflow run`. Successful runs print a summary of attached bundles so operators can confirm toggles before Grid wiring lands.

## Definition of Done
- Workflow runner attaches Aster metadata to every relevant job submission.
- Cache keys differentiate between Aster-on and Aster-off runs.
- Documentation covers enabling/disabling Aster and interpreting cache hits.

## Tests
- Unit tests for bundle detection and metadata propagation.
- Workflow stub tests verifying Grid receives Aster hints and supports per-stage disablement.
- CLI tests covering the new Aster flags and locator wiring, plus regression coverage for disabling stages.
