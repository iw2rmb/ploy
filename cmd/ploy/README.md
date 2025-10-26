# Ploy Workflow CLI

`ploy` is now a single-purpose CLI that claims workflow tickets from Grid,
reconstructs the default mods→build→test DAG, and dispatches stages to the
in-memory Grid stub. Legacy subcommands (apps, env, mods, security, etc.) were
removed during the workstation legacy teardown.

## Usage

```bash
ploy lanes describe --lane <lane-name> \
  [--commit <sha>] [--snapshot <fingerprint>] [--manifest <version>] \
  [--aster <toggle,...>]
ploy mod run --tenant <tenant> \
  [--ticket <ticket-id>|--ticket auto] \
  [--repo-url <url> --repo-base-ref <branch> --repo-target-ref <branch> \
   --repo-workspace-hint <dir>] \
  [--mods-plan-timeout <duration>] [--mods-max-parallel <n>] \
  [--aster <toggle,...>] \
  [--aster-step <stage=toggle,...|stage=off>]
ploy workflow cancel --tenant <tenant> --run-id <run-id> \
  [--workflow <workflow-id>] [--reason <text>]
ploy snapshot plan --snapshot <snapshot-name>
ploy snapshot capture --snapshot <snapshot-name> --tenant <tenant> \
  --ticket <ticket-id>
ploy environment materialize <commit-sha> --app <app> --tenant <tenant> \
  [--dry-run] [--manifest <name@version>] [--aster <toggle,...>]
ploy knowledge-base ingest --from <fixture.json>
ploy knowledge-base evaluate --fixture <samples.json>
ploy upload --job-id <ticket-id> [--kind repo|logs|report] <path>
ploy report --job-id <ticket-id> [--artifact-id <slot>] --output <path>
```

`lanes describe` inspects the bundled TOML lane specs under `configs/lanes`,
displays the runtime family, build/test commands, surfaced job defaults (image,
command, env, resources), and shows a deterministic cache-key preview that incorporates
commit/snapshot/manifest/Aster toggles. Aster inputs are only included when
`PLOY_ASTER_ENABLE` is set so the unfinished bundle integration can stay hidden
behind a feature flag. The preview mirrors what the workflow runner supplies to
Grid when dispatching stages.

`mod run` claims a ticket (auto-generating one if `--ticket auto`),
materialises the repository passed via `--repo-*` flags (when provided),
compiles the referenced integration manifest from `configs/manifests/`,
publishes checkpoints for every stage transition (including lane cache keys),
executes mods/build/test against a temporary workspace, and cleans up before
exit. Mods planner hints (`--mods-plan-timeout`, `--mods-max-parallel`)
flow into stage metadata so Grid can respect concurrency/timebox controls. When
build-gate fails with a retryable outcome the runner collects the failure
metadata, re-plans a healing branch using the Mods planner, and appends `#healN`
stages before continuing to static checks and tests. Provide
`PLOY_ASTER_ENABLE` is set the CLI resolves Aster bundle provenance after a
successful run so developers can confirm which toggles/bundles were attached to
each stage. Explicit ticket IDs remain a stub-only workflow until Grid
integration lands.

`workflow cancel` requests cancellation of a Workflow RPC run. The subcommand
requires legacy Grid credentials when targeting that backend; in-memory stubs
respond with a friendly reminder instead of attempting an unsupported
cancellation.
Ploy records the cancellation reason (when supplied) and echoes the run status
so operators can quickly confirm whether the request was accepted or the run
was already terminal.

`snapshot plan` inspects TOML specs under `configs/snapshots/`, counting
strip/mask/synthetic rules and surfacing per-table highlights before a capture
runs.

`snapshot capture` loads the fixture referenced in the spec, applies
strip/mask/synthetic rules, produces a deterministic fingerprint, uploads the
payload to the IPFS gateway discovered from Grid (falling back to the
deterministic in-memory publisher when discovery omits one), publishes metadata
to the current stub, and prints the returned CID.

`environment materialize` evaluates the integration manifest for a given
app/commit pair, validates required snapshots, optionally captures them
(execution mode), composes deterministic cache keys for each required lane, and
hydrates those caches through an in-memory hydrator. Dry-run mode avoids
snapshot capture/hydration and surfaces any gaps before Grid integration lands.

`knowledge-base ingest` merges incident fixtures into the workstation catalog
under `configs/knowledge-base/catalog.json`, enforcing duplicate safeguards and
preserving schema version ordering. `knowledge-base evaluate` loads curated
samples, runs them through the advisor with a conservative score floor, and
prints per-sample match results plus aggregate accuracy so operators can gauge
classifier drift without leaving the workstation.

`upload` and `report` reuse the cached SSH descriptor to move large payloads through the control-plane
slot APIs instead of issuing ad-hoc SCP sessions. See
[docs/next/ssh-transfer-migration.md](../../docs/next/ssh-transfer-migration.md) for rollout guidance,
required environment variables, and operational limits (slot TTL, digest verification, cleanup).

## Flags

- `--lane` — Lane identifier defined under `configs/lanes` (used by
  `lanes describe`).
- `--commit` / `--snapshot` / `--manifest` / `--aster` — Optional cache-key
  preview inputs consumed by the lane engine.
- `--tenant` — Tenant slug used to resolve subject namespaces. Required for
  `mod run`, `workflow cancel`, `snapshot capture`, and execution-mode
  `environment materialize`.
- `--ticket` — JetStream ticket identifier to claim (`mod run`) or metadata
  tag for snapshot captures. Defaults to `auto` for workflows; required for
  snapshot captures.
- `--snapshot` — Snapshot identifier defined under `configs/snapshots/*.toml`
  (required for `snapshot plan` and `snapshot capture`).
- `--app` — Application identifier resolved to an integration manifest (required
  for `environment materialize`).
- `--dry-run` — Skip snapshot capture and cache hydration while still reporting
  required resources (`environment materialize`).
- `--manifest` — Override manifest name/version in `<name>@<version>` form
  (`environment materialize`).
- `--aster` — Optional toggles to append to manifest-required Aster switches
  (`lanes describe`, `mod run`, `environment materialize`). The flag is
  ignored unless `PLOY_ASTER_ENABLE` is set.
- `--aster-step` — Stage-specific overrides for Aster behaviour when running
  workflows (`mod run`). Use `stage=toggle1,toggle2` to enable additional
  toggles or `stage=off` to disable Aster for that stage. Overrides are ignored
  unless `PLOY_ASTER_ENABLE` is set.
- `--repo-url` / `--repo-base-ref` / `--repo-target-ref` / `--repo-workspace-hint`
  — Repository materialisation inputs consumed by `mod run`. When `--repo-url` is provided, `--repo-target-ref` is
  required; `--repo-base-ref` defaults to the repository's default branch. The
  workspace hint creates an auxiliary directory (e.g. `mods/java`) before Mods
  stages execute.
- `--mods-plan-timeout` — Duration string passed to the Mods planner so Grid can
  timebox plan evaluation (`mod run`).
- `--mods-max-parallel` — Upper bound on concurrent Mods stages emitted by the
  planner (`mod run`).

## Exit Codes

- `0` — success (ticket claimed, stages completed, workspace cleaned).
- `1` — error (missing flags, unsupported subcommand, stage failure, or
  downstream error).

## Environment

- `PLOY_GRID_ID` — Optional legacy Grid identifier. Provide only when the CLI
  must talk to a Grid deployment instead of a local control plane.
- `GRID_BEACON_API_KEY` / `GRID_BEACON_URL` — Legacy Grid credentials. Omit
  when using SSH descriptors; include them only for legacy Grid workflows.
- `GRID_CLIENT_STATE_DIR` — Optional override for the grid client state
  directory. Defaults to `${XDG_CONFIG_HOME:-$HOME/.config}/ploy/grid/<grid-id>`.
- `GRID_WORKFLOW_SDK_STATE_DIR` — Backwards compatible override; when set it
  also dictates the grid client state directory.
- `PLOY_RUNTIME_ADAPTER` — Optional runtime adapter selector. Defaults to
  `local-step`; other adapters (`grid`, `k8s`, `nomad`) register here and
  unknown names cause the CLI to fail fast.
- `PLOY_ASTER_ENABLE` — Opt-in switch for the experimental Aster integration.
  When unset the CLI skips bundle lookups and omits Aster toggles from cache
  keys, manifests, and summaries. Without `PLOY_GRID_ID` the CLI falls back to the
  in-memory Grid and JetStream stubs for offline development.

## Development

- Build via `make build` (outputs to `dist/ploy`).
- Run unit tests with `make test` (ensures `go test -cover ./...` stays ≥60%
  overall, ≥90% on the runner package).
- Roadmap slices should extend `internal/workflow/runner` and keep the CLI
  focused on stateless execution against JetStream/Grid contracts.
- See `docs/MANIFESTS.md` for schema details and authoring guidance on
  integration manifests.
- Review `docs/DOCS.md` for the documentation matrix and editing conventions
  that keep the CLI guides aligned.
