# Ploy CLI Reference

The Ploy CLI exposes workflow execution, artifact management, and workstation
operations for the v2 control plane. Commands mirror the structure captured in
`../design/cli-command-tree/README.md` and the generated shell completions.

## Workflow Management

- `ploy workflow cancel --run-id <run-id> [--workflow <workflow-id>] [--reason <text>]`
  Cancel an in-flight workflow run and surface workflow control plane errors.

## Mod Workflows

- `ploy mod run [flags]`
  Dispatch a Mod ticket to the control plane, materialise repositories, and
  stream step checkpoints. See `ploy mod run --help` for the full flag catalog
  (ticket, repository materialisation, planner hints).

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

## SSH Transfer Commands

- `ploy upload --job-id <id> [--kind repo|logs|report] [--node-id <node>] <path>`
  Requests an upload slot via `/v1/transfers/upload`, copies the payload over the cached SSH
  descriptor, and commits the slot so the control plane can ingest the bytes into IPFS Cluster or the
  registry backend.
- `ploy report --job-id <id> [--artifact-id <slot>] [--node-id <node>] --output <path>`
  Reserves a download slot tied to the latest artifact for a job, pulls the staged file via SSH, and
  verifies the digest before writing it locally.

Both commands require `PLOY_CONTROL_PLANE_URL` (or a descriptor that already embeds the API endpoint)
and reuse the same SSH sockets as the rest of the CLI. See
[docs/next/ssh-transfer-migration.md](ssh-transfer-migration.md) for the full workflow and migration
guidance.

## Cluster Descriptors

Descriptors now encode only the SSH metadata needed to open tunnels:

```json
{
  "cluster_id": "lab",
  "address": "203.0.113.10",
  "ssh_identity_path": "/home/dev/.ssh/id_ed25519",
  "labels": {
    "role": "control-plane",
    "env": "staging"
  }
}
```

- `ploy cluster list`
  Show the cached descriptors with their SSH target, identity path, and labels so operators can confirm which key will be reused for future commands.

- `ploy cluster add --address <host> [--cluster-id <id>] [--identity <path>] [--label key=value] [--health-probe name=url] [--dry-run]`
  Bootstrap the first control-plane node by omitting `--cluster-id`; the CLI copies `ployd`, renders configs, and caches the descriptor locally.
  Provide `--cluster-id` to add workers over SSH tunnels using the descriptor metadata, optionally tagging nodes via `--label` and previewing the flow with `--dry-run`.

## Configuration (GitLab)

- `ploy config gitlab show [--cluster-id <id>]`
  Display the current GitLab integration configuration for the selected cluster (defaults to the cached descriptor).
- `ploy config gitlab set --file <path> [--cluster-id <id>]`
  Apply a GitLab configuration JSON document to the targeted cluster.
- `ploy config gitlab validate --file <path>`
  Validate a configuration file without persisting it.
- `ploy config gitlab status [--limit <n>] [--cluster-id <id>]`
  Inspect signer health, revision history, and rotation audit entries.
- `ploy config gitlab rotate --secret <name> --api-key <token> [--scope <scope> ... | --scopes <scope,...>] [--cluster-id <id>]`
  Rotate a GitLab secret and trigger node refresh on the selected cluster.

When no local descriptor exists for the requested cluster, these commands read the `discovery`
block returned by `/v1/config` to seed `${XDG_CONFIG_HOME}/ploy/clusters/<cluster>.json`, ensuring
future calls reuse the SSH and CA metadata without additional environment variables.

 

## Environment Materialisation

- `ploy environment materialize <commit-sha> --app <app> [--dry-run] [--manifest <name@version>] [--aster <toggle,...>]`
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
