# Deploy a Ploy Cluster (From Scratch)

This guide describes how to bootstrap a Ploy Next cluster and add additional `ployd` nodes on
Linux hosts (VPS or bare metal) over SSH.

## Prerequisites

- SSH access to all hosts with sudo privileges.
- Go 1.25.2 installed for building binaries.
- Docker Engine 28.0.1 (static binaries recommended).
- IPFS Cluster service 1.1.4 installed.
- etcd 3.6.0 available.

## Bootstrap Steps (Control Plane)

1. Provision host
   - Update OS packages.
   - Create user `ploy` with sudo rights; prepare `/var/lib/ploy` for the `ployd` runtime.

2. Install dependencies
   - Install Docker, etcd, and IPFS Cluster so `ployd` can launch jobs locally.
   - Create systemd units for etcd, IPFS Cluster, and `ployd`.

3. Run bootstrap via CLI
   - Execute `dist/ploy cluster add --address <ip>` to prepare the first control‑plane node.
     - Without `--cluster-id`, the script generates one and writes `/etc/ploy/cluster-id`.
     - With `--cluster-id`, the exact value is used to keep repeated runs deterministic.
   - The CLI uploads `ployd` (override with `--ployd-binary <path>`) and streams the embedded
     bootstrap script over SSH with flags: `--cluster-id`, `--node-id`, `--node-address`, and
     `--primary` (for the very first node).
   - On host: the script installs dependencies, writes `/etc/ploy/ployd.yaml`, installs the unit,
     and when `--primary` is set, enables HTTPS and runs
     `ployd bootstrap-ca --cluster-id <id> --node-id <node> --address <addr>`.
   - Certificates are minted on-host and written to `/etc/ploy/pki`; the CLI only keeps an SSH
     descriptor so future commands know how to reach the host.
   - Preflight checks verify package manager, `${PLOY_WORKDIR:-/var/lib/ploy}` disk, and port
     availability. Binaries are pinned in `/usr/local/bin` for deterministic upgrades.

   Example:

   ```bash
   dist/ploy cluster add --address 45.9.42.212
   ```

### TLS Bootstrap & Rotation

- Primary: CLI invokes the bootstrap with `--primary`; host wipes config, enables HTTPS, and runs
  `ployd bootstrap-ca` to mint CA + control‑plane leaf cert and persist to etcd.
- Workers: run `ploy cluster add --cluster-id <id>`; after install, the CLI registers the worker via
  `/v1/nodes`, scps issued certs to `/etc/ploy/pki`, rewrites
  `control_plane.endpoint=https://<control-plane>:8443`, and restarts `ployd`.

## Operations Tips

- Use `ploy config gitlab rotate --secret <name> --api-key <token> --scope <scope>` to rotate
  credentials; check with `ploy config gitlab status`.
- Follow job logs: `ploy jobs follow <job-id>`; final “Retention:” prints the bundle CID/TTL/expiry.
  See logs reference: [docs/next/logs.md](../next/logs.md).
- Prefer descriptors over `PLOY_CONTROL_PLANE_URL`; the CLI reads endpoint + CA from the descriptor
  (or from `/v1/config`).
- Control plane endpoints:
  - `GET /v1/config?cluster_id=<id>` to audit config; update with `PUT /v1/config` and `If-Match`.
  - `GET /v1/status?cluster_id=<id>` for queue depth and worker readiness (no-store).
  - `GET /v1/version` for build metadata (cache up to 60s).

## SSH Artifact Subsystem

The `/v1/transfers/*` APIs rely on an SFTP subsystem on each control‑plane node.

### Directory layout

```bash
sudo mkdir -p /var/lib/ploy/ssh-artifacts/{slots,logs}
sudo chown -R ploy:ploy /var/lib/ploy/ssh-artifacts
sudo chmod 0750 /var/lib/ploy/ssh-artifacts /var/lib/ploy/ssh-artifacts/slots
sudo setfacl -m "g:ploy-artifacts:rx" /var/lib/ploy/ssh-artifacts
```

### sshd configuration

```
Subsystem ploy-artifacts internal-sftp

Match Group ploy-artifacts
    ChrootDirectory /var/lib/ploy/ssh-artifacts
    ForceCommand /usr/local/libexec/ploy-slot-guard %u
    AllowTcpForwarding no
    X11Forwarding no
    PermitTTY no
```

Reload sshd after validating with `sshd -t`.

### Cleanup and verification

- Janitor in `ployd` sweeps expired slots and cleans directories.
- Add disk usage alerts for `/var/lib/ploy/ssh-artifacts`.
- Workstation smoke tests:

```bash
ploy upload --job-id smoke --kind repo ./fixtures/smoke.tar.gz
ploy report --job-id smoke --output /tmp/smoke-report.tar.gz
```

## SSH Tunnels

- Descriptors are canonical for SSH addresses and keys; re-run
  `ploy cluster add --address ... --dry-run` (or edit the descriptor) when IP/keys change.
- Persistent tunnels live under `~/.ploy/tunnels`; removing a socket forces reconnect.

