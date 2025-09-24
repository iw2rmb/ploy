# 04 Fixture & CI Alignment

- [ ] Status: Pending

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

## Tests
- Run fixture scripts on the dev VPS and confirm tests consume the seeded data successfully.
- Validate CI run(s) that exercise the new job complete and publish logs/artifacts.

## References
- [Design doc](../../../docs/design/mods-integration-tests/README.md)
- Depends on: [03-nomad-runner](03-nomad-runner.md)
