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
  - Execute `dist/ploy cluster add --address <ip>` to prepare the first control-plane node. If you omit `--cluster-id`, the CLI tells the script to generate one (it writes the chosen ID to `/etc/ploy/cluster-id` and echoes it back so the descriptor can be cached on the workstation). When you pass `--cluster-id`, that exact value is forwarded as a positional flag (`--cluster-id <id>`), so re-running the script on the same host or bootstrapping a secondary node stays deterministic.
  - The CLI uploads the `ployd` binary (defaults to the executable found alongside the CLI; override with `--ployd-binary <path>`) and then streams the embedded bootstrap shell script over SSH. The only data sent alongside the script are the positional flags: `--cluster-id`, `--node-id`, `--node-address`, and `--primary` for the very first control-plane node. Everything else (package installs, TLS settings, etc.) is decided locally by the script.
  - Inside the host, the script converges dependencies, rewrites `/etc/ploy/ployd.yaml`, and installs the systemd unit. When `--primary` is set, it wipes the old config, enables HTTPS, and then invokes `ployd bootstrap-ca --cluster-id <id> --node-id <node> --address <addr>` before ployd ever starts listening.
  - No workstation TLS juggling is required: ployd writes the CA + leaf PEMs to `/etc/ploy/pki`, publishes them to etcd, and `ploy cluster add` only needs to keep the SSH descriptor so future commands know how to reach the host.
  - The rest of the flow is unchanged: the CLI verifies `etcd`/`ployd` are active, runs preflight checks (package manager, disk at `${PLOY_WORKDIR:-/var/lib/ploy}`, and port availability), and pins all binaries in `/usr/local/bin` so later upgrades are deterministic.
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

### TLS Bootstrap & Rotation

- **Primary bootstrap**
  - The CLI calls `bash -s -- --cluster-id <id> --node-id <node> --node-address <addr> --primary`, then steps aside.
  - The script wipes `/etc/ploy/ployd.yaml`, ensures HTTPS is turned on, and runs `ployd bootstrap-ca` locally so the CA + control-plane leaf cert are minted directly on the host and stored in etcd.
  - `/etc/ploy/cluster-id` is updated with the final cluster ID (generated or provided) so later maintenance commands can introspect the host without talking to the workstation.

- **Worker bootstrap (`ploy cluster add --cluster-id ...`)**
  - The CLI invokes the same script but only passes `--cluster-id <id>`; the script reuses the shared logic (installs dependencies, writes config, restarts ployd). No worker-specific mode exists—every node runs the same binary/config layout.
  - After the script finishes, the CLI registers the worker via `/v1/nodes`, receives the issued worker cert/key/CA bundle, and scps them into `/etc/ploy/pki`. It then rewrites `/etc/ploy/ployd.yaml`’s `control_plane.endpoint` to `https://<control-plane>:8443` and restarts the daemon so the node speaks TLS immediately.

- **Reissuing control-plane certificates**
  - Hitting `/v1/security/certificates/control-plane` (or the forthcoming `ploy cluster cert issue` wrapper) will mint a fresh control-plane certificate using the existing CA. Example:

    ```bash
    curl -sS -X POST https://<cp>:8443/v1/security/certificates/control-plane \
      -H "Authorization: Bearer ${PLOY_TOKEN}" \
      -H "Content-Type: application/json" \
      -d '{"cluster_id":"cluster-alpha","node_id":"control-primary","address":"45.9.42.212"}'
    ```

    The response mirrors `/v1/nodes`: `certificate` and `ca_bundle` can be written straight into `/etc/ploy/pki`, after which `systemctl restart ployd` brings the node back up with a fresh cert.
  - The API automatically bootstraps the CA if it does not exist, so reinstalling from scratch does not require extra manual steps.

- **Verification & rotation**
  - `ploy cluster cert status --cluster-id <id>` queries `/v1/security/ca` and shows the active CA version, serial, expiry, and node totals.
  - Because the CA is stored in etcd, re-running bootstrap on the same host simply reuses the latest CA and issues a new control-plane leaf certificate; operators can safely retry failed installs without worrying about orphaned trust roots.

4. **Capture Descriptor Metadata**  
   - On success the CLI writes `${XDG_CONFIG_HOME}/ploy/clusters/<cluster>.json` containing the SSH
     metadata plus the resolved control-plane endpoint and CA bundle so future commands can dial the
     API without exporting environment variables.
   - Subsequent commands feed this descriptor into `pkg/sshtransport`, which keeps persistent SSH
     tunnels alive for control-plane HTTP calls—no manual resolver or PEM juggling required.
   - `ploy cluster list` and the bootstrap output both surface the descriptor so operators can copy
     the join hint when onboarding workers.
   - Record the same descriptor information centrally via `/v1/config` so new workstations can seed
     their cache automatically. Example payload:

     ```json
     {
       "cluster_id": "cluster-alpha",
       "config": {
         "discovery": {
           "default_descriptor": "cluster-alpha",
           "descriptors": [
             {
               "cluster_id": "cluster-alpha",
               "address": "control.alpha.ssh",
               "api_endpoint": "https://control.alpha:8443",
               "ca_bundle": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
             }
           ]
         }
       }
     }
     ```

5. **Configure Ploy CLI**  
   - Install `ploy` binary on operator workstation.  
   - Set environment variables (`PLOY_CONTROL_PLANE_URL`, GitLab API token, etc.) only when you need
     overrides beyond what the descriptor already records (for example, pointing automation at a
     non-default cluster).  

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
  - The CLI first SSHes into the worker, uploads `ployd`, reruns the unified bootstrap script with `--cluster-id <id>`, and verifies the `ployd` service is active. After registration succeeds the CLI copies the issued worker certificate, key, and cluster CA into `/etc/ploy/pki`, rewrites the worker's `/etc/ploy/ployd.yaml` so it speaks HTTPS back to the control plane, and restarts the service. It then uses `pkg/sshtransport` to open a tunnel back to the control plane and calls `/v1/nodes` through that tunnel to register the worker metadata and record probe outcomes.  
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
- Use `PLOY_CONTROL_PLANE_URL` only when unattended tooling cannot reuse the cached descriptor; otherwise the CLI reads the endpoint and CA bundle directly from the descriptor written during bootstrap or published through `/v1/config`.  
- Use `GET /v1/config?cluster_id=<id>` to audit the active control-plane configuration. Apply updates with `PUT /v1/config` and an `If-Match` header (use `0` for initial creation, the last seen revision for updates, or `*` to override). Every write is recorded by Prometheus metrics such as `ploy_config_updates_total`.  
- `GET /v1/status?cluster_id=<id>` surfaces an aggregated view of queue depth and worker readiness; the response is intentionally uncached so dashboards can poll it directly.  
- `GET /v1/version` returns the build metadata (`version`, `commit`, `built_at`) served by the control plane and is safe to cache client-side for up to a minute.

## SSH Artifact Subsystem

The `/v1/transfers/*` APIs rely on a hardened SFTP subsystem running on every control-plane node. Set
it up immediately after bootstrap so `ploy upload`/`ploy report` can move data without opening fresh
SSH sessions per transfer.

### Directory layout

```bash
sudo mkdir -p /var/lib/ploy/ssh-artifacts/{slots,logs}
sudo chown -R ploy:ploy /var/lib/ploy/ssh-artifacts
sudo chmod 0750 /var/lib/ploy/ssh-artifacts /var/lib/ploy/ssh-artifacts/slots
sudo setfacl -m "g:ploy-artifacts:rx" /var/lib/ploy/ssh-artifacts
```

Use a dedicated Unix group (for example, `ploy-artifacts`) for principals allowed to enter the chroot.
Add the `ploy` service account plus any automation user that needs to service transfers into this
group.

### sshd configuration

Append the following snippet to `/etc/ssh/sshd_config` (or the drop-in under `/etc/ssh/sshd_config.d/`):

```
Subsystem ploy-artifacts internal-sftp

Match Group ploy-artifacts
    ChrootDirectory /var/lib/ploy/ssh-artifacts
    ForceCommand internal-sftp -d /slots/%u
    AllowTcpForwarding no
    X11Forwarding no
    PermitTTY no
```

- `ChrootDirectory` confines sessions to the artifact tree so the CLI cannot escape into the rest of
  the host.
- `ForceCommand` rewrites the working directory to `/slots/<username>`; the CLI uses the slot ID as the
  SSH username when copying files so each slot is isolated automatically.
- Keep ControlMaster sockets enabled globally so `pkg/sshtransport` can reuse them for both HTTP tunnels
  and SFTP copies.

Reload sshd after testing the new config with `sshd -t`.

### Guard helper (optional but recommended)

When you need additional validation (for example, to prevent writes outside `/slots/<slot-id>/payload`),
wrap `internal-sftp` with a lightweight guard script:

```bash
sudo tee /usr/local/libexec/ploy-artifacts-guard <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
slot="$1"
dest="/slots/${slot}/payload"
exec /usr/lib/openssh/sftp-server -d "$dest"
EOF
sudo chmod 0755 /usr/local/libexec/ploy-artifacts-guard
```

Then update the `ForceCommand` line to `ForceCommand /usr/local/libexec/ploy-artifacts-guard %u`. The
guard ensures each slot name resolves to a single directory and lets you insert extra validation (size
checks, audit logs) without editing every host’s sshd configuration.

### Cleanup and verification

- Register a `systemd-tmpfiles` rule (`D /var/lib/ploy/ssh-artifacts/slots 0750 ploy ploy - -`) so slot
  directories older than the 30‑minute TTL are purged automatically.
- Monitor `journalctl -t sshd | grep ploy-artifacts` to confirm uploads log the username (slot ID),
  transferred bytes, and remote peer.
- Add disk usage alerts for `/var/lib/ploy/ssh-artifacts`; stalled uploads can fill the filesystem if
  left unattended.
- Smoke-test the subsystem from a workstation:

  ```bash
  ploy upload --job-id smoke --kind repo ./fixtures/smoke.tar.gz
  ploy report --job-id smoke --output /tmp/smoke-report.tar.gz
  ```

  Both commands should complete through the cached descriptor without prompting for an SSH password.

## SSH Tunnel Lifecycle

- Descriptors are the canonical source for SSH addresses and identity files. Re-run `ploy cluster add
  --address ... --dry-run` (or edit the descriptor) whenever a host IP or key rotates so that future
  tunnels reuse the latest metadata.
- `pkg/sshtransport.Manager` keeps persistent tunnels alive for control-plane HTTP clients. Use
  `rm -rf ~/.config/ploy/clusters/<old>.json` or `ploy cluster list` to prune stale descriptors and
  avoid dangling sockets.
- When troubleshooting connectivity, inspect `~/.ploy/tunnels` (default control-socket directory) to
  confirm tunnels are recycling; deleting a socket and rerunning the CLI command forces a reconnect.
