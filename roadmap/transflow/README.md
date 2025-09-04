# Transflow Roadmap (MVP → Iteration)

Goal: Orchestrate automated code transformation flows reusing the platform’s existing building blocks (ARF, orchestration, unified storage/config), delivered in thin, incremental phases that can ship and run ASAP.

Streams are designed to be largely independent, minimizing interference and enabling parallel delivery. Each stream is split into phases under `roadmap/transflow/stream-*/phase-*.md` with clear acceptance criteria and TDD guidance.

## Streams Overview

- Stream 1: MVP Transflow (OpenRewrite + Self‑Healing)
  - Phase 1: Run OpenRewrite recipe → build → on failure trigger self‑healing and re‑build
  - Phase 2: Expand recipes catalog + policy wiring (size caps, lane rules)
  - Phase 3: Observability (health, metrics, traces) and retry policies

- Stream 2: LLM‑Exec with LLM‑Plan
  - Phase 1: Generate plan (LLM‑Plan) and execute steps (LLM‑Exec) with sandbox; reuse unified config, storage, orchestration
  - Phase 2: Safety/policy gates (allow‑list ops, diff checks), attach logs/artifacts to storage
  - Phase 3: Feedback loop (summarize outcomes, feed back into plans)

- Stream 3: Merge Request to GitLab
  - Phase 1: Create MR with diffs and status, reuse internal/git + new GitLab client; config via config service
  - Phase 2: MR updates (comments, pipeline status, artifacts links)
  - Phase 3: Policy approval gates + auto‑merge rules

## Principles

- Reuse, don’t fork: Prefer `internal/arf`, `internal/orchestration`, unified storage/config, existing error envelope.
- TDD + phased rollout: Keep phases small with passing unit tests; validate E2E on VPS per CLAUDE.md.
- Stable interfaces: New capability should be surfaced behind existing facades where possible.

