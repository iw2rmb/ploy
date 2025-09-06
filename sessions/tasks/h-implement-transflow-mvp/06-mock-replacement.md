---
task: 06-mock-replacement
parent: h-implement-transflow-mvp
branch: feature/transflow-mvp-completion
status: pending
created: 2025-01-09
modules: [testing, mocks, services, integration]
---

# Replace Mocks with Real Service Calls

## Problem/Goal
Systematically replace mock implementations with real service calls in integration tests and production code, following the CLAUDE.md guidance of using real services wherever possible. This improves test fidelity and reduces mock-reality drift.

## Success Criteria

### RED Phase (Identify Mock Dependencies)
- [ ] Audit all tests for mock usage in integration scenarios
- [ ] Identify production code using mock/test interfaces inappropriately
- [ ] Write failing tests that use real services instead of mocks
- [ ] Document mock replacement candidates and priorities
- [ ] Establish baseline for mock vs real service usage

### GREEN Phase (Replace Mocks Systematically)  
- [ ] Replace SeaweedFS mocks with real storage calls in integration tests
- [ ] Replace Nomad mocks with real job submission in integration tests
- [ ] Replace GitLab mocks with real API calls (test project)
- [ ] Replace Consul mocks with real KV operations
- [ ] Update test utilities to support real service configurations
- [ ] All integration tests pass with real services
- [ ] Maintain unit test mocks for isolated testing

### REFACTOR Phase (Optimize Real Service Usage)
- [ ] Optimize real service calls for test performance
- [ ] Implement service health checks and fallbacks  
- [ ] Add service interaction logging for debugging
- [ ] Validate real service tests on VPS environment
- [ ] Document patterns for mock vs real service decisions

## TDD Implementation Plan

### 1. RED: Mock Usage Audit and Failing Tests
```bash
# Identify current mock usage across codebase
grep -r "Mock" --include="*.go" internal/ tests/ cmd/ | grep -v "_test.go" | head -20
grep -r "testify/mock" internal/ tests/ cmd/ | wc -l

# Find inappropriate mock usage in production code
find . -name "*.go" -not -name "*_test.go" -exec grep -l "mock\|Mock" {} \;

# Expected findings:
# - internal/transflow/runner.go: Uses orchestration mocks in production
# - internal/storage/: May have mock fallbacks in production
# - cmd/ploy/: Should not contain any mock references
```

```go
// Write failing tests that expect real services
func TestTransflowWithRealNomad(t *testing.T) {
    // Should fail initially - still using mocks
    testutils.RequireServices(t, "nomad") // Hard requirement, no skip
    
    config := &transflow.Config{
        NomadAddr: "http://localhost:4646", // Real Nomad
        Storage:   "http://localhost:8888", // Real SeaweedFS
    }
    
    // Create runner with real dependencies (should fail if mocks still used)
    runner := transflow.NewRunner(config)
    
    request := &transflow.Request{
        ID: "real-nomad-test",
        TargetRepo: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
        // ... full request
    }
    
    // This should use real Nomad job submission, not mocks
    result, err := runner.Execute(context.Background(), request)
    assert.NoError(t, err)
    
    // Verify real Nomad job was created
    nomadClient := nomad.NewClient(&nomad.Config{Address: config.NomadAddr})
    jobs, err := nomadClient.Jobs().List(&nomad.QueryOptions{})
    assert.NoError(t, err)
    
    // Should find actual job in Nomad
    found := false
    for _, job := range jobs {
        if strings.Contains(*job.ID, "real-nomad-test") {
            found = true
            break
        }
    }
    assert.True(t, found, "Real Nomad job should be created")
}
```

### 2. GREEN: Systematic Mock Replacement
```go
// Example: Replace SeaweedFS mocks with real storage
// internal/storage/seaweed.go - Ensure production uses real client
type SeaweedClient struct {
    filerURL string
    client   *http.Client
    // Remove: mock field or test interfaces
}

func NewSeaweedClient(filerURL string) *SeaweedClient {
    return &SeaweedClient{
        filerURL: filerURL,
        client:   &http.Client{Timeout: 30 * time.Second},
    }
}

// Remove test-specific constructor that injects mocks
// func NewSeaweedClientWithMock(mock MockInterface) *SeaweedClient // DELETE

// internal/transflow/runner.go - Use real dependencies
type Runner struct {
    config       Config
    nomadClient  *nomad.Client    // Real Nomad client
    storageClient *storage.SeaweedClient // Real storage client  
    gitProvider  git.Provider     // Real Git provider
    kbService    kb.Service       // Real KB service
}

func NewRunner(config Config) *Runner {
    return &Runner{
        config: config,
        nomadClient: nomad.NewClient(&nomad.Config{
            Address: config.NomadAddr,
        }),
        storageClient: storage.NewSeaweedClient(config.SeaweedFiler),
        gitProvider: git.NewGitLabProvider(config.GitLabURL, config.GitLabToken),
        kbService: kb.NewService(config.KB),
    }
}

// Keep unit test constructor for isolated testing
func NewRunnerWithMocks(config Config, deps MockDependencies) *Runner {
    // Only used in *_test.go files for unit tests
}
```

### 3. REFACTOR: Service Integration Optimization  
```go
// Add service health validation and fallbacks
func (r *Runner) validateServices(ctx context.Context) error {
    services := []struct {
        name string
        check func() error
    }{
        {"Nomad", func() error { 
            _, err := r.nomadClient.Agent().Self()
            return err
        }},
        {"SeaweedFS", func() error {
            return r.storageClient.HealthCheck(ctx)
        }},
        {"GitLab", func() error {
            return r.gitProvider.ValidateAuth(ctx)
        }},
        {"KB", func() error {
            return r.kbService.HealthCheck(ctx)
        }},
    }
    
    var failures []string
    for _, service := range services {
        if err := service.check(); err != nil {
            failures = append(failures, fmt.Sprintf("%s: %v", service.name, err))
        }
    }
    
    if len(failures) > 0 {
        return fmt.Errorf("service health checks failed: %s", strings.Join(failures, ", "))
    }
    
    return nil
}
```

## Mock Replacement Strategy

### Priority Replacement Order

#### 1. High Priority - Production Critical
- **Nomad Client**: Replace mocks in orchestration layer
- **SeaweedFS Client**: Replace mocks in storage operations  
- **Git Provider**: Replace mocks in repository operations
- **KB Service**: Replace mocks in learning operations

#### 2. Medium Priority - Integration Testing
- **Build API Client**: Use real build service calls
- **Consul KV Client**: Use real distributed locking
- **External HTTP Clients**: Use real HTTP calls with test servers

#### 3. Low Priority - Keep Mocks for Unit Tests
- **Unit test dependencies**: Keep mocks for isolated testing
- **Performance test doubles**: Keep mocks for controlled benchmarking
- **Error simulation**: Keep mocks for failure scenario testing

### Service Interface Patterns

#### Real Service Implementation
```go
// Use dependency injection to support both real and mock implementations
type Dependencies struct {
    Nomad   NomadClient
    Storage StorageClient  
    Git     GitProvider
    KB      KBService
}

// Production constructor uses real services
func NewDependencies(config Config) Dependencies {
    return Dependencies{
        Nomad:   nomad.NewClient(config.Nomad),
        Storage: storage.NewSeaweedClient(config.Storage),
        Git:     git.NewGitLabProvider(config.Git),
        KB:      kb.NewService(config.KB),
    }
}

// Test constructor allows mock injection for unit tests only
func NewTestDependencies(mocks TestMocks) Dependencies {
    return Dependencies{
        Nomad:   mocks.Nomad,
        Storage: mocks.Storage, 
        Git:     mocks.Git,
        KB:      mocks.KB,
    }
}
```

#### Interface Segregation
```go
// Define minimal interfaces that both real and mock implementations satisfy
type NomadClient interface {
    SubmitJob(ctx context.Context, job *nomad.Job) (*nomad.JobRegisterResponse, error)
    GetJobStatus(ctx context.Context, jobID string) (*nomad.Job, error)
    StopJob(ctx context.Context, jobID string) error
}

type StorageClient interface {
    Store(ctx context.Context, key string, data io.Reader) error
    Retrieve(ctx context.Context, key string) (io.ReadCloser, error)
    List(ctx context.Context, prefix string) ([]string, error)  
    Delete(ctx context.Context, key string) error
}
```

### Test Configuration Updates

#### Integration Test Helper Updates  
```go
// internal/testutils/integration.go - Support real services
func SetupRealServiceEnvironment(t *testing.T) *Config {
    t.Helper()
    
    // Require real services, no fallback to mocks
    RequireServices(t, "consul", "nomad", "seaweedfs")
    
    return &Config{
        ConsulAddr:      mustGetEnv("CONSUL_HTTP_ADDR", "localhost:8500"),
        NomadAddr:       mustGetEnv("NOMAD_ADDR", "http://localhost:4646"),
        SeaweedFiler:    mustGetEnv("SEAWEEDFS_FILER", "http://localhost:8888"),
        GitLabURL:       mustGetEnv("GITLAB_URL", "https://gitlab.com"),
        GitLabToken:     mustGetEnv("GITLAB_TOKEN"),
    }
}

func RequireServices(t *testing.T, services ...string) {
    for _, service := range services {
        if !isServiceHealthy(service) {
            t.Fatalf("Required service %s is not healthy. Ensure Docker services are running.", service)
        }
    }
}

// Remove SkipIfNoServices - integration tests should require real services
```

## Context Files
- @internal/testutils/mocks/ - Current mock implementations to replace
- @internal/transflow/runner.go - Production code that may use mocks
- @tests/integration/ - Integration tests that should use real services
- @docs/TESTING.md - Testing strategy guidance on mock usage

## User Notes

**Mock Replacement Guidelines:**

1. **Unit Tests**: Keep mocks for isolated, fast testing
   ```go
   func TestRunnerLogic_Unit(t *testing.T) {
       mockDeps := testutils.NewMockDependencies()
       // Use mocks for unit test isolation
   }
   ```

2. **Integration Tests**: Use real services  
   ```go
   func TestRunnerWorkflow_Integration(t *testing.T) {
       testutils.RequireServices(t, "nomad", "seaweedfs")
       config := testutils.SetupRealServiceEnvironment(t)
       // Use real services for integration testing
   }
   ```

3. **Production Code**: Never include mocks or test doubles
   ```go
   // ❌ BAD - mock in production code
   func NewRunner(config Config, mock ...MockClient) *Runner
   
   // ✅ GOOD - production uses real services only  
   func NewRunner(config Config) *Runner
   ```

**Service Health Requirements:**
- All real services must implement health checks
- Integration tests must validate service availability before running
- Production code should fail fast if required services unavailable
- Implement circuit breaker for transient service failures

**Performance Considerations:**
- Real service calls are slower than mocks - adjust test timeouts
- Use service connection pooling for better performance
- Implement caching for frequently accessed data
- Add performance benchmarks comparing mock vs real service performance

**Environment Setup:**
```bash
# Ensure Docker services running for integration tests
docker-compose up -d consul nomad seaweedfs-master seaweedfs-filer

# Set required environment variables
export NOMAD_ADDR=http://localhost:4646
export CONSUL_HTTP_ADDR=localhost:8500  
export SEAWEEDFS_FILER=http://localhost:8888
export GITLAB_URL=https://gitlab.com
export GITLAB_TOKEN=your-integration-test-token

# Run integration tests with real services
make test-integration-real-services
```

## Work Log
- [2025-01-09] Created mock replacement subtask with systematic approach to real service usage