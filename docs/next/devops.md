# Deployment & Operations Guide

This guide describes how to bootstrap a Ploy Next cluster and add additional
`ployd` nodes.
It assumes Linux hosts (VPS or bare metal) with SSH access.

## Prerequisites

- SSH access to all hosts with sudo privileges.
- Go 1.25.2 installed for building binaries.
- Docker Engine 28.0.1 (deployed via static binaries).
- IPFS Cluster service 1.1.4 installed.
- etcd 3.6.0 binary or package available.

## Bootstrap Steps (Beacon + Control Plane)

1. **Provision Host**  
   - Ensure OS packages are up to date.  
   - Create `ploy` user with sudo rights and `/var/lib/ploy` workspace for the
     `ployd` service runtime.

2. **Install Dependencies**  
   - Install Docker, etcd, and the IPFS Cluster daemon so `ployd` can launch
     jobs locally.  
   - Configure systemd units for etcd, IPFS Cluster, and prepare the `ployd`
     service definition.

3. **Run Bootstrap Script via CLI**  
  - Execute `dist/ploy cluster add --address <ip>` to prepare the first control-plane node. Omitting
    `--cluster-id` signals primary-node bootstrap, so the CLI copies `ployd`, renders configs, and
    caches the descriptor locally. The command automatically generates a 16-character lowercase-hex
    cluster identifier (persisted as the default descriptor) and a 4-character node identifier.
  - The bootstrap command installs `ployd` in control-plane mode; descriptors now carry only the SSH
    metadata needed for future tunnels rather than beacon URLs or CA material.
  - The CLI uploads the `ployd` binary (defaults to the executable found alongside the CLI; override with `--ployd-binary <path>`) and then streams the embedded bootstrap shell script over SSH. The script converges dependencies, writes the initial `/etc/ploy/ployd.yaml`, and installs the systemd unit.
  - Once the script completes, the CLI verifies `etcd` and `ployd` are active via `systemctl` before continuing.
  - On first start, `ployd` automatically creates the cluster certificate authority and records it in etcd—no manual CA bootstrap step is required.
  - The command runs preflight checks (package manager, disk at `${PLOY_WORKDIR:-/var/lib/ploy}`,
    and port availability) before installing Go 1.25.2, etcd 3.6.0, Docker 28.0.1, and IPFS
    Cluster 1.1.4.  
  - The CLI derives a cluster identifier and records the SSH target in the descriptor; configure DNS
    only if you prefer names over raw IPs.
  - SSH defaults to `root@<address>` with identity file `~/.ssh/id_rsa`, and the same identity is reused for both SSH and SCP transfers so there is no need to inject additional authorized-key payloads or `PLOY_SSH_ADMIN_KEYS_B64` values.  
  - The minimum disk check defaults to 4 GiB and is enforced automatically; ensure hosts satisfy it before running the bootstrap.
  - All binaries are pinned via static downloads inside `/usr/local/bin`, systemd units are
    refreshed, and logs summarise installed versions.  
  - etcd is installed as a systemd service bound to `127.0.0.1:{2379,2380}` so the CLI can finish
    bootstrap writes without exposing client ports publicly.
  - The script confines temporary files to `${PLOY_WORKDIR}` and ensures Docker is enabled with
    a sane `daemon.json` default.
  - Example command with overrides:

    ```bash
    dist/ploy cluster add \
      --address 45.9.42.212
    ```

4. **Capture Descriptor Metadata**  
   - On success the CLI writes `${XDG_CONFIG_HOME}/ploy/clusters/<cluster>.json` containing only the
     SSH metadata required to re-open tunnels (cluster ID, address/port, identity path, labels).
   - Subsequent commands feed this descriptor into `pkg/sshtransport`, which keeps persistent SSH
     tunnels alive for control-plane HTTP calls—no CA bundles or resolver entries are required.
   - `ploy cluster list` and the bootstrap output both surface the descriptor so operators can copy
     the join hint when onboarding workers.

5. **Configure Ploy CLI**  
   - Install `ploy` binary on operator workstation.  
   - Set environment variables (`PLOY_CONTROL_PLANE_URL`, GitLab API token, etc.) only when you need
     overrides beyond what the descriptor already records.  

6. **Verify**  
   - `ploy status` to ensure etcd, IPFS, and Docker integrations respond.

## Adding a Worker Node

1. **Provision Host**  
   - Create `ploy` user and workspace.  
   - Install Docker, IPFS Cluster client, and join the cluster (pinning mirror).  
   - Install etcd client tools if needed.

2. **Deploy Runtime via CLI**  
  - Run `ploy cluster add --cluster-id <cluster> --address <host-or-ip>`. Use `--user`, `--identity`, `--ssh-port`, or `--ployd-binary` if the defaults (`root`, `~/.ssh/id_rsa`, `22`, CLI-adjacent `ployd`) are unsuitable.  
   - The CLI derives the target cluster from the default cached descriptor (created during bootstrap)
     and generates a 4-character worker identifier automatically.
   - Provide at least one health endpoint using `--health-probe name=https://<addr>:9443/healthz`; multiple probes are allowed.  
  - The CLI first SSHes into the worker, uploads `ployd`, reruns the unified bootstrap script, and verifies the `ployd` service is active. It then uses `pkg/sshtransport` to open a tunnel back to the control plane and calls `/v1/nodes` through that tunnel to register the worker metadata and record probe outcomes.  
   - Use `--dry-run` to preview probes without modifying etcd; the command still validates SSH access and prints the registration payload so you can audit the request before running it for real.  
   - Confirm the worker shows up via `etcdctl get /ploy/clusters/<cluster>/registry/workers --prefix --keys-only` or `ploy cluster list --labels`.

3. **Validation**  
   - `etcdctl get /ploy/clusters/<cluster>/registry/workers --prefix --keys-only` to confirm the descriptor exists.  
   - Run a smoke Mod to confirm job submission, log streaming, and artifact uploads.

## Maintenance

- Monitor etcd health (`etcdctl endpoint status`) and IPFS Cluster pinning status regularly.  
- Use `ploy cluster cert status [--cluster-id <id>]` to confirm the active CA version, expiry, and node counts exposed by `/v1/security/ca`. The command automatically targets the default descriptor when `--cluster-id` is omitted.  
- Use `ploy config gitlab rotate --secret <name> --api-key <token> --scope <scope>` to push new GitLab credentials through the signer; follow up with `ploy config gitlab status` to confirm rotation state.  
- Stream `ploy jobs follow <job-id>` when closing out incidents; the final `Retention:` line echoes the job’s bundle CID, TTL, and expiry so teams can schedule inspections before GC removes the log bundle (see [docs/next/logs.md](logs.md)).  
- Provide the control-plane base URL via `PLOY_CONTROL_PLANE_URL` (or rely on the active cluster descriptor) so unattended tooling can authenticate control-plane requests.  
- Use `GET /v1/config?cluster_id=<id>` to audit the active control-plane configuration. Apply updates with `PUT /v1/config` and an `If-Match` header (use `0` for initial creation, the last seen revision for updates, or `*` to override). Every write is recorded by Prometheus metrics such as `ploy_config_updates_total`.  
- `GET /v1/status?cluster_id=<id>` surfaces an aggregated view of queue depth and worker readiness; the response is intentionally uncached so dashboards can poll it directly.  
- `GET /v1/version` returns the build metadata (`version`, `commit`, `built_at`) served by the control plane and is safe to cache client-side for up to a minute.

## SSH Tunnel Lifecycle

- Descriptors are the canonical source for SSH addresses and identity files. Re-run `ploy cluster add
  --address ... --dry-run` (or edit the descriptor) whenever a host IP or key rotates so that future
  tunnels reuse the latest metadata.
- `pkg/sshtransport.Manager` keeps persistent tunnels alive for control-plane HTTP clients. Use
  `rm -rf ~/.config/ploy/clusters/<old>.json` or `ploy cluster list` to prune stale descriptors and
  avoid dangling sockets.
- When troubleshooting connectivity, inspect `~/.ploy/tunnels` (default control-socket directory) to
  confirm tunnels are recycling; deleting a socket and rerunning the CLI command forces a reconnect.
