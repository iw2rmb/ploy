# SSH Transfer Migration Guide

SSH transfer slots replace direct IPFS uploads from workstations. This guide walks through adopting
`/v1/transfers/*`, the new `ploy upload`/`ploy report` commands, and the supporting SFTP subsystem.

## Overview

1. Upgrade the CLI and cached descriptors so every workstation knows where to send control-plane HTTP
   requests.
2. Enable the SSH artifact subsystem on control-plane nodes (directories, sshd configuration, optional
   guard helper, cleanup units).
3. Update automation and environment variables to point at the control plane instead of the IPFS
   gateway.
4. Backfill any artifacts that still live exclusively on operators’ laptops.
5. Cut over day-to-day workflows and monitor slot metrics.

## Prerequisites

- Ploy CLI built from the current main branch (includes `ploy upload/report`).
- Control-plane nodes reachable via SSH with the descriptors recorded under
  `${XDG_CONFIG_HOME:-$HOME/.config}/ploy/clusters/`.
- `PLOY_CONTROL_PLANE_URL` exported (or retrievable from the active descriptor) so the CLI and scripts
  can reach `/v1/transfers/*`.
- Service accounts with `artifact.write` / `artifact.read` scopes for automation.

## Step 1 — Refresh CLI and descriptors

1. Rebuild or download the latest `dist/ploy` binary and redistribute it to operators.
2. Verify each workstation still has a valid descriptor:

   ```bash
   ploy cluster list
   ploy cluster add --address <control-plane-ip> --dry-run  # refreshes metadata without mutating the host
   ```

3. Confirm `PLOY_CONTROL_PLANE_URL` resolves either from the environment or via the GitLab config API
   (`ploy config gitlab show`).

## Step 2 — Enable the SFTP subsystem

Follow [docs/how-to/deploy-a-cluster.md](../how-to/deploy-a-cluster.md#ssh-artifact-subsystem) to create
`/var/lib/ploy/ssh-artifacts`, add the `ploy-artifacts` group, and update `sshd_config` with the
`Subsystem ploy-artifacts internal-sftp` stanza. Reload sshd and run a smoke upload/report cycle from
an operator workstation.

## Step 3 — Update environment and automation

- Workstations no longer need to export IPFS credentials; `ploy artifact *`, `ploy upload`, and
  `ploy report` route everything through the control plane. Keep `PLOY_IPFS_CLUSTER_*` on the nodes so
  ployd can publish artifacts server-side.
- Ensure `PLOY_SSH_USER`, `PLOY_SSH_IDENTITY`, and `PLOY_SSH_SOCKET_DIR` (if overridden) match the
  principals configured in sshd. The defaults (`ploy`, `~/.ssh/id_rsa`, `~/.ploy/tunnels`) are often
  sufficient.
- Update any scripts that previously called `ploy artifact push/pull` to use:

  ```bash
  ploy upload --job-id $TICKET --kind repo ./repo-archive.tar.gz
  ploy report --job-id $TICKET --output ./reports/${TICKET}.tar.gz
  ```

  These commands automatically select the first cached node unless `--node-id` overrides it.

## Step 4 — Backfill artifact metadata

For artifacts that were uploaded directly to IPFS without control-plane metadata:

1. Locate the original payload (diff tarball, logs, Build Gate report) on the workstation or long-term
   storage.
2. Re-run `ploy upload --job-id <existing-job> --kind <diff|logs|report> <path>` so the control plane
   records the CID, digest, and retention fields. The job ID does not have to be active; it is only
   used for indexing purposes.
3. For Mods images, use Docker Hub for publishing; the control plane no longer exposes a registry upload
   surface (previously `PUT /v1/registry/<repo>/blobs/uploads/<slot>?digest=sha256:...`).
4. Validate pin status with `ploy artifact status <cid>` and remove any legacy copies.

## Step 5 — Cut over operators

- Communicate the new workflow through release notes or internal docs; reference this guide plus the
  [SSH transfer FAQ](ipfs.md#faq--troubleshooting).
- Remove legacy scripts that directly invoked IPFS Cluster or SCP. The only supported pathways should
  go through `ploy upload/report` or `/v1/artifacts/upload`.
- Monitor `ploy_artifact_http_requests_total` and `ploy_artifact_payload_bytes_total` in Prometheus to
  ensure traffic is flowing through the slot APIs instead of old endpoints.

## Verification checklist

- `ploy upload` succeeds for a sample job and the artifact shows up via `GET /v1/artifacts?job_id=...`.
- `ploy report` fetches a known artifact and validates the digest.
- `journalctl -t sshd | grep ploy-artifacts` shows slot IDs matching recent uploads.
- `systemd-tmpfiles --cat-config /etc/tmpfiles.d/ploy-artifacts.conf` confirms cleanup rules exist.
- No automation jobs invoke the deprecated direct IPFS helpers.

## References

- [docs/next/api.md](api.md#transfer-slots-ssh-uploads--downloads)
- [docs/next/ipfs.md](ipfs.md#ssh-transfer-workflow)
- [docs/how-to/deploy-a-cluster.md](../how-to/deploy-a-cluster.md#ssh-artifact-subsystem)
- [docs/runbooks/control-plane/ssh-transfer.md](../runbooks/control-plane/ssh-transfer.md)
