# Retention & Garbage Collection

Ploy v2 keeps artifact retention explicit so etcd metadata and IPFS pins stay in sync. This document
describes the lifecycle, cleanup controller, and CLI tooling.

## Retention Metadata

- Every job record under `mods/<ticket>/jobs/<job-id>` includes an `expires_at` timestamp. By
  default this is `completed_at + 7 days`, but the value can be overridden per job (e.g., via CLI
  flag).
- Jobs that are still running or awaiting retries never receive an `expires_at`; only terminal
  states (`succeeded`, `failed`, `cancelled`) do.

Example job record snippet:

```json
{
  "job_id": "job-8a9f",
  "state": "succeeded",
  "completed_at": "2025-10-08T12:45:00Z",
  "expires_at": "2025-10-15T12:45:00Z",
  "artifacts": {
    "diff_cid": "bafy...",
    "build_gate_cid": "bafy..."
  }
}
```

## Cleanup Controller

- The beacon (or dedicated controller) runs a background reconciliation loop (default every hour):
  1. Read job entries with `expires_at <= now`.
  2. For each job:
     - Ensure the Mod ticket is no longer in progress. If retries are pending or the ticket status
       is still `running`, skip the job and push `expires_at` forward by a small grace window (e.g.,
       1 hour).
     - Call the IPFS Cluster API to unpin each artifact CID.
     - Once unpin succeeds, delete the job record from etcd and emit an audit event.
  3. Optionally, maintain a “pending_gc” flag so operators can monitor upcoming deletions before
     they happen.
- If any step fails (e.g., IPFS unpin is temporarily unavailable), the controller logs the error and
  retries on the next cycle.

## CLI: `ploy gc`

- `ploy gc` invokes the same logic as the controller but on demand. It supports:
  - `--dry-run` — list the jobs and CIDs that would be deleted without mutating state.
  - `--kind mods|buildgate` — limit to certain job types.
  - `--older-than <duration>` — override the default retention window for the run.

Example dry run:

```bash
ploy gc --dry-run
JOB job-8a9f ticket=mod-123 diff=bafy... build_gate=bafy... expires=2025-10-08T12:45:00Z
JOB job-2ab1 ticket=mod-456 diff=bafy... build_gate=null expires=2025-10-07T09:20:00Z
```

Without `--dry-run`, the command performs the unpin + etcd deletion workflow, surfacing a summary
of cleaned CIDs.

## Incomplete Jobs & Failures

- Jobs stuck in `claimed` or `running` beyond a heartbeat timeout are handled by the scheduler first:
  they are marked `failed` and optionally requeued (respecting `--retry` budgets). Only then does
  `expires_at` start counting down.
- If the controller encounters an unexpected state (e.g., missing artifact metadata), it logs the
  issue and leaves the job untouched so operators can inspect it.

## Summary

- `expires_at` governs retention; both controller and CLI consult it.
- Beacon/controller automatically performs GC but operators can force it via `ploy gc`.
- Artifacts are unpinned before job metadata is removed, ensuring no dangling references or
  premature deletions.
