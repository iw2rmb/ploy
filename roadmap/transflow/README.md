# Transflow Roadmap (MVP → Iteration)

Goal: Orchestrate automated code transformation flows reusing the platform’s existing building blocks (ARF, orchestration, unified storage/config), delivered in thin, incremental phases that can ship and run ASAP.

Streams are designed to be largely independent, minimizing interference and enabling parallel delivery. Each stream is split into phases under `roadmap/transflow/stream-*/phase-*.md` with clear acceptance criteria and TDD guidance.

## Streams Overview

- Stream 1: MVP Transflow (Implemented + In Progress)
  - Phase 1 ✅ (Implemented): OpenRewrite recipe → Build check (no deploy) via `/v1/apps/:app/builds`
  - Phase 2 ⚠️ (In Progress): LangGraph planner/reducer as jobs for build‑error healing with parallel options (human, LLM‑exec, ORW‑generated)

- Stream 2: LLM Execution + Planning
  - Phase 1 ⚠️ (Partial): LLM‑Exec (sequential) as Nomad jobs, MCP context, ✅ model registry under `llms` (used by Stream 1 Phase 2 branches)
  - Phase 2: LLM‑Plan (LangGraph) to emit exec steps; optional beyond MVP since planner/reducer exist in Stream 1

- Stream 3: Merge Request to GitLab
  - Phase 1: Create/update MR on GitLab (project from `target_repo`, target `base_ref`)
  - Phase 2: Provider abstraction + GitHub (optional)

## Principles

- Reuse, don’t fork: Prefer `internal/arf`, `internal/orchestration`, unified storage/config, existing error envelope.
- TDD + phased rollout: Keep phases small with passing unit tests; validate E2E on VPS per CLAUDE.md.
- Stable interfaces: New capability should be surfaced behind existing facades where possible.

## MVP Additions: LangGraph as Jobs
- Planner job: Inputs = repo metadata + last_error + budgets + allowlists + KB snapshot. Output = `plan.json` with parallel options (human‑step, llm‑exec, ORW‑generated).
- Orchestrator fan‑out: Submit each option as its own Nomad job (separate branch); first success wins; cancel others.
- Reducer job: Inputs = run history (branch results, winner). Output = next actions (often stop; else new plan).
- Learning: Jobs read/write KB in SeaweedFS (cases, summaries) and Consul KV (locks). Error signatures and patch fingerprints dedup fixes.
  - See `roadmap/transflow/kb.md` for KB layout, canonicalization, compaction, and promotion rules.

## Job Types (Healing Options)
- human-step: Pause and wait for human intervention (e.g., push MR or commit); runner polls for branch updates.
- llm-exec: Direct LLM patch generation (diff-only) with MCP context/tools; apply and build-check.
- orw-gen → openrewrite: Use LLM to generate an OpenRewrite recipe (class/coords) from error context, then run the OpenRewrite job to apply and build-check.

Each job type is executed as an isolated Nomad job with explicit allowlists and budgets; LangGraph plans which to launch in parallel and the reducer reconciles results.

## Conventions
- Branch names: `workflow/<id>/<timestamp>` (workflow) and `exec/<id>/<seq>` (future parallel).
- Build app name: `tfw-<id>-<timestamp>`.
- Global controls: `lane` (override), `build_timeout` (default 10m) in transflow.yaml.
- MR defaults: add labels/scope `ploy`, `tfl` when supported; always include them in description.
