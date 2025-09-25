# Mods Integration Tests Refactor

## Purpose
Establish a repeatable way to execute Mods integration tests that currently depend on builder jobs, SeaweedFS, and controller endpoints. Today `go test ./internal/mods` fails outside of CI because those dependencies are hard-coded and unreachable from the VPS shell. This design describes the target workflow for running the suite inside a provisioned harness and mapping the required implementation tasks.

## Goals
- Decouple Mods integration tests from hard-coded DNS names and direct network calls so they can run in a controlled harness.
- Provide a Nomad/VPS job that exercises the refactored tests against real services when desired.
- Preserve lightweight unit coverage by allowing fakes/stubs to replace external clients locally.

## Non-Goals
- Rewriting Mods business logic beyond dependency injection hooks.
- Standing up new permanent infrastructure; the harness should reuse existing SeaweedFS, controller, and builder deployments.

## Approach
1. Introduce interfaces around SeaweedFS uploads, builder submissions, and Git pushes so tests can swap real clients for fakes.
2. Source service endpoints and credentials from environment variables and surface a canonical harness (CLI or Nomad job) that sets them.
3. Split fast unit suites from heavy integration suites via Go build tags and shared helpers.
4. Package fixtures and scripts so the VPS harness primes required repositories and buckets before running tests.
5. Update documentation (AGENTS, testing guides) to reflect the new workflow, keeping design and tasks in sync.

## Task Tracker
- [01-dependency-seams](../../../roadmap/mods-integration-tests/01-dependency-seams.md)
- [02-configurable-harness](../../../roadmap/mods-integration-tests/02-configurable-harness.md)
- [03-nomad-runner](../../../roadmap/mods-integration-tests/03-nomad-runner.md)
- [04-fixture-and-ci-alignment](../../../roadmap/mods-integration-tests/04-fixture-and-ci-alignment.md)

## Updates
- Added fixture seeding helper (`scripts/mods-seed-fixtures.sh`) and CI job (`mods-integration-harness`) to preload SeaweedFS artifacts and gate merges via the Nomad harness.
- Added VPS Nomad runner (`tests/nomad-jobs/mods-integration.nomad.hcl`) plus helper script (`scripts/run-mods-integration-vps.sh`) and Makefile entry (`mods-integration-vps`) to execute the integration suite via Nomad.
- Added harness configuration loader (`HarnessConfig`) to centralize controller and SeaweedFS endpoints with env overrides.
- Introduced artifact/builder/git dependency seams in `ModRunner`, plus memory-based storage and noop uploaders so workstation tests avoid SeaweedFS/NATS.

Maintain this design as tasks progress. Link additional tasks or notes here, and ensure status checkboxes in the roadmap stay accurate.
