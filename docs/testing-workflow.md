# Testing Workflow — RED → GREEN → REFACTOR

This guide explains how to follow the RED → GREEN → REFACTOR test-driven development (TDD) workflow for Ploy.

## Philosophy

Every code change should follow this cadence:

1. **RED** — Write a failing test first (unit test)
2. **GREEN** — Write minimal code to make the test pass
3. **REFACTOR** — Clean up code while keeping tests green
4. **E2E Later** — Add integration/E2E tests after core functionality is stable

This approach ensures:
- Code is testable by design
- Requirements are verified before implementation
- Refactoring is safe and confident
- Coverage stays high (≥60% overall, ≥90% on critical paths)

## Local Development Workflow

### 1. Before Starting Work

```bash
# Ensure your environment is clean
make fmt
make test

# Install pre-commit hooks (one-time setup)
make pre-commit-install
```

### 2. RED Phase — Write a Failing Test

Create or update a test file with a test that captures your requirement:

```go
// Example: internal/server/handlers/repos_test.go
func TestCreateRepo_ValidInput_ReturnsCreated(t *testing.T) {
    // Arrange
    handler := NewRepoHandler(mockStore)
    req := httptest.NewRequest("POST", "/v1/repos", strings.NewReader(`{"url":"https://github.com/example/repo"}`))
    rec := httptest.NewRecorder()

    // Act
    handler.CreateRepo(rec, req)

    // Assert
    if rec.Code != http.StatusCreated {
        t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
    }
}
```

**Run the test — it MUST fail:**

```bash
go test ./internal/server/handlers -v -run TestCreateRepo_ValidInput
```

Expected output:
```
--- FAIL: TestCreateRepo_ValidInput_ReturnsCreated (0.00s)
    repos_test.go:15: expected status 201, got 500
FAIL
```

This confirms you're testing something that doesn't exist yet.

### 3. GREEN Phase — Make the Test Pass

Write the minimal implementation to pass the test:

```go
// internal/server/handlers/repos.go
func (h *RepoHandler) CreateRepo(w http.ResponseWriter, r *http.Request) {
    var req CreateRepoRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }

    repo, err := h.store.CreateRepo(r.Context(), req.URL)
    if err != nil {
        http.Error(w, "failed to create repo", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(repo)
}
```

**Run the test again — it MUST pass:**

```bash
go test ./internal/server/handlers -v -run TestCreateRepo_ValidInput
```

Expected output:
```
--- PASS: TestCreateRepo_ValidInput_ReturnsCreated (0.00s)
PASS
ok      github.com/replicate/ploy/internal/server/handlers  0.123s
```

### 4. REFACTOR Phase — Clean Up

Now that the test is green, improve the code:
- Extract helper functions
- Improve naming
- Reduce duplication
- Add documentation

**Run tests after each refactoring step:**

```bash
go test ./internal/server/handlers -v
```

Tests should remain green. If they fail, undo the refactoring.

### 5. Add More RED → GREEN Cycles

Add tests for error cases, edge cases, and variations:

```go
func TestCreateRepo_InvalidURL_ReturnsBadRequest(t *testing.T) { /* ... */ }
func TestCreateRepo_DuplicateURL_ReturnsConflict(t *testing.T) { /* ... */ }
func TestCreateRepo_StoreError_ReturnsInternalError(t *testing.T) { /* ... */ }
```

Follow the RED → GREEN → REFACTOR cycle for each test.

### 6. Verify Coverage

Before committing, check that coverage meets thresholds:

```bash
make test-coverage-threshold
```

Expected output:
```
=== Coverage Summary ===
total: (statements) 67.8%
Coverage: 67.8% (threshold: 60%)
```

If coverage is below 60%, add more tests.

### 7. Run Full CI Checks Locally

Before pushing, run all checks that CI will run:

```bash
make ci-check
```

This runs:
- `make fmt` — Format code
- `make vet` — Go vet analysis
- `make staticcheck` — Static analysis (fast, core set)
- `make test-coverage-threshold` — Tests with 60% threshold

Optional:
- `make lint` — Full golangci-lint suite (slower; broader checks). Use when iterating on refactors or before larger PRs.

If all checks pass, you're ready to commit and push.

## E2E Tests — Later Phase

After core functionality is stable and unit-tested:

1. **Integration Tests** — Test components together (DB + handlers)
   - Location: `tests/integration/`
   - Run with: `go test -tags=integration ./tests/integration/...`
   - Require external services (Postgres, etc.)

2. **E2E Tests** — Test full system with real deployment
  - Location: `tests/e2e/mods/`
  - Documented in: `tests/e2e/mods/README.md`
   - Run against the local Docker cluster
   - Examples: `scenario-orw-pass.sh`

## Coverage Targets

Per `AGENTS.md` and CHANGELOG notes:

- **Overall coverage:** ≥60%
- **Critical paths:** ≥90%
  - Scheduler (`internal/server/scheduler`)
  - PKI (`internal/server/auth`)
  - Ingest handlers (node heartbeat, events, diffs)

Check per-component coverage:

```bash
# Overall
make test-coverage

# Specific component
go test -coverprofile=coverage.out ./internal/server/scheduler/...
go tool cover -func=coverage.out
```

## Test Organization

### File Naming

- Unit tests: `<package>_test.go` (same directory as code)
- Table-driven tests: Use descriptive subtests

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
    return &MockStore{t: t}
}

func AssertHTTPStatus(t *testing.T, got, want int) {
    t.Helper()
    if got != want {
        t.Errorf("status: got %d, want %d", got, want)
    }
}
```

## Common Patterns

### Table-Driven Tests

```go
func TestParseConfig(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    Config
        wantErr bool
    }{
        {"valid", `{"port":8080}`, Config{Port: 8080}, false},
        {"invalid", `{invalid}`, Config{}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseConfig(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("ParseConfig() error = %v, wantErr %v", err, tt.wantErr)
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("ParseConfig() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### HTTP Handler Tests

```go
func TestHandler(t *testing.T) {
    handler := NewHandler(mockStore)

    req := httptest.NewRequest("POST", "/v1/repos", strings.NewReader(`{}`))
    rec := httptest.NewRecorder()

    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusCreated {
        t.Errorf("status: got %d, want %d", rec.Code, http.StatusCreated)
    }
}
```

### Database Tests (with pgx + sqlc)

```go
func TestStoreCreateRepo(t *testing.T) {
    // Skip if no test database
    dsn := os.Getenv("PLOY_TEST_PG_DSN")
    if dsn == "" {
        t.Skip("PLOY_TEST_PG_DSN not set")
    }

    ctx := context.Background()
    pool, err := pgxpool.New(ctx, dsn)
    if err != nil {
        t.Fatal(err)
    }
    defer pool.Close()

    store := db.New(pool)

    repo, err := store.CreateRepo(ctx, "https://github.com/example/repo")
    if err != nil {
        t.Fatal(err)
    }
    if repo.URL != "https://github.com/example/repo" {
        t.Errorf("URL: got %s, want %s", repo.URL, "https://github.com/example/repo")
    }
}
```

### Race Detector

Always run with `-race` on tests involving concurrency:

```bash
go test -race ./internal/server/scheduler/...
```

## Debugging Test Failures

### Verbose Output

```bash
go test -v ./internal/server/handlers
```

### Run Specific Test

```bash
go test ./internal/server/handlers -run TestCreateRepo_ValidInput
```

### Show Coverage

```bash
go test -cover ./internal/server/handlers
```

### Generate HTML Coverage Report

```bash
go test -coverprofile=coverage.out ./internal/server/handlers
go tool cover -html=coverage.out
```

## Pre-Commit Checklist

Before every commit:

- [ ] All tests pass locally: `make test`
- [ ] Coverage meets threshold: `make test-coverage-threshold`
- [ ] Code is formatted: `make fmt`
- [ ] No vet issues: `make vet`
- [ ] No lint issues: `make lint`
- [ ] All CI checks pass: `make ci-check`

Or simply run:

```bash
make ci-check
```

## Pull Request Checklist

See `.github/pull_request_template.md` for the full checklist that enforces this workflow.

## References

- **AGENTS.md** — RED → GREEN → REFACTOR philosophy and coverage targets
- **GOLANG.md** — Go engineering standards (table-driven tests, race detector, fuzzing)
- **CHANGELOG.md** — Status and acceptance summary for recent slices
- **tests/guards/docs_guard_test.go** — Example of enforcing design doc coverage
- **Go Code Review Comments** — https://go.dev/wiki/CodeReviewComments

## Quick Reference

| Phase | Command | Expected Result |
|-------|---------|-----------------|
| **RED** | `go test ./pkg -run TestFoo` | Test fails |
| **GREEN** | `go test ./pkg -run TestFoo` | Test passes |
| **REFACTOR** | `go test ./pkg` | All tests pass |
| **Coverage** | `make test-coverage-threshold` | ≥60% coverage |
| **Local CI** | `make ci-check` | All checks pass |
| **Commit** | `git commit` | Pre-commit hooks pass |
| **Push** | `git push` | CI passes |

## Example Workflow

```bash
# 1. Start with RED
$ vim internal/server/handlers/repos_test.go  # Write failing test
$ go test ./internal/server/handlers -run TestCreateRepo
--- FAIL: TestCreateRepo (0.00s)
FAIL

# 2. Make it GREEN
$ vim internal/server/handlers/repos.go  # Implement minimal code
$ go test ./internal/server/handlers -run TestCreateRepo
--- PASS: TestCreateRepo (0.00s)
PASS

# 3. REFACTOR
$ vim internal/server/handlers/repos.go  # Clean up code
$ go test ./internal/server/handlers
PASS

# 4. Add more tests (repeat RED → GREEN → REFACTOR)
$ vim internal/server/handlers/repos_test.go
$ go test ./internal/server/handlers
PASS

# 5. Verify coverage
$ make test-coverage-threshold
Coverage: 68.5% (threshold: 60%)
✓ Coverage meets threshold

# 6. Run full CI checks
$ make ci-check
✓ Format check
✓ Go vet
✓ staticcheck
✓ Tests with coverage threshold
=== All CI checks passed ===

# 7. Commit and push
$ git add .
$ git commit -m "Add CreateRepo handler with tests"
$ git push
```

Now CI will run the same checks and should pass immediately.
