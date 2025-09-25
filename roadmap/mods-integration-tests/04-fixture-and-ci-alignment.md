# 04 Fixture & CI Alignment

- [x] Status: Done

## Why / What For
Fixtures (SeaweedFS artifacts, Git repos) and CI pipelines must align with the new harness. Without curated data and automation, test runs will continue to fail due to missing repositories or credentials.

## Required Changes
- Seed required artifacts (e.g., `mod-99` tarballs, sample GitLab repos) via scripts runnable before each harness invocation.
- Update CI workflows to trigger the Nomad job or equivalent harness, collecting test reports.
- Refresh documentation (AGENTS, docs/TESTING.md) to reference the new process and fixtures.

## Definition of Done
- Scripts exist to hydrate SeaweedFS buckets and Git repos used by integration tests.
- CI pipelines execute the harness in at least one stage and gate merges on its outcome.
- Documentation clearly states how to prepare fixtures locally and on the VPS.

## Implementation Notes
- Added SeaweedFS seeding script (`scripts/mods-seed-fixtures.sh`) that uploads fixture payloads from `tests/mods-fixtures` and validates Git remotes.
- Curated fixture inputs in `tests/mods-fixtures/` for mod-99, mod-gitlab-test, and self-healing scenarios.
- Wired GitHub Actions job `mods-integration-harness` to seed fixtures and invoke `make mods-integration-vps` gated on VPS secrets.

## Tests
- `./scripts/mods-seed-fixtures.sh` (locally or via CI) populates SeaweedFS and exits successfully.
- `make mods-integration-vps` (workstation or CI with secrets) completes using the seeded fixtures.

## References
- [Design doc](../../../docs/design/mods-integration-tests/README.md)
- Depends on: [03-nomad-runner](03-nomad-runner.md)
