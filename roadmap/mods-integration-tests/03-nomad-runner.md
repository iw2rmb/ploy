# 03 Nomad Runner

- [ ] Status: Pending

## Why / What For
Creating a dedicated Nomad job allows the full Mods integration suite to run inside the same network as SeaweedFS, builder jobs, and the controller. This delivers a reproducible VPS workflow instead of ad-hoc SSH runs.

## Required Changes
- Author a Nomad job (e.g., `tests/mods-integration.nomad.hcl`) that sets the required environment variables, volumes, and credentials.
- Include steps to fetch the repo at the requested revision and execute `go test ./internal/mods -tags=integration` (or equivalent).
- Provide helper scripts/CLI entrypoints (`make mods-integration-vps`) to submit the job and stream results.

## Definition of Done
- Submitting the Nomad job on the dev cluster provisions the test container, runs the suite, and exits with the correct status code.
- Logs and artifacts are retrievable for debugging (either via Nomad log wrappers or stored files).
- Documentation explains how to trigger the job and interpret results.

## Tests
- Dry-run the Nomad job (`nomad-job-manager.sh plan …`) to ensure spec validity.
- Execute on the dev cluster and confirm at least one integration scenario passes end-to-end using real services.

## References
- [Design doc](../../../docs/design/mods-integration-tests/README.md)
- Depends on: [01-dependency-seams](01-dependency-seams.md), [02-configurable-harness](02-configurable-harness.md)
