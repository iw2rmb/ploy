# Testing Guide

This document provides comprehensive guidance for testing in the Ploy project, following Test-Driven Development (TDD) principles and best practices.

## Table of Contents

1. [Testing Philosophy](#testing-philosophy)
2. [Test Types and Architecture](#test-types-and-architecture)
3. [Testing Environment Setup](#testing-environment-setup)
4. [Writing Tests](#writing-tests)
5. [Running Tests](#running-tests)
6. [Continuous Integration](#continuous-integration)
7. [Performance Testing](#performance-testing)
8. [Testing Tools and Utilities](#testing-tools-and-utilities)
9. [Best Practices](#best-practices)
10. [Troubleshooting](#troubleshooting)

## Testing Philosophy

Ploy follows a comprehensive testing strategy based on the testing pyramid:

- **70% Unit Tests**: Fast, isolated tests of individual components
- **20% Integration Tests**: Tests of component interactions and external services
- **10% End-to-End Tests**: Full system tests with real user scenarios

### Test-Driven Development (TDD)

We follow the Red-Green-Refactor cycle:

1. **Red**: Write a failing test that describes the desired behavior
2. **Green**: Write the minimal code to make the test pass
3. **Refactor**: Improve the code while keeping tests green

### Testing Principles

- **Fast Feedback**: Tests should run quickly to enable rapid development cycles
- **Reliable**: Tests should be deterministic and not flaky
- **Isolated**: Tests should not depend on external systems or other tests
- **Readable**: Tests should clearly express intent and expected behavior
- **Maintainable**: Tests should be easy to update when requirements change

## Test Types and Architecture

### Unit Tests

Unit tests focus on individual functions, methods, or small components in isolation.

**Location**: `*_test.go` files alongside source code
**Framework**: Go's built-in `testing` package + `testify/assert`
**Scope**: Single function or method
**Dependencies**: Mocked or stubbed

```go
func TestApplicationValidation(t *testing.T) {
    tests := []struct {
        name        string
        app         Application
        expectError bool
        errorMsg    string
    }{
        {
            name: "valid application",
            app: Application{
                Name:     "test-app",
                Language: "go",
                Lane:     "B",
            },
            expectError: false,
        },
        {
            name: "invalid name",
            app: Application{
                Name:     "",  // Invalid
                Language: "go",
                Lane:     "B",
            },
            expectError: true,
            errorMsg:    "application name cannot be empty",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.app.Validate()
            if tt.expectError {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errorMsg)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Integration Tests

Integration tests verify that different components work together correctly.

**Location**: `tests/integration/` directory
**Framework**: `testify/suite` + custom helpers
**Scope**: Multiple components or external service integration
**Dependencies**: Real services (Docker containers) or dedicated test instances

```go
func TestStorageIntegration(t *testing.T) {
    // Skip if services not available
    testutils.SkipIfNoServices(t)
    
    // Setup test environment
    config := testutils.SetupTestEnvironment(t)
    
    // Create storage client
    storageClient := storage.NewClient(config.SeaweedFSFiler)
    defer storageClient.Close()
    
    // Test store operation
    key := "test-artifact"
    data := strings.NewReader("test content")
    
    err := storageClient.Store(context.Background(), key, data)
    assert.NoError(t, err)
    
    // Test retrieve operation
    retrieved, err := storageClient.Retrieve(context.Background(), key)
    assert.NoError(t, err)
    defer retrieved.Close()
    
    content, err := io.ReadAll(retrieved)
    assert.NoError(t, err)
    assert.Equal(t, "test content", string(content))
}
```

### End-to-End Tests

End-to-end tests validate complete user workflows using the full system.

**Location**: `tests/e2e/` directory
**Framework**: `ginkgo` + `gomega` for BDD-style tests
**Scope**: Complete application lifecycle
**Dependencies**: Full deployed system

```go
var _ = Describe("Application Deployment", func() {
    BeforeEach(func() {
        // Ensure clean environment
        testutils.CleanupTestApps()
    })
    
    Context("when deploying a Go application", func() {
        It("should successfully deploy to Lane B", func() {
            By("creating a new Go application")
            app := testutils.CreateTestApp("go", "test-go-app")
            
            By("pushing the application")
            result := testutils.PushApp(app.Name)
            Expect(result.Success).To(BeTrue())
            
            By("waiting for deployment to be healthy")
            Eventually(func() string {
                return testutils.GetAppStatus(app.Name)
            }, "2m", "5s").Should(Equal("healthy"))
            
            By("verifying the application is accessible")
            resp := testutils.HTTPGet(fmt.Sprintf("http://%s.local.dev/", app.Name))
            Expect(resp.StatusCode).To(Equal(200))
        })
    })
})
```

## Testing Environment Setup

### Local Development Environment

The local testing environment provides all necessary services for comprehensive testing.

#### Prerequisites

- macOS (Intel or Apple Silicon)
- Docker Desktop 4.20+
- Go 1.21+
- Make

#### Automated Setup

```bash
# Run the complete setup
./iac/local/scripts/setup.sh

# Or use Make target
make setup-local-dev
```

#### Manual Setup

```bash
# 1. Install dependencies
cd iac/local
ansible-playbook -i inventory/localhost.yml playbooks/setup-macos.yml

# 2. Start services
docker-compose up -d

# 3. Wait for services
./scripts/wait-for-services.sh

# 4. Build binaries
make build
```

### Service Dependencies

The local environment includes:

- **Consul** (8500): Service discovery and configuration
- **Nomad** (4646): Container orchestration
- **SeaweedFS** (9333/8888): Distributed storage
- **Redis** (6379): Caching
- **Traefik** (80/8080): Load balancing

### Environment Variables

Key environment variables for testing:

```bash
# Service endpoints
# Ensure CONSUL_HTTP_ADDR=localhost:8500
# Ensure NOMAD_ADDR=http://localhost:4646
# Ensure SEAWEEDFS_MASTER=http://localhost:9333
# Ensure SEAWEEDFS_FILER=http://localhost:8888


# Testing flags
# Ensure PLOY_TEST_MODE=true
# Ensure PLOY_TEST_TIMEOUT=30s
```

## Writing Tests

### Test Organization

```
tests/
├── unit/                    # Pure unit tests
│   ├── storage/            # Storage layer tests
│   ├── build/              # Build system tests
│   └── validation/         # Input validation tests
├── integration/            # Integration tests
│   ├── api/                # API endpoint tests
│   ├── storage/            # Storage integration tests
│   └── deployment/         # Deployment workflow tests
├── e2e/                    # End-to-end tests
│   ├── deployment/         # Full deployment scenarios
│   ├── scaling/            # Scaling operations
│   └── recovery/           # Failure recovery tests
└── fixtures/               # Test data and fixtures
    ├── applications/       # Sample applications
    ├── nomad-jobs/        # Nomad job specifications
    └── data/              # Test datasets
```

### Test Naming Conventions

- **Test files**: `*_test.go`
- **Test functions**: `TestFunctionName` or `TestClassName_MethodName`
- **Sub-tests**: Use `t.Run()` with descriptive names
- **BDD scenarios**: Use Ginkgo's `Describe`, `Context`, `It`

### Using Test Utilities

The `internal/testutils` package provides comprehensive testing utilities:

#### Mocks

```go
// Create mock storage client
storageClient := mocks.NewMockStorageClient()
storageClient.SetupDefault()

// Configure specific behavior
storageClient.On("Store", mock.Anything, "test-key", mock.Anything).Return(nil)
storageClient.On("Retrieve", mock.Anything, "test-key").Return(mockData, nil)
```

#### Builders

```go
// Build test objects with fluent API
app := builders.NewApplicationBuilder().
    WithName("test-app").
    WithLanguage("go").
    WithLane("B").
    WithStatus("deployed").
    Build()

deployment := builders.NewDeploymentBuilder().
    WithAppID(app.ID).
    WithVersion("v1.0.0").
    WithStatus("healthy").
    Build()
```

#### Fixtures

```go
// Use predefined application fixtures
goApps := fixtures.GoApplicationFixtures()
tarball, err := fixtures.CreateTarballFromFixture(goApps["simple-go"])
require.NoError(t, err)

// Extract to temporary directory
tempDir := testutils.CreateTempDir(t, "test-app")
err = fixtures.ExtractTarballToDir(tarball, tempDir)
require.NoError(t, err)
```

### Table-Driven Tests

Use table-driven tests for comprehensive scenario coverage:

```go
func TestLaneDetection(t *testing.T) {
    tests := []struct {
        name           string
        files          map[string]string
        expectedLane   string
        expectedReason string
    }{
        {
            name: "Go with kraft.yaml - Lane A",
            files: map[string]string{
                "go.mod":     "module test\ngo 1.21\n",
                "main.go":    "package main\nfunc main() {}",
                "kraft.yaml": "specification: v0.6\nname: test\n",
            },
            expectedLane:   "A",
            expectedReason: "Go application with Unikraft configuration",
        },
        {
            name: "Java Spring Boot - Lane C",
            files: map[string]string{
                "pom.xml": springBootPomXML,
                "src/main/java/App.java": javaMainClass,
            },
            expectedLane:   "C",
            expectedReason: "Java application with Spring Boot framework",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Create temporary directory with files
            tempDir := testutils.CreateTempDir(t, "lane-test")
            for filename, content := range tt.files {
                testutils.CreateTempFile(t, tempDir, filename, content)
            }

            // Detect lane
            detector := lane.NewDetector()
            result, err := detector.DetectLane(tempDir)
            
            require.NoError(t, err)
            assert.Equal(t, tt.expectedLane, result.Lane)
            assert.Equal(t, tt.expectedReason, result.Reason)
        })
    }
}
```

### Error Testing

Always test error conditions:

```go
func TestStorageErrors(t *testing.T) {
    storageClient := mocks.NewMockStorageClient()
    
    t.Run("store failure", func(t *testing.T) {
        expectedErr := errors.New("storage unavailable")
        storageClient.SimulateFailure("store", expectedErr)
        
        err := storageClient.Store(context.Background(), "test", strings.NewReader("data"))
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "storage unavailable")
    })
    
    t.Run("network timeout", func(t *testing.T) {
        ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
        defer cancel()
        
        // Simulate slow operation
        storageClient.SimulateLatency(100 * time.Millisecond)
        
        err := storageClient.Store(ctx, "test", strings.NewReader("data"))
        assert.Error(t, err)
        assert.True(t, errors.Is(err, context.DeadlineExceeded))
    })
}
```

## Running Tests

### Make Targets

```bash
# Run all unit tests
make test-unit

# Run integration tests (requires Docker services)
make test-integration

# Run end-to-end tests
make test-e2e

# Run all tests
make test-all

# Run tests with coverage
make test-coverage

# Run tests in watch mode (for TDD)
make test-watch

# Run specific test
go test -v ./internal/storage -run TestStorageClient

# Run tests with race detection
go test -race ./...

# Run benchmarks
go test -bench=. ./...
```

### Test Configuration

Set environment variables for test behavior:

```bash
# Skip integration tests if services unavailable
# Ensure PLOY_SKIP_INTEGRATION=true

# Increase test timeout for slow tests
# Ensure PLOY_TEST_TIMEOUT=60s

# Enable debug logging in tests
# Ensure PLOY_TEST_DEBUG=true

```

### Parallel Testing

```bash
# Run tests in parallel
go test -parallel 4 ./...

# Run specific packages in parallel
go test -parallel 2 ./internal/... ./cmd/...
```

### Filtering Tests

```bash
# Run only unit tests
go test -short ./...

# Run specific test patterns
go test -run TestStorage ./...

# Run tests with specific build tags
go test -tags integration ./...
```

## Continuous Integration

### GitHub Actions

The project uses GitHub Actions for continuous testing (deployment is handled via `ployman api deploy`):

```yaml
# .github/workflows/test.yml
name: Test Suite

on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Run unit tests
        run: make test-unit
      
      - name: Upload coverage
        uses: codecov/codecov-action@v3

  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Start services
        run: docker-compose up -d
      
      - name: Wait for services
        run: ./iac/local/scripts/wait-for-services.sh
      
      - name: Run integration tests
        run: make test-integration
```

### Pre-commit Hooks

```bash
# Install pre-commit hooks
pre-commit install

# Run hooks manually
pre-commit run --all-files
```

Pre-commit configuration (`.pre-commit-config.yaml`):

```yaml
repos:
  - repo: local
    hooks:
      - id: go-fmt
        name: go fmt (make fmt)
        entry: make fmt
        language: system
        pass_filenames: false
        files: \.go$

      - id: golangci-lint
        name: golangci-lint
        entry: golangci-lint run
        language: system
        pass_filenames: false
        files: \.go$

      - id: go-test
        name: go test
        entry: make test-unit
        language: system
        pass_filenames: false
        files: \.go$
      
      - id: go-build
        name: go build
        entry: go build -o /tmp/ploy-test ./...
        language: system
        pass_filenames: false
        files: \.go$
```

## Performance Testing

### Load Testing

Use the load testing utilities for performance validation:

```go
func TestAPIPerformance(t *testing.T) {
    helper := integration.NewLoadTestHelper(t, "http://localhost:8081")
    
    // Run load test
    results := helper.RunLoadTest("/v1/health", 10, 30*time.Second)
    
    // Analyze results
    analysis := helper.AnalyzeResults()
    analysis.LogAnalysis(t)
    
    // Assert performance requirements
    analysis.AssertPerformance(t, 100*time.Millisecond, 0.01) // 100ms max, 1% error rate
}
```

### Benchmarking

Write benchmarks for critical code paths:

```go
func BenchmarkLaneDetection(b *testing.B) {
    detector := lane.NewDetector()
    tempDir := setupBenchmarkApp(b)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := detector.DetectLane(tempDir)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkStorageOperations(b *testing.B) {
    storageClient := storage.NewClient("http://localhost:8888")
    defer storageClient.Close()
    
    data := strings.NewReader("benchmark data")
    
    b.Run("Store", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            data.Seek(0, 0) // Reset reader
            err := storageClient.Store(context.Background(), fmt.Sprintf("bench-%d", i), data)
            if err != nil {
                b.Fatal(err)
            }
        }
    })
    
    b.Run("Retrieve", func(b *testing.B) {
        // Pre-populate data
        storageClient.Store(context.Background(), "bench-data", strings.NewReader("test"))
        
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            reader, err := storageClient.Retrieve(context.Background(), "bench-data")
            if err != nil {
                b.Fatal(err)
            }
            reader.Close()
        }
    })
}
```

## Testing Tools and Utilities

### Core Testing Libraries

- **`testing`**: Go's built-in testing framework
- **`testify/assert`**: Rich assertion library
- **`testify/mock`**: Mock object framework
- **`testify/suite`**: Test suite framework
- **`ginkgo`**: BDD testing framework
- **`gomega`**: Matcher library for Ginkgo

### Custom Test Utilities

- **`internal/testutils`**: Core testing utilities and helpers
- **`internal/testutils/mocks`**: Mock implementations for external services
- **`internal/testutils/builders`**: Test object builders with fluent API
- **`internal/testutils/fixtures`**: Test data and sample applications
- **`internal/testutils/integration`**: Integration testing helpers

### Development Tools

- **`gotestsum`**: Enhanced test output formatting
- **`go-test-coverage`**: Coverage analysis and reporting
- **`golangci-lint`**: Comprehensive linting
- **`dlv`**: Debugging support for tests

```bash
# Install development tools
go install github.com/gotestyourself/gotestsum@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run tests with enhanced output
gotestsum --format testname ./...

# Run linting
golangci-lint run
```

## Best Practices

### Test Structure

1. **Arrange-Act-Assert (AAA)**:
   ```go
   func TestExample(t *testing.T) {
       // Arrange
       client := NewClient()
       expectedResult := "expected"
       
       // Act
       result, err := client.DoSomething()
       
       // Assert
       require.NoError(t, err)
       assert.Equal(t, expectedResult, result)
   }
   ```

2. **Given-When-Then (GWT)** for BDD:
   ```go
   It("should return error for invalid input", func() {
       // Given
       invalidInput := ""
       
       // When
       result, err := processor.Process(invalidInput)
       
       // Then
       Expect(err).To(HaveOccurred())
       Expect(result).To(BeNil())
   })
   ```

### Test Data Management

1. **Use builders for complex objects**:
   ```go
   app := builders.NewApplicationBuilder().
       WithName("test-app").
       WithLanguage("go").
       Build()
   ```

2. **Isolate test data**:
   ```go
   func TestFunction(t *testing.T) {
       // Each test gets its own data
       testData := createTestData(t)
       defer cleanupTestData(testData)
       
       // Test implementation
   }
   ```

3. **Use fixtures for consistent test scenarios**:
   ```go
   fixture := fixtures.GoApplicationFixtures()["simple-go"]
   tarball, err := fixtures.CreateTarballFromFixture(fixture)
   ```

### Mock Management

1. **Setup default behaviors**:
   ```go
   mockClient := mocks.NewMockStorageClient()
   mockClient.SetupDefault()
   ```

2. **Configure specific test behavior**:
   ```go
   mockClient.On("Store", "specific-key", mock.Anything).Return(expectedError)
   ```

3. **Verify interactions**:
   ```go
   defer mockClient.AssertExpectations(t)
   ```

### Error Testing

1. **Test error conditions explicitly**:
   ```go
   t.Run("handles storage failure", func(t *testing.T) {
       mockStorage.SimulateFailure("store", errors.New("disk full"))
       
       err := service.SaveData(data)
       assert.Error(t, err)
       assert.Contains(t, err.Error(), "disk full")
   })
   ```

2. **Test timeout scenarios**:
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
   defer cancel()
   
   _, err := slowOperation(ctx)
   assert.True(t, errors.Is(err, context.DeadlineExceeded))
   ```

### Performance Considerations

1. **Use `testing.Short()` for expensive tests**:
   ```go
   func TestExpensiveOperation(t *testing.T) {
       if testing.Short() {
           t.Skip("skipping expensive test in short mode")
       }
       // Expensive test logic
   }
   ```

2. **Parallel test execution**:
   ```go
   func TestParallelSafe(t *testing.T) {
       t.Parallel()
       // Test that can run in parallel
   }
   ```

3. **Benchmark critical paths**:
   ```go
   func BenchmarkCriticalFunction(b *testing.B) {
       for i := 0; i < b.N; i++ {
           CriticalFunction()
       }
   }
   ```

## Troubleshooting

### Common Issues

#### Tests Hanging or Timing Out

```bash
# Check for deadlocks or infinite loops
go test -timeout 30s ./...

# Enable race detection
go test -race ./...

# Use a debugger
dlv test -- -test.run TestProblematic
```

#### Flaky Tests

1. **Identify timing issues**:
   ```go
   // Use Eventually for asynchronous operations
   testutils.AssertEventually(t, func() bool {
       return service.IsReady()
   }, 5*time.Second, "service should be ready")
   ```

2. **Fix resource cleanup**:
   ```go
   func TestWithCleanup(t *testing.T) {
       resource := setupResource(t)
       t.Cleanup(func() {
           resource.Close()
       })
       // Test logic
   }
   ```

#### Service Dependencies

1. **Check service availability**:
   ```bash
   # Verify services are running
   docker-compose ps
   
   # Check service health
   ./iac/local/scripts/wait-for-services.sh
   ```

2. **Use service detection in tests**:
   ```go
   func TestIntegration(t *testing.T) {
       testutils.SkipIfNoServices(t)
       // Integration test logic
   }
   ```

#### Memory Issues

1. **Profile memory usage**:
   ```bash
   # Run tests with memory profiling
   go test -memprofile=mem.out ./...
   go tool pprof mem.out
   ```

2. **Check for resource leaks**:
   ```go
   func TestResourceManagement(t *testing.T) {
       before := runtime.NumGoroutine()
       
       // Test logic that should not leak goroutines
       runTest()
       
       runtime.GC()
       after := runtime.NumGoroutine()
       
       assert.Equal(t, before, after, "goroutine leak detected")
   }
   ```

### Debug Techniques

1. **Enable verbose output**:
   ```bash
   go test -v ./...
   ```

2. **Run specific tests**:
   ```bash
   go test -run TestSpecific ./path/to/package
   ```

3. **Add debug logging**:
   ```go
   func TestDebug(t *testing.T) {
       t.Log("Debug information")
       t.Logf("Variable value: %v", variable)
   }
   ```

4. **Use test helpers for debugging**:
   ```go
   func helperFunction(t *testing.T, param string) {
       t.Helper() // This function won't appear in stack traces
       // Helper logic
   }
   ```

### Performance Debugging

1. **CPU profiling**:
   ```bash
   go test -cpuprofile=cpu.out -bench=. ./...
   go tool pprof cpu.out
   ```

2. **Trace analysis**:
   ```bash
   go test -trace=trace.out ./...
   go tool trace trace.out
   ```

3. **Memory profiling**:
   ```bash
   go test -memprofile=mem.out ./...
   go tool pprof mem.out
   ```

## Conclusion

This testing guide provides the foundation for comprehensive testing in the Ploy project. By following TDD principles and utilizing the provided tools and utilities, developers can ensure high-quality, reliable code that meets user requirements.

Key takeaways:

- **Follow the testing pyramid**: Focus on unit tests, with selective integration and E2E tests
- **Use TDD**: Write tests first to drive better design and ensure requirements are met
- **Leverage test utilities**: Use mocks, builders, and fixtures for efficient test creation
- **Test error conditions**: Ensure robust error handling through comprehensive error testing
- **Maintain test quality**: Keep tests fast, reliable, isolated, readable, and maintainable

For questions or contributions to the testing infrastructure, please refer to the project documentation or create an issue in the repository.
