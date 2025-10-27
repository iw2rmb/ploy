# SSH Transfer Slot Recovery

## Purpose

Operators use this runbook to diagnose and recover SSH-based artifact transfers. Typical triggers are
`ploy upload` failures, digest mismatches during `/v1/transfers/{slot}/commit`, or artifacts stuck in
`pinning` because the underlying slot never committed. Slots are now persisted in etcd under
`/ploy/clusters/<cluster>/transfers/slots/<slot-id>`, so control-plane restarts no longer drop active
reservations. Inspect the key with `etcdctl get ... | jq` before deciding whether to abort.

## Prerequisites

- SSH access (sudo) to the control-plane node hosting `/var/lib/ploy/ssh-artifacts`.
- A control-plane API token with `artifact.read`/`artifact.write` scopes.
- `curl`, `jq`, and `sha256sum` available on the operator workstation.
- Cluster descriptor cached locally so `pkg/sshtransport` can reopen tunnels if needed.

## Symptoms

- `ploy upload --job-id <id>` exits with `upload payload via ssh` or `commit upload slot` errors.
- `/v1/transfers/{slot}/commit` responds with `payload size mismatch` or `payload digest mismatch`.
- `/v1/artifacts/{id}` shows `pin_state: pinning` for longer than expected because the slot never
  progressed beyond `pending`.
- Disk usage under `/var/lib/ploy/ssh-artifacts` grows steadily even when no transfers are running.
- Guard denials appear in `journalctl -t sshd`.

## Procedures

### Abort and recreate a failed upload

1. Capture the slot ID from the CLI error (`slot-y7bn0oec8k4q`).
2. Abort it so the control plane releases the reservation:

   ```bash
   curl -sS -X POST "https://$PLOY_CONTROL_PLANE_URL/v1/transfers/slot-y7bn0oec8k4q/abort" \
     -H "Authorization: Bearer $PLOY_TOKEN" | jq .
   ```

3. Remove any staged payload on the node to avoid leaking disk:

   ```bash
   sudo rm -rf /var/lib/ploy/ssh-artifacts/slots/slot-y7bn0oec8k4q
   ```

4. Rerun `ploy upload` (the CLI will request a fresh slot automatically). If SSH fails before the copy
   starts, verify `journalctl -t sshd | grep ploy-artifacts` for authentication errors and refresh the
   descriptor with `ploy cluster add --address <ip> --dry-run`.

### Validate a staged payload before committing

1. SSH to the node that owns the slot (`slot.node_id` in the `/v1/transfers/upload` response).
2. Hash the staged file:

   ```bash
   sudo sha256sum /var/lib/ploy/ssh-artifacts/slots/<slot-id>/payload
   sudo du -h /var/lib/ploy/ssh-artifacts/slots/<slot-id>/payload
   ```

3. If the digest and size match expectations, commit manually:

   ```bash
   curl -sS -X POST "https://$PLOY_CONTROL_PLANE_URL/v1/transfers/<slot-id>/commit" \
     -H "Authorization: Bearer $PLOY_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"size":7340032,"digest":"sha256:..."}' | jq .
   ```

4. The control plane publishes the payload to IPFS Cluster, records artifact metadata, and removes the
   slot directory automatically. If the commit fails again, archive `/var/lib/ploy/ssh-artifacts/slots/<slot-id>`
   for later analysis and open an incident.

### Investigate guard denials

1. Inspect the guard logs:

   ```bash
   journalctl -u sshd -g ploy-slot-guard --since -15m
   ```

   Look for `slot guard: slot <id> expired` or `slot guard: slot <id> not pending` messages.
2. Confirm the slot still exists and is pending in etcd. Guard rejections because of expiration usually
   mean the client retried after TTL; request a fresh slot.
3. If denials persist for active slots, capture the event (slot ID, node ID, username) and open an
   incident so the control-plane team can inspect the slot guard logs with higher verbosity.

### Clean orphaned slot directories

1. List stale slots (the TTL is 30 minutes by default):

   ```bash
   sudo find /var/lib/ploy/ssh-artifacts/slots -maxdepth 1 -mindepth 1 -mmin +45 -print
   ```

2. Ensure no active `sftp-server` processes reference those paths (`sudo lsof +D /var/lib/ploy/ssh-artifacts/slots`).
3. Remove the directories with `sudo rm -rf` and monitor `journalctl -t sshd` for new uploads that might
   be reusing the slot IDs.
4. The janitor built into `ployd` performs this sweep every minute; inspect the ployd logs for janitor
   errors when manual cleanup becomes frequent.
5. Add or verify the `systemd-tmpfiles` rule documented in [docs/next/devops.md](../../next/devops.md)
   as a safety net in case the janitor is disabled.

### Resolve pinning stalls

1. Identify the artifact record:

   ```bash
   ploy artifact status <cid>
   # or
   curl -sS "https://$PLOY_CONTROL_PLANE_URL/v1/artifacts?job_id=<job>&kind=<kind>" -H "Authorization: Bearer $PLOY_TOKEN" | jq .
   ```

2. If `pin_state` remains `pinning` and `pin_retry_count` increases, inspect IPFS Cluster:

   ```bash
   ipfs-cluster-ctl --host $PLOY_IPFS_CLUSTER_API pins ls <cid>
   ```

3. Restart the local IPFS Cluster peer or adjust replication settings if the cluster lacks capacity.
4. When retries exhaust and the state flips to `failed`, consider deleting the artifact or re-uploading
   via a new slot so the reconciler gets a clean payload to publish.

## References

- [docs/next/ipfs.md](../../next/ipfs.md#ssh-transfer-workflow)
- [docs/next/devops.md](../../next/devops.md#ssh-artifact-subsystem)
- [docs/runbooks/control-plane/job-recovery.md](job-recovery.md)
