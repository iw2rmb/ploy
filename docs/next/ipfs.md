# IPFS Cluster Integration

Ploy Next replaces the workstation filesystem artifact cache with an IPFS Cluster
backed store. Every step execution publishes diff bundles, log archives, and
auxiliary assets directly to the cluster so any node can hydrate artifacts by
CID.

## Artifact Pipeline

1. The step runtime captures diff tarballs from the writable workspace mount
   and streams them to the cluster via the embedded client in
   `internal/workflow/artifacts`.
2. Log buffers are uploaded using the same pathway. Both uploads compute a
   SHA-256 digest before transmission so the resulting CID and digest can be
   recorded in workflow checkpoints.
3. Replication factors default to the workstation configuration:
   - `PLOY_IPFS_CLUSTER_REPL_MIN`
   - `PLOY_IPFS_CLUSTER_REPL_MAX`
   Operators can override these values per upload via CLI flags when testing.
4. Additional metadata (artifact name, kind) is stored with each pin to aid
   debugging and operational tooling.

## CLI Commands

The CLI routes artifact management commands to the cluster client:

- `ploy artifact push [--name <name>] [--kind <kind>] <path>` uploads an
  artifact and prints the CID, digest, size, and replication settings.
- `ploy artifact pull <cid> [--output <path>]` downloads and optionally writes
  the artifact to disk, reporting the digest for verification.
- `ploy artifact status <cid>` reports peer pin states and replication
  thresholds to help operators detect skew.
- `ploy artifact rm <cid>` initiates an unpin request when an artifact is no
  longer required.

## Operational Guidance

- Ensure `PLOY_IPFS_CLUSTER_API` points at an IPFS Cluster peer reachable from
  the workstation. Authentication can be provided via
  `PLOY_IPFS_CLUSTER_TOKEN` (bearer) or
  `PLOY_IPFS_CLUSTER_USERNAME`/`PLOY_IPFS_CLUSTER_PASSWORD` (basic auth).
- The `tests/integration/artifacts` suite exercises the client behaviour against
  a mocked cluster API. Extend this coverage as real cluster endpoints become
  available.
- When diagnosing replication issues, start with `ploy artifact status` to see
  which peers report lagging pins. Combine with cluster daemon logs to isolate
  connectivity or disk pressure problems.

## SSH Transfer Workflow

Artifact uploads no longer hit the IPFS Cluster API directly from the workstation. Instead, the CLI
requests a short-lived slot (`POST /v1/transfers/upload`), copies the payload over the existing SSH
ControlMaster socket (`ploy upload --job-id <id> <path>`), and commits the slot so the control plane
can ingest the bytes into IPFS Cluster. Downloads follow the same pattern with
`POST /v1/transfers/download` and `ploy report --job-id <id> --output <path>`.

- Slots are scoped to a job/stage and resolve to a node ID plus `remote_path`
  (`/slots/<slot-id>/payload`). The control plane also records the absolute `local_path`
  (`/var/lib/ploy/ssh-artifacts/slots/<slot-id>/payload`) for the slot guard and janitor.
- The transfer manager enforces a 10 GiB ceiling per slot and expires unused reservations after
  30 minutes. The CLI receives both values up front and aborts if the local file exceeds the budget.
- Commits redeploy the payload into IPFS Cluster (artifacts) or the registry store (OCI blobs) and log
  the digest, caller, and byte count so auditors can trace who uploaded what.

## Slot Lifecycle

Slots move through three states:

1. **pending** — Returned by `/v1/transfers/upload|download`. The slot reserves disk under
   `/var/lib/ploy/ssh-artifacts/slots/<slot-id>` but is not yet visible to the artifact store. The
   guard validates this state before launching `internal-sftp`.
2. **committed** — `POST /v1/transfers/{slot}/commit` validates the declared `size`/`sha256:` digest,
   publishes the payload, records metadata (job_id, kind, CID), and removes the temporary directory.
3. **aborted** — Triggered via `POST /v1/transfers/{slot}/abort` or by CLI error handling. Aborted
  slots leave the staged file behind for inspection until the TTL expires; the janitor aborts and
  removes the directory on the next sweep so disk usage stays bounded.

## Monitoring & Recovery

- `ploy_artifact_http_requests_total` and `ploy_artifact_payload_bytes_total` surface success/error
  rates and throughput for `/v1/artifacts*`. Create alerts for sustained `5xx` rates or payload spikes.
- `ploy_registry_http_requests_total` / `ploy_registry_payload_bytes_total` provide similar coverage
  for OCI blob and manifest routes.
- Host-level monitoring should include `journalctl -u sshd | grep ploy-artifacts` (every SFTP
  subsystem invocation logs the user, remote host, and byte counts) and disk usage under
  `/var/lib/ploy/ssh-artifacts` to catch leaked slots.
- The artifact metadata includes `pin_state`, `pin_replicas`, `pin_retry_count`, and
  `pin_error`. Query them via `ploy artifact status <cid>` or `GET /v1/artifacts/{id}` to confirm
  whether IPFS Cluster finished replicating a payload.

If SSH copies fail repeatedly, inspect `~/.ploy/tunnels` to ensure the cached descriptors still point
at reachable hosts and rerun `ploy cluster add --address <ip> --dry-run` to refresh metadata.

## FAQ & Troubleshooting

- **How do I resume a failed upload?** Rerun `ploy upload` with the same `--job-id`. The CLI requests a
  new slot automatically. If you need to reuse the original slot (for forensic purposes), copy the
  payload into `/var/lib/ploy/ssh-artifacts/slots/<slot-id>/payload` (or use the recorded `remote_path`
  plus the staging root), verify the digest (`sha256sum payload`), then call
  `POST /v1/transfers/<slot>/commit` manually. Always abort (`.../abort`) before retrying if the first
  attempt died mid-transfer so the control plane can release the reservation.
- **Digest mismatch on commit** — The control plane recalculates `sha256` server-side. When it reports
  `payload digest mismatch`, re-hash the local file (`sha256sum <path>`) and compare it with the CLI
  log. Copying via network shares or re-packaging tarballs between slot creation and commit can mutate
  bytes; request a new slot and retry from the original source archive to avoid partial writes.
- **Artifact stuck in `pinning`** — `pin_state: pinning` indicates IPFS Cluster has not reached the
  requested replication factor. Check `pin_replicas` and `pin_error` via `ploy artifact status <cid>`;
  if retries exceed expectations, inspect the cluster (`ipfs-cluster-ctl pin ls <cid>`) and ensure the
  cluster peers listed in `docs/envs/README.md` are online. The control-plane reconciler promotes
  `pin_state` to `failed` after successive errors so alerts can fire.
- **Cleaning staged files** — Slots live under `/var/lib/ploy/ssh-artifacts/slots`. To prune abandoned
  payloads, run `sudo find /var/lib/ploy/ssh-artifacts/slots -maxdepth 1 -mmin +45 -print -exec rm -rf
  {} +`. Pair this with a `systemd-tmpfiles` rule (`D /var/lib/ploy/ssh-artifacts/slots 0750 ploy ploy
  -`) so cleanup runs automatically if the control plane is offline.
