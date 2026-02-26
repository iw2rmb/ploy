# Node Container Cleanup: Disk-Pressure Model

## Decision
Use a single cleanup strategy:
- Keep all job containers by default after completion.
- Clean containers only when disk space is low.
- Run cleanup immediately before claiming new work.

This replaces all retain-related behavior.

## Removed Concepts
Remove from design and implementation:
- YAML retain policy fields (`retain_container.*`).
- Crash-specific retain policy/retry retain logic.
- Periodic retained-container cleanup worker.
- Node cleanup env vars.
- Cleanup decision endpoint(s).
- Completion-response cleanup decision payloads.

`POST /v1/jobs/{job_id}/complete` stays unchanged (`204`).

## Cleanup Trigger
Cleanup runs before each node claim attempt.

Guard condition:
- Free space on Docker data-root filesystem must be at least `1 GiB`.

If free space is below `1 GiB`:
1. Run FIFO cleanup of eligible containers.
2. Re-check free space.
3. If still below `1 GiB`, do not claim a job (claim loop backs off/retries later).

## Disk Source of Truth
Use Docker daemon data-root filesystem (not workspace filesystem):
- Read Docker data-root via Docker Engine info (`DockerRootDir`).
- Compute free bytes on that mountpoint.

## Cleanup Ordering and Eligibility
Ordering:
- FIFO by container `created` timestamp (oldest first).

Eligible containers:
- Only stopped containers (`exited`/`dead`/non-running states).
- Only ploy-managed containers.

Ploy-managed identification:
- Reuse existing ploy labels (`com.ploy.run_id` and/or `com.ploy.job_id`).
- Do not introduce new retain/cleanup labels.

## Runtime Ownership
Node runtime is sole cleanup owner.

Implications:
- Step runner must not remove containers on completion.
- Gate executor must not remove containers on completion.
- Cleanup occurs only in disk-pressure pre-claim path.

## Sanity Check Against Current Code
Current behavior (to be changed):
- Generic runner removes non-retained containers.
- Gate executor removes gate containers.
- Claim loop has no Docker data-root free-space guard.

Target behavior:
- No immediate post-completion deletion.
- Pre-claim disk guard enforces `>= 1 GiB` free on Docker data-root.
- FIFO deletion of oldest stopped ploy-managed containers when below threshold.

## Operational Notes
- This model is intentionally simple and deterministic.
- It avoids policy branching and server/node cleanup handshake complexity.
- Disk-pressure cleanup cadence is naturally tied to work intake pressure.
