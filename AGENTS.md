# Job/Run Investigation (Strict Short Runbook)

Use this flow for any "why did run/job fail?" request.

## Exec summary
- This repository is the orchestrator/control-plane, not the user project that failed to build/test.
- For job/run failure analysis, default to runtime evidence from the failed run/job first (status, logs, artifacts, preserved temp), then inspect orchestrator code only when evidence points there.
- Do not answer language/build-tool behavior questions from orchestrator internals unless explicitly asked to analyze orchestrator implementation.
- Active investigation target is remote cluster `round-leaf-6114` on `s_v.v.kovalev@10.120.34.186`.

## 1) Assume remote cluster first
- Do not start with broad repo searches.
- Use remote host as first source of truth:
  - `export PLOY_SSH='s_v.v.kovalev@10.120.34.186'`
  - `ssh "$PLOY_SSH" 'hostname'`
- Remote Docker requires sudo; use `VPS_PWD` as sudo password:
  - `ssh "$PLOY_SSH" "printf '%s\n' \"$VPS_PWD\" | sudo -S docker ps --format '{{.Names}}'"`
- Expected core containers include `ploy-server-1` and `ploy-node-1`.

## 2) Control-plane access source (in order)
- Remote DB first: `ssh "$PLOY_SSH" 'psql "$PLOY_DB_DSN"'`.
- DB is reachable only on the remote host; do not start with local `psql`.
- If API auth is needed, use `~/.config/ploy/default` (cluster profile for `round-leaf-6114`; may be symlink) for server URL/token.

## 2.1) Direct DB fast path for simple status questions
- If question is a single factual check (for example: "what is this job status?"), answer from DB first, before collecting full runtime evidence.
- Only continue to logs/artifacts/container inspection when DB result is missing, ambiguous, or conflicts with observed behavior.

## 3) Minimum required evidence
- `ploy run status <run-id> --json`
- `ploy job log --format raw <job-id>`
- `ssh "$PLOY_SSH" "printf '%s\n' \"$VPS_PWD\" | sudo -S docker logs <job-container-id>"` where label `com.ploy.job_id=<job-id>`
- `ssh "$PLOY_SSH" "printf '%s\n' \"$VPS_PWD\" | sudo -S docker inspect <job-container-id>"` (state + mounts)
- To resolve `<job-container-id>` on remote host:
  - `ssh "$PLOY_SSH" "printf '%s\n' \"$VPS_PWD\" | sudo -S docker ps -aq --filter label=com.ploy.job_id=<job-id>"`

## 3.1) Fast route by exit code (first triage branch)
- If `exit_code = -1`: treat as ploy/orchestrator-internal failure first.
  - Prioritize `ploy-node-1` logs for that `job_id` immediately.
  - Typical scope: pre-container/setup/population failures (for example input/materialization issues).
- If `job_type = heal` and `exit_code = 1`: treat as job payload/tooling failure first.
  - Prioritize runtime artifacts from `*_mig-out.bin`, especially `out/amata/runs/**` (`events.ndjson`, `snapshot.json`, provider outputs when present).
  - Only pivot to ploy internals after runtime evidence is insufficient/inconsistent.

## 4) Artifact retrieval path (no re-discovery)
- Always fetch artifacts with:
  - `ploy mig fetch --run <run-id> --artifact-dir <dir>`
- If needed, unpack `*_mig-out.bin` (`tar -xzf ...`) and inspect:
  - `out/amata/runs/*/events.ndjson`
  - `out/amata/runs/*/snapshot.json`
  - `out/heal.json`
- For heal/payload analysis, correlate artifact behavior with MIG sources in:
  - `/Users/v.v.kovalev/@scale/ploy-lib/migs`

## 5) Important caveat
- `/out` bundle may not include per-step provider files (`stderr.txt`, `transcript.txt`, `provider-metadata.json`).
- If missing there and temp mounts are already cleaned, report clearly: exact provider-internal error is unrecoverable post-factum.

## 6) Failure temp preservation (node remote path)
- On failed jobs, node preserves investigation copies automatically (no env flag):
  - `/tmp/ploy-preserved/<run-id>/<job-id>/<timestamp>/`
  - contains `in/`, `out/`, `workspace/` (when available)
- On remote cluster, inspect via:
  - `ssh "$PLOY_SSH" "printf '%s\n' \"$VPS_PWD\" | sudo -S find /tmp/ploy-preserved -maxdepth 5 -type d | tail -n 50"`
- To copy all preserved evidence locally:
  - `mkdir -p ./tmp && ssh "$PLOY_SSH" "printf '%s\n' \"$VPS_PWD\" | sudo -S tar -C /tmp -czf - ploy-preserved" > ./tmp/ploy-preserved.tgz`

## 7) Response contract
- Separate:
  - immediate failure cause (this job)
  - underlying product/build failure (if different)
- State explicitly what is proven vs unavailable.

## 8) Maintenance note
- Keep this runbook's investigation routes current as failure patterns evolve (especially exit-code routing and temp-preservation paths).
