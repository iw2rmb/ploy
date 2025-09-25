# Changelog

## [2025-09-25] Event Contract Stub
- Added `internal/workflow/contracts` with schema version `2025-09-25`, subject helpers, and validation logic for workflow tickets and checkpoints.
- Wired `internal/workflow/runner` to claim tickets, validate payloads, and publish an initial `claimed` checkpoint through a JetStream stub.
- Updated the CLI to require `--tenant`, bootstrap the in-memory bus, and reflect the new behaviour in usage docs.
- Documented the subject map and example payloads in `docs/design/shift/event-contracts.md`; roadmap slice `01-event-contracts` now marked complete.

## [2025-09-25] Legacy Teardown
- Removed all legacy API, Nomad, Consul, SeaweedFS, and deployment scaffolding.
- Replaced the repo with a CLI-only stub (`ploy workflow run`) that validates ticket input and returns `ErrNotImplemented`.
- Added guardrail tests that fail if legacy binaries or imports reappear.
- Simplified the build system (`Makefile`) to focus on the workflow CLI.
- Rewrote documentation (`README.md`, `docs/DOCS.md`, `cmd/ploy/README.md`) to describe the Shift architecture and roadmap alignment.

## [History]
Prior releases documented Nomad-based services, security engines, and lane orchestration. Refer to the Git history before `2025-09-25` for archival details.
