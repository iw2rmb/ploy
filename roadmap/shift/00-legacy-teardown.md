# Legacy Teardown
- [x] Done

## Why / What For
Erase the Nomad-era controller footprint so the Ploy CLI can stand alone and depend solely on Grid contracts.

## Required Changes
- Delete controller binaries (`cmd/ployd`, `cmd/ployman api`, related HTTP routers, and background workers).
- Rip out orchestration packages (`internal/orchestration`, Nomad templates, Consul helpers) and the Ansible/ployman deployment scripts tied to them.
- Remove SeaweedFS artifact code paths, Nomad-specific lane metadata, and build-gate service integrations.
- Drop outdated docs, runbooks, and onboarding steps that reference the legacy API or Nomad workflows.

## Definition of Done
- Repository builds without any Nomad/Consul/Traefik/SeaweedFS references.
- CLI `ploy workflow run` (stub) is the only supported entrypoint, and CI fails if legacy binaries reappear.
- Documentation reflects the CLI + Grid architecture exclusively.

## Tests
- Static analysis/linters confirm no packages import removed dependencies.
- Build/test pipeline executes with CLI-only entrypoints.
- Markdown link check validates updated documentation set.
