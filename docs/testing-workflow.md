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
- Put reusable test helpers under `internal/testutil/*` (for example: `gitrepo`, `clienv`, `stdcapture`, `golden`, `assertx`, `workspace`).
- If a shared helper already exists, do not add a duplicated local helper.
- Keep `internal/testutil` packages dependency-light to avoid import cycles.

### Handler Test Fixture Stores

Handler tests use domain-focused fixture stores instead of a single monolithic mock. Each store embeds `store.Store` and implements only the methods its handler domain calls:

| Store | File | Domain |
|-------|------|--------|
| `jobStore` | `test_fixture_job_test.go` | Job completion, status, claiming, healing, stale recovery |
| `migStore` | `test_fixture_mig_test.go` | Mig CRUD, spec, mig-repo, run submission |
| `runStore` | `test_fixture_run_test.go` | Run listing, timing, delete, batch ops, pull, ingest |
| `artifactStore` | `test_fixture_artifact_test.go` | Artifact download/repo, diffs, SBOM compat |
| `configStore` | `test_fixture_config_test.go` | Global env, spec bundles |
| `nodeStore` | `test_fixture_node_test.go` | Node CRUD, heartbeat, draining |
| `repoListStore` | `test_fixture_repolist_test.go` | Repo listing |

Shared generic helpers (`mockResult`, `mockCall`) live in `test_mock_helpers_test.go`. Fixture builders and assertion helpers live in `test_helpers_test.go`.

### Oversized Test Files

- Split oversized test files by behavior domain instead of keeping multi-domain monoliths.
- Prefer one domain per file (for example: runtime, profile-target, mounts-limits, recovery, error-path).
- Move shared fixtures/builders into dedicated `*_fixture_test.go` files and reuse them across domain files.

### Test LOC Guardrails

- Treat test code as production code: duplication is a defect, not a convenience.
- Keep one canonical table per behavior path; do not add parallel tables that assert the same branch with different phrasing.
- When adding a new canonical test owner, delete superseded tests in the same change.
- Prefer typed fixture builders over repeated inline JSON/YAML blobs.
- Reuse assertion helpers for repeated shape checks (`command/env/stack/target` style checks), instead of per-case ad-hoc assertions.
- Keep table-driven suites focused: add a case only when it exercises a distinct branch, error mapping, or contract edge.
- Avoid “combination explosion” suites. If behavior is already locked by one dimension, do not multiply cases across orthogonal dimensions.
- For contract/refactor slices, require a net test LOC justification in PR notes when test files grow significantly (for example `>200` added lines in a package).
- Refactor trigger: if a test file crosses ~400 LOC, split by behavior domain and extract shared fixtures before adding more cases.
- Prefer deterministic helpers + concise case tables over large fixture literals repeated across tests.

```go
// internal/testutil/testutil.go
package testutil

func NewMockStore(t *testing.T) *MockStore {
    // helper setup
}
```

## Redundancy Guardrails

`make redundancy-check` (wired into `make ci-check`) enforces two categories of
structural signals in the hotspot packages
(`internal/server/handlers`, `internal/nodeagent`, `internal/workflow/contracts`,
`internal/store`):

### 1. LOC guardrail

Every `*_test.go` file in a hotspot package must stay below **1000 lines**.
Files approaching the limit are a sign that the file has grown across behavior
domains and should be split first (see [Test LOC Guardrails](#test-loc-guardrails)).

### 2. Parallel-entrypoint guardrail

Flags exported production symbols that form parallel families — two copies of
the same logic coexisting instead of being consolidated:

| Pattern | Example | Diagnosis |
|---------|---------|-----------|
| Base + versioned (`FooV2`) | `Compute` + `ComputeV2` | versioned copy was added instead of refactoring |
| Multiple versioned forms | `FooV1` + `FooV2` | both versions left in the tree |
| Legacy/deprecated shadow | `Foo` + `FooLegacy` | old form was not removed after replacement |

### Interpreting failures

```
FAIL: LOC: internal/nodeagent/testutil_test.go: 1043 lines (limit 1000)
```
Split the file by behavior domain before adding more cases.

```
FAIL: PARALLEL_FAMILY: internal/server/handlers: 'Compute' and 'ComputeV2' coexist
```
Delete the superseded form or consolidate the two into one. There is no
exemption mechanism — the finding must be resolved in source code.

### Remediation flow

1. Run `make redundancy-check` locally to see the full finding list.
2. For LOC findings: split the file by behavior domain and move shared fixtures
   to a `*_fixture_test.go` file.
3. For parallel-family findings: remove the superseded symbol or consolidate
   the versioned forms into one. The script has no allowlist mechanism — the
   only way to clear a finding is to eliminate the parallel family from the
   source code.
4. Re-run `make redundancy-check` to confirm findings are resolved.

## References

- `tests/e2e/migs/README.md` — Workflow-level E2E scenarios
- `docs/migs-lifecycle.md` — Run lifecycle and status behavior
