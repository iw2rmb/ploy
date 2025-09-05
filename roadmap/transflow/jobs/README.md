## LangGraph Job Interfaces (Planner/Reducer)

This doc specifies the contract for running LangGraph as short‑lived Nomad jobs. Jobs are containerized and invoked via `internal/orchestration`; the orchestrator is responsible for preparing inputs (env/args, context files) and consuming outputs (artifacts, stdout JSON).

### Common Contract

- Driver: `docker` (or `containerd`); Job type: `batch`.
- Image: pinned `langchain/langgraph` runner image (Python) with MCP adapters and validators.
- Volumes:
  - `/workspace/context`: read‑only prefetched repo files and fetched URLs (per run).
  - `/workspace/kb`: optional read‑only KB snapshot (cases/summary/index bundles).
  - `/workspace/out`: writable artifacts dir (planner: `plan.json`; reducer: `next.json`).
- Env (shared):
  - `MODEL` — model identifier `name@version` (resolved via llms registry).
  - `TOOLS` — JSON string allowlist for MCP tools (e.g., file/search/build/openrewrite) and their scoped config. Validate against `schemas/tools.schema.json`.
  - `LIMITS` — JSON with limits: `{ "max_steps": N, "max_tool_calls": N, "timeout": "30m" }`. Validate against `schemas/limits.schema.json`.
  - `CONTEXT_DIR` — `/workspace/context` (mounted by orchestrator).
  - `KB_DIR` — `/workspace/kb` if present; empty otherwise.
  - `OUTPUT_DIR` — `/workspace/out` (job writes artifacts here).
  - `RUN_ID` — orchestrator run identifier (for logging/trace correlation).
  - `LANGCHAIN_TRACING_V2` / `LANGSMITH_*` — optional; default off.
- Args:
  - `--mode planner|reducer` — selects the runnable.
  - `--input <file>` — optional path to extra JSON input (e.g., reducer history).

### Planner Job

- Inputs
  - Env `MODEL`, `TOOLS`, `LIMITS`, `CONTEXT_DIR`, optional `KB_DIR` (KB snapshot), `OUTPUT_DIR`.
  - Planner reads normalized last_error and minimal repo metadata from `CONTEXT_DIR/inputs.json` (written by orchestrator), e.g.:
    `{ "language": "java", "lane": "C", "last_error": {"stdout": "...", "stderr": "..."}, "deps": {"pom.xml": "sha256:..."} }`.
- Outputs
  - `STDOUT`: a compact JSON with status and pointers, e.g. `{ "ok": true, "plan": "out/plan.json" }`.
  - `OUTPUT_DIR/plan.json`: validated plan schema:
    `{ "plan_id": "...", "options": [ {"id": "human-1", "type": "human"}, {"id": "llm-1", "type": "llm-exec", "inputs": {...}}, {"id": "orw-1", "type": "orw-gen", "inputs": {...}} ] }`.
  - `OUTPUT_DIR/manifest.json`: run manifest (prompts/version/checksums/timing).
- Exit codes
  - `0`: success with `plan.json`
  - `2`: planner could not propose options (escalate to human-step)
  - non‑zero: fatal error (retry per policy or halt)

### Reducer Job

- Inputs
  - Env `MODEL`, `TOOLS`, `LIMITS`, `OUTPUT_DIR`.
  - Arg `--input /workspace/context/history.json` containing branch results summary, for example:
    `{ "plan_id": "...", "branches": [ {"id": "llm-1", "status": "success", "artifact": "s3://.../diff.patch" }, {"id": "orw-1", "status": "failed" } ], "winner": "llm-1" }`.
- Outputs
  - `STDOUT`: `{ "ok": true, "next": "out/next.json" }`.
  - `OUTPUT_DIR/next.json`: `{ "action": "stop" }` or `{ "action": "new_plan", "plan": "..." }`.
  - `OUTPUT_DIR/manifest.json`: run manifest (as above).
- Exit codes
  - `0`: success; next actions produced
  - `3`: ambiguous outcome (orchestrator should halt for human-step)
  - non‑zero: fatal error (retry per policy)

### Tool Allowlist (TOOLS)

Example:
```
{
  "file": { "allow": ["src/**", "pom.xml"] },
  "search": { "provider": "rg", "allow": ["src/**"] },
  "build": { "endpoint_env": "PLOY_CONTROLLER", "app": "${APP_NAME}", "env": "dev" },
  "openrewrite": { "allow_groups": ["org.openrewrite.recipe"], "timeout": "10m" }
}
```

### Limits (LIMITS)

```
{ "max_steps": 8, "max_tool_calls": 12, "timeout": "30m", "max_tokens": 200_000 }
```

### Artifacts

- Planner: `out/plan.json`, `out/manifest.json`; optional traces under `out/logs/`.
- Reducer: `out/next.json`, `out/manifest.json`; optional traces under `out/logs/`.

Schemas:
- Plan schema: see `schemas/plan.schema.json` (plan_id + options[] of type human|llm-exec|orw-gen).
- Next schema: see `schemas/next.schema.json` (action stop|new_plan; optional embedded plan).

### Error Handling

- All non‑zero exits must include a short JSON on STDOUT: `{ "ok": false, "error": "..." }`.
- Orchestrator records STDOUT/STDERR and artifacts paths into the run manifest in SeaweedFS.

## Orchestrator Glue

1) Prepare host directories:
   - Context (planner): write `inputs.json` with {language, lane, last_error{stdout,stderr}, deps…} under a per-run dir; mount as `transflow-context`.
   - KB snapshot (optional): sync `cases/`, `summaries/` and optional vector index bundle into a dir; mount as `transflow-kb`.
   - History (reducer): write `history.json` under a per-run dir; mount as `transflow-history`.
   - Out: create a writable dir per run; mount as `transflow-out` to collect artifacts.

2) Build env strings:
   - MODEL: resolve via llms registry (e.g., `gpt-4o-mini@2024-08-06`).
   - TOOLS_JSON: JSON string for allowlisted tools (file/search/build/openrewrite) and config.
   - LIMITS_JSON: JSON string for limits (steps/tool_calls/timeout/tokens).
   - RUN_ID: set to a unique per-run id.

3) Replace HCL placeholders `${MODEL}`, `${TOOLS_JSON}`, `${LIMITS_JSON}`, `${RUN_ID}` with concrete values before submit.

4) Submission & Wait:
   - For batch jobs (planner/reducer), prefer a `SubmitAndWaitTerminal` approach: register job, poll allocation/task state until terminal (complete/failed), then collect artifacts. `WaitHealthy` is not ideal for batch tasks.
   - For long-running tasks, `WaitHealthy` applies; not used here.

## Go Example: Submitting Planner Job

```go
package examples

import (
    "fmt"
    "os"
    "strings"
    orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

func SubmitPlanner(model, toolsJSON, limitsJSON, runID, hclPath string) error {
    // Read template
    raw, err := os.ReadFile(hclPath)
    if err != nil { return err }
    hcl := string(raw)

    // Replace placeholders (simple example; use a real template engine in prod)
    replacer := strings.NewReplacer(
        "${MODEL}", model,
        "${TOOLS_JSON}", toolsJSON,
        "${LIMITS_JSON}", limitsJSON,
        "${RUN_ID}", runID,
    )
    rendered := replacer.Replace(hcl)

    // Write to a temp file
    tmp := fmt.Sprintf("/tmp/%s-planner.hcl", runID)
    if err := os.WriteFile(tmp, []byte(rendered), 0644); err != nil { return err }

    // Submit and wait for the task to complete (healthy alloc = task started)
    if err := orchestration.SubmitAndWaitHealthy(tmp, 1, 30*60*1e9); err != nil { // 30m
        return err
    }
    // Orchestrator can now read artifacts from the mounted `transflow-out` host dir
    return nil
}
```

Repeat for `reducer.hcl` with an `--input` history.json path in the mounted history dir.
### LLM-Exec Job

- Template: see `llm_exec.hcl`.
- Runner args: `--mode exec` (the image's entrypoint should interpret this and produce a unified diff at `/workspace/out/diff.patch`).
- Env: `MODEL`, `TOOLS` (see tools.schema.json), `LIMITS` (see limits.schema.json), `CONTEXT_DIR`, `OUTPUT_DIR`, `RUN_ID`.
- Volumes: mount prefetched context read-only at `/workspace/context` and artifacts dir at `/workspace/out`.
- Orchestrator must validate the produced diff (see `../../diff_validator.md`) before apply/commit.
