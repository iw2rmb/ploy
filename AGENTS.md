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

## Go Analysis Tooling (MANDATORY)

- Source of truth: `.golangci.yml` in repo. Use `make` targets below.
- Local quick pass (run before PRs):
  - `make fmt` — `go fmt` + `goimports` organization
  - `make vet` — `go vet ./...` core analyzers
  - `make lint` — `golangci-lint run` using `.golangci.yml`
  - `staticcheck ./...` — supplementary static analysis (config: `staticcheck.conf`)
- Security and vulnerabilities:
  - `make sec` — `gosec ./...` security rules
  - `govulncheck ./...` — known vulnerabilities in code and deps
- Reliability and tests:
  - `go test -race ./...` — data race detector
  - Coverage gate: `make test-coverage-threshold` (60% min; 90% for critical)
- Modules hygiene:
  - `go mod tidy -v && go mod verify`
- Notes:
  - `golangci-lint` aggregates high-signal linters (errcheck, ineffassign, revive, gocritic, bodyclose, gosec, unparam, cyclo/cognit, etc.). No need to run them individually.
  - Prefer `gofumpt -l -w .` locally if you want stricter formatting; `gofmt` remains the baseline.
- Install helpers (developers):
  - `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
  - `go install golang.org/x/tools/cmd/goimports@latest`
  - `go install honnef.co/go/tools/cmd/staticcheck@latest`
  - `go install github.com/securego/gosec/v2/cmd/gosec@latest`
  - `go install golang.org/x/vuln/cmd/govulncheck@latest`

### Pre-commit Hooks

- Install once: `pipx install pre-commit` or `pip install pre-commit`
- Enable in this repo: `pre-commit install`
- Config: `.pre-commit-config.yaml` runs `make fmt` and `golangci-lint run` on commit.
- Run manually on all files: `pre-commit run --all-files`
- CI: GitHub Actions runs pre-commit hooks on all files (`.github/workflows/ci.yml` job: Pre-commit Hooks).
- Required status check: Configure branch protection to require "CI / Pre-commit Hooks".
  - If Probot Settings is installed, `.github/settings.yml` enforces this for `main` and `develop`.
  - Otherwise, set it manually in GitHub → Settings → Branches → Branch protection rules.

## VPS Testing

**Setup**: `ssh root@$TARGET_HOST` → `su - ploy`  
**Nomad**: ONLY use `/opt/hashicorp/bin/nomad-job-manager.sh` (never direct `nomad` commands)

**E2E via Dev API (Allowed from Workstation)**
- You may run E2E tests locally when they call the VPS Dev API endpoint (e.g., set `PLOY_CONTROLLER=https://api.dev.ployman.app/v1`).
- These tests exercise VPS services remotely and are considered VPS-side execution (REFACTOR phase), even if invoked from the workstation.
- Do not spin up or depend on local Nomad/Consul/Gateway for these tests.
- Example:
  - `E2E_LOG_CONFIG=1 PLOY_CONTROLLER=https://api.dev.ployman.app/v1 go test ./tests/e2e -tags e2e -v -run TestModsE2E_JavaMigrationComplete -timeout 10m`

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

## Commands

**LOCAL**: 
- `make test-unit`, `make test-coverage-threshold`, build verification
- Deploy API: `./bin/ployman api deploy --monitor` (run on workstation)

Git hygiene (MANDATORY) before any deploy:
- Always commit and push your changes to the remote branch before invoking any deploy commands.
  - Run `pre-commit run --all-files` locally and ensure it passes.
  - `git add -A && git commit -m "<message>" && git push`.
  - Only then run `./bin/ployman api deploy --monitor` or other deployment commands.

Notes:
- Run `./bin/ployman api deploy --monitor` on your workstation. Do not run it on the VPS.
- Never use direct Nomad commands; if needed remotely, only via `/opt/hashicorp/bin/nomad-job-manager.sh` as invoked by platform tooling.
- Container images (platform services and job runners) must be pushed to the VPS Docker Registry (Docker Registry v2). Do not rely on public registries in VPS workflows.
  - Examples: `openrewrite-jvm`, `langgraph-runner`, lane-specific images.
  - Configure image refs in environment (e.g., `MODS_ORW_APPLY_IMAGE`, `MODS_PLANNER_IMAGE`, `MODS_REDUCER_IMAGE`, `MODS_LLM_EXEC_IMAGE`) to point at the internal registry.

**VPS**:
- You may SSH to the VPS to fetch logs and perform required diagnostics/operations (e.g., `ssh root@$TARGET_HOST`, then `su - ploy`).
- Avoid running `ployman` deploys directly on the VPS unless explicitly requested.

**NEVER**: Integration tests against local infrastructure, direct Nomad commands
  - Exception: Running E2E tests from your workstation that target the VPS Dev API is allowed (see above).

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
3. Ensure all changes are committed and pushed to the remote repository
4. **Deploy to VPS** for integration testing (REFACTOR phase)
4. **Update documentation** (`CHANGELOG.md`, `FEATURES.md`, and `docs/LANES.md` for lane behavior)
5. **Merge to main** and return to worktree branch

**NO EXCEPTIONS**.

## Specialized Agents

Use Task tool for complex domain-specific tasks. Available agents in `.claude/agents.json`.
