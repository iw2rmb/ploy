# AGENTS.md

**MANDATORY**: Follow this file for every prompt execution.

## TDD Framework (CRITICAL)

- **LOCAL**: Unit tests, build compilation (RED/GREEN phases)  
- **VPS**: Integration/E2E tests (REFACTOR phase)
- **Coverage**: 60% minimum, 90% for critical components
- **Cycle**: RED (write failing tests) → GREEN (minimal code) → REFACTOR (on VPS)

## Ploy Overview

Deployment lanes A-G auto-selected by project structure. Update `FEATURES.md`, `CHANGELOG.md` for changes.  
**WASM features**: Reference `docs/WASM.md` for Lane G implementation.

## Go Formatting (MANDATORY)

- After editing any Go file, run `goimports` and `gofmt` to automatically fix imports and format code.
- Before running tests or committing, run `staticcheck` for static analysis to catch bugs and code smells.
- Recommended commands:
  - `goimports -w .` (updates imports and writes changes)
  - `gofmt -s -w .` (simplifies and formats code)
  - `staticcheck ./...` (runs static analysis across the module)

## VPS Testing

**Setup**: `ssh root@$TARGET_HOST` → `su - ploy`  
**Nomad**: ONLY use `/opt/hashicorp/bin/nomad-job-manager.sh` (never direct `nomad` commands)

## Commands

**LOCAL**: 
- `make test-unit`, `make test-coverage-threshold`, build verification
- Deploy API: `./bin/ployman api deploy --monitor` (run on workstation)

Notes:
- Run `./bin/ployman api deploy --monitor` on your workstation. Do not run it on the VPS.
- Never use direct Nomad commands; if needed remotely, only via `/opt/hashicorp/bin/nomad-job-manager.sh` as invoked by platform tooling.
- Container images (platform services and job runners) must be pushed to the VPS Docker Registry (Docker Registry v2). Do not rely on public registries in VPS workflows.
  - Examples: `openrewrite-jvm`, `langgraph-runner`, lane-specific images.
  - Configure image refs in environment (e.g., `TRANSFLOW_ORW_APPLY_IMAGE`, `TRANSFLOW_PLANNER_IMAGE`, `TRANSFLOW_REDUCER_IMAGE`, `TRANSFLOW_LLM_EXEC_IMAGE`) to point at the internal registry.

**VPS**:
- Use for runtime inspection and logs only (e.g., `ssh root@$TARGET_HOST`, then `su - ploy`).
- Do not run `ployman` deploys directly on the VPS.

**NEVER**: Integration tests locally, direct Nomad commands

## Nomad Integration (RECOMMENDED)

- VPS (production/test clusters):
  - Mandatory: submit jobs via `/opt/hashicorp/bin/nomad-job-manager.sh`.
  - Rationale: central retries/backoff for 429/5xx, HCL→JSON conversion via `nomad job run -output`, environment defaults, and Consul service cleanup.
  - In code, the orchestration layer auto-detects the wrapper and uses it for submit/wait/log flows.

- Non‑VPS (local/dev tools, CI):
  - Use the official Nomad SDK with resilience:
    - HTTP retry/backoff for 429/5xx with jitter (config via env: `NOMAD_HTTP_MAX_RETRIES`, `NOMAD_HTTP_BASE_DELAY`, `NOMAD_HTTP_MAX_DELAY`).
    - Concurrency limits for submissions (env: `NOMAD_SUBMIT_MAX_CONCURRENCY`, default 4).
    - Prefer blocking queries for status when possible; avoid tight polling.
    - Use blocking queries with index/wait (config via env: `NOMAD_BLOCKING_WAIT`, default `30s`).

- Never call raw `nomad` CLI from app code on the VPS. If a direct CLI call is unavoidable, route it through the job manager wrapper.

## Docker Registry Usage (VPS)

- The VPS provides an internal Docker Registry (Docker Registry v2) for storing and serving images used by Nomad jobs.
- All platform images should be published to this registry and referenced by fully qualified names in Nomad specs and environment variables.
- Typical images:
  - `openrewrite-jvm` — ORW apply job (produces `output.tar` and `diff.patch`)
  - `langgraph-runner` — Planner/Reducer/LLM-exec jobs (produces `plan.json`, `next.json`, `diff.patch`)
- Avoid external registries (e.g., GHCR) for VPS job execution paths unless explicitly allowed.


## Mandatory Update Protocol (CRITICAL)

For EVERY code change:

1. **Write failing tests** (RED phase)
2. **Write minimal code** to pass tests (GREEN phase)  
3. **Deploy to VPS** for integration testing (REFACTOR phase)
4. **Update documentation** (`CHANGELOG.md`, `FEATURES.md` as needed)
5. **Merge to main** and return to worktree branch

**NO EXCEPTIONS**.

## Specialized Agents

Use Task tool for complex domain-specific tasks. Available agents in `.claude/agents.json`.

## Sessions System Behaviors

@CLAUDE.sessions.md
