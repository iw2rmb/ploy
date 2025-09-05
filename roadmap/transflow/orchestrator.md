## Orchestrator Flow: Branch Mapping, History, and Cancellation

This doc describes how the transflow orchestrator maps branch outputs into `history.json`, enforces first‑success‑wins semantics, and cancels outstanding jobs.

### Branch Mapping

- From `plan.json` options → create branch specs:
  - human-step → wait-for-MR/update watcher; on event, run build-check; status = success/failed/timeout
  - llm-exec → submit LLM job; expect `diff.patch` → apply → build-check; status by build result
  - orw-gen → generate ORW recipe via LLM → run ORW apply job → build-check; status by build result (see `jobs/orw_generated_branch.md`)

- Branch output record (to include in history):
```
{
  "id": "llm-1",
  "status": "success|failed|canceled|timeout",
  "artifact": "<path or URL>",
  "notes": "optional details"
}
```
Schema: see `jobs/schemas/branch_record.schema.json`.

### First-Success-Wins

- Orchestrator monitors all branches concurrently.
- On first `status=success`:
  - Mark `winner = <branch_id>`
  - Issue cancellation to other branches:
    - For Nomad jobs: `internal/orchestration.DeregisterJob(jobName, purge=true)`
    - For human-step: stop watcher and mark `canceled`
  - Gather artifacts/logs from winner; apply final patch if not yet applied (for ORW branches it is already applied in the branch executor).
  - See `jobs/human_step_watcher.md` for watcher interface.

### History JSON

- Build `history.json` for reducer input:
```
{
  "plan_id": "<plan_id>",
  "branches": [ <branch records...> ],
  "winner": "<branch_id>"
}
```
- Validate against `jobs/schemas/history.schema.json` before passing to reducer.

### Cancellation Semantics

- LLM/ORW branch jobs should be idempotent on cancel; ensure they write partial logs but no success artifact.
- Orchestrator treats canceled branches as terminal; no retries once a winner exists.
See `jobs/cancellation.md` for signals and idempotency rules.

### Error Paths

- If all branches end in failed/timeout, orchestrator may:
  - Re-invoke planner (bounded retries via `self_heal.max_retries`), or
  - Halt for human-step and record outcome.

### Run Manifest

- Orchestrator writes a manifest per run with (validate against `jobs/schemas/run_manifest.schema.json`):
  - repo metadata, lane, timestamps
  - build gate outcomes
  - planner stdout + plan path
  - branch job IDs and outcomes
  - reducer stdout + next actions
  - pointers to KB case updates
Code Sketch
- See `orchestrator_fanout_sketch.md` for a Go-style pseudocode of the fan‑out loop (spawn branches, pick winner, cancel leftovers, build history.json, call reducer).
