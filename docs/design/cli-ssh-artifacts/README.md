# CLI SSH Artifact Handling

- Status: Review
- Owner: Codex
- Created: 2025-10-24
- Sequence: 3 of 3 (`SSH Transport Pivot`)

## Summary
Enable repository uploads and report downloads over the SSH transport introduced for CLI control-plane calls. Provide simple, reliable file transfer without relying on HTTP endpoints.

## Goals
- Reuse the existing SSH control sockets managed by `pkg/sshtransport` so uploads/downloads do not punch new TCP sessions per transfer.
- Keep CLI UX close to the historical workstation commands (`ploy upload`, `ploy report`) while aligning with the current command tree (`ploy artifact …`).
- Support large payloads (multi‑GB repo snapshots) with resumable semantics so flaky links can recover without re-sending the whole file.
- Preserve deterministic digests so transferred payloads can be verified before they are staged into IPFS Cluster or exposed to operators.

## Non-Goals
- Replacing tunnel setup or control-plane API calls (handled in other design docs).
- Building new object store abstractions beyond SSH-forwarded services.

## Current State
- Artifact transfer still relies on HTTPS endpoints; the CLI opens tunnels for control-plane RPCs but not for bulk file copy, and `/v1/artifacts/upload` intentionally returns `501`.
- The CLI only knows how to talk to IPFS Cluster directly (`ploy artifact push/pull`). There is no workflow for uploading a repository snapshot or downloading structured reports tied to a mods ticket.
- `pkg/sshtransport` keeps persistent ControlMaster sockets for port-forwarding, but it does not currently expose any primitive for file I/O over those sockets.

## User Stories
1. **Repository snapshot upload** — An operator runs `ploy upload --ticket mods-1234 --path /worktrees/java17` to push a deterministic tarball of the repo to the control plane so the worker can hydrate the job without GitLab credentials.
2. **Structured report download** — After a SHIFT build-gate completes, the operator runs `ploy report --ticket mods-1234 --stage build-gate --output report.json` to pull the JSON bundle produced by the worker for offline inspection.
3. **Diagnostic bundle retrieval** — Support engineers can fetch crash dumps or `ployd` log archives produced on the worker without exposing extra TCP listeners; they re-use the same SSH descriptor cached on their workstation.

## Proposed Changes
1. **SFTP/SCP Subsystem**
   - Install a dedicated subsystem entry (`Subsystem ploy-artifacts internal-sftp`) on every control-plane node. The bootstrap script will create `/var/lib/ploy/ssh-artifacts/{uploads,reports}` with `ploy:ploy` ownership and 0750 perms.
   - Declare a `Match Group ploy-artifacts` block in `sshd_config` that chroots the subsystem to `/var/lib/ploy/ssh-artifacts` and disables shell/PTY allocation. Workers add the `ploy` service account to this group so ControlMaster tunnels still function.
   - Restrict file paths to per-job workspaces inside the chroot via a new `ploy-artifacts-guard` helper (installed under `/usr/local/libexec/`) invoked through `ForceCommand internal-sftp -d %d/%u`. The helper whitelists `<kind>/<job-id>/<object-id>` directories and rejects absolute paths.
2. **CLI Integration**
   - Extend `pkg/sshtransport.Manager` with `CopyTo(ctx, opts)` and `CopyFrom(ctx, opts)` helpers that resolve the ControlMaster socket for a node, then invoke `sftp`/`scp -T` against the shared connection.
   - Introduce a new `internal/cli/transfer` package that wraps compression, digesting, and resumable chunk orchestration. `cmd/ploy` surfaces two commands: `ploy upload` (repo snapshots/log bundles) and `ploy report` (download structured artifacts).
   - Teach the control-plane HTTP surface to issue short-lived "slots" (metadata only) via `/v1/transfers/upload` and `/v1/transfers/download`. Slots carry the authoritative `job_id`, expected size/digest, `node_id`, and the remote relative path the CLI must target over SSH.
3. **Resumable / Large Transfers**
   - Chunk payloads larger than 256 MiB into 32 MiB blocks. Each block lands as `.partN` under the slot directory; the CLI keeps a `.manifest` file (JSON) describing completed blocks and digests so uploads can resume.
   - When the final block succeeds, the CLI sends `POST /v1/transfers/{slot}/commit` to atomically concatenate the `.part*` files via `ployd` and publish the resulting tarball into the existing IPFS ingestion queue. Downloads mirror this by supporting sparse range requests that re-open the SFTP channel at the failed offset.
4. **Security & Auditing**
   - `ployd` tails `/var/log/secure` (or journal) for `subsystem=ploy-artifacts` events, emitting structured logs with `job_id`, `slot_id`, `node_id`, caller fingerprint, byte counts, timing, and result.
   - Enforce quotas: default 10 GiB per slot, 50 GiB per tenant per day. Slots inherit the job/tenant identity and are rejected if the caller exceeds negotiated limits. All slots expire automatically (e.g., 30 minutes) if the CLI never commits.

## Architecture Overview

### Transfer Slot API
- **Upload slot (`POST /v1/transfers/upload`)** — Input: `job_id`, `kind` (repo, logs, report), optional expected size/digest. Output: `slot_id`, `node_id`, `remote_path`, `max_size`, `expires_at`, plus a bearer token scoped to that slot.
- **Download slot (`POST /v1/transfers/download`)** — Input: `job_id`/`artifact_id`. Output: same metadata plus `expected_digest` so the CLI can verify the download.
- **Commit/abort (`POST /v1/transfers/{slot}/commit|abort`)** — Called after `CopyTo`/`CopyFrom` finishes to flush metadata into etcd/IPFS and release quota.
- Slots are stored in etcd under `transfers/<slot-id>` with TTLs matching `expires_at`. The scheduler ties slots back to the job so only the node currently running the job is used for uploads.

### CLI Upload Flow
1. CLI resolves the control-plane base URL and obtains an HTTP token (unchanged from existing config flows).
2. CLI calls `POST /v1/transfers/upload` to reserve a slot. The control plane picks the node currently running the job (or the default control-plane node when pre-staging repos) and returns slot metadata.
3. CLI streams the payload through `transfer.Uploader`, which compresses the directory into a deterministic tarball, calculates SHA-256 incrementally, and writes 32 MiB chunks to `CopyTo`.
4. `CopyTo` asks the tunnel manager for the node listed in the slot, executes `sftp -b - -s ploy-artifacts` with `-S <control-path>` so the existing ControlMaster handles authentication, and uploads chunks as `<remote_path>/.partN`.
5. After all parts upload and the digest matches the slot contract, the CLI issues `commit`, sending final size/digest. Control plane records the artifact and triggers background ingestion (e.g., move tarball into IPFS queue directory).

### CLI Report/Download Flow
1. Operator requests a slot via `POST /v1/transfers/download` specifying ticket/stage. Control plane maps it to the node that wrote the report and responds with slot metadata and digest.
2. CLI calls `CopyFrom`, which reuses the ControlMaster socket and runs `sftp` in read mode, supporting optional `--resume` to reopen from a byte offset.
3. The CLI writes to stdout or `--output`, validates the digest, and acknowledges via `/commit`. Control plane can then mark the report as "retrieved" or keep it for future requests.

### sshtransport Extensions
- Add a new `type Copier interface { CopyTo(ctx context.Context, nodeID string, opts CopyOptions) error; CopyFrom(...) }` implemented by the manager. The implementation will:
  - Reuse `ensureTunnel` to guarantee the ControlMaster is alive.
  - Derive the control socket path via the same hashing logic used by `sshFactory`, so `scp`/`sftp` can pass `-o ControlPath=<path>` without redoing key exchange.
  - Execute `sftp`/`scp` commands with context + timeout, piping stdin/stdout to the caller’s readers/writers.
  - Surface progress callbacks so the CLI can emit `Transferred 512 MiB / 1.2 GiB (42%)` style updates.

### Node Services
- `ployd` gains a lightweight artifact reconciler:
  - Watches `/var/lib/ploy/ssh-artifacts/uploads/*` for completed manifests. On commit it moves the tarball into the workflow’s workspace and queues an IPFS publish.
  - Exposes `GET /v1/transfers/{slot}` for debugging (control-plane internal only).
- A systemd unit `ploy-artifacts@.service` performs background cleanup of abandoned slot directories based on TTLs from etcd.
- Existing bootstrap assets get a new task `50-ssh-artifacts.sh` that installs the subsystem, guard binary, directories, SELinux labels, and logrotate snippets.

## Failure & Retry Handling
- Upload slots expire automatically. The CLI treats HTTP 410 responses as "slot gone", surfaces actionable guidance, and offers `ploy upload --resume` which re-requests a slot and reuses previously uploaded `.part*` files when possible.
- If `CopyTo` reports `ErrBackoffActive` (tunnel unhealthy), the CLI retries with exponential backoff but keeps the same slot. After three failures it aborts and lets the control plane reassign the slot to another node.
- Commits verify the remote digest before acknowledging success. If the digest mismatches, the slot is quarantined and operators can inspect the staging directory via SSH.
- Download resume uses HTTP `Range` semantics mirrored over SFTP by seeking the remote file via the SFTP `offset` command.

## Security & Observability
- All slot mutations require standard OAuth scopes plus the finer-grained scope `artifacts:transfer`. Control-plane auth middleware reuses the existing JWT verifier.
- The guard helper enforces `<kind>/<job-id>/...` naming and ensures the total size written stays below the negotiated `max_size` (it tracks bytes via the SFTP `stat` callbacks). Attempts to escape the chroot or overwrite other slots return `SSH_FX_PERMISSION_DENIED` and are logged.
- Each transfer produces metrics:
  - `ploy_cli_transfer_bytes_total{direction="upload"}`
  - `ploy_cli_transfer_errors_total{reason=...}`
  - `ploy_cli_transfer_duration_seconds`
  These feed into both `ployd` Prometheus and the CLI (which emits a final summary to stderr).

## Rollout Plan
1. **Phase 0 – Hidden flag**: ship `sshtransport.Copy*` behind `PLOY_ENABLE_SSH_TRANSFERS`. CLI commands default to legacy behaviour (erroring with "unimplemented") until this flag is set.
2. **Phase 1 – Upload only**: enable slot API + upload path for mods tickets. Reports still download via HTTP/IPFS.
3. **Phase 2 – Full parity**: turn on report download slots, retire the HTTP upload endpoint, and update docs/runbooks.
4. **Phase 3 – Legacy removal**: delete the old IPFS-direct upload helpers once all workflows rely on SSH transfers.


## Work Plan
1. Add SFTP subsystem config/tests on nodes (bootstrap script, `ployd` guard helper, CI sshd harness).
2. Implement `sshtransport.CopyTo/CopyFrom`, slot API handlers, and etcd persistence.
3. Add `internal/cli/transfer`, expose `ploy upload/report`, and link them to the slot API + copy helpers.
4. Ship integration tests covering upload/download success, permissions, resumable scenarios, and backpressure; stress-test multi-GB payloads via the VPS lab jobs.
5. **Slice A – Artifact metadata backend** (`docs/design/cli-ssh-artifacts-slice-a.md`)
   - Track state in etcd, wire `/v1/artifacts`, and hook slot commits to IPFS.
6. **Slice B – Registry (OCI) backend** (`docs/design/cli-ssh-artifacts-slice-b.md`)
   - Implement `/v1/registry/*`, blob staging, and manifest storage.
7. **Slice C – Pin state + CLI parity** (`docs/design/cli-ssh-artifacts-slice-c.md`)
   - Surface pin health/metrics and move CLI artifact commands to the HTTP API.
8. **Slice D – Schema & ops docs** (`docs/design/cli-ssh-artifacts-slice-d.md`)
   - Document the new flows, migrations, and operational guidance.

## Testing Strategy
- **Unit**: mock the `Copier` interface to ensure CLI commands handle slot expiry, digest mismatches, and resume markers. Add `sshtransport` tests that stub out the `sftp` binary and assert command-line construction.
- **Integration**: spin up an OpenSSH container in CI with the new subsystem enabled; run Go tests that exercise full upload/download flows via `make test TRANSFER_E2E=1`.
- **Stress**: nightly VPS lab job uploads 5 GiB repo snapshots over lossy links (tc netem) to validate resumable behaviour and slot cleanup.
- **Security**: add regression tests that try to escape the chroot or exceed quotas; expect `permission denied` and audit log entries.

## Documentation
- Update `cmd/ploy/README.md`, `docs/runbooks/*.md`, and `docs/next/devops.md` with the new commands, slot lifecycle, and troubleshooting tips (`ssh -S ~/.ploy/tunnels/<id> -O check` etc.).
- Document SFTP server configuration, directory layout, quota defaults, and how to rotate the `ploy` service account key inside `docs/envs/README.md`.
- Add FAQ entries describing how to resume interrupted uploads and where the temporary files live on nodes.

## Dependencies
- Requires tunnel manager from `cli-ssh-transport` plus the shared cache (`internal/controlplane/tunnel`).
- Relies on SSH surface prepared in `controlplane-ssh-surface` and the updated bootstrap assets.
- Slot persistence depends on etcd availability; ingestion still relies on IPFS Cluster once the tarball is committed.

## Open Questions / Follow-Ups
1. Should we opportunistically support rsync-style delta uploads for extremely large repositories, or is chunked SFTP sufficient?
2. Do we need per-tenant encryption-at-rest for the staging directories, or is the short TTL plus chroot isolation enough?
3. Can we reuse the same slot API for future `ploy gc download` flows (e.g., pulling archived mods metadata)?
