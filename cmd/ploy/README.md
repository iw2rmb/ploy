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

Note on `--json` output:
- When `--json` is supplied (e.g., `ploy mod run --json`), stdout emits a compact JSON summary (fields include `ticket_id`, `final_state`, optional `artifact_dir`, `mr_url`).
- Human‑readable progress and logs continue to print to stderr, so scripts can safely pipe stdout to `jq` without mixing formats.

Quick capture example:
```bash
TICKET=$(ploy mod run --json \
  --repo-url https://gitlab.com/org/repo.git \
  --repo-base-ref main \
  --repo-target-ref workflow/upgrade \
  --follow | jq -r '.ticket_id')
```

`lanes describe` inspects the bundled TOML lane specs under `configs/lanes`,
displays the runtime family, build/test commands, surfaced job defaults (image,
command, env, resources), and shows a deterministic cache-key preview that incorporates
commit/manifest/Aster toggles. Aster inputs are only included when
`PLOY_ASTER_ENABLE` is set so the unfinished bundle integration can stay hidden
behind a feature flag. The preview mirrors what the workflow runner supplies to
the runtime when dispatching stages.

`mod run` submits a run to the control plane (server assigns the ticket id),
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

When a followed run completes successfully, pass `--artifact-dir <dir>` to
download referenced artifacts and generate `<dir>/manifest.json`. The manifest
lists artifacts with `stage`, `name`, `cid`, `digest`, `size` (bytes written),
and the local `path`. Filenames are sanitized and deterministic; when a content
digest is available it prefixes the name, otherwise the artifact CID is used.

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

`upload` uses the cached mTLS cluster descriptor to post gzipped bundles to the control‑plane HTTPS API (no SSH). The CLI always targets the default descriptor at `~/.config/ploy/clusters/default`.
It targets `POST /v1/runs/{id}/artifact_bundles` and enforces the 1 MiB bundle cap locally before sending.
The deprecated `--job-id` flag remains as an alias for `--run-id` for backward compatibility.

## Flags

- `--lane` — Lane identifier defined under `configs/lanes` (used by
  `lanes describe`).
- `--commit` / `--manifest` / `--aster` — Optional cache-key
  preview inputs consumed by the lane engine.
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
- `--spec` — Path to a YAML/JSON spec file defining mod parameters, Build Gate settings,
  and healing configuration for `mod run`. CLI flags (e.g., `--mod-image`, `--gitlab-pat`)
  override corresponding spec values when both are present. When a canonical `mod`
  section exists, overrides apply inside `mod` (e.g., `mod.env`, `mod.image`, `mod.command`).
  The spec supports inline
  environment variables (`env`), file-based secrets (`env_from_file`), Build Gate healing
  (`build_gate_healing`), and GitLab MR settings. See `docs/schemas/mod.example.yaml`
  for the full schema and `tests/e2e/mods/README.md` for usage examples.
- `--repo-url` / `--repo-base-ref` / `--repo-target-ref` / `--repo-workspace-hint`
  — Repository materialisation inputs consumed by `mod run`. When `--repo-url` is provided, `--repo-target-ref` is
  required; `--repo-base-ref` defaults to the repository's default branch. The
  workspace hint creates an auxiliary directory (e.g. `mods/java`) before Mods
  stages execute.
- `--mods-plan-timeout` — Duration string passed to the Mods planner to timebox
  plan evaluation (`mod run`).
- `--mods-max-parallel` — Upper bound on concurrent Mods stages emitted by the
  planner (`mod run`).
- `--artifact-dir` — Download final artifacts to the given directory after a
  successful run (`mod run --follow`). A `manifest.json` file is created with
  artifact metadata.
- Streaming guards (long-lived SSE):
  - `mods logs` and `runs follow` support `--idle-timeout <duration>` (default `45s`) to cancel when no events arrive, and `--timeout <duration>` to cap overall stream time.
- `--cap` — Overall time limit for `--follow`. When the duration elapses, the CLI stops following; use `--cancel-on-cap` to cancel the ticket too (e.g., `--cap 5m --cancel-on-cap`).

## Build Gate Healing

When a Build Gate fails before the main mod runs, the node agent can execute a healing
sequence configured via the `build_gate_healing` block in the spec. This enables automated
repair of build failures using tools like Codex or other LLM-based workflows.

**How it works:**
1. The node runs the Build Gate before the main mod container.
2. If the gate fails and `build_gate_healing` is configured, the node executes each healing
   step in sequence (mods under `build_gate_healing.mods[]`).
3. After all healing steps complete, the gate is re-run. If it passes, the main mod proceeds.
4. The healing loop can retry up to `build_gate_healing.retries` times (default: 1).
5. If the gate still fails after exhausting retries, the run terminates with status `failed`
   and reason `build-gate`. When `mr_on_fail` is enabled, an MR is still created.

**Spec format:**
```yaml
build_gate_healing:
  retries: 1
  mods:
    - image: docker.io/you/mods-codex:latest
      command: ["mod-codex", "--input", "/workspace", "--out", "/out"]
      env:
        CODEX_PROMPT: "Fix the build error in /in/build-gate.log"
      env_from_file:
        CODEX_AUTH_JSON: ~/.codex/auth.json
      retain_container: false
```

**Cross-phase inputs:**
- `/in/build-gate.log` — First Build Gate failure log (mounted read-only for healing mods).
- `/in/prompt.txt` — Optional prompt file (mounted when provided in spec).

See `docs/schemas/mod.example.yaml` for a complete example and `tests/e2e/mods/README.md`
for end-to-end usage with `mods-codex`.

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
