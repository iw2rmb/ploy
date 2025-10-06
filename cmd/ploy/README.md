# Ploy Workflow CLI

`ploy` is now a single-purpose CLI that claims workflow tickets from Grid,
reconstructs the default mods→build→test DAG, and dispatches stages to the
in-memory Grid stub. Legacy subcommands (apps, env, mods, security, etc.) were
removed during the SHIFT legacy teardown.

## Usage

```bash
ploy lanes describe --lane <lane-name> \
  [--commit <sha>] [--snapshot <fingerprint>] [--manifest <version>] \
  [--aster <toggle,...>]
ploy workflow run --tenant <tenant> \
  [--ticket <ticket-id>|--ticket auto] [--mods-plan-timeout <duration>] \
  [--mods-max-parallel <n>] [--aster <toggle,...>] \
  [--aster-step <stage=toggle,...|stage=off>]
ploy mod run --tenant <tenant> \
  [--ticket <ticket-id>|--ticket auto] \
  [--repo-url <url> --repo-base-ref <branch> --repo-target-ref <branch> \
   --repo-workspace-hint <dir>] \
  [--mods-plan-timeout <duration>] [--mods-max-parallel <n>]
ploy snapshot plan --snapshot <snapshot-name>
ploy snapshot capture --snapshot <snapshot-name> --tenant <tenant> \
  --ticket <ticket-id>
ploy environment materialize <commit-sha> --app <app> --tenant <tenant> \
  [--dry-run] [--manifest <name@version>] [--aster <toggle,...>]
ploy knowledge-base ingest --from <fixture.json>
ploy knowledge-base evaluate --fixture <samples.json>
```

`lanes describe` inspects TOML lane specs under `lanes/`, displays the
runtime family, build/test commands, surfaced job defaults (image, command, env,
resources), and shows a deterministic cache-key preview that incorporates
commit/snapshot/manifest/Aster toggles. Aster inputs are only included when
`PLOY_ASTER_ENABLE` is set so the unfinished bundle integration can stay hidden
behind a feature flag. The preview mirrors what the workflow runner supplies to
Grid when dispatching stages.

`mod run` claims a ticket (auto-generating one if `--ticket auto`),
materialises the repository passed via `--repo-*` flags (when provided),
compiles the referenced integration manifest from `configs/manifests/`,
publishes checkpoints for every stage transition (including lane cache keys),
executes mods/build/test against a temporary workspace, and cleans up before
exit. When `GRID_ENDPOINT` targets a Grid cluster running v2025.11.0 or newer
the CLI queries `/v1/cluster/info` to discover the API endpoint, JetStream
route list, IPFS gateway, feature map, and Grid version before connecting;
Grid-less runs fall back to the in-memory stubs. Mods planner hints
(`--mods-plan-timeout`, `--mods-max-parallel`) flow into stage metadata so Grid
can respect concurrency/timebox controls. When build-gate fails with a
retryable outcome the runner collects the failure metadata, re-plans a healing
branch using the Mods planner, and appends `#healN` stages before continuing to
static checks and tests. When `GRID_ENDPOINT` is omitted the in-memory Grid stub
remains active and still refuses stages whose lanes are not declared in the
manifest. When `PLOY_ASTER_ENABLE` is set the CLI resolves Aster bundle
provenance after a successful run so developers can confirm which
toggles/bundles were attached to each stage. Explicit ticket IDs remain a
stub-only workflow until Grid integration lands.

`workflow run` remains available for generic workflow execution; it mirrors the
Mods defaults but omits repo materialisation unless the new `--repo-*` flags are
provided.

`workflow cancel` requests cancellation of a Workflow RPC run. The subcommand
requires `GRID_ENDPOINT` so the CLI can reach a real Grid instance; in-memory
stubs respond with a friendly reminder instead of attempting a cancellation.
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

## Flags

- `--lane` — Lane identifier defined under `lanes/*.toml`.
  `lanes describe`).
- `--commit` / `--snapshot` / `--manifest` / `--aster` — Optional cache-key
  preview inputs consumed by the lane engine.
- `--tenant` — Tenant slug used to resolve subject namespaces. Required for
  `workflow run`, `snapshot capture`, and execution-mode
  `environment materialize`.
- `--ticket` — JetStream ticket identifier to claim (`workflow run`) or metadata
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
  (`lanes describe`, `workflow run`, `environment materialize`). The flag is
  ignored unless `PLOY_ASTER_ENABLE` is set.
- `--aster-step` — Stage-specific overrides for Aster behaviour when running
  workflows (`workflow run`). Use `stage=toggle1,toggle2` to enable additional
  toggles or `stage=off` to disable Aster for that stage. Overrides are ignored
  unless `PLOY_ASTER_ENABLE` is set.
- `--repo-url` / `--repo-base-ref` / `--repo-target-ref` / `--repo-workspace-hint`
  — Repository materialisation inputs consumed by `mod run` (also available to
  `workflow run`). When `--repo-url` is provided, `--repo-target-ref` is
  required; `--repo-base-ref` defaults to the repository's default branch. The
  workspace hint creates an auxiliary directory (e.g. `mods/java`) before Mods
  stages execute.
- `--mods-plan-timeout` — Duration string passed to the Mods planner so Grid can
  timebox plan evaluation (`mod run` / `workflow run`).
- `--mods-max-parallel` — Upper bound on concurrent Mods stages emitted by the
  planner (`mod run` / `workflow run`).

## Exit Codes

- `0` — success (ticket claimed, stages completed, workspace cleaned).
- `1` — error (missing flags, unsupported subcommand, stage failure, or
  downstream error).

## Environment

- `GRID_ENDPOINT` — Workflow RPC base URL (`https://grid-dev.example`) used by
  `workflow run` and `workflow cancel`; it also enables discovery via
  `/v1/cluster/info` for API endpoint, JetStream routes, IPFS gateway, feature
  map, and version configuration.
- `GRID_WORKFLOW_SDK_STATE_DIR` — Optional override for the Workflow RPC SDK
  cache location. Ploy now defaults this to `${XDG_CONFIG_HOME:-$HOME/.config}/ploy/grid`
  so manifests and CA bundles persist across CLI restarts.
- `PLOY_ASTER_ENABLE` — Opt-in switch for the experimental Aster integration.
  When unset the CLI skips bundle lookups and omits Aster toggles from cache
  keys, manifests, and summaries. When `GRID_ENDPOINT` is omitted the CLI falls
  back to the in-memory Grid and JetStream stubs for offline development.

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
