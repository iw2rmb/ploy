# CLI Surface Refresh

## Why
- The CLI remains the operator’s primary interface for Mods execution, artifact management, and cluster administration (see `docs/v2/README.md` and `docs/v2/cli.md`).
- Removing Grid compatibility requires updated commands, help output, and workflows aligned with the new control plane and IPFS integrations.

## Required Changes
- Audit existing CLI commands, removing Grid-specific flags and flows while introducing v2 cluster bootstrap, node lifecycle, and artifact commands.
- Update command help, validation, and examples to reference Ploy nodes, SHIFT gating, and IPFS-based artifacts only.
- Implement SSE log streaming clients for job tails backed by the new job execution APIs.
- Ensure CLI configuration management covers beacon discovery, trust bundles, and credential references created in other tasks.

## Definition of Done
- CLI help tree documents all v2 functionality with no stale Grid references.
- Bootstrap, Mods submission, artifact operations, and observability commands exercise the new APIs successfully in local smoke tests.
- CLI UX is validated through operator-focused walkthrough docs stored alongside command reference updates.

## Tests
- Unit tests for CLI command validation, config loading, and error messaging.
- Integration tests that run `make build` binaries against local control-plane mocks, covering end-to-end Mods submissions.
- Snapshot/Golden tests for `ploy help` output to guard against regressions in command documentation.
