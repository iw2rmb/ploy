# Control Plane Job Recovery

## Purpose

Operators use this runbook to diagnose and recover stuck jobs in the etcd-backed scheduler without
Typical symptoms include jobs stuck in `running` despite the worker disappearing or
jobs failing to re-queue after retry budget resets.

## Prerequisites

- `etcdctl` configured with the cluster CA (`export ETCDCTL_ENDPOINTS=http://127.0.0.1:2379`).
- Access to the control-plane API host(s) or the etcd cluster.
- Knowledge of the affected Mod ticket (`mod-xxxx`) and job ID when available.

## Symptoms & Diagnosis

- **Job stuck in `running` with stale lease**

    ```sh
    etcdctl get --prefix "leases/jobs/"
    etcdctl get "mods/<ticket>/jobs/<job-id>" | jq .
    ```

  - If the lease key no longer exists but the job record still shows `state: "running"` and
     `lease_id != 0`, the worker heartbeat failed and the watcher has not re-queued the job yet.

- **Job never transitioned out of `queued`**

    ```sh
    etcdctl get "queue/mods/default/<job-id>" | jq .
    ```

  - Confirm `retry_attempt` and `max_attempts` are within expected bounds.

- **Retry budget exhausted unexpectedly**

    ```sh
    etcdctl get "mods/<ticket>/jobs/<job-id>" | jq '.retry_attempt, .max_attempts, .error'
    ```

## Recovery Steps

### Requeue a Stuck Running Job

- Inspect the job to confirm the lease expired:

    ```sh
    etcdctl get "mods/<ticket>/jobs/<job-id>" | jq '.state, .lease_id, .claimed_by'
    ```

- Reset the job to `queued`, clearing claim metadata and incrementing the retry counter:

    ```sh
    etcdctl txn <<'EOF'
    cmp = value("mods/<ticket>/jobs/<job-id>") != ""
    then
      put "mods/<ticket>/jobs/<job-id>" '{
        "id": "<job-id>",
        "ticket": "<ticket>",
        "step_id": "<step>",
        "priority": "default",
        "state": "queued",
        "retry_attempt": <retry+1>,
        "max_attempts": <max>,
        "claimed_by": "",
        "lease_id": 0,
        "claimed_at": "",
        "lease_expires_at": "",
        "enqueued_at": "<RFC3339 timestamp>"
      }'
      put "queue/mods/default/<job-id>" '{
        "job_id": "<job-id>",
        "ticket": "<ticket>",
        "step_id": "<step>",
        "priority": "default",
        "retry_attempt": <retry+1>,
        "max_attempts": <max>,
        "enqueued_at": "<same timestamp>"
      }'
    endif
    EOF
    ```

- Verify the queue entry exists and the job state is now `queued`.

### Mark a Job Failed for Inspection

- Set the job to `inspection_ready` so the container remains available:

    ```sh
    etcdctl put "mods/<ticket>/jobs/<job-id>" '{
      ...
      "state": "inspection_ready",
      "completed_at": "<RFC3339 timestamp>",
      "error": {
        "reason": "manual_intervention",
        "message": "operator requested inspection"
      }
    }'
    ```

- Add/Update the GC marker to defer cleanup:

    ```sh
    etcdctl put "gc/jobs/<job-id>" '{
      "job_id": "<job-id>",
      "ticket": "<ticket>",
      "state": "inspection_ready",
      "expires_at": "<timestamp +24h>"
    }'
    ```

- Notify the on-call engineer to capture container state or IPFS artifacts.

### Force-Fail and GC a Job

- Transition the job to `failed` with an explicit reason:

    ```sh
    etcdctl put "mods/<ticket>/jobs/<job-id>" '{
      ...
      "state": "failed",
      "completed_at": "<RFC3339 timestamp>",
      "error": {
        "reason": "operator_force_fail",
        "message": "job failed permanently"
      }
    }'
    ```

- Delete any remaining queue or lease keys:

    ```sh
    etcdctl del "queue/mods/default/<job-id>"
    etcdctl del "leases/jobs/<job-id>"
    ```

- GC controller will pick up the `gc/jobs/<job-id>` entry on the next sweep; confirm after the run.

## Verification

- `curl -s $CONTROL_PLANE_URL/v1/jobs/<job-id>?ticket=<ticket>` should reflect the updated state.
- `etcdctl get "queue/mods/default/<job-id>"` returns empty after completion.
- Integration tests (`go test -tags integration ./tests/integration/controlplane`) pass after the
  change if code adjustments accompanied the recovery.

## References

- [docs/next/queue.md](../../v2/queue.md) — Scheduler queue semantics.
- [docs/next/etcd.md](../../v2/etcd.md) — Keyspace contracts.
- [docs/next/job.md](../../v2/job.md) — Job lifecycle and API touchpoints.
