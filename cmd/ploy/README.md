# Ploy Workflow CLI

`ploy` is the operator CLI for the Ploy control plane. It submits runs, manages
mig projects, follows run and job status, applies run diffs locally, downloads
run artifacts, and administers nodes and tokens. Global environment management is
available under `ploy config env`.

## Usage

The CLI uses Cobra for command structure and help. To see available commands:

```bash
ploy --help                    # List all top-level commands
ploy <command> --help          # Get help for a specific command
ploy help <command>            # Alternative help syntax
```

Common command patterns:

```bash
ploy run <spec-path>[:<step-name>] [<repo-path>|<namespace/repo[:ref]>] [--follow] [--apply] [--pull[=path]] # submit a single-repo run
ploy run status <run-id> [--json|--follow]                                  # inspect a run
ploy run apply <run-id> [path] [--force]                                     # apply a run patch locally
ploy run pull <run-id> [artifacts-path]                                      # download final run artifacts
ploy job status <job-id>                                                     # inspect one job as JSON
ploy mig run <mig-id|name> [<namespace/repo[:ref]> ...] [--failed] [--follow] # execute a mig project over its repo set
ploy spec schema                                                             # print the mig JSON Schema
ploy spec validate docs/schemas/mig.example.yaml                             # validate a mig spec
```

Run IDs (`<run-id>`) are KSUID-backed strings.
Treat them as opaque identifiers when passing them between commands or scripts.

`ploy run` submits a spec file or directory against one repository source. Add
`:<step-name>` to submit only one named step from the expanded spec. Local repo
paths submit `HEAD`; remote selectors use `namespace/repo`, optionally suffixed
with `:<branch>` or `:<sha>`. Use `--follow` to wait for the run's terminal
status, `--pull[=path]` to wait for success and download artifacts, or `--apply`
to wait for success and apply the resulting patch to a clean local worktree.

`ploy mig run` executes an existing mig project over its managed repo set. Use
`ploy mig add --name <name> --spec <path>`, `ploy mig repo add`, and
`ploy mig spec set` to manage the project before running it.

When follow mode is used, the CLI displays a summarized per-repo job graph that
refreshes until the run reaches a terminal state. The job graph shows step index,
job type, job ID, display name, status glyph, duration, and status for each job.
For running jobs, follow mode also shows `STD[O]UT` and `STD[E]RR` preview rows
(collapsed by default). Press `o` to expand/collapse stdout previews for all
currently running jobs, and `e` for stderr previews. Failed jobs are kept expanded.
Note: `--follow` does not stream container logs. Use `ploy job log --follow <job-id>` for container log streaming.
Use `ploy job status <job-id>` to print the current job row and execution fields
as JSON for investigation scripts.

For `ploy mig run --follow`, `--cap` enforces an overall time limit. If exceeded,
the CLI exits follow mode; add `--cancel-on-cap` to also cancel the run.

When a `ploy run` submission completes successfully, pass `--pull[=<dir>]` to
download referenced artifacts and generate `<dir>/manifest.json`. The manifest
lists artifacts with `stage`, `name`, `cid`, `digest`, `size` (bytes written),
and the local `path`. Filenames are sanitized and deterministic; when a content
digest is available it prefixes the name, otherwise the artifact CID is used.

## Mig Projects

Mig projects are long-lived containers with a unique name, a current spec, and a managed repo set.

```bash
# Create a mig project.
ploy mig add --name my-mig --spec mig.yaml

# Update the mig's current spec.
ploy mig spec set my-mig mig.yaml

# Manage the mig's repo set.
ploy mig repo add my-mig org/repo-a:main
ploy mig repo add my-mig org/repo-b:main
ploy mig repo list my-mig

# Execute the mig project (all repos by default).
ploy mig run my-mig

# Execute only specific repos.
ploy mig run my-mig org/repo-a:main org/repo-b:main

# Re-run only repos whose last terminal state is Fail.
ploy mig run my-mig --failed
```

## Run Commands

For the long-lived current-state reference, see `docs/runs.md`.

```bash
# Submit from the current git worktree.
ploy run ./mig.yaml

# Submit a selected local repo path.
ploy run ./mig.yaml ../service-a

# Submit a remote repo selector. Missing ref defaults to master.
ploy run ./mig.yaml org/service-a:main

# Follow until terminal.
ploy run ./mig.yaml org/service-a:main --follow

# Follow until success and download artifacts.
ploy run ./mig.yaml org/service-a:main --pull=./artifacts

# Apply a completed run to the current repo.
ploy run apply <run-id>

# Download artifacts for an existing run.
ploy run pull <run-id> ./artifacts
```

## Mig Project Runs

`mig run` executes an existing mig project over all repos in its managed set, an
explicit subset, or repos whose last terminal state was `Fail`.

```bash
ploy mig add --name java17 --spec mig.yaml
ploy mig repo add java17 org/repo-a:main
ploy mig repo add java17 org/repo-b:main

# Execute all repos.
ploy mig run java17 --follow

# Execute only selected repos.
ploy mig run java17 org/repo-a:main

# Re-run only repos whose last terminal state is Fail.
ploy mig run java17 --failed
```

### Mig Workflow Summary

| Command                  | Description                                   |
|--------------------------|-----------------------------------------------|
| `mig add --name <name>`  | Create a mig project                          |
| `mig repo add`           | Add a repository to a mig project             |
| `mig run <mig>`          | Run a mig project                             |
| `run apply <run-id>`     | Apply diffs for the current repo from a run   |
| `run pull <run-id>`      | Download final artifacts for a run            |
| `mig pull [<mig>]`       | Pull diffs for the current repo from a mig    |
| `run status --follow`    | Follow run status until terminal              |
| `job status <job-id>`    | Print one job status as JSON                  |

See `docs/migs-lifecycle.md` for the relationship between waves, runs, and jobs.

### Pull Migs Changes Locally

After a run completes, you can apply the Migs-generated changes into your local
repository using either `ploy run apply <run-id>` (run-based) or `ploy mig pull` (mig-based).
These commands apply stored diffs from the control plane to the current worktree.

```bash
# From a repo that participated in a Migs run:
cd service-a

# Run-based apply (you know the run_id):
ploy run apply <run-id>

# Mig-based pull (default: last succeeded):
ploy mig pull <mig-id|name>
```

**How it works:**
1. Derives the current repo identity from the git remote (default: `origin`).
2. Verifies the working tree is clean (no uncommitted changes).
3. Resolves the run via `POST /v1/runs/{run_id}/pull` (or `POST /v1/migs/{mig_id}/pull` for mig-based pull).
4. Fetches repo details and verifies local `HEAD` matches the run's `source_commit_sha`.
5. Downloads and applies all stored Migs diffs via `git apply`.

**Arguments:**
- `<run-id>` — Run ID (KSUID string), for `ploy run apply`.
- `[<mig-id|name>]` — Mig ID or name (optional), for `ploy mig pull`.

**Flags:**
- `--force` — For `ploy run apply`, allow local `HEAD` to differ from the run source SHA.
  This never bypasses dirty worktree checks.

**Examples:**

```bash
# Apply changes from a run ID.
ploy run apply <run-id>

# Download final run artifacts.
ploy run pull <run-id> [artifacts-path]

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

## Interactive TUI (`ploy tui`)

`ploy tui` launches a full-screen terminal UI for browsing migrations, runs, and jobs
without chaining multiple CLI commands. It opens in alternate screen mode.

```bash
ploy tui
```

### Screens and navigation

| Screen | Title | Description |
|--------|-------|-------------|
| Root | `PLOY` | Root selector. Choose Migrations, Runs, or Jobs. |
| Migrations list | `PLOY \| MIGRATIONS` | Two side-by-side columns (`PLOY` + `MIGRATIONS`) with list height matched to terminal height; migrations are ordered newest-to-oldest. |
| Migration details | `MIGRATION <name>` | Migration detail: repository count and run count. |
| Runs list | `PLOY \| RUNS` | Two side-by-side columns (`PLOY` + `RUNS`) with list height matched to terminal height; runs are ordered newest-to-oldest with `DD MM HH:mm` timestamp. |
| Run details | `RUN` | Run detail: repository count and job count. |
| Jobs list | `PLOY \| JOBS` | Two side-by-side columns (`PLOY` + `JOBS`) with list height matched to terminal height; each row shows job, mig name, run id, and repo id. |

**Keys:**
- `Enter` — drill into the selected item.
- `Esc` — return to the previous screen.
- `q` — quit.

The TUI uses the same `PLOY_SERVER_URL` and optional `PLOY_AUTH_TOKEN` environment
variables as all other `ploy` commands (see `docs/envs/README.md`).

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

- Mig spec files use canonical `steps[]` shape for both single-step and
  multi-step runs. Each step supports
  `image`/`command`/`envs` plus Hydra file-record fields (`in`, `out`, `home`)
  for deterministic file injection via content-addressed bundles.
  CLI-authored specs can use `steps[].ref` entries such as
  `ref: ../shared/mig.yaml:deprecations`; refs are expanded before submission
  and only the selected step is imported. A ref wrapper may also declare
  `envs`; those envs are merged into the imported step and win on key conflicts.
  See `docs/schemas/mig.example.yaml` for the full schema and
  `tests/e2e/migs/README.md` for usage examples.
- `ploy run --pull[=path]` and `ploy run pull <run-id> [path]` download run
  artifacts and write `manifest.json`.
- `ploy mig run --cap <duration>` applies only with `--follow`. When the duration
  elapses, the CLI stops following; use `--cancel-on-cap` to cancel the run too.

## Global Environment Configuration

The `ploy config env` commands manage global environment variables that are automatically
injected into target components. This provides a centralized way to configure
credentials and other shared settings without embedding them in every spec file.

### Key Concepts

**Targets** control which components receive each variable:
- `server` — Inject into the server process
- `nodes` — Inject into node agent processes
- `gates` — Inject into gate jobs (`pre_gate`, `post_gate`)
- `steps` — Inject into step jobs (`mig`)

The `set` command uses **`--on` selectors** for convenience:
- `all` → server, nodes, gates, steps (all targets)
- `jobs` → gates, steps (default)
- `server`, `nodes`, `gates`, `steps` → single target

The `show` and `unset` commands use **`--from`** to specify the target when a key
exists for multiple targets. When omitted and the key is unambiguous (single target),
the target is inferred automatically.

**Secrets** are redacted in list/show output by default. Use `--raw` with `show` to reveal the
full value.

**Precedence**: Per-run env vars in spec files take precedence over global env.
Existing keys in the spec are never overwritten by global config.

### Commands

```bash
# List all global environment variables (secret values redacted)
ploy config env list

# Show a specific variable (use --raw to reveal secret values)
ploy config env show --key OPENAI_API_KEY --raw

# Set a variable with an inline value (default --on jobs → gates, steps)
ploy config env set --key OPENAI_API_KEY --value sk-...

# Set a non-secret variable (visible in list output)
ploy config env set --key CUSTOM_VAR --value myvalue --on gates --secret=false

# Delete a variable (use --from when key exists for multiple targets)
ploy config env unset --key OLD_VAR

```

### Common Variables

| Variable / Field | Description | Recommended Target |
|------------------|-------------|-------------------|
| `OPENAI_API_KEY` | OpenAI API key for LLM-integrated migs | `jobs` |

See `docs/envs/README.md` § "Global Env Configuration" for detailed semantics and
`docs/migs-lifecycle.md` for how these variables flow into job containers.

## Build Gate

Build Gate runs as regular jobs in the unified `jobs` queue. Gate jobs are
claimed and executed by nodes through the same claim loop as other run jobs.

**CLI-visible gate summaries:**
Gate results are surfaced via `GET /v1/runs/{id}/status`:
- `Gate: passed duration=1234ms`
- `Gate: failed pre-gate duration=567ms`

See `docs/build-gate/README.md` for current gate contract and execution details.

## Job Graph And DAG State

Migs runs execute as a directed acyclic graph (DAG) of jobs. The graph structure
surfaces via `ploy run status <run-id> --json` and
`GET /v1/runs/{id}/status`. Run status includes a `stages` map. Each job has a
`next_id` for execution ordering and optional metadata identifying the job phase.

**Job phases (job_type):**
- `pre_gate` — Build Gate validation before migs run
- `mig` — Main mig execution (code transformation)
- `post_gate` — Build Gate validation after migs succeed

**DAG structure:**

```
pre-gate → mig-0 → post-gate
```

**Status inspection:**

Use `ploy run status <run-id> --json` or `GET /v1/runs/{id}/status` to view
run-level state:

```bash
$ curl -sk "$PLOY_CONTROL_PLANE_URL/v1/runs/migs-abc123/status" | jq .
Run migs-abc123: running
Gate: failed pre-gate duration=567ms
Jobs:
  [1000] a1b2c3d4: succeeded
  [2000] e5f6g7h8: running
  [2500] i9j0k1l2: pending
```

The `[next_id]` ordering reflects execution sequence.

**API response:**

The `GET /v1/runs/{id}/status` endpoint returns `RunSummary` with:
- `stages` — Map of job ID (KSUID string) to `StageStatus` (state, next_id, attempts)
- `metadata["gate_summary"]` — Human-readable gate result

See `internal/migs/api/types.go` for the full schema.

## Exit Codes

- `0` — command completed successfully.
- `1` — command failed, including argument errors, unknown commands, API errors,
  run/job failures surfaced by follow mode, or downstream tool errors.

## Development

- Build via `make build` (outputs to `dist/ploy`).
- Run unit tests with `make test`.
- Run coverage with `make test-coverage` or `make coverage-all`.
- See `docs/schemas/mig.example.yaml` for the current mig spec shape.
- Review `docs/runs.md`, `docs/migs-lifecycle.md`, and `docs/envs/README.md`
  when changing command behavior or durable CLI documentation.
