# Ploy Test Suite

This directory contains integration tests, end-to-end tests, and smoke test orchestration for Ploy.

## Directory Structure

```
tests/
├── README.md                 # This file
├── smoke_tests.sh           # Orchestrates smoke tests across integration and e2e
├── integration/             # Integration tests (require PLOY_TEST_PG_DSN)
│   ├── build_test.go
│   ├── happy_path_test.go
│   ├── lab_smoke_test.go
│   ├── server_insecure_test.go
│   ├── smoke_workflow_test.go  # Comprehensive workflow validation
│   └── mods/                   # Mod-specific integration tests
├── e2e/                     # End-to-end scenarios (require cluster)
│   └── mods/
│       ├── README.md        # E2E documentation
│       ├── scenario-selftest.sh
│       ├── scenario-orw-pass.sh
│       └── scenario-orw-fail/
│           ├── run.sh
│           └── mod.yaml
└── guards/                  # Build-time guards (lints, docs)
    ├── docs_guard_test.go
    └── lints_guard_test.go
```

## Quick Start

### Prerequisites

1. **Build the CLI:**
   ```bash
   make build
   ```
   This creates `dist/ploy`.

2. **For integration tests, set up a test database:**
   ```bash
   export PLOY_TEST_PG_DSN="postgresql://user:pass@localhost:5432/ploy_test"
   ```
   The database must exist and have migrations applied. See `internal/store/migrations/` for schema.

3. **For e2e tests, configure a cluster:**
   - Ensure `~/.config/ploy/clusters/default` exists with cluster descriptor.
   - Set `PLOY_GITLAB_PAT` if testing GitLab MR creation.

### Running Smoke Tests

The `smoke_tests.sh` script orchestrates tests across multiple layers:

**Quick mode (fast tests only):**
```bash
bash tests/smoke_tests.sh --quick
```
- Runs unit tests for critical packages (backoff, SSE, GitLab client)
- Runs CLI smoke tests (version, help)
- Runs integration tests if `PLOY_TEST_PG_DSN` is set
- **Duration:** ~1-2 minutes
- **Use case:** Local development validation, pre-commit checks

**Full mode (includes e2e):**
```bash
bash tests/smoke_tests.sh --full
```
- All tests from quick mode
- E2E selftest scenario (container execution)
- **Duration:** ~3-5 minutes
- **Use case:** Pre-push validation, post-refactor verification

**Skip e2e tests explicitly:**
```bash
SKIP_E2E=1 bash tests/smoke_tests.sh --full
```

### Running Tests Manually

**Unit tests (critical packages):**
```bash
# Backoff retry logic
go test -v ./internal/workflow/backoff/...

# SSE stream client
go test -v ./internal/cli/stream/...

# GitLab MR client
go test -v ./internal/nodeagent/gitlab/...
```

**Integration tests (require PLOY_TEST_PG_DSN):**
```bash
# All integration tests
go test -v ./tests/integration/...

# Specific tests
go test -v ./tests/integration -run=TestHappyPath_CreateRepoModRun
go test -v ./tests/integration -run=TestLabSmoke
go test -v ./tests/integration -run=TestServerStartStop_InsecureMode
go test -v ./tests/integration -run=TestSmokeWorkflow_EndToEnd
```

**E2E tests (require cluster):**
```bash
# Selftest: minimal container execution
bash tests/e2e/mods/scenario-selftest.sh

# OpenRewrite Java 11→17 (passing branch)
bash tests/e2e/mods/scenario-orw-pass.sh

# OpenRewrite Java 11→17 with healing (failing branch)
bash tests/e2e/mods/scenario-orw-fail/run.sh
```

See `tests/e2e/mods/README.md` for detailed e2e documentation.

## Test Categories

### Unit Tests
- **Location:** Colocated with source code (e.g., `internal/workflow/backoff/backoff_test.go`)
- **Purpose:** Validate individual components in isolation
- **Coverage target:** ≥90% for critical workflow packages
- **Run via:** `go test ./internal/...`

### Integration Tests
- **Location:** `tests/integration/`
- **Purpose:** Validate interactions between components (database, server, store)
- **Prerequisites:** PostgreSQL test database
- **Key tests:**
  - `happy_path_test.go`: Database CRUD operations
  - `lab_smoke_test.go`: Run → stage → log → diff workflow
  - `server_insecure_test.go`: Server start/stop and HTTP handlers
  - `smoke_workflow_test.go`: Comprehensive end-to-end workflow through store layer

### E2E Tests
- **Location:** `tests/e2e/mods/`
- **Purpose:** Validate complete workflows with real containers and cluster
- **Prerequisites:** Configured cluster, Docker images, optional GitLab PAT
- **Scenarios:**
  - `scenario-selftest.sh`: Minimal container execution (echo test)
  - `scenario-orw-pass.sh`: OpenRewrite mod on passing branch
  - `scenario-orw-fail/run.sh`: OpenRewrite mod with Build Gate healing

### Guards
- **Location:** `tests/guards/`
- **Purpose:** Build-time guardrails (documentation, lints)
- **Run via:** `make test` or `go test ./tests/guards/...`

## Critical Workflows Validated

The smoke test suite validates these critical paths:

1. **Database operations:**
   - Run creation and status updates
   - Stage management and foreign keys
   - Log streaming (chunked appends)
   - Diff storage and retrieval
   - Event ordering and "since" queries

2. **Server lifecycle:**
   - Start/stop with graceful shutdown
   - HTTP endpoint registration
   - Authorization middleware (insecure mode for tests)
   - SSE event fanout service

3. **SSE streaming:**
   - Event stream creation and consumption
   - Reconnection with exponential backoff
   - Last-Event-ID preservation across reconnects
   - Idle timeout and cancellation

4. **Retry and backoff:**
   - Exponential backoff with jitter
   - Context cancellation handling
   - Policy-based retry strategies (GitLab, heartbeat, claim loop, etc.)
   - Permanent error detection

5. **GitLab MR client:**
   - MR creation via `gitlab.com/gitlab-org/api/client-go`
   - Retry on transient failures (429, 5xx)
   - PAT redaction in error messages
   - Domain normalization (localhost, 127.0.0.1 → HTTP)

6. **CLI functionality:**
   - Version and help commands
   - Subcommand help (mod, server, etc.)
   - Flag parsing and validation
   - Run inspection commands: `run status`, `run logs`, `mod run repo status`

7. **Container execution (e2e):**
   - Mod container lifecycle
   - Log streaming from container
   - Artifact collection
   - Build Gate validation via HTTP API (repo+diff model)
   - Healing workflow (Build Gate failure → healing mods → re-gate via remote workers)
   - Decoupled execution: Mods and Build Gate can run on different nodes

## Writing New Tests

### Integration Test Template

```go
package integration

import (
	"context"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/store"
)

func TestYourFeature(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := store.NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Test logic here...
	t.Log("✓ Test completed successfully")
}
```

### E2E Test Template

```bash
#!/usr/bin/env bash
set -euo pipefail

# E2E: Your scenario description

TS=$(date +%y%m%d%H%M%S)
ARTIFACT_DIR="./tmp/your-scenario/${TS}"
mkdir -p "${ARTIFACT_DIR}"

dist/ploy mig run \
  --repo-url https://github.com/example/repo.git \
  --repo-base-ref main \
  --repo-target-ref feature/test \
  --job-image your-mod:latest \
  --follow \
  --artifact-dir "${ARTIFACT_DIR}"

echo "OK: your scenario"
echo "Artifacts saved to: ${ARTIFACT_DIR}"
```

## Smoke Test Design Principles

1. **Fast by default:** Quick mode (<2 min) enables frequent local validation
2. **Layered:** Unit → integration → e2e, allowing progressive validation
3. **Fail-fast:** Tests exit early on first failure for quick feedback
4. **Self-documenting:** Clear test names and log output
5. **Isolated:** Each test cleans up after itself (via `t.Cleanup` or transaction rollback)
6. **Skippable:** Tests skip gracefully when prerequisites are missing (e.g., no DSN, no cluster)

## CI Integration

The smoke test suite is designed for CI environments:

```yaml
# Example GitHub Actions workflow
- name: Run smoke tests (quick)
  run: bash tests/smoke_tests.sh --quick
  env:
    PLOY_TEST_PG_DSN: ${{ secrets.TEST_PG_DSN }}

- name: Run smoke tests (full)
  if: github.ref == 'refs/heads/main'
  run: bash tests/smoke_tests.sh --full
  env:
    PLOY_TEST_PG_DSN: ${{ secrets.TEST_PG_DSN }}
    SKIP_E2E: "1"  # Skip e2e if cluster not available in CI
```

## Troubleshooting

### Integration tests fail: "connection refused"
- Ensure PostgreSQL is running and `PLOY_TEST_PG_DSN` is correct.
- Verify database exists and migrations are applied.

### E2E tests fail: "cluster not configured"
- Ensure `~/.config/ploy/clusters/default` exists.
- Verify cluster is accessible (network, credentials).

### E2E tests fail: "image not found"
- Ensure Docker images are built and pushed to registry.
- For private images, ensure Docker is logged in: `docker login`.

### Timeouts in smoke_tests.sh
- Default timeout is 5 minutes per test.
- For slow environments, adjust timeout in `run_test()` function.
- Check system resources (CPU, memory, disk).

## References

- **Engineering guide:** `AGENTS.md` and `docs/testing-workflow.md` (TDD, coverage targets)
- **E2E documentation:** `tests/e2e/mods/README.md` (container workflows)
- **System docs:** `docs/` (current contracts and operations)
- **Agent guide:** `AGENTS.md` (TDD discipline, RED→GREEN→REFACTOR)

---

**Validation status:** This test suite validates the critical workflows modified during the library reuse roadmap (backoff, SSE, GitLab client, CLI). All tests follow TDD discipline with coverage ≥90% on critical packages.
