# 03 Nomad Runner

- [x] Completed (pre-2025-09; SSH runner replacement)

## Why / What For

The initial plan called for a dedicated Nomad job so Mods integration tests
could reach SeaweedFS, builder jobs, and the controller from inside the cluster
network. After prototyping, we replaced the batch job with an SSH-driven runner
that reuses the existing `/home/ploy/ploy` checkout, keeping the workflow simple
while still exercising real services on the VPS.

## Required Changes

- Provide a helper (`scripts/run-mods-integration-vps.sh`) that SSHes to the
  VPS, ensures the desired commit is fetched, and executes
  `go test ./internal/mods -tags=integration` as the `ploy` user.
- Keep the Makefile entry (`mods-integration-vps`) so workstation developers can
  trigger the suite with a single command once their branch is pushed.
- Document the expectation that fixes require a redeploy (or manual `git pull`)
  so `/home/ploy/ploy` stays in sync with the tested commit.

## Definition of Done

- Running `make mods-integration-vps` fetches the current worktree commit on
  `TARGET_HOST` and executes the integration suite without additional manual
  steps.
- Failures surface directly in the CLI output, with logs available in `go test`
  output on the VPS.
- Documentation calls out the SSH workflow and the requirement to push changes
  before invoking the runner.

## Current Status

- SSH-based runner script replaces the original Nomad job, reusing
  `/home/ploy/ploy`.
- Makefile target `mods-integration-vps` remains the entry point for workstation
  developers.
- Documentation records the push-before-run expectation for VPS usage.

## Implementation Notes

- `scripts/run-mods-integration-vps.sh` now orchestrates the SSH workflow,
  performs `git fetch`/`git checkout <commit>`, and runs
  `go test -tags=integration ./internal/mods` under `ploy`.
- The earlier Nomad job (`tests/nomad-jobs/mods-integration.nomad.hcl`) was
  removed to avoid double-maintaining harness logic and to rely on the existing
  `/home/ploy/ploy` checkout.
- The Makefile target remains so CLI usage is unchanged
  (`make mods-integration-vps`).

## Tests

- `go test ./internal/mods -run TestModsFixtureScriptsAndHarness` verifies
  helper scripts.
- Manual run: `TARGET_HOST=<vps> make mods-integration-vps` (commit must be
  pushed to the remote referenced by the VPS repo).
- Continue RED → GREEN → REFACTOR: keep failing harness tests until SSH workflow
  lands, add minimal script changes, then refactor after manual verification.

## References

- [Design doc](../../../docs/design/mods-integration-tests/README.md)
- Depends on: [01-dependency-seams](01-dependency-seams.md),
  [02-configurable-harness](02-configurable-harness.md)
