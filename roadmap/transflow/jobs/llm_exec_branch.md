## LLM-Exec Branch (Contract)

The `llm-exec` branch generates a minimal, deterministic code patch via an LLM chain (diff-only), applies it, commits, and runs the build gate.

### Planner → Branch Inputs
- From `plan.json` option with `type=llm-exec` and inputs (example):
```
{
  "id": "llm-1",
  "type": "llm-exec",
  "inputs": {
    "prompts": [
      "Fix compile errors after migrating to Java 17; prefer minimal changes; do not add new dependencies unless required"
    ],
    "context": [
      "src/main/java/**/*.java",
      "pom.xml"
    ],
    "limits": { "max_steps": 8, "max_tool_calls": 12, "timeout": "30m" }
  }
}
```

### Job Env/Args (example)
- Env:
  - `MODEL` — resolved via llms registry (e.g., `gpt-4o-mini@2024-08-06`)
  - `TOOLS` — JSON allowlist config for MCP tools (`file`, `search`, `build`, optional `openrewrite`)
  - `LIMITS` — JSON limits (steps/tool_calls/timeout/tokens)
  - `CONTEXT_DIR` — `/workspace/context` (prefetched files + optional fetched URLs)
  - `OUTPUT_DIR` — `/workspace/out`
  - `RUN_ID` — unique id
- Args:
  - `--mode exec` (runner-specific; subject to actual image)

### Execution Flow
1) Prefetch context (done by orchestrator) to `CONTEXT_DIR`.
2) Run an LCEL chain: prompt → model → structured output parser → diff validator.
3) Write unified diff to `out/diff.patch`.
4) Orchestrator applies patch to the workflow branch and commits with a descriptive message.
5) Trigger build gate; set branch status by build result.

### Outputs
- Success: branch record `status=success` with `artifact` pointing to `diff.patch` (and commit hash recorded in the run manifest).
- Failure: branch record `status=failed`; include brief notes.

### Safety & Limits
- Unified diff only; reject binary changes and paths outside allowlist.
- Keep edits minimal; format/lint under lane policy if configured.
- Honor budgets/timeouts; default deny network except model and allowed MCP endpoints.

### Suggested Commit Message Template
```
feat(java17): minimal compile fix for JDK 17

Reason: build failed after Java11→17 migration
Scope: limited to files in diff; no dependency changes unless necessary
```

