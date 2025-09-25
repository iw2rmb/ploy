# Aster Hook Integration
- [ ] Pending

## Why / What For
Wire Aster’s AST-pruned bundles into Ploy’s workflow so Grid can route to accelerator runtimes and cache appropriately.

## Required Changes
- Detect and ingest Aster bundle metadata per workflow step.
- Extend cache key composer and job submission payloads with Aster toggles.
- Provide CLI controls to enable/disable Aster per step and surface bundle provenance.

## Definition of Done
- Workflow runner attaches Aster metadata to every relevant job submission.
- Cache keys differentiate between Aster-on and Aster-off runs.
- Documentation covers enabling/disabling Aster and interpreting cache hits.

## Tests
- Unit tests for bundle detection and metadata propagation.
- Workflow stub tests verifying Grid receives Aster hints and selects correct runtime.
- Regression tests to ensure disabling Aster falls back to baseline cache keys without reintroducing Nomad paths.
