# Workflow Runner CLI

- [x] Done (2025-09-26)

## Why / What For

Deliver `ploy mod run` (formerly `ploy workflow run`) as the radical replacement
for the legacy controller, executing one ticket end-to-end and exiting.

## Required Changes

- Create CLI command with JetStream consumer loop, DAG reconstruction, and Grid
  Workflow RPC client.
- Replace Nomad submission code paths with Grid RPC calls and artifact
  publishing hooks.
- Add temp workspace lifecycle manager that scrubs all local state on exit.

## Current Status (2025-09-26)

- CLI rebuilds the default mods→build→test DAG via the runner planner.
- Checkpoints stream through the JetStream stub while stages dispatch through
  the in-memory Grid client.
- Temporary workspaces are scrubbed on exit, keeping the workflow runner
  stateless.

## Definition of Done

- CLI handles happy path (including auto ticket claim, retries, and failure
  surfacing) for mods/build/test using JetStream + Grid stubs.
- Legacy API binaries and Nomad wrappers remain absent from build outputs.
- Usage docs describe invocation flags, environment placeholders, and stub
  behaviour.

## Tests

- Unit coverage for DAG planning, retries, workspace lifecycle, and stub
  behaviour (runner package at 94.5%).
- CLI integration tests for auto ticket handling, flag validation, and
  usage/printing helpers.
- `go test -cover ./...` enforced with ≥60% repo and ≥90% workflow runner
  coverage.
- Maintain RED → GREEN → REFACTOR cadence: land failing DAG planner tests, add
  minimal CLI wiring, then refactor once coverage targets hold.
