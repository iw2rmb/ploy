# Continue: Transflow Java11→17 MR Pipeline (State + Next Steps)

## Key Takeaways

- CLI is REST-only. All orchestration (Nomad jobs, HCL templates) runs on the API (VPS). No local Nomad usage.
- API now embeds all Transflow HCL templates and writes them to a per-run temp workspace:
  - `api/transflow/templates/{planner.hcl,llm_exec.hcl,orw_apply.hcl,reducer.hcl}`
  - Runner reads templates relative to its `workspaceDir`.
- orw-apply container setup fixed: removed read-only bind mount for `/workspace/input.tar` to let the runner create the tar inside the container.
- Status reliability improved:
  - Added top-level execution timeout (default 45m, `PLOY_TRANSFLOW_EXEC_TIMEOUT`) and panic guard.
  - `/v1/transflow/status/:id` enriched with `duration` and `overdue` (default overdue if >30m, configurable via `PLOY_TRANSFLOW_OVERDUE`).
- We cancelled all previously running executions; older failures remain listed for history.

## Current Signals / Observations (last session)

- Health endpoint is healthy (Consul/Nomad/SeaweedFS OK; Vault unhealthy but not used here).
- `orw-apply` previously failed due to input.tar mount — fixed and deployed.
- One long run earlier timed out at Nomad wait: server recorded a terminal failure (timeout) — execution did not hang forever.
- At times, `/v1/transflow/run` POST hit TLS handshake timeouts (transient ingress/network issues). Status/list still worked.

## What To Do Next

1) Deploy latest API (if not already)

- Ensure branch is set during deploy:
  - `DEPLOY_BRANCH=feature/transflow-mvp-completion ./bin/ployman api deploy --monitor`

2) Run Transflow and monitor

- Start (CLI REST):
  - `./bin/ploy transflow run -f test-java11to17-transflow.yaml -v`
- Poll status (the CLI already does this), but you can also query manually:
  - `curl -sS https://api.dev.ployman.app/v1/transflow/list | jq` 
  - `curl -sS https://api.dev.ployman.app/v1/transflow/status/<id> | jq`
- Look at `duration` and `overdue` fields. If overdue or too slow, cancel:
  - `curl -sS -X DELETE https://api.dev.ployman.app/v1/transflow/<id>`

3) If it looks stuck or slow, inspect server-side logs

- On VPS as `ploy` user:
  - Get running API alloc: `/opt/hashicorp/bin/nomad-job-manager.sh running-alloc --job ploy-api`
  - Logs for API task: `/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id <alloc> --task api --both --lines 1000`
  - When orw-apply submits, find its job:
    - `nomad status -short | sed -n '/orw-apply/p'`
    - Alloc logs: `/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id <alloc> --task openrewrite-apply --both --lines 1000`
  - Expect to see runner creating `input.tar`, executing Maven rewrite, and writing `/workspace/out/diff.patch`.

4) Verify MR creation and diff (GitLab)

- After completion, the CLI prints MR URL. You can also pull the MR diff via API:
  - Project path: `iw2rmb/ploy-orw-java11-maven` → URL-encode: `iw2rmb%2Fploy-orw-java11-maven`
  - `curl -sS -H "Authorization: Bearer $GITLAB_TOKEN" "https://gitlab.com/api/v4/projects/iw2rmb%2Fploy-orw-java11-maven/merge_requests/<iid>/changes" | jq`

## Nice-to-Have Improvements (Follow-up)

- Phase tracking: add a light-weight phase indicator (init, clone, create-branch, orw-apply, build, push, mr) updated by the server runner to `/v1/transflow/status`. This eliminates guesswork on “what’s running”.
- Include last job metadata in status (e.g. `job_name`, `last_alloc_id`) when submitted.
- Stricter fast-fail for no-diff cases (we added guards in runner and fixed orw-apply setup; confirm consistent terminal error if `diff.patch` missing/empty).

## Env knobs

- `PLOY_TRANSFLOW_EXEC_TIMEOUT` (e.g., `45m`) — hard cap for a transflow execution, ensures terminal failure if exceeded.
- `PLOY_TRANSFLOW_OVERDUE` (e.g., `30m`) — status enrichment for running executions; marks `overdue: true` if exceeded.

## Known Non-blockers

- Vault is reported unhealthy; unused for this workflow.
- Periodic transient TLS/SSH issues; rerun succeeds after a short interval.

