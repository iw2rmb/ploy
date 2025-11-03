# Ploy Workflow CLI

`ploy` is a single-purpose CLI that claims workflow tickets from the Ploy control plane,
reconstructs the default mods→build→test DAG, and dispatches stages via the
configured runtime adapter. Legacy subcommands (apps, env, mods, security, etc.) were
removed during the workstation legacy teardown.

## Usage

```bash
ploy lanes describe --lane <lane-name> \
  [--commit <sha>] [--manifest <version>] \
  [--aster <toggle,...>]
ploy mod run \
  [--ticket <ticket-id>|--ticket auto] \
  [--repo-url <url> --repo-base-ref <branch> --repo-target-ref <branch> \
   --repo-workspace-hint <dir>] \
  [--mods-plan-timeout <duration>] [--mods-max-parallel <n>] [--cap <duration>] [--cancel-on-cap] \
  [--aster <toggle,...>] \
  [--aster-step <stage=toggle,...|stage=off>]
ploy environment materialize <commit-sha> --app <app> \
  [--dry-run] [--manifest <name@version>] [--aster <toggle,...>]
ploy knowledge-base ingest --from <fixture.json>
ploy knowledge-base evaluate --fixture <samples.json>
ploy upload --run-id <uuid> [--stage-id <uuid>] [--build-id <uuid>] [--name <string>] <path>
```

`lanes describe` inspects the bundled TOML lane specs under `configs/lanes`,
displays the runtime family, build/test commands, surfaced job defaults (image,
command, env, resources), and shows a deterministic cache-key preview that incorporates
commit/manifest/Aster toggles. Aster inputs are only included when
`PLOY_ASTER_ENABLE` is set so the unfinished bundle integration can stay hidden
behind a feature flag. The preview mirrors what the workflow runner supplies to
the runtime when dispatching stages.

`mod run` claims a ticket (auto-generating one if `--ticket auto`),
materialises the repository passed via `--repo-*` flags (when provided),
compiles the referenced integration manifest from `configs/manifests/`,
publishes checkpoints for every stage transition (including lane cache keys),
executes mods/build/test against a temporary workspace, and cleans up before
exit. Mods planner hints (`--mods-plan-timeout`, `--mods-max-parallel`)
flow into stage metadata so the control plane can respect concurrency/timebox controls. `--cap` enforces an overall
time limit for `--follow`. If exceeded, the CLI exits the follow; add `--cancel-on-cap` to also cancel the ticket. When
build-gate fails with a retryable outcome the runner collects the failure
metadata, re-plans a healing branch using the Mods planner, and appends `#healN`
stages before continuing to static checks and tests. When
`PLOY_ASTER_ENABLE` is set the CLI resolves Aster bundle provenance after a
successful run so developers can confirm which toggles/bundles were attached to
each stage.

`environment materialize` evaluates the integration manifest for a given
app/commit pair, composes deterministic cache keys for each required lane, and
hydrates those caches through an in-memory hydrator. Dry-run mode avoids
hydration and still reports required resources.

`knowledge-base ingest` merges incident fixtures into the workstation catalog
under `configs/knowledge-base/catalog.json`, enforcing duplicate safeguards and
preserving schema version ordering. `knowledge-base evaluate` loads curated
samples, runs them through the advisor with a conservative score floor, and
prints per-sample match results plus aggregate accuracy so operators can gauge
classifier drift without leaving the workstation.

`upload` uses the cached mTLS cluster descriptor to post gzipped bundles to the control‑plane HTTPS API (no SSH).
It targets `POST /v1/runs/{id}/artifact_bundles` and enforces the 1 MiB bundle cap locally before sending.
The deprecated `--job-id` flag remains as an alias for `--run-id` for backward compatibility.

## Flags

- `--lane` — Lane identifier defined under `configs/lanes` (used by
  `lanes describe`).
- `--commit` / `--manifest` / `--aster` — Optional cache-key
  preview inputs consumed by the lane engine.
- `--ticket` — Ticket identifier to claim (`mod run`). Defaults to `auto` for workflows.
- `--app` — Application identifier resolved to an integration manifest (required
  for `environment materialize`).
- `--dry-run` — Skip cache hydration while still reporting
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
- `--mods-plan-timeout` — Duration string passed to the Mods planner to timebox
  plan evaluation (`mod run`).
- `--mods-max-parallel` — Upper bound on concurrent Mods stages emitted by the
  planner (`mod run`).
- Streaming guards (long-lived SSE):
  - `mods logs` and `runs follow` support `--idle-timeout <duration>` (default `45s`) to cancel when no events arrive, and `--timeout <duration>` to cap overall stream time.
- `--cap` — Overall time limit for `--follow`. When the duration elapses, the CLI stops following; use `--cancel-on-cap` to cancel the ticket too (e.g., `--cap 5m --cancel-on-cap`).

## Exit Codes

- `0` — success (ticket claimed, stages completed, workspace cleaned).
- `1` — error (missing flags, unsupported subcommand, stage failure, or
  downstream error).

## Environment
- `PLOY_RUNTIME_ADAPTER` — Optional runtime adapter selector. Defaults to
  `local-step`; other adapters (`k8s`, `nomad`) can register here and
  unknown names cause the CLI to fail fast.
- `PLOY_ASTER_ENABLE` — Opt-in switch for the experimental Aster integration.
  When unset the CLI skips bundle lookups and omits Aster toggles from cache
  keys, manifests, and summaries.

## Development

- Build via `make build` (outputs to `dist/ploy`).
- Run unit tests with `make test` (ensures `go test -cover ./...` stays ≥60%
  overall, ≥90% on the runner package).
- Roadmap slices should extend `internal/workflow/runner` and keep the CLI
  focused on stateless execution against the new control-plane contracts.
- See `docs/MANIFESTS.md` for schema details and authoring guidance on
  integration manifests.
- Review `docs/DOCS.md` for the documentation matrix and editing conventions
  that keep the CLI guides aligned.
