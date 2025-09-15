# Ploy Test Suite

Comprehensive testing infrastructure organized by scope and environment.

## Directory Structure

```
tests/
├── unit/                    # Isolated component tests
├── integration/             # Component interaction tests
├── e2e/                     # End-to-end workflow tests
├── vps/                     # Production VPS environment tests
├── acceptance/              # MVP acceptance validation
├── behavioral/              # User behavior and lifecycle tests
├── performance/             # Performance benchmarks and load tests
├── scripts/                 # Test execution automation
├── apps/                    # Test application fixtures
├── nomad-jobs/              # Nomad job test definitions
├── guardrails/              # Code quality and import validation
└── documentation/           # Documentation validation tests
```

## Test Execution

- **Unit**: `go test ./tests/unit/...`
- **Integration**: `go test ./tests/integration/...`
- **E2E**: `go test -tags=e2e ./tests/e2e/...`
- **VPS**: `go test -tags=vps ./tests/vps/...` (assumes `TARGET_HOST` is set)
- **Scripts**: `./tests/scripts/test-*.sh`

## Test Levels

1. **Unit** → Isolated functions and methods
2. **Integration** → Service interactions and contracts
3. **E2E** → Complete workflow validation
4. **VPS** → Production environment validation
5. **Acceptance** → MVP requirements verification

Run tests in order: unit → integration → e2e → vps for progressive validation.
