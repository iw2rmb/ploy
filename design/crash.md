# Crash Reconciliation Policy (Interim)

## Scope
Node startup behavior after node process crash/restart.

## Policy
At node startup, before normal claim loop:

1. Discover ploy job containers.
2. For containers in `running` state:
   - restore tracking and continue normal wait/log/status flow.
3. For containers in terminal state (`exited`/`dead`/crashed):
   - reconcile only when container `finished_at >= now-120s`.
   - submit completion through normal `/v1/jobs/{job_id}/complete` path.

## Notes
- The 120s window is intentional to cover expected 90s stale/recovery delays with margin.
- Use terminal timestamp (`finished_at`), not container create time.
- Completion upload must be idempotent; treat already-terminal conflicts as non-fatal.
- This is interim policy and can be widened later if startup lag/clock skew needs more margin.
