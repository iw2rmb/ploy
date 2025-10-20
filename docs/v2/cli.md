# Ploy CLI Reference

This catalog captures the initial Ploy v2 CLI surface. Commands stay focused on operator workflows:
running Mods locally, managing artifacts, and administering a lightweight Ploy node cluster.

## Mod Workflows

- `ploy mod plan` — Generate a Mods plan locally from the target repository for review before dispatch.
- `ploy mod run` — Submit a plan or prepared Mod definition to the scheduler and stream per-step status
  and build gate results.
- `ploy mod resume` — Operator-initiated recovery for Mods that were paused or cancelled (e.g., workstation restart).
  Automatic retries for failed steps are handled by the scheduler based on `--retry` and do not require this command.
- `ploy mod inspect <ticket>` — Display metadata, current step graph, logs, executing nodes, and artifact CIDs
  (diffs, build gate reports) for an active or completed Mod.
- `ploy mod logs <ticket>` — Stream Mod-level job logs (SSE) across all executed steps.

## Artifact Management

- `ploy artifact push <path>` — Upload a repository snapshot, diff bundle, or OCI image to the configured
  IPFS Cluster, returning the CID.
- `ploy artifact pull <cid> [--output <path>]` — Download an artifact by CID for local inspection or reruns.
- `ploy artifact list [--repo <name>]` — Enumerate artifacts known to the cluster, optionally scoped to a
  repository.
- `ploy artifact gc` — Trigger garbage collection policies (unpin unused CIDs, compact metadata) across participating nodes.

## Node Administration

- `ploy node add <address>` — Register a new node via the beacon, issue credentials, and add it to the
  scheduler rotation.
- `ploy node remove <node-id>` — Drain and deregister a node, revoking credentials and clearing workload assignments.
- `ploy node list` — Show registered nodes with health, capabilities (Docker, SHIFT, IPFS), and workload counts.
- `ploy node heal <node-id>` — Run built-in diagnostics and attempt automated remediation (restart SHIFT, resync IPFS pins).
- `ploy node logs <node-id>` — Stream daemon logs from a specific node over SSE (includes scheduler,
  SHIFT, Docker wrapper).

## Cluster Management

- `ploy deploy bootstrap [--config <file>]` — Stand up or extend a Ploy cluster using the embedded shell
  script (installs dependencies, configures beacon mode, generates CA, records consent).
- `ploy cluster connect --beacon-ip <addr> --api-key <key>` — Trust and cache cluster metadata (including
  CA and version tag fetched from `/v2/version`) for an existing deployment.
- `ploy cluster list` — Show locally cached clusters with version tags and last refresh time.
  Cluster descriptors stored locally take precedence over ad-hoc flags when present.

## Beacon Operations

- `ploy beacon promote <node-id>` — Elect or re-elect the designated beacon node when rotating hosts.
- `ploy beacon rotate-ca` — Regenerate the cluster CA and distribute new certificates to nodes and workstations.

## Cluster Bootstrap & Configuration

- `ploy deploy bootstrap [--config <file>]` — Stand up or extend a Ploy cluster:
  initializes etcd, beacon, IPFS Cluster peers, and installs the CA bundle.
- `ploy deploy upgrade [--version <tag>]` — Roll out binary/configuration updates to nodes with
  coordinated drains.
- `ploy config set <key> <value>` — Persist cluster-scoped settings in etcd (e.g., `gitlab.api_key`,
  IPFS endpoints, feature flags). These values apply to all nodes once written.
- `ploy config show` — Display the effective configuration sourced from the beacon (cluster-scoped)
  merged with any local overrides (cluster descriptors cached via `ploy cluster connect`).

## Observability & Support

- `ploy status` — Summarize cluster health, active Mods, build gate pass/fail counts, and artifact throughput.
- `ploy logs <node-id>` — Alias for `ploy node logs`; tails daemon logs via SSE.
- `ploy logs job <job-id>` — Stream stdout/stderr for an individual job (Mod step or build gate run) over SSE.
- `ploy beacon sync` — Force-refresh the local discovery cache, DNS stub, and trust material from the beacon.
- `ploy doctor` — Run workstation diagnostics (Docker availability, CA install, IPFS connectivity) and suggest fixes.
