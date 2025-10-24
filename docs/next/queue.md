# Work Queue & Scheduling

Ploy v2 uses etcd as the canonical queue and scheduling engine for Mods and build gate jobs. This
document describes how jobs enter the queue, how nodes claim work, capacity reporting, and retry
behaviour.

## Queue Layout

- Waiting jobs live under `queue/<kind>/<priority>/<job-id>`.
  - `<kind>` is `mods` or `buildgate`.
  - `<priority>` can be `default`, `high`, etc. (lexicographic order determines pull order).
  - The value stores the job payload including ticket, step ID, retry counters, and metadata used
    for scheduling.
- Example payload:

  ```json
  {
    "job_id": "job-8a9f",
    "ticket": "mod-123",
    "step_id": "apply-1",
    "priority": "default",
    "retry_attempt": 0,
    "max_attempts": 2,
    "enqueued_at": "2025-10-08T12:34:56Z"
  }
  ```

Jobs may override defaults via CLI flags:

- `--priority` moves the job into a different priority bucket.
- `--retry` sets the maximum additional attempts (default 0, meaning a single execution).

## Claim Flow

Workers poll the queue continuously:

1. Fetch the oldest entry within the relevant prefix (`queue/mods/*`, `queue/buildgate/*`).
2. Load the job record to confirm it is still `queued` and capture the current modification revision.
3. Grant a lease (`leases/jobs/<job-id>`) with TTL (default 120s) and build an optimistic
   transaction:
   - Compare the queue key revision to ensure no other worker removed it.
   - Compare the job record revision to ensure the job is still `queued`.
   - Delete the queue key.
   - Update the job record with state `running`, `claimed_by`, `claimed_at`, and `lease_id`.
   - Persist the lease key with the granted lease so etcd expires it if the worker disappears.
4. If the transaction fails, revoke the lease and retry the next queue entry.

Because the queue deletion, job mutation, and lease creation occur in a single transaction, only one
worker can claim a job even under heavy contention.

## Execution & Completion

- After claiming a job, the node hydrates the workspace, runs the container, and updates the job
  record with status, timestamps, and artifact CIDs (see `docs/next/job.md`).
- When the job finishes, the worker calls the control-plane completion API which transitions the job
  to `succeeded`, `failed`, or `inspection_ready`, clears the lease, and writes a GC marker.

## Retry Behaviour

- When a job fails and `retry_attempt < max_attempts`, the scheduler re-enqueues it with the attempt
  counter incremented, preserving `priority` and metadata.
- Jobs store `enqueued_at` timestamps so operators can identify starvation or long waits.

## Node Failure & Failover

- The lease watcher monitors `leases/jobs/`. When a lease expires (worker crash or missing
  heartbeat), the scheduler transitions the job back to `queued` and re-creates the queue entry.
- If the retry budget is exhausted the job transitions to `failed` with reason `lease_expired` so
  operators can inspect before GC removes the record.

## Summary

- etcd stores queue items, job records, and lease keys so all coordination lives in one system.
- Optimistic transactions guarantee single-claim semantics without leader election.
- Automatic lease expiry handles crashed workers and keeps the queue populated with retryable jobs.
- Operators can inspect queue entries, job records, and lease keys directly in etcd or via the
  control-plane HTTP API.
