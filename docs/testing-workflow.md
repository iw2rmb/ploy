# Testing Workflow

This guide describes the local testing workflow for Ploy.

## Local Development Workflow

### 1. Before Starting Work

```bash
# Ensure your environment is clean
make fmt
make test

# Install pre-commit hooks (one-time setup)
make pre-commit-install
```

### 2. Add or Update Tests for the Change

For each behavior change, update the closest test suite:
- Unit tests near the package under change.
- Integration tests when component boundaries are involved.
- E2E scenarios for workflow-level behavior.

For parser/contract refactors under `internal/workflow/**`, keep characterization
coverage for:
- release coercion (`string`/numeric forms)
- command polymorphism parsing (string vs array inputs)
- stack gate terminal metadata shape (`StaticChecks` + `LogFindings`)
- manifest decode/validation error wrapping
- stack detection ambiguity evidence

### 3. Run Targeted Tests While Iterating

```bash
go test ./internal/server/handlers -v -run TestCreateRepo_ValidInput
```

Add error-path and edge-case tests for the same behavior slice:

```go
func TestCreateRepo_InvalidURL_ReturnsBadRequest(t *testing.T) { /* ... */ }
func TestCreateRepo_DuplicateURL_ReturnsConflict(t *testing.T) { /* ... */ }
func TestCreateRepo_StoreError_ReturnsInternalError(t *testing.T) { /* ... */ }
```

### 4. Run Full Local Checks

Before pushing, run the same core checks expected in CI:

```bash
make ci-check
```

This runs:
- `make fmt` — Format code
- `make vet` — Go vet analysis
- `make staticcheck` — Static analysis (fast, core set)
- `make test` — Unit and guardrail tests

Optional:
- `make lint` — Full golangci-lint suite (slower; broader checks)

For migration slices that add guardrails under `tests/guards/`, run:

```bash
go test ./tests/guards/...
```

## E2E Tests

When unit/integration tests are stable, validate full workflows against the local
Docker cluster:

1. **Integration Tests** — Test components together (DB + handlers)
   - Location: `tests/integration/`
   - Run with: `go test -tags=integration ./tests/integration/...`

2. **E2E Tests** — Test full system with local deployment
   - Location: `tests/e2e/migs/`
   - Documented in: `tests/e2e/migs/README.md`
   - Examples:
     - `scenario-prep-ready.sh` (prep success + run gating)
     - `scenario-prep-fail.sh` (prep failure + evidence + run gating)
     - `scenario-orw-pass.sh`

## Test Organization

### File Naming

- Unit tests: `<package>_test.go` (same directory as code)
- Table-driven tests: use descriptive subtests

```go
func TestRepoHandler(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantCode int
    }{
        {"valid repo", `{"url":"https://github.com/example/repo"}`, 201},
        {"invalid URL", `{"url":"not-a-url"}`, 400},
        {"empty body", ``, 400},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Test Helpers

- Location: `internal/testutil/` or `<package>/testutil_test.go`
- Examples: `cmd/ploy/test_support_test.go`

```go
// internal/testutil/testutil.go
package testutil

func NewMockStore(t *testing.T) *MockStore {
    // helper setup
}
```

## References

- `AGENTS.md` — Engineering policies and local validation requirements
- `tests/e2e/migs/README.md` — Workflow-level E2E scenarios
- `docs/migs-lifecycle.md` — Run lifecycle and status behavior
