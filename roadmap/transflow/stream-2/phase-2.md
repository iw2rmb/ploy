## Stream 2 · Phase 2 — LLM-Plan → LLM-Exec (Sequential)

Goal: add `llm-plan` that emits a sequence of `llm-exec` steps, then run them sequentially as Nomad jobs via internal/orchestration. Optional beyond MVP (LangGraph planner/reducer already covers healing planning).

Scope
- `llm-plan` consumes repo state + last error (stdout/stderr) and outputs JSON list of `llm-exec` specs.
- Execute produced exec-steps in order, committing after each.
- Respect global `lane` and `build_timeout` and run build after plan completes.

Reuse
- `llm-exec` from Phase 1; runner orchestration from Stream-1.

Implementation Steps
- Add `llm-plan` job (Nomad) that reads repo state + last error, emits JSON array of exec specs to stdout.
- Define plan output schema and validate before scheduling `llm-exec` jobs.
- Sequentially submit each `llm-exec` via `internal/orchestration`; commit after each; run final build.
- Unit tests: plan schema validation; sequential exec flow; failure propagation when an exec yields invalid diff.

Deliverables
- Plan runner with clear input/output contract and logs; submits resulting `llm-exec` jobs via platform job manager.

Acceptance
- For a failing build, plan emits 1+ exec steps; after execution, build passes.

Out of scope
- Parallel exec.
