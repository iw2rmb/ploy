# Testing Package

Centralized testing utilities for the Ploy codebase. This package consolidates all test helpers, mocks, builders, and fixtures into a single, well-organized location.

## Structure

```
internal/testing/
├── mocks/        # Mock implementations for all interfaces
├── builders/     # Test data builders with fluent interfaces
├── fixtures/     # Static test data and golden files
├── assertions/   # Custom assertion helpers
├── helpers/      # General test helper functions
├── integration/  # Integration testing framework
└── database/     # Database testing utilities
```

## Usage

### Mocks

```go
import "github.com/ploy/internal/testing/mocks"

storage := mocks.NewStorageClient().
    WithFile("key", []byte("data"))
```

### Builders

```go
import "github.com/ploy/internal/testing/builders"

app := builders.NewApp().
    WithName("test-app").
    WithVersion("1.0.0").
    Build()
```

### Assertions

```go
import "github.com/ploy/internal/testing/assertions"

assertions.RequireNoError(t, err)
assertions.RequireErrorContains(t, err, "expected message")
```

## Migration from Old Packages

This package replaces:
- `internal/testutil/`
- `internal/testutils/`

All duplicate test utilities have been consolidated here with improved APIs and better organization.

## Guidelines

1. **No Production Dependencies**: This package should only be imported in test files
2. **Consistent APIs**: All builders use fluent interfaces, all mocks follow similar patterns
3. **Documentation**: Every public function must have documentation
4. **Testing**: Even test utilities need tests (in separate `*_test.go` files)