# Workflow Runner CLI
- [ ] Pending

## Why / What For
Deliver `ploy workflow run` as the radical replacement for the legacy controller, executing one ticket end-to-end and exiting.

## Required Changes
- Create CLI command with JetStream consumer loop, DAG reconstruction, and Grid Workflow RPC client.
- Replace Nomad submission code paths with Grid RPC calls and artifact publishing hooks.
- Add temp workspace lifecycle manager that scrubs all local state on exit.

## Definition of Done
- CLI handles happy path for mods/build/test ticket using JetStream + Grid stubs.
- Legacy API binaries and Nomad wrappers are removed from build outputs.
- Usage docs describe invocation flags and environment requirements.

## Tests
- Unit tests for DAG reconstruction and retry logic.
- CLI integration test using fake JetStream/Grid servers to confirm run-to-exit flow.
- Coverage check ensures ≥90% for workflow runner package.
