# Changelog

## [2025-09-25] Legacy Teardown
- Removed all legacy API, Nomad, Consul, SeaweedFS, and deployment scaffolding.
- Replaced the repo with a CLI-only stub (`ploy workflow run`) that validates ticket input and returns `ErrNotImplemented`.
- Added guardrail tests that fail if legacy binaries or imports reappear.
- Simplified the build system (`Makefile`) to focus on the workflow CLI.
- Rewrote documentation (`README.md`, `docs/DOCS.md`, `cmd/ploy/README.md`) to describe the Shift architecture and roadmap alignment.

## [History]
Prior releases documented Nomad-based services, security engines, and lane orchestration. Refer to the Git history before `2025-09-25` for archival details.
