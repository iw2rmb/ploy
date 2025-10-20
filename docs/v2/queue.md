# Work Queue & Scheduling

Ploy v2 uses etcd as the canonical queue and scheduling engine for Mods and build gate jobs. This
document describes how jobs enter the queue, how nodes claim work, capacity reporting, and retry
behaviour.

## Queue Layout

- Waiting jobs live under `queue/<kind>/<priority>/<job-id>`.
  - `<kind>` is `mods` or `buildgate`.
  - `<priority>` can be `default`, `high`, etc. (lexicographic order determines pull order).
  - The value stores the job payload including resource requirements and metadata.
- Example payload:

  ```json
  {
    "ticket": "mod-123",
    "step_id": "apply-1",
    "cpu": 1000,          // milliCPU
    "mem": 512,           // MiB
    "retry": 0,
    "enqueued_at": "2025-10-08T12:34:56Z"
  }
  ```

Jobs may override defaults via CLI flags:

- `--cpu` (milliCPU) and `--mem` (MiB) control resource requirements (defaults: 1000m CPU, 512 MiB).
- `--retry` sets the maximum additional attempts (default 0).

## Capacity Reporting

- Every `ploynode` publishes free capacity to etcd every 15 seconds under
  `nodes/<node-id>/capacity`:

  ```json
  {
    "cpu_free": 6000,
    "mem_free": 8192,
    "heartbeat": "2025-10-08T12:35:00Z",
    "revision": 42
  }
  ```

- Nodes update the record immediately after claiming or finishing a job so the queue reflects the
  latest capacity.

## Pulling Work

Every minute a node wakes up and:

1. Reads a slice of queue keys (`queue/mods/*`, `queue/buildgate/*`) ordered by priority+
   submission time.
2. For each entry, checks if its `cpu`/`mem` fit within the node’s published `cpu_free`/`mem_free`.
3. If a match is found, the node issues an etcd transaction:
   - Compare the queue key version to ensure it still exists.
   - Compare the node capacity revision to ensure no other claim changed it.
   - Delete the queue key.
   - Create/Update the job record under `mods/<ticket>/jobs/<job-id>` (state = `claimed`,
     `claimed_by = node-id`).
   - Update `nodes/<node-id>/capacity` with the reduced `cpu_free`/`mem_free`.
4. If no job fits, the node leaves the queue untouched and retries after the sleep interval.

Because each claim is wrapped in a transaction, jobs are claimed once even when multiple nodes race
to pull work.

## Execution & Completion

- After claiming a job, the node hydrates the workspace, runs the container, and updates the job
  record with status, timestamps, and artifact CIDs (see `docs/v2/job.md`).
- When the job finishes, the node restores its free capacity and writes a completion event.

## Retry Behaviour

- If a job fails and should be retried, the control plane re-enqueues it with `retry` decremented and
  a new queue key (e.g., `queue/mods/default/mod-123/apply-1/retry-2`).
- Jobs store `enqueued_at` timestamps so operators can identify starvation or long waits, but there
  is no hard time limit enforced.

## Node Failure & Failover

- Since the queue entry is removed only after the job is claimed, jobs that never reach `state:
  running` remain queued.
- If a node crashes mid-job, heartbeat monitoring (every 15s capacity updates) detects the stale
  node. The control plane can mark the job as failed and re-enqueue it according to the `retry`
  policy.
- Capacity updates ensure other nodes see the crash (capacity entry stops updating) and avoid
  routing new work to unresponsive nodes.

## Summary

- etcd stores both queue items and node capacity, enabling atomic claim operations.
- Nodes poll the queue periodically and only grab work that fits current resources.
- Retries are controlled by job metadata; no separate leader election is required.
- Operators can inspect queue entries and timestamps through etcd or future CLI tooling to monitor
  backlogs.
