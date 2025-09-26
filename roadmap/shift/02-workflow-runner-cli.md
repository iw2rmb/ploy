# Workflow Runner CLI
- [x] Done (2025-09-26)

## Why / What For
Deliver `ploy workflow run` as the radical replacement for the legacy controller, executing one ticket end-to-end and exiting.

## Required Changes
- Create CLI command with JetStream consumer loop, DAG reconstruction, and Grid Workflow RPC client.
- Replace Nomad submission code paths with Grid RPC calls and artifact publishing hooks.
- Add temp workspace lifecycle manager that scrubs all local state on exit.

Status: The CLI now rebuilds the default mods→build→test DAG via the runner planner, streams checkpoints through the JetStream stub, dispatches stages through the in-memory Grid client, and removes the temporary workspace on exit.

## Definition of Done
- CLI handles happy path (including auto ticket claim, retries, and failure surfacing) for mods/build/test using JetStream + Grid stubs.
- Legacy API binaries and Nomad wrappers remain absent from build outputs.
- Usage docs describe invocation flags, environment placeholders, and stub behaviour.

## Tests
- Extensive unit coverage for DAG planning, stage retries, error paths, workspace lifecycle, and stub behaviour (runner package at 94.5% coverage).
- CLI integration tests cover auto ticket handling, flag validation, and usage/printing helpers.
- `go test -cover ./...` enforced with ≥60% repo and ≥90% workflow runner coverage.
