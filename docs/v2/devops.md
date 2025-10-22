# Deployment & Operations Guide

This guide describes how to bootstrap a Ploy v2 cluster and add additional nodes.
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
   - Create `ploy` user with sudo rights and `/var/lib/ploy` workspace.

2. **Install Dependencies**  
   - Install Docker, etcd, IPFS Cluster daemon.  
   - Configure systemd units for etcd and IPFS Cluster services.

3. **Run Bootstrap Script via CLI**  
   - Execute `ploy deploy bootstrap --config bootstrap.yaml` (or similar) from the workstation.  
   - Use `--dry-run` to preview the embedded script (`docs/v2/implement.sh`) before shipping it over SSH.  
   - The command runs preflight checks (package manager, disk at `${PLOY_WORKDIR:-/var/lib/ploy}`,
     and port availability) before installing Go 1.25.2, etcd 3.6.0, Docker 28.0.1, and IPFS
     Cluster 1.1.4.  
   - All binaries are pinned via static downloads inside `/usr/local/bin`, systemd units are
     refreshed, and logs summarise installed versions.  
   - The script confines temporary files to `${PLOY_WORKDIR}` and ensures Docker is enabled with
     a sane `daemon.json` default.
   - Example `bootstrap.yaml`:

     ```yaml
     host: beacon.example.com
     user: root
     identity_file: /home/operator/.ssh/ploy
     min_disk_gb: 80
     required_ports: [2379, 2380, 9094, 9095]
     ```

4. **Capture Cluster Metadata**  
   - The CLI copies the CA bundle to the workstation’s trust store and writes a cluster descriptor
     (beacon address, API key, CA path) under `${XDG_CONFIG_HOME}/ploy/clusters/<id>.json`.
   - Metadata includes the cluster version tag (retrieved via `GET /v2/version`) so the CLI can detect
     drift and prompt for refresh.
   - Enables fast reconnection via `ploy cluster connect --beacon-ip <ip> --api-key <key>` when joining
     an existing deployment; version mismatches trigger a metadata refresh.

5. **Configure Ploy CLI**  
   - Install `ploy` binary on operator workstation.  
   - Set environment variables (`PLOY_BEACON_URL`, `PLOY_CA_PATH`, GitLab API token in etcd).  
   - Run `ploy beacon promote` if beacon rotation is required.

6. **Verify**  
   - `ploy beacon status` (or API equivalent) to confirm healthy nodes list.  
   - `ploy status` to ensure etcd, IPFS, and Docker integrations respond.

## Adding a Worker Node

1. **Provision Host**  
   - Create `ploy` user and workspace.  
   - Install Docker, IPFS Cluster client, and join the cluster (pinning mirror).  
   - Install etcd client tools if needed.

2. **Deploy Runtime via CLI**  
   - Run `ploy node add <address> --config node.yaml` (or equivalent) from the workstation.  
   - The CLI reuses the embedded shell script to install dependencies, configure the node, register it
     with the beacon/etcd, and launch `ploynode` in worker mode.  
   - Confirm secrets/certificates delivered (TLS cert under `/etc/ploy/pki/`).

3. **Validation**  
   - `ploy node list` to verify status.  
   - Run a smoke Mod to confirm job submission, log streaming, and artifact uploads.

## Maintenance

- Rotate CA via `ploy beacon rotate-ca` and redeploy certificates to nodes.  
- Use `ploy logs job <job-id>` for debugging, and clean up old containers using node operations.  
- Monitor etcd health (`etcdctl endpoint status`) and IPFS Cluster pinning status regularly.
- Use `ploy config gitlab rotate --secret <name> --api-key <token> --scope <scope>` to push new GitLab credentials through the signer. The command talks to the control plane, writes the encrypted secret, and emits rotation events so workers refresh immediately.  
- Inspect signer health with `ploy config gitlab status [--secret <name>]`. The output includes audit feed metadata from the rotation revocation pipeline (last rotation, revoked nodes, recent failures) outlined in `.archive/gitlab-rotation-revocation/README.md`.  
- Stream `ploy jobs follow <job-id>` when closing out incidents; the final `Retention:` line echoes the job’s bundle CID, TTL, and expiry so teams can schedule inspections before GC removes the log bundle (see [docs/v2/logs.md](logs.md)).  
- For unattended rotations, provide the control-plane base URL via `PLOY_CONTROL_PLANE_URL` or ensure the active cluster descriptor contains the control plane endpoint and CA bundle so the CLI can authenticate requests.

This operational flow keeps Ploy nodes consistent and ensures the control plane remains
authoritative via etcd and beacon mode.
