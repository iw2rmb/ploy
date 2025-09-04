# Stream 2 / Phase 1 — LLM‑Plan + LLM‑Exec

Objective: Generate a high‑level plan via LLM and execute the plan steps in a sandboxed environment, reusing unified config/orchestration/storage.

## Reuse First

- Config/Storage: `internal/config.Service` + unified `internal/storage` for artifacts/logs.
- Orchestration: `internal/orchestration` for ephemeral jobs when needed.
- Error envelope and routing from API server.

## Scope

- HTTP entrypoint: `/v1/transflow/llm/plan_exec`
- Flow:
  1) Accept prompt/context (repo summary, diffs)
  2) LLM‑Plan: produce structured steps (JSON plan)
  3) LLM‑Exec: execute steps with strict allow‑list (file edits, simple commands); persist artifacts/logs
  4) Return plan, execution results, and links to artifacts

## Acceptance Criteria

- Enforce allow‑listed ops; reject dangerous actions
- Persist logs and outputs; include links in response
- Unit tests stub the LLM and assert shape + policy enforcement

## TDD Plan

1) RED: Tests for plan shape, allow‑list enforcement, storage persistence
2) GREEN: Implement minimal LLM stub + executor with allow‑list
3) REFACTOR: Factor plan/exec interfaces for future backends

