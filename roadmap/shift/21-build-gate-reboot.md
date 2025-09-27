# Build Gate Reboot
- [ ] In Progress (2025-09-27)

## Why / What For
Reintroduce the workstation-friendly build gate with Grid integration, static analysis, and log parsing so every workflow validates build quality before tests run.

## Required Changes
- Track sub-tasks under `roadmap/build-gate/` covering stage planning, sandbox execution, static check adapters, and log retrieval.
- Keep the build gate design document (`docs/design/build-gate/README.md`) synchronised with implementation milestones and metadata schema updates.
- Update CLI/docs to reflect new stage names, flags, and checkpoint metadata as milestones ship.

## Definition of Done
- All build gate tasks through log retrieval and Knowledge Base wiring are complete.
- Workflow checkpoints expose structured build gate metadata consumed by downstream tooling.
- CLI and docs document build gate behaviour and configuration, with tests meeting coverage targets.

Status: Stage planning, metadata sanitisation, and contract wiring landed on 2025-09-27 (see `roadmap/build-gate/01-stage-planning-and-metadata.md`). Sandbox runner shipped on 2025-10-05 (see `roadmap/build-gate/02-sandbox-runner.md`), while the adapter registry and log retrieval tasks remain pending.

## Tests
- Refer to individual tasks under `roadmap/build-gate/` for milestone-specific tests.
- Repository-wide `go test -cover ./...` enforces coverage expectations after each milestone.
