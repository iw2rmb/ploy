# Mods End-to-End Scenarios (Grid)

This directory houses the revived Mods E2E harness. The suite is guarded by the
`e2e` build tag so it only runs when explicitly requested:

```bash
go test -tags e2e ./tests/e2e -v
```

## Current Scenarios

- `simple-openrewrite` — Java 11→17 OpenRewrite sample upgrade. ✅ Passes on
  workstation harness (`go test -tags e2e`) using the Grid stub and Mods lanes.
- `buildgate-self-heal` — OpenRewrite plus healing after the build gate fails.
  ✅ Healing branch validated via in-memory Grid stub; real Grid smoke deferred
  to SHIFT lanes.
- `parallel-healing-options` — Parallel OpenRewrite + LLM remediation paths.
  ✅ Planner metadata reflects parallel healing; SHIFT follow-up covers real Grid
  reconciliation.

Each scenario is defined in code (`scenarios.go`) with links back to the legacy
GitLab fixtures. The workstation harness now runs green against the in-memory
Grid stub, while remaining integration gaps for SHIFT are tracked per scenario.

## Environment Requirements

Set the following variables before invoking the suite:

- `GRID_ENDPOINT` — Grid Workflow RPC endpoint (e.g. `https://grid.dev`)
- `PLOY_E2E_TENANT` — Tenant slug to claim tickets under
- `PLOY_E2E_TICKET_PREFIX` — Optional prefix for ad-hoc ticket IDs (default `e2e`)
- `PLOY_E2E_REPO_OVERRIDE` — Optional Git repo URL override for scenarios
- `PLOY_E2E_GITLAB_TOKEN` — Optional PAT for GitLab cleanup (branch deletion)

When any mandatory variable is missing, the tests skip with a helpful message.

## How This Diff Relates to Legacy Mods E2E

The original shell-based harness and controller HTTP flows (removed in
`da348c89`) assumed the Nomad controller stack. This suite keeps the intent but
targets the new Grid workflow runner:

- Test helpers now expect `ploy mod run` to execute Mods scenarios end-to-end.
- Scenarios reference Grid lanes and Mods stage kinds.
- Missing behaviour is enumerated per scenario so implementation owners know
  what to resurrect (stage job specs, build gate healing, etc.).

As Grid integration lands, update `scenario.MissingFeatures` and remove the
explicit failure guards once a scenario reaches GREEN. Remember to re-run
`go test -tags e2e ./tests/e2e` and capture results in `CHANGELOG.md` when a
scenario is restored.
