# Ploy Workflow CLI

`ploy` is a single-purpose CLI that claims workflow runs from the Ploy control plane,
reconstructs the default mods→build→test DAG, and dispatches stages via the
configured runtime adapter. Legacy subcommands (apps, env, mods, security, etc.) were
removed during the workstation legacy teardown.

## Usage

The CLI uses Cobra for command structure and help. To see available commands:

```bash
ploy --help                    # List all top-level commands
ploy <command> --help          # Get help for a specific command
ploy help <command>            # Alternative help syntax
```

Common command patterns:

```bash
ploy lanes describe --lane <lane-name> \
  [--commit <sha>] [--manifest <version>] \
  [--aster <toggle,...>]
ploy mod run \
  [--repo-url <url> --repo-base-ref <branch> [--repo-target-ref <branch>] \
   --repo-workspace-hint <dir>] \
  [--mods-plan-timeout <duration>] [--mods-max-parallel <n>] [--cap <duration>] [--cancel-on-cap] \
  [--aster <toggle,...>] \
	  [--aster-step <stage=toggle,...|stage=off>]
ploy environment materialize <commit-sha> --app <app> \
  [--dry-run] [--manifest <name@version>] [--aster <toggle,...>]
ploy upload --run-id <run-id> [--name <string>] <path>
```

Run IDs (`<run-id>`) are KSUID-backed strings.
Treat them as opaque identifiers when passing them between commands or scripts.

Note on `--json` output:
- When `--json` is supplied (e.g., `ploy mod run --json`), stdout emits a compact JSON summary (fields include `run_id`, `final_state`, optional `artifact_dir`, `mr_url`).
- Human‑readable progress and logs continue to print to stderr, so scripts can safely pipe stdout to `jq` without mixing formats.

Quick capture example:
```bash
TICKET=$(ploy mod run --json \
  --repo-url https://gitlab.com/org/repo.git \
  --repo-base-ref main \
  --repo-target-ref workflow/upgrade \
  --follow | jq -r '.run_id')
```

`lanes describe` inspects the bundled TOML lane specs under `configs/lanes`,
displays the runtime family, build/test commands, surfaced job defaults (image,
command, env, resources), and shows a deterministic cache-key preview that incorporates
commit/manifest/Aster toggles. Aster inputs are only included when
`PLOY_ASTER_ENABLE` is set so the unfinished bundle integration can stay hidden
behind a feature flag. The preview mirrors what the workflow runner supplies to
the runtime when dispatching stages.

`mod run` submits a run to the control plane (server assigns the run id),
materialises the repository passed via `--repo-*` flags (when provided),
compiles the referenced integration manifest from `configs/manifests/`,
publishes checkpoints for every stage transition (including lane cache keys),
executes mods/build/test against a temporary workspace, and cleans up before
exit. Mods planner hints (`--mods-plan-timeout`, `--mods-max-parallel`)
flow into stage metadata so the control plane can respect concurrency/timebox controls. `--cap` enforces an overall
time limit for `--follow`. If exceeded, the CLI exits the follow; add `--cancel-on-cap` to also cancel the run. When
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

## Batched Mod Runs

`mod run` supports two usage patterns: **single-repo runs** and **batch runs** that
operate over multiple repositories under a shared run spec. In a batch, `ploy mod run`
submits the spec once, then `ploy mod run repo add` attaches multiple repositories
under the same run via `run_repos`.

### Single-Repo Run (Default)

A single-repo run specifies all repository parameters inline with the initial command.
The run executes immediately against that repository:

```bash
# Single repository run — executes mods against one repo and follows logs.
ploy mod run --spec mod.yaml \
  --repo-url https://github.com/example/repo.git \
  --repo-base-ref main \
  --repo-target-ref feature-branch \
  --follow
```

This is the most common usage for quick, ad-hoc transformations.

### Batch Run (Multiple Repositories)

Batch runs allow orchestrating the same mod spec across multiple repositories.
First, create a batch run with a name but no repository; then attach repos
incrementally:

```bash
# Step 1: Create a named batch run (no repository attached yet).
ploy mod run --spec mod.yaml --name my-batch

# Step 2: Add repositories to the batch.
ploy mod run repo add \
  --repo-url https://github.com/org/repo-a.git \
  --base-ref main \
  --target-ref upgrade-deps \
  my-batch

ploy mod run repo add \
  --repo-url https://github.com/org/repo-b.git \
  --base-ref main \
  --target-ref upgrade-deps \
  my-batch

# Step 3: Optionally follow logs for the entire batch.
ploy runs follow my-batch
```

Each attached repository creates a `run_repo` entry, and jobs execute per-repo
according to the batch scheduler. Batch runs simplify fleet-wide updates where
the same transformation (e.g., Java 17 upgrade) applies to many repositories.

### Restart a Repo Within a Batch

If a repository job fails or needs reprocessing with a different branch, use
`mod run repo restart`:

```bash
# Restart repo-a with a hotfix branch (use repo-id from `mod run repo status`).
# Repo IDs are NanoID(8) strings (e.g., "a1b2c3d4").
ploy mod run repo restart \
  --repo-id <repo-id> \
  --target-ref hotfix \
  my-batch
```

This re-queues the repository under the same batch without recreating the run.

### Remove a Repo From a Batch

To remove a repository from an in-progress batch (e.g., if it was added by mistake):

```bash
# Repo IDs are NanoID(8) strings (e.g., "a1b2c3d4").
ploy mod run repo remove \
  --repo-id <repo-id> \
  my-batch
```

### Batch Workflow Summary

| Command                  | Description                                  |
|--------------------------|----------------------------------------------|
| `mod run --name <batch>` | Create a batch run (no repos yet)            |
| `mod run repo add`       | Attach a repository to an existing batch     |
| `mod run repo remove`    | Detach a repository from a batch             |
| `mod run repo restart`   | Re-queue a repo job with optional new branch |
| `runs follow <batch>`    | Follow logs for all repos in a batch         |

See `docs/mods-lifecycle.md` for the relationship between runs, `run_repos`, and jobs.

---

`mod resume` requests resumption of a failed or canceled Mods run via the
control plane `POST /v1/mods/{id}/resume` endpoint. This enables continuation
of previously interrupted workflows without resubmitting the entire spec.

```bash
ploy mod resume <run-id>
# Output: Resume requested
```

The command handles the following server responses:
- **202 Accepted**: Resume successfully initiated; eligible jobs are requeued.
- **200 OK**: Run is already running (idempotent) or all jobs succeeded.
- **404 Not Found**: Run does not exist.
- **409 Conflict**: Run state is not resumable (e.g., already succeeded).
- **400 Bad Request**: Invalid run ID format.

Only runs in `failed` or `canceled` state can be resumed. Succeeded runs
cannot be resumed since there are no jobs to requeue.

`environment materialize` evaluates the integration manifest for a given
app/commit pair, composes deterministic cache keys for each required lane, and
hydrates those caches through an in-memory hydrator. Dry-run mode avoids
hydration and still reports required resources.

`upload` uses the cached mTLS cluster descriptor to post gzipped bundles to the control‑plane HTTPS API (no SSH). The CLI always targets the default descriptor at `~/.config/ploy/clusters/default`.
It targets `POST /v1/runs/{id}/artifact_bundles` and enforces the 1 MiB bundle cap locally before sending.

## Shell Completion

The CLI provides shell completion for bash, zsh, fish, and PowerShell via the `completion` command:

```bash
# Generate completion script for your shell
ploy completion bash > /etc/bash_completion.d/ploy  # bash
ploy completion zsh > ~/.zsh/completion/_ploy       # zsh
ploy completion fish > ~/.config/fish/completions/ploy.fish  # fish
ploy completion powershell > ploy.ps1               # PowerShell
```

To load completions in your current shell session:

```bash
# bash
source <(ploy completion bash)

# zsh
source <(ploy completion zsh)

# fish
ploy completion fish | source

# PowerShell
ploy completion powershell | Out-String | Invoke-Expression
```

The completion command is powered by Cobra and provides:
- Command completion for all subcommands (mod, cluster, config, etc.)
- Flag completion for available options
- Context-aware suggestions based on command hierarchy

Note: Cluster management commands (deploy, node, rollout, token) are nested under `ploy cluster`. For example:
- `ploy cluster deploy` — Deploy control-plane server
- `ploy cluster node add` — Add worker nodes
- `ploy cluster rollout server|nodes` — Roll out binary updates
- `ploy cluster token create|list|revoke` — Manage API tokens

For persistent setup instructions specific to your shell, run:
```bash
ploy completion <shell> --help
```

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
  — Repository materialisation inputs consumed by `mod run`. When `--repo-url` is provided, `--repo-base-ref` selects the base branch (commonly `main`). `--repo-target-ref` is optional; when omitted, the node derives a default of `/mod/<run-id>` (using the run ID, a KSUID string) for workspace context and MR source branch. The workspace hint creates an auxiliary directory (e.g. `mods/java`) before Mods stages execute.
- `--mods-plan-timeout` — Duration string passed to the Mods planner to timebox
  plan evaluation (`mod run`).
- `--mods-max-parallel` — Upper bound on concurrent Mods stages emitted by the
  planner (`mod run`).
- `--artifact-dir` — Download final artifacts to the given directory after a
  successful run (`mod run --follow`). A `manifest.json` file is created with
  artifact metadata.
- Streaming guards (long-lived SSE):
  - `mods logs` and `runs follow` use resilient SSE streams backed by `github.com/tmaxmax/go-sse` and a shared exponential backoff policy.
  - `--idle-timeout <duration>` (default `45s`): Cancels the stream when no events arrive within the specified duration. Set to `0` to disable idle timeout.
  - `--timeout <duration>` (default `0`, unlimited): Caps the overall stream time. When exceeded, the CLI exits the stream.
  - `--max-retries <int>` (default `3` for `mods logs`, `3` for `runs follow`): Maximum number of reconnect attempts. Set to `-1` for unlimited retries.
  - `--retry-wait <duration>` (deprecated; default `1s` for `mods logs`, `500ms` for `runs follow`): Initial wait duration between reconnect attempts. The backoff policy applies exponential growth with jitter (2x multiplier, capped at 30s). This flag is preserved for backward compatibility; the shared backoff policy handles reconnect delays.
  - Reconnection semantics: On connection errors or mid-stream failures, the client automatically reconnects with exponential backoff (starting at `--retry-wait` if set, otherwise 250ms for SSE streams). Backoff resets after successfully receiving events. Last-Event-ID is preserved across reconnects to resume from the last processed event.
  - Server `retry` hints are not supported: The library-backed SSE client does not consume server-sent `retry` fields. Reconnect delays are driven entirely by the shared backoff policy.
- `--cap` — Overall time limit for `--follow`. When the duration elapses, the CLI stops following; use `--cancel-on-cap` to cancel the run too (e.g., `--cap 5m --cancel-on-cap`).

## Structured Log Format

The `ploy mods logs` and `ploy runs follow` commands consume enriched log events
from the Mods SSE stream (`GET /v1/mods/{id}/events`). A shared log printer
(`internal/cli/logs`) formats these events consistently across both commands.

### Log record fields

Each `event: log` frame contains a JSON `LogRecord` with core and optional
enriched fields for execution context:

| Field        | Type   | Description                                                       |
|--------------|--------|-------------------------------------------------------------------|
| `timestamp`  | string | RFC 3339 timestamp when the log line was captured                 |
| `stream`     | string | Output stream (`stdout` or `stderr`)                              |
| `line`       | string | Log message content                                               |
| `node_id`    | string | Execution node identifier (NanoID string, optional)               |
| `job_id`     | string | Job identifier (KSUID string, optional)                           |
| `mod_type`   | string | Step type: `pre_gate`, `mod`, `post_gate`, `heal`, `re_gate` (opt)|
| `step_index` | int    | Job ordering index, e.g., 1000, 2000 (optional)                   |

### Output formats

**Structured (default, `--format structured`):**

When enriched fields are present:
```
2025-10-22T10:00:00Z stdout node=a1b2c3d4 mod=mod step=2000 job=e5f6g7h8 Step started
```

When only core fields are available:
```
2025-10-22T10:00:00Z stdout Step started
```

**Raw (`--format raw`):**

Prints only the log line content, omitting timestamps and context:
```
Step started
```

### Example usage

```bash
# Follow logs in structured format (default)
ploy mods logs <run-id>

# Follow logs in raw format (message only)
ploy mods logs <run-id> --format raw

# Follow a run with structured log output
ploy runs follow <run-id>
```

See `docs/mods-lifecycle.md` § 7.2 for the complete SSE payload specification.

## GitLab MR Integration

The GitLab merge request client uses `gitlab.com/gitlab-org/api/client-go` for typed API interactions and integrates with the shared backoff policy for resilient operation.

### Retry Behavior

GitLab API calls automatically retry on transient failures using the `internal/workflow/backoff` shared helper:
- **Retry policy**: `GitLabMRPolicy` provides 4 max attempts (1 initial + 3 retries) with a 1s initial interval, 2x multiplier (1s, 2s, 4s backoff schedule), and 50% jitter for robustness.
- **Retryable conditions**: Rate limits (HTTP 429), server errors (5xx), and network failures without an HTTP response (e.g., connection refused, DNS failures).
- **Non-retryable conditions**: Client errors (4xx except 429), context cancellation, and missing response data are treated as permanent failures and do not trigger retries.
- **Context cancellation**: All retry operations honor `context.Context` cancellation and exit early when the context is done.

### Security & PAT Redaction

Personal Access Tokens (PATs) are automatically redacted from all error messages and logs to prevent credential leakage:
- The client redacts both literal PATs and URL-encoded variants (query-escaped, path-escaped).
- PATs are never logged or written to disk on worker nodes.
- Tokens are transmitted securely via mTLS from the control plane to nodes.
- All errors flowing out of client-go-backed operations pass through the redaction layer.

### Configuration

GitLab credentials can be configured globally on the control plane or overridden per run via CLI flags:
- **Global config**: Use `ploy config gitlab set --file <config.json>` to configure domain and PAT once (see `docs/how-to/create-mr.md`).
- **Per-run override**: Use `--gitlab-domain` and `--gitlab-pat` flags to override for a single run.
- **Domain normalization**: The client accepts bare hostnames (e.g., `gitlab.com`) or full URLs (e.g., `https://gitlab.com`) and normalizes them for API calls. Localhost and 127.0.0.1 addresses default to HTTP; all other domains default to HTTPS.
- **Authentication headers**: The client-go wrapper sets both `Authorization: Bearer <token>` and `PRIVATE-TOKEN: <token>` headers for compatibility with different GitLab configurations.

### Implementation Notes

The node agent uses `internal/nodeagent/gitlab/mr_client.go` with the following behavior:
- **Project ID encoding**: External callers provide URL-encoded project IDs (e.g., `org%2Fproject`), which are decoded before passing to client-go (the library re-encodes internally).
- **Optional fields**: Description and labels are trimmed and only included when non-empty. Labels are split by commas and passed as a slice to client-go.
- **Error handling**: All API errors include PAT redaction via `redactError()` to ensure tokens never appear in logs or returned errors.

See `docs/how-to/create-mr.md` for end-to-end usage examples and `internal/nodeagent/gitlab/mr_client.go` for implementation details.

## Build Gate Healing

When a Build Gate fails before the main mod runs, the node agent can execute a healing
sequence configured via the `build_gate_healing` block in the spec. This enables automated
repair of build failures using tools like Codex or other LLM-based workflows.

**How it works (jobs-based gate model):**
1. Gate checks run as jobs in the unified `jobs` queue (`pre_gate` and `re_gate` phases)
   and are claimed by nodes via `/v1/nodes/{id}/claim`.
2. If the pre-gate job fails and `build_gate_healing` is configured, the node executes each
   healing step in sequence (mods under `build_gate_healing.mods[]`) against the same
   workspace and Build Gate logs.
3. After all healing steps complete, a `re_gate` job runs as another job in the queue. If it
   passes, the main mod proceeds.
4. The healing loop can retry up to `build_gate_healing.retries` times (default: 1).
5. If the gate still fails after exhausting retries, the run terminates with status `failed`
   and reason `build-gate`. When `mr_on_fail` is enabled, an MR is still created.

**Execution path:**
Gate and healing steps are executed by the same nodeagent process that runs Mods jobs. Gate
jobs are regular jobs in the unified queue; there is no separate HTTP Build Gate worker
mode and no `buildgate_worker_enabled` toggle.

**CLI-visible gate summaries:**
Gate results are surfaced via `ploy mod inspect <run-id>` in the same format regardless
of execution location:
- `Gate: passed duration=1234ms`
- `Gate: failed pre-gate duration=567ms`

Gate jobs may run on any node that claims them from the unified queue, but the CLI output
and `Metadata["gate_summary"]` in `GET /v1/mods/{id}` responses remain unchanged. The
gate executor logic abstracts execution location, ensuring consistent gate status
reporting.

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

## Job Graph and DAG State

Mods runs execute as a directed acyclic graph (DAG) of jobs. The graph structure
surfaces via `GET /v1/mods/{id}` in `RunSummary.stages` and through the
`ploy mod inspect` command. Each job has a `step_index` for execution ordering
and optional metadata identifying the job phase.

**Job phases (mod_type):**
- `pre_gate` — Build Gate validation before mods run
- `mod` — Main mod execution (code transformation)
- `post_gate` — Build Gate validation after mods succeed
- `heal` — Healing mod execution (when pre/post gate fails)
- `re_gate` — Build Gate re-validation after healing

**DAG structure:**

```
pre-gate → mod-0 → post-gate
          │
          └─(fail)→ heal → re-gate → mod-0
```

When a gate fails with a retryable outcome, the runner branches into the healing
flow. The heal job attempts to fix the build issue, then re-gate validates the
fix. If re-gate passes, the DAG continues to the next mod.

**CLI inspection:**

Use `ploy mod inspect <run-id>` to view job-level state:

```bash
$ ploy mod inspect mods-abc123
Run mods-abc123: running
MR: https://gitlab.com/org/repo/-/merge_requests/1
Gate: failed pre-gate duration=567ms
Jobs:
  [1000] a1b2c3d4: succeeded
  [2000] e5f6g7h8: running
  [2500] i9j0k1l2: pending
```

The `[step_index]` ordering reflects execution sequence. Healing jobs inserted
dynamically appear with midpoint indices (e.g., 1500 between 1000 and 2000).

**API response:**

The `GET /v1/mods/{id}` endpoint returns `RunSummary` with:
- `stages` — Map of job ID (KSUID string) to `StageStatus` (state, step_index, attempts)
- `metadata["gate_summary"]` — Human-readable gate result
- `metadata["mr_url"]` — Merge request URL if created

See `internal/mods/api/types.go` for the full schema.

## Exit Codes

- `0` — success (run claimed, stages completed, workspace cleaned).
- `1` — error (missing flags, unsupported subcommand, stage failure, or
  downstream error).

## Environment
- `PLOY_RUNTIME_ADAPTER` — Optional runtime adapter selector. Defaults to
  `local-step`; other adapters (e.g., `k8s`) can register here and
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
