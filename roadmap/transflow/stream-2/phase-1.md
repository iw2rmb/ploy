# Stream 2 · Phase 1 — LLM‑Exec (Sequential) + MCP Context

Goal: introduce `llm-exec` step that can produce deterministic patches, sequential only, executed as Nomad jobs via internal/orchestration. Used as a healing branch in Stream 1 Phase 2.

Reuse First
- Config/Storage: `internal/config.Service` + unified `internal/storage` for artifacts/logs.
- Orchestration: submit jobs via platform Nomad job manager (no direct `nomad`).
- Git: reuse repo checkout and commit utilities; extend for applying validated diffs.

Scope
- Model registry (CRUD) in `ployman` CLI (schema validation only), stored under `llms` bucket.
- `llm-exec` runner: `model@version`, `prompts[]`, `context[]` (repo files/globs + HTTPS URLs prefetched per run), `mcp_tools[]` (env-only per MCP spec).
- Output must be unified diff or no-op; validate and apply; then commit.
- Build check after exec using global `lane` and `build_timeout`.

Implementation Steps
- Add `ployman models` commands: `list|get|add|update|delete` storing JSON under `llms/` in configured artifacts storage.
- Define a minimal job template (HCL) for `llm-exec` that mounts workspace snapshot and injects MCP env.
- Submit job via `internal/orchestration.SubmitAndWaitHealthy` (facade). Collect logs via unified storage or job logs stub.
- Validate diff output (reject non-diff responses); apply using git utilities; commit; run build step.

Acceptance
- Given a prompt that refactors a file, runner applies validated diff, commits, and build passes.

TDD Plan
1) RED: tests for diff validation + apply, context prefetch, and job submission contract.
2) GREEN: minimal job that returns a deterministic diff; apply and commit; run build.
3) REFACTOR: clean interfaces for job submissions and model registry lookup.

Implementation Steps
- ployman CLI: implement `models list|get|add|update|delete` storing JSON blobs under `llms/` bucket (artifacts storage), schema: {name, version, provider, params}.
- Job HCL: add minimal Nomad job template for `llm-exec` that mounts a workspace volume, injects MCP envs, and writes unified diff to stdout.
- Runner: resolve model via ployman registry; prefetch context files/URLs to a temp path; submit job via `internal/orchestration.SubmitAndWaitHealthy`; capture logs; parse diff; apply via git; commit; run build step.
