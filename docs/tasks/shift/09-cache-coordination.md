# Checkpoint Cache Coordination

- [x] Done (2025-09-26)

## Why / What For

Expose deterministic lane cache keys in workflow checkpoints so Grid can make
cache reuse decisions without introspecting stage payloads.

## Required Changes

- Extend the event contract to carry `cache_key` values on every workflow
  checkpoint and bump the schema version.
- Teach the workflow runner to compose cache keys per stage and attach them to
  checkpoint publications.
- Wire the CLI to derive cache keys from lane specs so workstation runs match
  expected Grid behaviour.

## Definition of Done

- Checkpoints published by the runner include a non-empty cache key for every
  stage transition.
- Event-contract documentation reflects the new schema and schema version.
- CLI wiring loads lane specs and computes cache keys before dispatching stages
  to the Grid stub.

## Tests

- Runner unit tests assert checkpoints include cache keys for mods/build/test
  stages.
- CLI tests ensure the cache composer is injected when invoking `workflow run`.
- Repository-wide `go test -cover ./...` remains ≥60% overall with runner
  coverage above 90%.
