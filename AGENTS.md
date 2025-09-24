# AGENTS.md

**MANDATORY**: Follow this file for every prompt execution.

## Before You Start
- [ ] Commit to the RED → GREEN → REFACTOR cadence for the upcoming change.
- [ ] Plan local unit tests and coverage checks before touching code or docs.
- [ ] Verify required environment variables (`TARGET_HOST`, `PLOY_CONTROLLER`, etc.) are discoverable.
- [ ] Confirm you understand the VPS vs workstation split for the task at hand.
- [ ] Skim `docs/DOCS.md` so AGENTS.md and scoped READMEs stay aligned with documentation conventions.

## Local Development

### TDD Framework (CRITICAL)

- **LOCAL**: Unit tests, build compilation (RED/GREEN phases)
- **VPS**: Integration/E2E tests (REFACTOR phase)
- **Coverage**: 60% minimum, 90% for critical components
- **Cycle**: RED (write failing tests) → GREEN (minimal code) → REFACTOR (on VPS)

### Go Tooling (MANDATORY)

- Prefer the MCP tools shipped with `mcp-golang` for common tasks:
  - `mcp_golang__format_source` (or the `format-lint-test` playbook) instead of `make fmt`.
  - `mcp_golang__lint_package` for lint/static analysis.
  - `mcp_golang__test_with_coverage` for unit tests + coverage.
  - `run_playbook` → `format-lint-test` to chain the full RED/GREEN cycle when you want a single command.
- If the MCP server is unavailable, fall back to the legacy commands (`make fmt`, `make lint`, `staticcheck ./...`, `make test-coverage-threshold`, etc.).
- Active deployment lane: **D (Docker)** only. Legacy references to lanes A/B/C/E/F/G remain for historical context.
- Follow `.golangci.yml` and `staticcheck.conf` as the sources of truth for enabled checks.
- Keep modules tidy with `go mod tidy -v && go mod verify` when dependencies change.
- For security scans, prefer `mcp_golang__dependency_hygiene` with `enableGovulnCheck: true`; fall back to `make sec` / `govulncheck ./...` only if the MCP server is unavailable.

### Test File Naming (MANDATORY)

- Use descriptive, focused test filenames (e.g., `handler_crud_test.go`, `platform_handlers_test.go`).
- Do not use catch-all names like `_more_test.go`, `_extra_test.go`, or `_ext_test.go`.
- Split tests by concern when it improves readability and navigation.

### Pre-commit Hooks

- Install once: `pipx install pre-commit` or `pip install pre-commit`
- Enable in this repo: `pre-commit install`
- Config: `.pre-commit-config.yaml` runs `make fmt` and `golangci-lint run` on commit.
- Run manually on all files: `pre-commit run --all-files`
- CI: GitHub Actions runs pre-commit hooks on all files (`.github/workflows/ci.yml` job: Pre-commit Hooks).
- Required status check: Configure branch protection to require "CI / Pre-commit Hooks".
  - If Probot Settings is installed, `.github/settings.yml` enforces this for `main` and `develop`.
  - Otherwise, set it manually in GitHub → Settings → Branches → Branch protection rules.

## VPS Workflows

### VPS Testing

**Setup**: `ssh root@$TARGET_HOST` → `su - ploy`
**Nomad**: ONLY use `/opt/hashicorp/bin/nomad-job-manager.sh` (never direct `nomad` commands)

### Environment & Context (MANDATORY)

- Always check for required environment variables before asking the user to provide them. Prefer existing values from the session.
  - Common vars: `TARGET_HOST`, `PLOY_CONTROLLER`, `PLOY_SEAWEEDFS_URL`, `GITHUB_PLOY_DEV_USERNAME`, `GITHUB_PLOY_DEV_PAT`, `GITLAB_TOKEN`.
  - In Go/tests: use `os.Getenv` to probe; in shell: use `printenv`/`echo ${VAR:-}`. In the Codex CLI harness, also consider provided `environment_context`.
- If a variable is missing, suggest how to set it concisely; do not ask for values already present.
- Do not echo secrets verbatim in logs. Mask sensitive values when printing (e.g., show `abcd****wxyz`).
- When invoking helper scripts or subprocesses, propagate relevant env vars automatically (controller, target host, follow/lines settings, etc.).

### SSH On VPS (Latency-Safe Ops)

- Keep SSH operations short to avoid timeouts and blocking:
  - Prefer single, bounded commands per SSH call; avoid long-running tails.
  - Use explicit timeouts: `ssh -o ConnectTimeout=10` and tool-level `--timeout` flags.
  - Break multi-step flows into separate SSH calls with brief waits between steps.
  - Run commands as separate steps so progress is visible (emit concise preambles before each).
  - For logs, fetch snapshots (fixed `--lines`) rather than `--follow` by default.
  - For Nomad actions, use the wrapper’s bounded commands: `wait --timeout`, `logs --lines`, `allocs --format human`.
  - If a command may be slow (playbooks, large uploads), report progress and prefer controller/API paths when available.
  - On repeated failures/timeouts, fall back to Ansible playbooks to reconcile state instead of tight SSH loops.

### E2E via Dev API (Allowed from Workstation)

- You may run E2E tests locally when they call the VPS Dev API endpoint (ensure `PLOY_CONTROLLER` points to `https://api.dev.ployman.app/v1`).
- These tests exercise VPS services remotely and are considered VPS-side execution (REFACTOR phase), even if invoked from the workstation.
- Do not spin up or depend on local Nomad/Consul/Gateway for these tests.
- Example:
  - `E2E_LOG_CONFIG=1 go test ./tests/e2e -tags e2e -v -run TestModsE2E_JavaMigrationComplete -timeout 10m`  (assumes `PLOY_CONTROLLER` is already set)

### Platform Logs (Debugging)

- Fetch controller or proxy logs via Dev API for quick diagnosis:
  - Controller logs: `curl -sS "$PLOY_CONTROLLER/platform/api/logs?lines=200"`
  - Traefik logs: `curl -sS "$PLOY_CONTROLLER/platform/traefik/logs?lines=200"`
- Query params:
  - `lines` — number of log lines to return (default 200)
  - `follow` — set to `true` to follow (SSE-style support may be added later; currently returns snapshot)
- Notes:
  - These endpoints route through the VPS job-manager wrapper to retrieve Nomad allocation logs.
  - Task inference is automatic for known services (api → task "api", traefik → task "traefik").

- Helper script: `tests/e2e/deploy/fetch-logs.sh` aggregates app logs, platform logs, and (optionally) builder job logs via SSH. Export `APP_NAME`, and optionally `LANE`, `SHA`, `LINES`, and `TARGET_HOST`.

### Time-based Log Slicing (Recommended)

- Prefer slicing platform logs by timestamp instead of restarting services or fetching overly large windows.
- Strategy:
  1) Record a start timestamp right before triggering a build, e.g. `START_TS=$(date '+%Y-%m-%d %H:%M:%S')`.
  2) After the run, fetch platform logs and filter to lines at or after `START_TS`.
     - The Nomad job manager (`/opt/hashicorp/bin/nomad-job-manager.sh`) supports `--since "YYYY-MM-DD HH:MM:SS"` on `logs` to apply a time-based filter when log lines begin with `[YYYY-MM-DD HH:MM:SS]`.
     - Alternatively, use `tests/e2e/deploy/fetch-logs.sh` and set one of:
       - `START_TS_SOURCE=vps` (requires `TARGET_HOST`) to auto-resolve `START_TS` via `date '+%Y-%m-%d %H:%M:%S'` on the VPS (avoids timezone skew).
       - `START_TS_SOURCE=platform` to extract the latest bracketed timestamp from a small platform log snapshot.
       - Or pre-set `START_TS` manually and optionally combine with `FILTER_MARKERS` to narrow to key markers (e.g., `[Lane E]`, `[Orch]`).
- This keeps log slices small and focused per run without bouncing the API allocations.

#### Mods Scenario `SINCE_FMT` (Practical)

- For `tests/e2e/mods/orw-apply-llm-plan-seq`, derive a VPS-friendly timestamp from SSE and pass it to the Nomad log wrapper when fetching allocation logs.
  - `SINCE_RAW=$(grep -hEo '"time":"[^"]+"' tests/e2e/mods/logs/<MOD_ID>/events*.sse | head -n1 | sed -E 's/.*"time":"([^"]+)".*/\1/')`
  - `SINCE_FMT="${SINCE_RAW:0:10} ${SINCE_RAW:11:8}"`
  - `ssh -o ConnectTimeout=10 root@$TARGET_HOST "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id <ALLOC_ID> --both --lines 800 --since "$SINCE_FMT"'" > tests/e2e/mods/logs/<MOD_ID>/last_job.logs`
  - Or set `START_TS_SOURCE=vps` (requires `TARGET_HOST`) to auto-resolve the timestamp in helpers like `tests/e2e/deploy/fetch-logs.sh`.
  - If SeaweedFS isn’t reachable from the workstation, fetch artifacts via SSH on the VPS: `curl -fsS 'http://seaweedfs-filer.storage.ploy.local:8888/artifacts/<KEY>'`.

### Build-Gate Drill-down

- Build-gate errors may include `(deployment_id=…)` in events and an `X-Deployment-ID` header in API responses. Use it to fetch detailed logs:
  - `curl -sS "$PLOY_CONTROLLER/apps/<app>/builds/<deployment_id>/logs?lines=1200"`
  - Attach relevant excerpts to scenario summaries and use time slicing for focused inspection.

### Nomad Integration (RECOMMENDED)

- VPS (production/test clusters):
  - Mandatory: submit jobs via `/opt/hashicorp/bin/nomad-job-manager.sh`.
  - Rationale: central retries/backoff for 429/5xx, HCL→JSON conversion via `nomad job run -output`, environment defaults, and Consul service cleanup.
  - In code, the orchestration layer auto-detects the wrapper and uses it for submit/wait/log flows.

- Non-VPS (local/dev tools, CI):
  - Use the official Nomad SDK with resilience:
    - HTTP retry/backoff for 429/5xx with jitter (config via env: `NOMAD_HTTP_MAX_RETRIES`, `NOMAD_HTTP_BASE_DELAY`, `NOMAD_HTTP_MAX_DELAY`).
    - Concurrency limits for submissions (env: `NOMAD_SUBMIT_MAX_CONCURRENCY`, default 4).
    - Prefer blocking queries for status when possible; avoid tight polling.
    - Use blocking queries with index/wait (config via env: `NOMAD_BLOCKING_WAIT`, default `30s`).

- Never call raw `nomad` CLI from app code on the VPS. If a direct CLI call is unavoidable, route it through the job manager wrapper.

### Docker Registry Usage (VPS)

- The VPS provides an internal Docker Registry (Docker Registry v2) for storing and serving images used by Nomad jobs.
- All platform images should be published to this registry and referenced by fully qualified names in Nomad specs and environment variables.
- Typical images:
  - `openrewrite-jvm` — ORW apply job (produces `output.tar` and `diff.patch`)
  - `langgraph-runner` — Planner/Reducer/LLM-exec jobs (produces `plan.json`, `next.json`, `diff.patch`)
- Avoid external registries (e.g., GHCR) for VPS job execution paths unless explicitly allowed.

## Deploy & Release

### Commands

**LOCAL**:
- `make test-unit`, `make test-coverage-threshold`, build verification
- Deploy API: `./bin/ployman api deploy --monitor` (run on workstation)

**Git hygiene (MANDATORY) before any deploy:**
- Always commit and push your changes to the remote branch before invoking any deploy commands.
  - Run `pre-commit run --all-files` locally and ensure it passes.
  - `git add -A && git commit -m "<message>" && git push`.
  - Only then run `./bin/ployman api deploy --monitor` or other deployment commands.

**Notes:**
- Run `./bin/ployman api deploy --monitor` on your workstation. Do not run it on the VPS.
- Never use direct Nomad commands; if needed remotely, only via `/opt/hashicorp/bin/nomad-job-manager.sh` as invoked by platform tooling.
- Container images (platform services and job runners) must be pushed to the VPS Docker Registry (Docker Registry v2). Do not rely on public registries in VPS workflows.
  - Examples: `openrewrite-jvm`, `langgraph-runner`, lane-specific images.
  - Configure image refs in environment (e.g., `MODS_ORW_APPLY_IMAGE`, `MODS_PLANNER_IMAGE`, `MODS_REDUCER_IMAGE`, `MODS_LLM_EXEC_IMAGE`) to point at the internal registry.

### Mandatory Update Protocol (CRITICAL)

For EVERY code change:

1. **Write failing tests** (RED phase)
2. **Write minimal code** to pass tests (GREEN phase)
3. Ensure all changes are committed and pushed to the remote repository
4. **Deploy to VPS** for integration testing (REFACTOR phase)
5. **Update documentation** (`CHANGELOG.md`, `FEATURES.md`, and `docs/LANES.md` for lane behavior)
6. **Merge to main** and return to worktree branch

**NO EXCEPTIONS**.

### API Deployment (Correct Procedure)

- Preferred (workstation):
  - Ensure clean tree and passing hooks: `pre-commit run --all-files`.
  - Commit and push: `git add -A && git commit -m "<message>" && git push`.
  - Export `PLOY_CONTROLLER` (e.g., `https://api.dev.ployman.app/v1`).
  - Deploy: `./bin/ployman api deploy --monitor`.
  - Verify: `curl -sS "$PLOY_CONTROLLER/health"` and `curl -sS "$PLOY_CONTROLLER/ready"`.

- Bootstrap/admin (Ansible, when explicitly needed):
  - Env: `TARGET_HOST`, `PLOY_PLATFORM_DOMAIN` (e.g., `dev.ployman.app`), `GITHUB_PLOY_DEV_USERNAME`, `GITHUB_PLOY_DEV_PAT`.
  - Run: `ansible-playbook -i iac/dev/inventory/hosts.yml iac/dev/playbooks/api.yml -e target_host=$TARGET_HOST -e PLOY_PLATFORM_DOMAIN=dev.ployman.app`.
  - The playbook deploys the API via the Nomad job manager wrapper and waits for readiness.

**VPS**:
- You may SSH to the VPS to fetch logs and perform required diagnostics/operations (e.g., `ssh root@$TARGET_HOST`, then `su - ploy`).
- Avoid running `ployman` deploys directly on the VPS unless explicitly requested.

**NEVER**: Integration tests against local infrastructure, direct Nomad commands
  - Exception: Running E2E tests from your workstation that target the VPS Dev API is allowed (see above).

## Feature Design & Task Tracking
- Create or update a design doc for every net-new feature/refactor at `docs/design/<feature>/README.md` before starting implementation.
- Break the design into executable tasks and document each one in `roadmap/<feature>/<order>-<short-desc>.md` (e.g. `roadmap/mods-integration-tests/01-dependencies.md`).
- Every task document must include: **Why / What For**, **Required Changes**, **Definition of Done**, **Tests** (unit / integration / e2e expectations), and a status checkbox (`- [ ] Pending`, `- [x] Done`).
- Cross-reference the roadmap tasks from the design doc so contributors can jump between high-level intent and actionable work.
- Update both design doc and task files as implementation evolves; mark status checkboxes when finishing work.

## Reference

### Ploy Overview

Deployment lanes A-G auto-selected by project structure. Update `FEATURES.md`, `CHANGELOG.md` for changes.

### Specialized Agents

Use Task tool for complex domain-specific tasks. Available agents in `.claude/agents.json`.

### Documentation

Keep this file and every README under subdirectories consistent with the guidance in [`docs/DOCS.md`](docs/DOCS.md). Update that document if you change the expected structure.
