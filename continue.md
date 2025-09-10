# Continue: Transflow Java11→17 MR Pipeline (State + Detailed Plan)

## Key Takeaways (Updated)

- CLI is REST-only. All orchestration (Nomad jobs, HCL templates) runs on the API (VPS). No local Nomad usage.
- API embeds all Transflow HCL templates and writes them to a per-run temp workspace:
  - `api/transflow/templates/{planner.hcl,llm_exec.hcl,orw_apply.hcl,reducer.hcl}`
  - Runner reads templates relative to its `workspaceDir`.
- orw-apply container setup stabilized:
  - Mount cloned repo as context; container creates `/workspace/input.tar` internally.
  - Fixed undefined `register_recipe_metadata` (defined early + guarded) to prevent exit 127.
  - Runner writes `/workspace/out/error.log` on failures; controller persists and includes snippet in status.
- Status reliability improved:
  - Added top-level execution timeout (default 45m, `PLOY_TRANSFLOW_EXEC_TIMEOUT`) and panic guard.
  - `/v1/transflow/status/:id` enriched with `duration` and `overdue` (default overdue if >30m, configurable via `PLOY_TRANSFLOW_OVERDUE`).
- We cancelled stale executions as needed; older failures remain for history.

Bottom line: orw-apply reliably produces `diff.patch`; remaining visibility gaps after orw-apply are addressed in the plan below.

## Current Signals / Observations (latest)

- Health endpoint is healthy (Consul/Nomad/SeaweedFS OK; Vault unhealthy but not used here).
- orw-apply jobs hit BUILD SUCCESS and generate `/workspace/out/diff.patch`.
- Controller sometimes reported timeout despite alloc completion — fixed by reading Terminated `exit_code`; runner also falls back if `diff.patch` exists.
- Remaining gap: insufficient real-time phase telemetry for apply/build/push/MR.

## Operational How-To (Quick Reference)

Deploy latest API from feature branch:
`DEPLOY_BRANCH=feature/transflow-mvp-completion ./bin/ployman api deploy --monitor`

Start and monitor a run:
- `./bin/ploy transflow run -f test-java11to17-transflow.yaml -v`
- `curl -sS https://api.dev.ployman.app/v1/transflow/status/<id> | jq`
- Cancel: `curl -sS -X DELETE https://api.dev.ployman.app/v1/transflow/<id>`

Inspect VPS logs (as `ploy` user):
- `/opt/hashicorp/bin/nomad-job-manager.sh running-alloc --job ploy-api`
- `/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id <alloc> --task api --both --lines 2000`
- For orw-apply jobs:
  - `/opt/hashicorp/bin/nomad-job-manager.sh allocs --job <orw-apply-job> --format json`
  - `/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id <alloc> --task openrewrite-apply --both --lines 1000`

Verify MR & diff (GitLab):
- Project: `iw2rmb/ploy-orw-java11-maven` → `iw2rmb%2Fploy-orw-java11-maven`
- `curl -sS -H "Authorization: Bearer $GITLAB_TOKEN" "https://gitlab.com/api/v4/projects/iw2rmb%2Fploy-orw-java11-maven/merge_requests/<iid>/changes" | jq`

## Detailed Plan: Real-Time Observability + Fail-Fast (Critical)

### Immediate (today)
- Server runner updates status at each phase boundary (phase + steps[] with timestamps).
- Record last_job metadata (job name, alloc ID, submitted time) in status.
- Persist and expose error_log; include first 1KB in status.error.
- Orchestration: use alloc Terminated `exit_code`; runner falls back on diff presence after wait timeout.

### Near-Term (1–2 days)
- Event push API: `POST /v1/transflow/event {execution_id, step, phase, level, message, ts}`. Jobs/runner POST start/ok/fail.
- Controller log tailer: tail last alloc logs; update status on success/error markers; record last_log_preview.
- Nomad event stream: subscribe to alloc events; update status on Start/Terminated.
- Standard job status.json: each job writes `/workspace/out/status.json` (step/state/message/ts/metrics) for the controller to persist.

### Longer-Term (3–5 days)
- Live logs endpoint (SSE): `GET /v1/transflow/logs/:id?follow=true` streams step events + job tails. CLI `ploy transflow watch` displays live progress.
- Metrics & alerts: Prometheus metrics per phase and alerts when durations exceed baselines.
- Conformance across job types: planner/llm-exec/reducer/human-step all emit standard events, status.json, error.log.

### Acceptance Criteria
- `/v1/transflow/status/:id` shows current phase and last step with timestamps.
- On any failure, `status.error` updates within seconds (with error snippet).
- Artifacts include `diff_patch` (or clear no-diff failure) and `error_log` when applicable.
- CLI watch shows live progress and immediate failures.

## Env Knobs

- `PLOY_TRANSFLOW_EXEC_TIMEOUT` (e.g., `45m`) — hard cap for a transflow execution, ensures terminal failure if exceeded.
- `PLOY_TRANSFLOW_OVERDUE` (e.g., `30m`) — status enrichment for running executions; marks `overdue: true` if exceeded.

## Known Non-Blockers
## Appendix: Recent Fixes

- orw-apply runner: defined/guarded `register_recipe_metadata`; standardized `error.log` writes.
- Mount strategy: repo as context; container builds `input.tar` internally.
- Orchestration: job-manager wait uses Terminated `exit_code` for success/fail.
- Runner: fallback on `diff.patch` after wait timeout; per-phase timeouts + diagnostics.
- Vault is reported unhealthy; unused for this workflow.
- Periodic transient TLS/SSH issues; rerun succeeds after a short interval.
