## MVP: transflow (Minimal Viable Implementation)

Purpose: deliver a basic, working transflow that can apply OpenRewrite recipes, heal build errors via LangGraph planner/reducer jobs with parallel options, and open a GitLab MR.

## Implementation Status

✅ **Fully Implemented:**
- OpenRewrite recipe execution with ARF integration
- Build check via `/v1/apps/:app/builds` (sandbox mode, no deploy)
- YAML configuration parsing and validation
- Git operations (clone, branch, commit, push)
- Diff validation and application utilities
- GitLab MR integration with environment variable configuration
- Complete CLI integration (`ploy transflow run`) with full end-to-end workflow
- Test mode infrastructure with mock implementations for CI/testing

⚠️ **Partially Implemented:**
- LangGraph healing (planner/reducer infrastructure exists, orchestration in progress)
- Self-healing config parsing and data structures
- Job templates for healing branches (human-step, llm-exec, orw-generated)

❌ **Not Implemented:**
- Model registry in `ployman` CLI
- KB (knowledge base) read/write for learning

### In-Scope (Must Work)

- OpenRewrite recipe execution
  - Reuse existing ARF/OpenRewrite pipeline (services/openrewrite-jvm, API/CLI `ploy arf transform`).
  - Input: repo URL + branch, ordered recipe IDs. Output: local patch/commit applied to a workflow branch.

 - LLM plan and exec
  - `llm-exec` (Stream 2/Phase 1): run model with prompts + prefetched context (repo paths, HTTPS URLs), apply generated patch, commit; execute as Nomad jobs via internal/orchestration.
  - `llm-plan` (Stream 2/Phase 2): produce a list of `llm-exec` steps (sequential only in MVP) from repo state + last error.
  - Model registry: minimal CRUD in `ployman` CLI with schema validation; stored under `llms` namespace.
  - MCP tools: declare per-step (MCP spec), env-only config; prefetch context per execution (no cache/persist).

- Build check (sandbox mode, no deploy)
  - Use existing build API to verify the code builds without deploying.
  - Client: `internal/cli/common/deploy.go::SharedPush` (POST tarball to controller `POST /v1/apps/:app/builds?sha=...&env=dev[&lane=...]`).
  - Server handler: `internal/build/trigger.go` performs lane detection (if lane not provided), builds artifacts (Unikraft/OSv/OCI/etc.), signs, generates SBOM, uploads to storage; it does not deploy to Nomad.
  - Success criteria: HTTP 200; response includes `lane`, `image` or `dockerImage`. For logs, optional `GET /v1/apps/:app/logs`.
  - Status endpoint (optional): `GET /v1/apps/:app/status` for additional visibility.
  - App naming: derive unique app name `tfw-<transflow-id>-<timestamp>` unless explicitly provided.
  - Lane override: allow explicit `lane` in transflow; fallback to auto-detect when absent.
  - Timeout: request timeout configurable per run; default 10m for build check.

 - Git operations + MR (Stream 3)
  - Create a workflow branch at start; commit changes from steps; push branch to origin using provided token envs.
  - GitLab MR: infer project from `target_repo`, target branch is `base_ref`; envs `GITLAB_URL`, `GITLAB_TOKEN`.
  - Apply default MR labels/scope: `ploy`, `tfl` (if available).

- LangGraph healing (planner/reducer as jobs)
  - Planner job: Input = repo metadata + last_error + KB snapshot; Output = `plan.json` with parallel options: human‑step, llm‑exec, orw‑generated.
  - Orchestrator fan‑out: run each option as an independent Nomad job (branch); first success wins; cancel others.
  - Reducer job: Input = branch results + winner; Output = next actions (usually stop; else new `plan.json`).
  - KB: cases and summaries persisted in SeaweedFS; locks via Consul KV; error/patch dedup via normalized signatures/fingerprints.
  - Consistent learning & dedup: see `roadmap/transflow/kb.md`. Planner reads `summary.json`; branches write `cases/` and patch blobs; compactor maintains summaries and optional vector bundles.
  - Job types:
    - human-step: wait for human MR/commit; runner polls branch; success if build passes afterward.
    - llm-exec: generate diff-only patch (MCP tools allowed); apply and build-check.
    - orw-gen → openrewrite: generate ORW recipe (class/coords) via LLM; run OpenRewrite job; build-check.

- Transflow runner (orchestrator)
  - Parse `roadmap/transflow/transflow.yaml` and run steps sequentially: recipe → llm-plan → llm-exec → MR.
  - Minimal logging; write per-step logs and final summary.

### What Already Exists (Leverage)

- OpenRewrite execution: `services/openrewrite-jvm`, ARF endpoints and CLI (`internal/cli/arf/*`, `api/arf/*`).
- Git clone/diff/commit helpers: `api/arf/git_operations.go` (extend with push).
- Lane/build/deploy system (not used in MVP flow): lane detection, Nomad templates, push deploy (`internal/cli/common/deploy.go`).
- Git provider envs expected for MR (GitLab): set via environment variables.

### New Work (Minimal)

- New `transflow` orchestrator
  - CLI entry: `ploy transflow run -f transflow.yaml`.
  - Step engine: call existing ARF recipe execution; implement LLM plan/exec runners; manage workflow branch lifecycle; run Build check before MR.

- LLM runners
  - `llm-exec`: container/job that receives model+prompts+context; produces unified diff or patch; apply and commit (used in parallel healing options).
  - MCP injection via env; context prefetcher for repo files and HTTPS URLs.

- Model registry
  - `ployman models {list|get|add|update|delete}` with basic schema validation; stored alongside existing recipe metadata.

 - MR integration (GitLab)
  - Push branch using token env; call GitLab REST to create/update MR (project inferred from `target_repo` URL, source branch, target=`base_ref`), adding default labels `ploy`,`tfl` when supported.

- Build check integration
  - Implement a `build` step that tars the workspace and invokes `SharedPush` with `IsPlatform=false`, `Environment=dev`, and optional lane override from transflow.
  - Expand `DeployConfig` to include `Timeout time.Duration`; `SharedPush` honors `config.Timeout` sourced from transflow's global `build_timeout`.
  - Treat non-200 as build failure; record response JSON in step logs for diagnostics.

 - LangGraph planner/reducer integration
  - Add job templates and CLI/orchestrator glue to invoke planner after a failing build, and reducer after branch completion.
  - Persist `plan.json`, branch job results, and a compact run manifest; first‑success‑wins cancellation.

### Interfaces (High-Level)

- Input: `roadmap/transflow/transflow.yaml` (id, repo, branch, steps; global lane/build_timeout).
- Output: GitLab MR link, `plan.json`/branch results, step logs, final summary JSON.
- Env: `GITLAB_URL`, `GITLAB_TOKEN` for MR; model/MCP creds via env only.

### Out of Scope (MVP)

- Parallel execution and branch-per-exec merging.
- Secure network policy and allowlists.
- Budgets/limits (tokens, cost, time).
- Static analysis, vulnerability scans, compile gates.
- Deploy/test gates and lane-driven runtime checks.
- SeaweedFS/Consul KV persistence beyond what ARF already uses implicitly.
- Auto-formatters outside of step-specific behavior.
- End-to-end integration tests; coverage thresholds.

### Risks and Mitigations

- LLM determinism: constrain outputs to unified diff/patch; validate before apply.
- Context quality: ensure prefetch resolves globs/URLs deterministically at run time; log sources.
- MR creation: handle idempotency (update existing MR for same branch).

### Next Steps

1) Add `ploy transflow run` CLI + YAML parser. 2) Wire recipe step to ARF. 3) Implement LangGraph planner/reducer jobs and orchestrator fan‑out logic. 4) Implement LLM‑exec runner with MCP/env/context prefetch (for branch options). 5) Implement git push + GitLab MR creation. 6) Minimal logs and summary output.
### Test Case: JDK 11 → 17 Migration

- Minimal transform request (single OpenRewrite recipe step):

POST /v1/transforms
Content-Type: application/json

{
  "id": "java11to17",
  "target_repo": "https://git.example.com/org/app.git",
  "base_ref": "refs/heads/main",
  "target_branch": "refs/heads/main",
  "lane": "C",
  "steps": [
    {
      "type": "recipe",
      "engine": "openrewrite",
      "recipes": [
        "org.openrewrite.java.migrate.Java11toJava17"
      ]
    }
  ]
}

- Expected flow:
  1) Apply ORW recipe; commit on workflow/<id>/<timestamp>.
  2) Build check via /v1/apps/:app/builds (no deploy).
  3) If build fails, run LangGraph planner job; branches: human-step, llm-exec, orw-gen→openrewrite.
  4) First success wins; reducer finalizes next actions (usually stop).

## Test Repository

**Java 11 Maven Test Repository for OpenRewrite Migration Testing:**
- **URL**: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
- **Purpose**: Real Java 11 codebase with OpenRewrite-transformable patterns for testing complete transflow workflows
- **Contains**: Maven project structure, Java 11 code patterns (string operations, Optional usage, stream processing), JUnit 5 tests
- **Used by**: Self-healing tests, integration tests, CLI validation with real repository scenarios
- **Configuration**: Pre-configured with OpenRewrite Maven plugin for Java 11→17 migration recipes

## CLI Usage and Examples

### Basic Usage

```bash
# Run a complete transflow workflow
ploy transflow run -f transflow.yaml

# Run with verbose output
ploy transflow run -f transflow.yaml --verbose

# Dry run to validate configuration
ploy transflow run -f transflow.yaml --dry-run

# Use test mode for development/CI
ploy transflow run -f transflow.yaml --test-mode
```

### Configuration Examples

**Basic Java 11→17 Migration:**
```yaml
# transflow.yaml
version: v1alpha1
id: java11-to-17-migration
target_repo: https://gitlab.com/your-org/your-java-project.git
target_branch: refs/heads/main
base_ref: refs/heads/main
lane: C
build_timeout: 15m

steps:
  - type: recipe
    id: java-migration
    engine: openrewrite
    recipes:
      - org.openrewrite.java.migrate.Java11toJava17
      - org.openrewrite.java.cleanup.CommonStaticAnalysis
      - org.openrewrite.java.RemoveUnusedImports

self_heal:
  enabled: true
  max_retries: 2
  cooldown: 30s
```

**With GitLab MR Creation:**
```bash
# Set GitLab environment variables
export GITLAB_URL=https://gitlab.com
export GITLAB_TOKEN=your-gitlab-token

# Run workflow with MR creation
ploy transflow run -f transflow.yaml
```

**Multiple Recipe Steps:**
```yaml
version: v1alpha1
id: comprehensive-cleanup
target_repo: https://gitlab.com/your-org/legacy-project.git
target_branch: refs/heads/modernization
base_ref: refs/heads/main

steps:
  - type: recipe
    id: import-cleanup
    engine: openrewrite
    recipes:
      - org.openrewrite.java.RemoveUnusedImports
      - org.openrewrite.java.OrderImports

  - type: recipe
    id: code-modernization
    engine: openrewrite
    recipes:
      - org.openrewrite.java.cleanup.SimplifyBooleanExpression
      - org.openrewrite.java.cleanup.UnnecessaryParentheses
      - org.openrewrite.java.migrate.Java11toJava17

self_heal:
  enabled: true
  max_retries: 3
```

### Advanced Options

**Specialized Execution Modes:**
```bash
# Execute only planner step (for debugging healing workflows)
ploy transflow run -f transflow.yaml --render-planner

# Execute only LLM step with custom model
TRANSFLOW_MODEL=gpt-4o-mini@2024-08-06 \
ploy transflow run -f transflow.yaml --exec-llm-first

# Execute OpenRewrite application step
ploy transflow run -f transflow.yaml --exec-orw-first

# Apply first successful transformation and stop
ploy transflow run -f transflow.yaml --apply-first
```

**Testing and Development:**
```bash
# Test mode - uses mock implementations for all external services
ploy transflow run -f transflow.yaml --test-mode

# Plan mode - shows execution plan without running
ploy transflow run -f transflow.yaml --plan

# Reduce mode - processes healing results
ploy transflow run -f transflow.yaml --reduce
```

### Expected Workflow Output

```
Starting Transflow: java11-to-17-migration
✓ Cloning repository: https://gitlab.com/your-org/your-java-project.git
✓ Creating branch: workflow/java11-to-17-migration/20250905151234
✓ Executing recipe: org.openrewrite.java.migrate.Java11toJava17
✓ Committing changes: Apply Java 11 to 17 migration recipes
✓ Building project: tfw-java11-to-17-migration-20250905151234
✓ Pushing branch to remote
✓ Creating GitLab MR: https://gitlab.com/your-org/your-java-project/-/merge_requests/42

Workflow completed successfully!
  Branch: workflow/java11-to-17-migration/20250905151234
  Build Version: 20250905151234-abc123
  Duration: 2m 34s
  MR URL: https://gitlab.com/your-org/your-java-project/-/merge_requests/42
```

### Error Handling and Self-Healing

When builds fail and self-healing is enabled, the system will:

1. **Analyze the failure** using LangGraph planner
2. **Generate healing options** (human intervention, LLM fixes, additional recipes)
3. **Execute options in parallel** with first-success-wins logic
4. **Apply successful fix** and continue workflow
5. **Create MR** with healing summary included

**Self-Healing Output Example:**
```
⚠ Build failed: compilation errors in Main.java:15
🔧 Self-healing enabled, starting recovery...
✓ Planner job completed: 3 healing options generated
  → Option 1: llm-exec (confidence: 0.8)
  → Option 2: org.openrewrite.java.cleanup.UnnecessaryParentheses
  → Option 3: human-step
✓ Executing healing options in parallel...
✓ llm-exec option succeeded after 45s
✓ Retrying build with healed changes...
✓ Build successful: tfw-java11-to-17-migration-20250905151234-healed
✓ Continuing workflow...
```

### Environment Configuration

**Required Environment Variables:**
- `GITLAB_TOKEN`: GitLab personal access token for MR creation
- `GITLAB_URL`: GitLab instance URL (default: https://gitlab.com)

**Optional Environment Variables:**
- `TRANSFLOW_MODEL`: LLM model for healing (default: gpt-4o-mini@2024-08-06)
- `TRANSFLOW_TOOLS`: MCP tools configuration JSON
- `TRANSFLOW_LIMITS`: Execution limits configuration JSON
- `NOMAD_ADDR`: Nomad cluster address for job submission

### Integration with Existing Systems

The transflow CLI integrates seamlessly with existing Ploy infrastructure:
- **ARF Pipeline**: Reuses `ploy arf transform` for recipe execution
- **Build System**: Uses existing `/v1/apps/:app/builds` API for validation
- **Git Operations**: Leverages ARF git handling with automatic configuration
- **Job Orchestration**: Uses existing Nomad job submission infrastructure
