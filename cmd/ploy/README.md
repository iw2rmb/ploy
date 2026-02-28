# Ploy Workflow CLI

`ploy` is a single-purpose CLI that claims workflow runs from the Ploy control plane,
reconstructs the default migs→build→test DAG, and dispatches stages via the
configured runtime adapter. Legacy subcommands (apps, env, migs, security, etc.) were
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
ploy run --repo <url> --base-ref <ref> --target-ref <ref> --spec <path|->  # submit a single-repo run
ploy mig run <mig-id|name> [--repo <url> ...] [--failed]                   # execute a mig project over its repo set
ploy mig run \
  [--repo-url <url> --repo-base-ref <branch> [--repo-target-ref <branch>] \
   --repo-workspace-hint <dir>] \
  [--migs-plan-timeout <duration>] [--migs-max-parallel <n>] [--cap <duration>] [--cancel-on-cap]
ploy environment materialize <commit-sha> --app <app> \
  [--dry-run] [--manifest <name@version>]
```

Run IDs (`<run-id>`) are KSUID-backed strings.
Treat them as opaque identifiers when passing them between commands or scripts.

Note on `--json` output:
- When `--json` is supplied (e.g., `ploy mig run --json`), stdout emits a compact JSON summary (fields include `run_id`, `final_state`, optional `artifact_dir`, `mr_url`).
- Human‑readable progress continues to print to stderr, so scripts can safely pipe stdout to `jq` without mixing formats.

Quick capture example:
```bash
TICKET=$(ploy mig run --json \
  --repo-url https://gitlab.com/org/repo.git \
  --repo-base-ref main \
  --repo-target-ref workflow/upgrade \
  --follow | jq -r '.run_id')
```

`mig run` submits a run to the control plane (server assigns the run id),
materialises the repository passed via `--repo-*` flags (when provided),
compiles the referenced integration manifest (when provided),
publishes checkpoints for every stage transition (including lane cache keys),
executes migs/build/test against a temporary workspace, and cleans up before
exit. Mods planner hints (`--migs-plan-timeout`, `--migs-max-parallel`)
flow into stage metadata so the control plane can respect concurrency/timebox controls.

When `--follow` is set, the CLI displays a summarized per-repo job graph that
refreshes until the run reaches a terminal state. The job graph shows step index,
job type, job ID, display name, status glyph, duration, and status for each job.
Note: `--follow` does not stream container logs. Use `ploy run logs <run-id>` for log streaming.

`--cap` enforces an overall time limit for `--follow`. If exceeded, the CLI exits the follow; add `--cancel-on-cap` to also cancel the run. When
build-gate fails with a retryable outcome the runner collects the failure
metadata, re-plans a healing branch using the Mods planner, and appends `#healN`
stages before continuing to static checks and tests.

When a followed run completes successfully, pass `--artifact-dir <dir>` to
download referenced artifacts and generate `<dir>/manifest.json`. The manifest
lists artifacts with `stage`, `name`, `cid`, `digest`, `size` (bytes written),
and the local `path`. Filenames are sanitized and deterministic; when a content
digest is available it prefixes the name, otherwise the artifact CID is used.

## Mod Projects

Mod projects are long-lived containers with a unique name, a current spec, and a managed repo set.

```bash
# Create a mig project.
ploy mig add --name my-mig --spec mig.yaml

# Update the mig's current spec.
ploy mig spec set my-mig mig.yaml

# Manage the mig's repo set.
ploy mig repo add my-mig --repo https://github.com/org/repo-a.git --base-ref main --target-ref upgrade
ploy mig repo add my-mig --repo https://github.com/org/repo-b.git --base-ref main --target-ref upgrade
ploy mig repo list my-mig

# Execute the mig project (all repos by default).
ploy mig run my-mig

# Execute only specific repos (repeatable).
ploy mig run my-mig --repo https://github.com/org/repo-a.git --repo https://github.com/org/repo-b.git

# Re-run only repos whose last terminal state is Fail.
ploy mig run my-mig --failed
```

## Batched Mod Runs

`mig run` supports two usage patterns: **single-repo runs** and **batch runs** that
operate over multiple repositories under a shared run spec. In a batch, `ploy mig run`
submits the spec once, then `ploy mig run repo add` attaches multiple repositories
under the same run via `run_repos`.

### Single-Repo Run (Default)

A single-repo run specifies all repository parameters inline with the initial command.
The run executes immediately against that repository:

```bash
# Single repository run — executes migs against one repo and follows job graph.
ploy mig run --spec mig.yaml \
  --repo-url https://github.com/example/repo.git \
  --repo-base-ref main \
  --repo-target-ref feature-branch \
  --follow
```

This is the most common usage for quick, ad-hoc transformations.

### Batch Run (Multiple Repositories)

Batch runs allow orchestrating the same mig spec across multiple repositories.
First, create a batch run with a name but no repository; then attach repos
incrementally:

```bash
# Step 1: Create a named batch run (no repository attached yet).
ploy mig run --spec mig.yaml --name my-batch

# Step 2: Add repositories to the batch.
ploy mig run repo add \
  --repo-url https://github.com/org/repo-a.git \
  --base-ref main \
  --target-ref upgrade-deps \
  my-batch

ploy mig run repo add \
  --repo-url https://github.com/org/repo-b.git \
  --base-ref main \
  --target-ref upgrade-deps \
  my-batch

# Step 3: Optionally stream logs for the entire batch.
ploy run logs my-batch
```

Each attached repository creates a `run_repo` entry, and jobs execute per-repo
according to the batch scheduler. Batch runs simplify fleet-wide updates where
the same transformation (e.g., Java 17 upgrade) applies to many repositories.

### Restart a Repo Within a Batch

If a repository job fails or needs reprocessing with a different branch, use
`mig run repo restart`:

```bash
# Restart repo-a with a hotfix branch (discover repo-id from `ploy run status --json`).
# Repo IDs are NanoID(8) strings (e.g., "a1b2c3d4").
ploy mig run repo restart \
  --repo-id <repo-id> \
  --target-ref hotfix \
  my-batch
```

This re-queues the repository under the same batch without recreating the run.

### Remove a Repo From a Batch

To remove a repository from an in-progress batch (e.g., if it was added by mistake):

```bash
# Repo IDs are NanoID(8) strings (e.g., "a1b2c3d4").
ploy mig run repo remove \
  --repo-id <repo-id> \
  my-batch
```

### Batch Workflow Summary

| Command                  | Description                                   |
|--------------------------|-----------------------------------------------|
| `mig run --name <batch>` | Create a batch run (no repos yet)             |
| `mig run repo add`       | Attach a repository to an existing batch      |
| `mig run repo remove`    | Detach a repository from a batch              |
| `mig run repo restart`   | Re-queue a repo job with optional new branch  |
| `run pull <run-id>`      | Pull diffs for the current repo from a run    |
| `mig pull [<mig>]`       | Pull diffs for the current repo from a mig    |
| `run logs <batch>`       | Stream logs/events for all repos in a batch   |

See `docs/migs-lifecycle.md` for the relationship between runs, `run_repos`, and jobs.

### Pull Mods Changes Locally

After a run completes, you can pull the Mods-generated changes into your local
repository using either `ploy run pull <run-id>` (run-based) or `ploy mig pull` (mig-based).
These commands reconstruct the Mods branch locally by fetching stored diffs from the
control plane and applying them to a new branch.

```bash
# From a repo that participated in a Mods run:
cd service-a

# Run-based pull (you know the run_id):
ploy run pull <run-id>

# Mod-based pull (default: last succeeded):
ploy mig pull <mig-id|name>
```

**How it works:**
1. Derives the current repo identity from the git remote (default: `origin`).
2. Verifies the working tree is clean (no uncommitted changes).
3. Resolves `(run_id, repo_id)` via `POST /v1/runs/{run_id}/pull` (or `POST /v1/migs/{mod_id}/pull` for mig-based pull).
4. Fetches the run's `base_ref` from the origin remote (`git fetch <origin> <base_ref> --depth=1`).
5. Creates a new branch at the fetched commit using the run's `target_ref`.
6. Downloads and applies all stored Mods diffs via `git apply`.

**Arguments:**
- `<run-id>` — Run ID (KSUID string), for `ploy run pull`.
- `[<mig-id|name>]` — Mod ID or name (optional), for `ploy mig pull`.

**Flags:**
- `--origin <remote>` — Git remote to match (default: `origin`). Use this when your
  repository has multiple remotes.
- `--dry-run` — Validate and print planned actions without creating branches or applying
  patches. Useful for previewing what changes would be pulled.

**Examples:**

```bash
# Pull changes from a run ID.
ploy run pull <run-id>

# Preview what would be pulled without making changes.
ploy run pull --dry-run <run-id>

# Pull from a run using a specific remote.
ploy run pull --origin upstream <run-id>

# Pull changes from the latest successful run for a mig.
ploy mig pull <mig-id|name>

# Pull changes from the latest failed run for a mig.
ploy mig pull --last-failed <mig-id|name>
```

**Requirements:**
- Must be run inside a git repository.
- Working tree must be clean (commit or stash changes first).
- The origin remote URL must match the `repo_url` used when the run was created.
- The run must exist and have diffs available.

### Local Repo Pull Workflow (`ploy pull`)

For a streamlined local development workflow, `ploy pull` manages the entire
run lifecycle for your current repo HEAD. It tracks run state locally and
handles run initiation, following, and diff pulling automatically.

```bash
# In your local repo, initiate/reuse a run and pull diffs when done.
ploy pull --follow

# Force a new run even if one already exists for this HEAD.
ploy pull --new-run --follow

# Preview what would happen without making changes.
ploy pull --dry-run
```

**How it works:**
1. Reads the current HEAD SHA via `git rev-parse HEAD`.
2. Checks for saved pull state in `<git-dir>/ploy/pull_state.json`.
3. If no state exists or `--new-run` is set: infers the mig from the repo
   and creates a mig-project run scoped to this repo (unless `--dry-run`, which
   prints intended actions and exits without initiating a run or saving state).
4. If state exists and SHA matches: reuses the saved run ID.
5. If SHA mismatch: requires `--new-run` to initiate a fresh run.
6. With `--follow`: displays a job graph until run completes.
7. On success: pulls diffs using the same logic as `ploy run pull`.

**Flags:**
- `--new-run` — Force initiating a new run, overwriting any saved pull state.
- `--follow` — Follow the run until completion (displays job graph).
- `--origin <remote>` — Git remote to match (default: `origin`).
- `--dry-run` — Validate and print planned actions without mutating. Does not initiate
  a run or save pull state.
- `--cap <duration>` — Optional time cap for `--follow` (e.g., `30m`, `1h`).
- `--cancel-on-cap` — Cancel run if cap exceeded.

**Examples:**

```bash
# Full workflow: initiate (or reuse) run, follow until done, pull diffs.
ploy pull --follow

# Force a fresh run even if the current HEAD already has one.
ploy pull --new-run --follow

# Check if a previous run succeeded and pull diffs (no --follow).
ploy pull

# Preview what would happen.
ploy pull --dry-run
```

---

`environment materialize` evaluates the integration manifest for a given
app/commit pair, composes deterministic cache keys for each required lane, and
hydrates those caches through an in-memory hydrator. Dry-run mode avoids
hydration and still reports required resources.

`upload` uses the cached bearer-token cluster descriptor to post gzipped bundles to the control‑plane API. The CLI always targets the default descriptor under `PLOY_CONFIG_HOME` (or XDG/home default).
It targets `POST /v1/runs/{id}/artifact_bundles` and enforces the 10 MiB bundle cap locally before sending.

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
- Command completion for all subcommands (mig, cluster, config, etc.)
- Flag completion for available options
- Context-aware suggestions based on command hierarchy

Note: Token management commands are nested under `ploy cluster token`:
- `ploy cluster token create|list|revoke` — Manage API tokens

For persistent setup instructions specific to your shell, run:
```bash
ploy completion <shell> --help
```

## Flags

- `--commit` / `--manifest` — Optional cache-key
  preview inputs consumed by the lane engine.
- `--app` — Application identifier resolved to an integration manifest (required
  for `environment materialize`).
- `--dry-run` — Skip cache hydration while still reporting
  required resources (`environment materialize`).
- `--manifest` — Override manifest name/version in `<name>@<version>` form
  (`environment materialize`).
- `--spec` — Path to a YAML/JSON spec file defining mig parameters, Build Gate settings,
  and healing configuration for `mig run`. CLI flags (e.g., `--job-image`, `--gitlab-pat`)
  override corresponding spec values when both are present. Specs use canonical shapes:
  top-level fields for single-step runs (`image`, `command`, `env`)
  and `migs[]` for multi-step runs. The spec supports inline environment variables (`env`),
  file-based secrets (`env_from_file`), Build Gate healing/router (`build_gate.healing.by_error_kind`, `build_gate.router`),
  and GitLab MR settings. See `docs/schemas/mig.example.yaml` for the full schema and
  `tests/e2e/migs/README.md` for usage examples.
- `--repo-url` / `--repo-base-ref` / `--repo-target-ref` / `--repo-workspace-hint`
  — Repository materialisation inputs consumed by `mig run`. Allowed `--repo-url` schemes: `https://`, `ssh://`, `file://`. When `--repo-url` is provided, `--repo-base-ref` selects the base branch (commonly `main`). `--repo-target-ref` is optional; when omitted, the node derives a default of `ploy/{run_name|run_id}` (using the run name when set or the run ID, a KSUID string, otherwise) for workspace context and MR source branch. The workspace hint creates an auxiliary directory (e.g. `migs/java`) before Mods stages execute.
- `--migs-plan-timeout` — Duration string passed to the Mods planner to timebox
  plan evaluation (`mig run`).
- `--migs-max-parallel` — Upper bound on concurrent Mods stages emitted by the
  planner (`mig run`).
- `--artifact-dir` — Download final artifacts to the given directory after a
  successful run (`mig run --follow`). A `manifest.json` file is created with
  artifact metadata.
- Streaming guards (long-lived SSE):
  - `run logs` uses resilient SSE streams backed by `github.com/tmaxmax/go-sse` and a shared exponential backoff policy.
  - `--idle-timeout <duration>` (default `45s`): Cancels the stream when no events arrive within the specified duration. Set to `0` to disable idle timeout.
  - `--timeout <duration>` (default `0`, unlimited): Caps the overall stream time. When exceeded, the CLI exits the stream.
  - `--max-retries <int>` (default `3` for `run logs`): Maximum number of reconnect attempts. Set to `-1` for unlimited retries.
  - Reconnection semantics: On connection errors or mid-stream failures, the client automatically reconnects with exponential backoff (250ms initial interval, 2x multiplier with jitter, capped at 30s). Backoff resets after successfully receiving events. Last-Event-ID is preserved across reconnects to resume from the last processed event.
  - Server `retry` hints are not supported: The library-backed SSE client does not consume server-sent `retry` fields. Reconnect delays are driven entirely by the shared backoff policy.
- `--cap` — Overall time limit for `--follow`. When the duration elapses, the CLI stops following; use `--cancel-on-cap` to cancel the run too (e.g., `--cap 5m --cancel-on-cap`).

## Structured Log Format

The `ploy run logs` command consumes enriched log events
from the run SSE stream (`GET /v1/runs/{id}/logs`). A shared log printer
(`internal/cli/logs`) formats these events consistently.

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
| `job_type`   | string | Step type: `pre_gate`, `mig`, `post_gate`, `heal`, `re_gate` (opt)|
| `next_id` | int    | Job ordering index, e.g., 1000, 2000 (optional)                   |

### Output formats

**Structured (default, `--format structured`):**

When enriched fields are present:
```
2025-10-22T10:00:00Z stdout node=a1b2c3d4 mig=mig step=2000 job=e5f6g7h8 Step started
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
ploy run logs <run-id>

# Follow events in raw format (message only)
ploy run logs <run-id> --format raw
```

See `docs/migs-lifecycle.md` § 7.2 for the complete SSE payload specification.

## Global Environment Configuration

The `ploy config env` commands manage global environment variables that are automatically
injected into job containers across the cluster. This provides a centralized way to configure
credentials, CA bundles, and other shared settings without embedding them in every spec file.

### Key Concepts

**Scopes** control which job types receive each variable:
- `all` — Inject into every job type (migs, healing, gates)
- `migs` — Inject into `mig` and `post_gate` jobs (code modification phases)
- `heal` — Inject into `heal` and `re_gate` jobs (healing/retry phases)
- `gate` — Inject into `pre_gate`, `re_gate`, and `post_gate` jobs (gate execution phases)

**Secrets** are redacted in list/show output by default. Use `--raw` with `show` to reveal the
full value.

**Precedence**: Per-run env vars (in spec files or CLI flags) take precedence over global env.
Existing keys in the spec are never overwritten by global config.

### Commands

```bash
# List all global environment variables (secret values redacted)
ploy config env list

# Show a specific variable (use --raw to reveal secret values)
ploy config env show --key CA_CERTS_PEM_BUNDLE
ploy config env show --key OPENAI_API_KEY --raw

# Set a variable from a file (common for certs and JSON credentials)
ploy config env set --key CA_CERTS_PEM_BUNDLE --file ca-bundle.pem --scope all
ploy config env set --key CODEX_AUTH_JSON --file ~/.codex/auth.json --scope migs

# Set a variable with an inline value
ploy config env set --key OPENAI_API_KEY --value sk-... --scope all

# Set a non-secret variable (visible in list output)
ploy config env set --key CUSTOM_VAR --value myvalue --scope all --secret=false

# Delete a variable
ploy config env unset --key OLD_VAR
```

### Common Variables

| Variable | Description | Recommended Scope |
|----------|-------------|-------------------|
| `CA_CERTS_PEM_BUNDLE` | PEM-encoded CA certificates for TLS trust | `all` |
| `CODEX_AUTH_JSON` | Codex authentication credentials | `migs` |
| `OPENAI_API_KEY` | OpenAI API key for LLM-integrated migs | `all` |

See `docs/envs/README.md` § "Global Env Configuration" for detailed semantics and
`docs/migs-lifecycle.md` for how these variables flow into job containers.

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

When a Build Gate fails before the main mig runs, the node agent can execute a healing
sequence configured via `build_gate.healing.by_error_kind` in the spec. This enables automated
repair of build failures using tools like Codex or other LLM-based workflows.

**How it works (jobs-based gate model):**
1. Gate checks run as jobs in the unified `jobs` queue (`pre_gate` and `re_gate` phases)
   and are claimed by nodes via `/v1/nodes/{id}/claim`.
2. If the pre-gate job fails and `build_gate.healing.by_error_kind` is configured, the node executes
   the selected healing action under `build_gate.healing.by_error_kind.<error_kind>` against the same workspace
   and Build Gate logs.
3. After all healing steps complete, a `re_gate` job runs as another job in the queue. If it
   passes, the main mig proceeds.
4. The healing loop retries up to `build_gate.healing.by_error_kind.<error_kind>.retries` (default: 1).
5. If the gate still fails after exhausting retries, the run terminates with status `failed`
   and reason `build-gate`. When `mr_on_fail` is enabled, an MR is still created.

**Execution path:**
Gate and healing steps are executed by the same nodeagent process that runs Mods jobs. Gate
jobs are regular jobs in the unified queue; there is no separate HTTP Build Gate worker
mode and no `buildgate_worker_enabled` toggle.

**CLI-visible gate summaries:**
Gate results are surfaced via `GET /v1/runs/{id}/status` in the same format regardless
of execution location:
- `Gate: passed duration=1234ms`
- `Gate: failed pre-gate duration=567ms`

Gate jobs may run on any node that claims them from the unified queue, but the CLI output
and `Metadata["gate_summary"]` in `GET /v1/runs/{id}/status` responses remain unchanged. The
gate executor logic abstracts execution location, ensuring consistent gate status
reporting.

**Spec format (healing under Build Gate):**
```yaml
build_gate:
  router:
    spec_path: ./healing/router/spec.yaml
  healing:
    by_error_kind:
      infra:
        spec_path: ./healing/infra/spec.yaml
        retries: 1
        image: docker.io/you/migs-codex:latest
        command: ["mig-codex", "--input", "/workspace", "--out", "/out"]
        env:
          CODEX_PROMPT: "Fix the infra build error in /in/build-gate.log"
        env_from_file:
          CODEX_AUTH_JSON: ~/.codex/auth.json
        expectations:
          artifacts:
            - path: /out/prep-profile-candidate.json
              schema: prep_profile_v1
      code:
        spec_path: ./healing/code/spec.yaml
        retries: 1
        image: docker.io/you/migs-codex:latest
        env:
          CODEX_PROMPT: "Fix the code build error in /in/build-gate.log"
```

`spec_path` is an optional CLI-side composition key for Build Gate router/healing
objects. Supported locations:
- `build_gate.router.spec_path`
- `build_gate.healing.by_error_kind.<error_kind>.spec_path`
CLI reads the referenced YAML/JSON object and deep-merges it into the target
object. Inline fields next to `spec_path` override loaded fields. The key is
removed before submit.

For `infra` healing with `expectations.artifacts` schema `prep_profile_v1`, the
healing container is expected to write `/out/prep-profile-candidate.json`. The
candidate is considered for repo `prep_profile` promotion only after the immediate
follow-up `re_gate` succeeds. Failed `re_gate` results never promote candidates.

**Cross-phase inputs:**
- `/in/build-gate.log` — First Build Gate failure log (mounted read-only for healing migs).
- `/in/prompt.txt` — Optional prompt file (mounted when provided in spec).

**Container cleanup model:**
- Containers are retained after step/gate completion.
- Cleanup trigger: before claim; threshold: 1 GiB free on Docker data-root filesystem.

See `docs/schemas/mig.example.yaml` for a complete example and `tests/e2e/migs/README.md`
for end-to-end usage with `migs-codex`.

## Job Graph and DAG State

Mods runs execute as a directed acyclic graph (DAG) of jobs. The graph structure
surfaces via `GET /v1/runs/{id}/status` in `RunSummary.stages` and through the
Run status includes a `stages` map. Each job has a `next_id` for execution ordering
and optional metadata identifying the job phase.

**Job phases (job_type):**
- `pre_gate` — Build Gate validation before migs run
- `mig` — Main mig execution (code transformation)
- `post_gate` — Build Gate validation after migs succeed
- `heal` — Healing mig execution (when pre/post gate fails)
- `re_gate` — Build Gate re-validation after healing

**DAG structure:**

```
pre-gate → mig-0 → post-gate
          │
          └─(fail)→ heal → re-gate → mig-0
```

When a gate fails with a retryable outcome, the runner branches into the healing
flow. The heal job attempts to fix the build issue, then re-gate validates the
fix. If re-gate passes, the DAG continues to the next mig.

**CLI inspection:**

Use `GET /v1/runs/{id}/status` to view run-level state:

```bash
$ curl -sk "$PLOY_CONTROL_PLANE_URL/v1/runs/migs-abc123/status" | jq .
Run migs-abc123: running
MR: https://gitlab.com/org/repo/-/merge_requests/1
Gate: failed pre-gate duration=567ms
Jobs:
  [1000] a1b2c3d4: succeeded
  [2000] e5f6g7h8: running
  [2500] i9j0k1l2: pending
```

The `[next_id]` ordering reflects execution sequence. Healing jobs inserted
dynamically appear with midpoint indices (e.g., 1500 between 1000 and 2000).

**API response:**

The `GET /v1/runs/{id}/status` endpoint returns `RunSummary` with:
- `stages` — Map of job ID (KSUID string) to `StageStatus` (state, next_id, attempts)
- `metadata["gate_summary"]` — Human-readable gate result
- `metadata["mr_url"]` — Merge request URL if created

See `internal/migs/api/types.go` for the full schema.

## Exit Codes

- `0` — success (run claimed, stages completed, workspace cleaned).
- `1` — error (missing flags, unsupported subcommand, stage failure, or
  downstream error).

## Environment
- `PLOY_RUNTIME_ADAPTER` — Optional runtime adapter selector. Defaults to
  `local-step`; other adapters (e.g., `k8s`) can register here and
  unknown names cause the CLI to fail fast.

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
