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

- Health endpoint is healthy (Consul/Nomad/SeaweedFS services are expected; Vault unhealthy but unused here).
- orw-apply generally allocates and produces `/workspace/out/diff.patch` in under a minute (confirmed on VPS via alloc events with exit_code=0).
- One recent run failed during orw-apply with exit code 6 due to SeaweedFS upload DNS failure (`Could not resolve host: seaweedfs-filer.service.consul`). With `set -e` this aborted the task.
- Another symptom was the controller waiting at phase=apply because it looked for `diff.patch` in its own temp workspace (bind/mount assumption) instead of pulling from alloc/task-side storage.
- SSE previously lacked granular events; added emits now show `diff-found`, `diff-apply-started`, `build-gate-start`, and final `build-gate-succeeded/failed`.

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
- Task-side artifact upload: orw-apply uploads `diff.patch` (and `output.tar` when configured) to SeaweedFS using `SEAWEEDFS_URL`, `DIFF_KEY`/`OUTPUT_KEY`. Controller records artifacts if present (checks storage first) — no bind‑mount dependency.
- Guard uploads as best-effort (network failure will not abort apply). Emit explicit `orw-apply job completed` event when wait returns.
- Preflight HCL validation for planner/reducer/llm-exec/orw-apply; fail fast on template mistakes.
- Alloc appearance guard (SDK path): error out in ~90s if 0 allocs; wrapper path uses polling; can mirror guard if needed.
- SSE: add `X-Accel-Buffering: no` to improve proxy behavior.
- Emit granular apply/build events: `diff-found`, `diff-apply-started`, `build-gate-start`, `build-gate-failed/succeeded`.

### Near-Term (1–2 days)
- Event push API: `POST /v1/transflow/event {execution_id, step, phase, level, message, ts}`. Jobs/runner POST start/ok/fail.
- Controller log tailer: tail last alloc logs; update status on success/error markers; record last_log_preview.
- Nomad event stream: subscribe to alloc events; update status on Start/Terminated.
- Standard job status.json: each job writes `/workspace/out/status.json` (step/state/message/ts/metrics) for the controller to persist.
- Wrapper alloc guard: mirror the 90s no‑allocs guard for the job-manager wrapper wait path to avoid long waits when no allocations can be placed.
- SeaweedFS reachability: ensure `PLOY_SEAWEEDFS_URL` points to a resolvable/healthy filer; consider fallbacks (host IP) for environments without Consul DNS.

### Longer-Term (3–5 days)
- Live logs endpoint (SSE): `GET /v1/transflow/logs/:id?follow=true` streams step events + job tails. CLI `ploy transflow watch` displays live progress.
- Metrics & alerts: Prometheus metrics per phase and alerts when durations exceed baselines.
- Conformance across job types: planner/llm-exec/reducer/human-step all emit standard events, status.json, error.log.

### Acceptance Criteria
- `/v1/transflow/status/:id` shows current phase and last step with timestamps.
- On any failure, `status.error` updates within seconds (with error snippet). Non‑zero task exits are always surfaced (apply/build/planner/reducer).
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
- SeaweedFS integration: pass `SEAWEEDFS_URL`, `DIFF_KEY`/`OUTPUT_KEY` to tasks; task uploads `diff.patch`/`output.tar` (best‑effort). Controller records artifacts if already present in storage.
- SSE transparency: added `diff-found`, `diff-apply-started`, `build-gate-start`, `build-gate-failed/succeeded`, and `orw-apply job completed` event.
- DC parameterization: all transflow Nomad jobs use `${NOMAD_DC}` (default dc1).
- Lane C hygiene: Java/Node lane‑C templates now gate health checks, service tags, metrics/JMX behind flags. Defaults: disabled for regular apps, enabled for platform services.

## Key Takeaways (Updated)
- The apparent “halt” at apply was not lack of allocations. In several cases, orw-apply completed and produced `diff.patch`; the controller was waiting because it didn’t see the artifact locally.
- Task-side SeaweedFS uploads plus controller Exists‑checks eliminate bind/mount coupling and make artifact detection deterministic.
- Non‑zero container exits (e.g., SeaweedFS DNS upload failure) must not abort apply — uploads are now best‑effort; errors are logged but won’t fail the core path.
- SSE signals now show the exact phase transitions so you can distinguish apply wait vs build gate.

## What’s Next
- Start a fresh run and expect: `orw-apply job completed` → `diff-found` (immediate) → `diff-apply-started` → `build-gate-start`.
- If build fails, a `build-gate-failed` event and terminal status.error appear within seconds.
- If no allocs are created (rare), the 90s guard trips with an explicit error. We can mirror this guard to the wrapper wait if you’d like.
- Ensure `PLOY_SEAWEEDFS_URL` resolves in your environment (or use host IP); otherwise task‑side uploads will be skipped (still non‑blocking).

DEV LOGGING NOTE (keep logs tidy):
- This is a dev environment; to simplify troubleshooting and avoid sifting through thousands of lines on each run, clear Nomad task logs between test cycles. You can use the job-manager helper or Nomad HTTP API to remove old allocations before a new run. Example (as ploy user):
  - `/opt/hashicorp/bin/nomad-job-manager.sh stop --job ploy-api && sleep 2 && /opt/hashicorp/bin/nomad-job-manager.sh run --job ploy-api`
  - Or prune specific allocations for transient jobs (planner/reducer/orw-apply) after you’ve captured artifacts. Keep only the latest run’s logs for clarity.
  - Additionally, consider rotating `/var/lib/nomad/alloc` logs or truncating via the job manager’s wrappers if the files get large.
