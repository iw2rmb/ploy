# Ploy CLI Reference

The Ploy CLI exposes workflow execution, artifact management, and workstation
operations for the v2 control plane. Commands mirror the structure captured in
`../design/cli-command-tree/README.md` and the generated shell completions.

## Workflow Management

- `ploy workflow cancel --tenant <tenant> --run-id <run-id> [--workflow <workflow-id>] [--reason <text>]`
  Cancel an in-flight workflow run and surface workflow control plane errors.

## Mod Workflows

- `ploy mod run [flags]`
  Dispatch a Mod ticket to the control plane, materialise repositories, and
  stream step checkpoints. See `ploy mod run --help` for the full flag catalog
  (tenant, ticket, repository materialisation, planner hints).

## Observability Commands

- `ploy mods logs [--format structured|raw] <ticket>`
  Follow aggregated Mod logs over SSE with configurable retry semantics.
- `ploy jobs follow [--format structured|raw] <job-id>`
  Stream a single job's logs over SSE with the same reconnect behaviour.

## Artifact Management

- `ploy artifact push [--name <name>] [--kind <diff|logs>] [--replication-min <n>] [--replication-max <n>] [--local] <path>`
  Upload an artifact to the configured IPFS Cluster.
- `ploy artifact pull [--output <path>] <cid>`
  Download an artifact and optionally persist it to disk.
- `ploy artifact status <cid>`
  Inspect replication state and pin health.
- `ploy artifact rm <cid>`
  Unpin an artifact from the cluster.

## Cluster Descriptors

- `ploy cluster connect --beacon-ip <addr> --api-key <key>`
  Cache beacon metadata and trust bundles locally.
- `ploy cluster list`
  List cached cluster descriptors and their last refresh time.

## Configuration (GitLab)

- `ploy config gitlab show`
  Display the current GitLab integration configuration.
- `ploy config gitlab set --file <path>`
  Apply a GitLab configuration JSON document.
- `ploy config gitlab validate --file <path>`
  Validate a configuration file without persisting it.
- `ploy config gitlab status [--limit <n>]`
  Inspect signer health, revision history, and rotation audit entries.
- `ploy config gitlab rotate --secret <name> --api-key <token> [--scope <scope> ... | --scopes <scope,...>]`
  Rotate a GitLab secret and trigger node refresh.

## Snapshot Management

- `ploy snapshot plan --snapshot <snapshot-name>`
  Preview strip, mask, and synthetic rules for a snapshot definition.
- `ploy snapshot capture --snapshot <snapshot-name> --tenant <tenant> --ticket <ticket-id>`
  Execute a snapshot capture and publish the resulting metadata.

## Environment Materialisation

- `ploy environment materialize <commit-sha> --app <app> --tenant <tenant> [--dry-run] [--manifest <name@version>] [--aster <toggle,...>]`
  Plan or hydrate integration environments for the specified commit.

## Manifest Tooling

- `ploy manifest schema`
  Print the integration manifest JSON schema.
- `ploy manifest validate [--rewrite=v2] <path> [<path> ...]`
  Validate one or more manifests and optionally rewrite them to v2.

## Knowledge Base Commands

- `ploy knowledge-base ingest --from <fixture.json>`
  Append incidents to the workstation knowledge base catalog.
- `ploy knowledge-base evaluate --fixture <samples.json>`
  Evaluate classifier accuracy using curated samples.

## Shell Completion Assets

Updated completion scripts live under `cmd/ploy/autocomplete/`:

- `ploy.bash` for bash
- `ploy.zsh` for zsh
- `ploy.fish` for fish

Regenerate the scripts with `go run ./tools/autocomplete`. The command tree
source of truth remains `../design/cli-command-tree/README.md`, and the
completion generator consumes the definitions recorded there.
