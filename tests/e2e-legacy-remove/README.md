# Mods End-to-End Scenarios

This directory houses the revived Mods E2E harness. The suite is guarded by the
`e2e` build tag so it only runs when explicitly requested:

```bash
go test -tags e2e ./tests/e2e -v
```

## Current Scenarios

- `simple-openrewrite` — Java 11→17 OpenRewrite sample upgrade. ✅ Runs against
  the control plane through the workstation harness (`go test -tags e2e`).
  credentials are present.
- `buildgate-self-heal` — OpenRewrite plus healing after the build gate fails.
  ✅ Exercises the live path; expect retries and healing branches to run
  against the real infrastructure.
- `parallel-healing-options` — Parallel OpenRewrite + LLM remediation paths.
  ✅ Drives the live client; parallel reconciliation depends on the staged
  roadmap work landing upstream.
- `TestModsScenariosLiveLegacy` — When `PLOY_GRID_ID` and `GRID_BEACON_API_KEY`
  are configured (plus optional `GRID_BEACON_URL`),
  runs the same scenario against the live workflow RPC by shelling out to
  `ploy mod run`.
  Additional scenarios can be toggled via `PLOY_E2E_LIVE_SCENARIOS`.

Each scenario is defined in code (`scenarios.go`) with links back to the legacy
GitLab fixtures. The workstation harness now always drives the live client,
so any missing behaviour must be implemented upstream rather than simulated with
stubs.

## Environment Requirements

Set the following variables before invoking the suite:

- **Codex CLI note:** when asking Codex to execute these tests, wrap the command
  so it sources your environment in the same shell session, for example:

  ```bash
  zsh -lc 'source ~/.zshenv && go test -tags e2e ./tests/e2e -v'
  ```

  Replace the `-run` filter or add inline exports as needed.
- Remember to source `~/.zshenv` (or otherwise export them into your shell) so
  the CLI picks up the expected environment.
  (legacy environment variables have been removed; see docs/envs/README.md)
- `GRID_BEACON_API_KEY` — Beacon service API key used to bootstrap discovery, trust
  material, and workflow credentials.
- `GRID_BEACON_URL` — Optional beacon override (`https://beacon.getgrid.dev`
  by default).
- `PLOY_E2E_TICKET_PREFIX` — Optional prefix for ad-hoc ticket IDs (default `e2e`)
- `PLOY_E2E_REPO_OVERRIDE` — Optional Git repo URL override for scenarios
- `PLOY_E2E_GITLAB_TOKEN` — Optional PAT for GitLab cleanup (branch deletion)
- `PLOY_E2E_LIVE_SCENARIOS` — Comma-separated scenario IDs to execute against
  live control plane (defaults to `simple-openrewrite`).

When mandatory variables are missing or the credentials are invalid, the live
Legacy-backed tests fail fast so misconfiguration is surfaced immediately. If
beacon replies without control-plane metadata and the grid client reports
`gridclient: grid not found`, run `gridctl grid client backfill --grid-id $PLOY_GRID_ID`
to publish the required `manifestHost` and CA bundle before rerunning the suite.

## How This Diff Relates to Legacy Mods E2E

The original shell-based harness and controller HTTP flows (removed in
`da348c89`) assumed the Nomad controller stack. This suite keeps the intent but
targets the new workflow runner:

- Test helpers now expect `ploy mod run` to execute Mods scenarios end-to-end.
- Scenarios reference Mods stage kinds.
- Missing behaviour is enumerated per scenario so implementation owners know
  what to resurrect (stage job specs, build gate healing, etc.).

As integration lands, update `scenario.MissingFeatures` and remove the
explicit failure guards once a scenario reaches GREEN. Remember to re-run
`go test -tags e2e ./tests/e2e` and capture results in `CHANGELOG.md` when a
scenario is restored.
