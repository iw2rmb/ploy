# Job/Run Investigation (Strict Short Runbook)

Use this flow for any "why did run/job fail?" request.

## Exec summary
- This repository is the orchestrator/control-plane, not the user project that failed to build/test.
- For job/run failure analysis, default to runtime evidence from the failed run/job first (status, logs, artifacts, preserved temp), then inspect orchestrator code only when evidence points there.
- Do not answer language/build-tool behavior questions from orchestrator internals unless explicitly asked to analyze orchestrator implementation.

## 1) Assume local cluster first
- Check running containers directly (`ploy-server-1`, `ploy-node-1`).
- Do not start with broad repo searches.

## 2) Control-plane access source (in order)
- Local DB first: `psql -d ploy` (or `postgres -d ploy` alias if present).
- If API auth needed, use `~/.config/ploy/default` (may be symlink) for server URL/token.

## 3) Minimum required evidence
- `ploy run status <run-id> --json`
- `ploy job log --format raw <job-id>`
- `docker logs <job-container-id>` where label `com.ploy.job_id=<job-id>`
- `docker inspect <job-container-id>` state + mounts

## 4) Artifact retrieval path (no re-discovery)
- Always fetch artifacts with:
  - `ploy mig fetch --run <run-id> --artifact-dir <dir>`
- If needed, unpack `*_mig-out.bin` (`tar -xzf ...`) and inspect:
  - `out/amata/runs/*/events.ndjson`
  - `out/amata/runs/*/snapshot.json`
  - `out/heal.json`

## 5) Important caveat
- `/out` bundle may not include per-step provider files (`stderr.txt`, `transcript.txt`, `provider-metadata.json`).
- If missing there and temp mounts are already cleaned, report clearly: exact provider-internal error is unrecoverable post-factum.

## 6) Failure temp preservation (node local path)
- On failed jobs, node preserves investigation copies automatically (no env flag):
  - `/tmp/ploy-preserved/<run-id>/<job-id>/<timestamp>/`
  - contains `in/`, `out/`, `workspace/` (when available)
- In local Docker cluster, inspect via:
  - `docker exec ploy-node-1 sh -lc 'find /tmp/ploy-preserved -maxdepth 5 -type d | tail -n 50'`
  - `docker cp ploy-node-1:/tmp/ploy-preserved ./tmp/ploy-preserved`

## 7) Response contract
- Separate:
  - immediate failure cause (this job)
  - underlying product/build failure (if different)
- State explicitly what is proven vs unavailable.
