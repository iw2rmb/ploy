# Phase 1: TDD Foundation & Infrastructure

## Progress Summary (2025-08-26)

**Status**: ✅ COMPLETED (Core Foundation)  
**Key Accomplishments**:
- ✅ Test utilities package (`internal/testutil/`) with comprehensive mock implementations
- ✅ MockStorageClient with full interface coverage
- ✅ MockEnvStore for environment variable testing
- ✅ Test fixtures and builders for dynamic test data
- ✅ Testing standards documented in `docs/TESTING.md`
- ✅ Coverage reporting infrastructure established
- ✅ Table-driven testing patterns established

**Remaining Infrastructure Tasks**:
- [ ] Docker Compose stack setup (for full local development)
- [ ] Setup scripts for macOS and Linux
- [ ] Team training sessions

**Note**: Core testing foundation is complete and operational. Infrastructure tasks (Docker, scripts) can be completed as needed for full local development environment.

## Overview

This phase establishes the foundational infrastructure for Test-Driven Development in the Ploy platform. We'll create a comprehensive testing framework, local development environment, and establish testing standards that will support all future TDD efforts.

## Objectives

1. Create comprehensive test utilities package with mock implementations
2. Set up local development environment for macOS and Linux
3. Establish testing standards and conventions
4. Configure continuous integration pipeline
5. Create test data management system

## Implementation Plan

### Infrastructure Setup
- Test utilities package creation
- Local environment configuration
- Docker Compose stack setup

### Standards & Integration
- Testing standards documentation
- CI/CD pipeline configuration
- Team training and rollout

## Deliverables

### 1. Test Utilities Package (`internal/testutil/`)

#### 1.1 Mock Implementations (`mocks.go`)
```go
package testutil

import (
    "context"
    "sync"
    nomadapi "github.com/hashicorp/nomad/api"
    consulapi "github.com/hashicorp/consul/api"
)

// MockNomadClient provides a mock implementation of Nomad client
type MockNomadClient struct {
    mu sync.RWMutex
    
    // Job operations
    Jobs        map[string]*nomadapi.Job
    Allocations map[string]*nomadapi.Allocation
    
    // Tracking
    CallCount   map[string]int
    LastError   error
    
    // Behavior control
    ShouldFail  bool
    FailAfter   int
    Latency     time.Duration
}

func NewMockNomadClient() *MockNomadClient {
    return &MockNomadClient{
        Jobs:        make(map[string]*nomadapi.Job),
        Allocations: make(map[string]*nomadapi.Allocation),
        CallCount:   make(map[string]int),
    }
}

// MockConsulClient provides a mock implementation of Consul client
type MockConsulClient struct {
    mu sync.RWMutex
    
    // KV operations
    KVStore    map[string][]byte
    
    // Service operations  
    Services   map[string]*consulapi.AgentService
    
    // Health checks
    HealthChecks map[string]*consulapi.HealthCheck
    
    // Sessions
    Sessions   map[string]*consulapi.SessionEntry
    
    // Tracking
    CallCount  map[string]int
    LastError  error
}

// MockStorageClient provides a mock implementation of storage operations
type MockStorageClient struct {
    mu sync.RWMutex
    
    // File storage
    Files      map[string][]byte
    Metadata   map[string]map[string]string
    
    // Behavior control
    ShouldFail bool
    Latency    time.Duration
    
    // Tracking
    UploadCount   int
    DownloadCount int
}
```

#### 1.2 Test Fixtures (`fixtures.go`)
```go
package testutil

// TestApp creates a sample application for testing
func TestApp(name string) *App {
    return &App{
        Name:     name,
        Language: "go",
        Lane:     "A",
        Version:  "1.0.0",
        EnvVars: map[string]string{
            "PORT": "8080",
            "ENV":  "test",
        },
    }
}

// TestBuildConfig creates a sample build configuration
func TestBuildConfig() *BuildConfig {
    return &BuildConfig{
        Lane:      "A",
        Builder:   "unikraft",
        Timeout:   300,
        Resources: DefaultResources(),
    }
}

// FixtureRepository provides test data repository
type FixtureRepository struct {
    Apps    []App
    Builds  []Build
    Domains []Domain
}

func NewFixtureRepository() *FixtureRepository {
    return &FixtureRepository{
        Apps:    generateTestApps(),
        Builds:  generateTestBuilds(),
        Domains: generateTestDomains(),
    }
}
```

#### 1.3 Test Builders (`builders.go`)
```go
package testutil

// AppBuilder provides fluent interface for creating test apps
type AppBuilder struct {
    app *App
}

func NewAppBuilder(name string) *AppBuilder {
    return &AppBuilder{
        app: &App{Name: name},
    }
}

func (b *AppBuilder) WithLane(lane string) *AppBuilder {
    b.app.Lane = lane
    return b
}

func (b *AppBuilder) WithLanguage(lang string) *AppBuilder {
    b.app.Language = lang
    return b
}

func (b *AppBuilder) WithEnvVar(key, value string) *AppBuilder {
    if b.app.EnvVars == nil {
        b.app.EnvVars = make(map[string]string)
    }
    b.app.EnvVars[key] = value
    return b
}

func (b *AppBuilder) Build() *App {
    return b.app
}

// RequestBuilder provides fluent interface for creating HTTP requests
type RequestBuilder struct {
    method  string
    path    string
    body    interface{}
    headers map[string]string
}

func NewRequestBuilder() *RequestBuilder {
    return &RequestBuilder{
        headers: make(map[string]string),
    }
}
```

#### 1.4 Custom Assertions (`assertions.go`)
```go
package testutil

import (
    "testing"
    "reflect"
)

// AssertErrorContains checks if error contains expected message
func AssertErrorContains(t *testing.T, err error, expected string) {
    t.Helper()
    if err == nil {
        t.Errorf("expected error containing '%s', got nil", expected)
        return
    }
    if !strings.Contains(err.Error(), expected) {
        t.Errorf("expected error containing '%s', got '%s'", expected, err.Error())
    }
}

// AssertEventuallyTrue waits for condition to become true
func AssertEventuallyTrue(t *testing.T, condition func() bool, timeout time.Duration) {
    t.Helper()
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if condition() {
            return
        }
        time.Sleep(100 * time.Millisecond)
    }
    t.Errorf("condition did not become true within %v", timeout)
}

// AssertJSONEqual compares JSON structures
func AssertJSONEqual(t *testing.T, expected, actual interface{}) {
    t.Helper()
    expectedJSON, _ := json.Marshal(expected)
    actualJSON, _ := json.Marshal(actual)
    
    if !bytes.Equal(expectedJSON, actualJSON) {
        t.Errorf("JSON not equal\nExpected: %s\nActual: %s", 
            expectedJSON, actualJSON)
    }
}
```

### 2. Local Development Environment

#### 2.1 Docker Compose Stack (`iac/local/docker-compose.yml`)
```yaml
version: '3.8'

services:
  consul:
    image: consul:1.16
    container_name: ploy-consul
    ports:
      - "8500:8500"
      - "8600:8600/udp"
    command: agent -dev -client=0.0.0.0
    environment:
      - CONSUL_BIND_INTERFACE=eth0
    networks:
      - ploy-network

  nomad:
    image: nomad:1.6
    container_name: ploy-nomad
    ports:
      - "4646:4646"
      - "4647:4647"
      - "4648:4648"
    command: agent -dev -bind=0.0.0.0
    environment:
      - NOMAD_LOCAL_CONFIG={"datacenter":"dc1","data_dir":"/nomad/data","log_level":"INFO"}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - nomad-data:/nomad/data
    networks:
      - ploy-network
    depends_on:
      - consul

  seaweedfs-master:
    image: chrislusf/seaweedfs
    container_name: ploy-seaweedfs-master
    ports:
      - "9333:9333"
      - "19333:19333"
    command: master -ip=seaweedfs-master -port=9333 -metricsPort=9324
    volumes:
      - seaweed-master:/data
    networks:
      - ploy-network

  seaweedfs-volume:
    image: chrislusf/seaweedfs
    container_name: ploy-seaweedfs-volume
    ports:
      - "8080:8080"
      - "18080:18080"
    command: volume -mserver=seaweedfs-master:9333 -port=8080 -ip=seaweedfs-volume -max=100
    volumes:
      - seaweed-volume:/data
    networks:
      - ploy-network
    depends_on:
      - seaweedfs-master

  seaweedfs-filer:
    image: chrislusf/seaweedfs
    container_name: ploy-seaweedfs-filer
    ports:
      - "8888:8888"
      - "18888:18888"
    command: filer -master=seaweedfs-master:9333 -port=8888
    volumes:
      - seaweed-filer:/data
    networks:
      - ploy-network
    depends_on:
      - seaweedfs-master
      - seaweedfs-volume

  postgres:
    image: postgres:15-alpine
    container_name: ploy-postgres
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=ploy
      - POSTGRES_PASSWORD=ploy-test
      - POSTGRES_DB=ploy_test
    volumes:
      - postgres-data:/var/lib/postgresql/data
    networks:
      - ploy-network

  redis:
    image: redis:7-alpine
    container_name: ploy-redis
    ports:
      - "6379:6379"
    command: redis-server --appendonly yes
    volumes:
      - redis-data:/data
    networks:
      - ploy-network

volumes:
  nomad-data:
  seaweed-master:
  seaweed-volume:
  seaweed-filer:
  postgres-data:
  redis-data:

networks:
  ploy-network:
    driver: bridge
```

#### 2.2 Environment Setup Script (`iac/local/setup.sh`)
```bash
#!/bin/bash
set -e

echo "🚀 Setting up Ploy local testing environment..."

# Check prerequisites
check_prerequisite() {
    if ! command -v $1 &> /dev/null; then
        echo "❌ $1 is not installed. Please install it first."
        exit 1
    fi
    echo "✅ $1 is installed"
}

echo "📋 Checking prerequisites..."
check_prerequisite docker
check_prerequisite docker-compose
check_prerequisite go
check_prerequisite make

# Create necessary directories
echo "📁 Creating directories..."
mkdir -p configs
mkdir -p test-data
mkdir -p coverage

# Start Docker Compose stack
echo "🐳 Starting Docker services..."
docker-compose -f iac/local/docker-compose.yml up -d

# Wait for services to be ready
echo "⏳ Waiting for services to be ready..."
sleep 10

# Check service health
check_service() {
    local service=$1
    local port=$2
    local path=${3:-/}
    
    if curl -f http://localhost:$port$path > /dev/null 2>&1; then
        echo "✅ $service is ready"
    else
        echo "❌ $service is not responding"
        exit 1
    fi
}

check_service "Consul" 8500 /v1/status/leader
check_service "Nomad" 4646 /v1/status/leader
check_service "SeaweedFS Master" 9333 /dir/status
check_service "SeaweedFS Filer" 8888

# Create test configuration
echo "📝 Creating test configuration..."
cat > configs/test-config.yaml <<EOF
storage:
  type: seaweedfs
  endpoint: http://localhost:8888
  master: http://localhost:9333
  
consul:
  address: localhost:8500
  
nomad:
  address: http://localhost:4646
  
postgres:
  host: localhost
  port: 5432
  user: ploy
  password: ploy-test
  database: ploy_test
  
redis:
  address: localhost:6379
EOF

# Run database migrations
echo "🗄️ Running database migrations..."
go run ./tools/migrate up

# Generate mocks
echo "🎭 Generating mocks..."
go generate ./...

echo "✨ Local testing environment is ready!"
echo ""
echo "Available services:"
echo "  - Consul UI: http://localhost:8500"
echo "  - Nomad UI: http://localhost:4646"
echo "  - SeaweedFS: http://localhost:9333"
echo "  - Redis: localhost:6379"
echo ""
echo "Run 'make test-local' to execute tests"
```

### 3. Testing Standards Document

#### 3.1 TDD Workflow
```markdown
# TDD Workflow Standards

## Red-Green-Refactor Cycle

1. **Red Phase**: Write a failing test
   - Define expected behavior
   - Run test to ensure it fails
   - Commit test

2. **Green Phase**: Make test pass
   - Write minimal code to pass
   - Run test to ensure it passes
   - Commit implementation

3. **Refactor Phase**: Improve code quality
   - Refactor while keeping tests green
   - Extract common patterns
   - Commit refactoring

## Test Naming Conventions

### Unit Tests
```go
func TestComponentName_MethodName_Scenario(t *testing.T) {
    // Test implementation
}

// Examples:
func TestStorageClient_Upload_Success(t *testing.T)
func TestStorageClient_Upload_NetworkError(t *testing.T)
func TestLanePicker_Detect_GoModule(t *testing.T)
```

### Table-Driven Tests
```go
func TestLaneDetection(t *testing.T) {
    tests := []struct {
        name     string
        files    []string
        expected string
        wantErr  bool
    }{
        {
            name:     "Go project with go.mod",
            files:    []string{"go.mod", "main.go"},
            expected: "A",
            wantErr:  false,
        },
        // More test cases...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## Test Organization

### Directory Structure
```
package/
├── handler.go
├── handler_test.go       # Unit tests
├── integration_test.go   # Integration tests (build tag)
├── testdata/            # Test fixtures
│   ├── valid_config.yaml
│   └── invalid_config.yaml
└── benchmark_test.go    # Performance tests
```

### Build Tags
```go
//go:build integration
// +build integration

package storage_test

func TestStorageIntegration(t *testing.T) {
    // Integration test requiring real services
}
```

## Mock Usage Guidelines

### When to Mock
- External services (Nomad, Consul, Storage)
- Network calls
- File system operations
- Time-dependent operations

### When NOT to Mock
- Pure functions
- Data structures
- Simple utilities
- Value objects

### Mock Best Practices
```go
// Good: Interface-based mocking
type StorageClient interface {
    Upload(ctx context.Context, key string, data []byte) error
    Download(ctx context.Context, key string) ([]byte, error)
}

// Good: Behavior verification
mock.EXPECT().Upload(gomock.Any(), "test-key", []byte("data")).Return(nil)

// Bad: Over-mocking
// Don't mock simple data structures or pure functions
```

## Coverage Requirements

### Target Coverage
- New code: 90% minimum
- Critical paths: 95% minimum
- Overall project: 80% target

### Coverage Exceptions
- Generated code
- Test utilities
- Main functions
- Panic handlers

### Coverage Commands
```bash
# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Check coverage threshold
go test -cover ./... | grep -E "ok.*coverage: [8-9][0-9]\.[0-9]%|100\.0%"
```

## Test Data Management

### Fixtures
```go
// Use testdata directory for static fixtures
data, err := os.ReadFile("testdata/sample.json")

// Use builders for dynamic data
app := testutil.NewAppBuilder("test-app").
    WithLane("A").
    WithLanguage("go").
    Build()
```

### Test Isolation
```go
func TestWithDatabase(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    defer cleanupTestDB(t, db)
    
    // Isolated transaction
    tx := db.Begin()
    defer tx.Rollback()
    
    // Test logic
}
```

## Performance Testing

### Benchmarks
```go
func BenchmarkLaneDetection(b *testing.B) {
    files := generateTestFiles()
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        DetectLane(files)
    }
}
```

### Load Testing
```go
func TestConcurrentUploads(t *testing.T) {
    client := NewStorageClient()
    var wg sync.WaitGroup
    
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            err := client.Upload(fmt.Sprintf("file-%d", id), data)
            assert.NoError(t, err)
        }(i)
    }
    
    wg.Wait()
}
```
```

### 4. CI/CD Pipeline Configuration

#### 4.1 GitHub Actions Workflow (`.github/workflows/test.yml`)
```yaml
name: Test Suite

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go-version: [1.20, 1.21]
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: ${{ matrix.go-version }}
    
    - name: Cache Go modules
      uses: actions/cache@v3
      with:
        path: ~/go/pkg/mod
        key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
        restore-keys: |
          ${{ runner.os }}-go-
    
    - name: Install dependencies
      run: go mod download
    
    - name: Generate mocks
      run: go generate ./...
    
    - name: Run unit tests
      run: go test -v -cover -coverprofile=coverage.out ./...
    
    - name: Check coverage threshold
      run: |
        coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
        echo "Coverage: $coverage%"
        if (( $(echo "$coverage < 60" | bc -l) )); then
          echo "Coverage is below 60% threshold"
          exit 1
        fi
    
    - name: Upload coverage to Codecov
      uses: codecov/codecov-action@v3
      with:
        file: ./coverage.out
        flags: unittests
        name: codecov-umbrella

  integration-tests:
    runs-on: ubuntu-latest
    needs: unit-tests
    
    services:
      consul:
        image: consul:1.16
        ports:
          - 8500:8500
        options: >-
          --health-cmd="curl -f http://localhost:8500/v1/status/leader || exit 1"
          --health-interval=10s
          --health-timeout=5s
          --health-retries=5
      
      nomad:
        image: nomad:1.6
        ports:
          - 4646:4646
        options: >-
          --privileged
          --health-cmd="curl -f http://localhost:4646/v1/status/leader || exit 1"
          --health-interval=10s
          --health-timeout=5s
          --health-retries=5
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: 1.21
    
    - name: Start SeaweedFS
      run: |
        docker run -d -p 9333:9333 -p 8080:8080 -p 8888:8888 \
          --name seaweedfs \
          chrislusf/seaweedfs server -master.port=9333 -volume.port=8080 -filer
    
    - name: Wait for services
      run: |
        timeout 60 bash -c 'until curl -f http://localhost:8500/v1/status/leader; do sleep 1; done'
        timeout 60 bash -c 'until curl -f http://localhost:4646/v1/status/leader; do sleep 1; done'
        timeout 60 bash -c 'until curl -f http://localhost:9333/dir/status; do sleep 1; done'
    
    - name: Run integration tests
      run: go test -v -tags=integration ./...
      env:
        CONSUL_HTTP_ADDR: localhost:8500
        NOMAD_ADDR: http://localhost:4646
        SEAWEEDFS_MASTER: http://localhost:9333

  lint:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: 1.21
    
    - name: golangci-lint
      uses: golangci/golangci-lint-action@v3
      with:
        version: latest
        args: --timeout=5m
```

### 5. Test Data Management System

Note: SQL-based test database setup is deferred; the example below is retained for reference only and is not used in the current system.

#### 5.1 Test Database Setup (`internal/testutil/database.go`)
```go
package testutil

import (
    "database/sql"
    "fmt"
    "testing"
    
    "github.com/golang-migrate/migrate/v4"
    "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/source/file"
)

// SetupTestDB creates a test database for integration tests
func SetupTestDB(t *testing.T) *sql.DB {
    t.Helper()
    
    // Connect to postgres
    // Deferred: SQL-based DB connection string (example only)
    db, err := sql.Open("sqlite3", "file:test.db?_foreign_keys=on")
    if err != nil {
        t.Fatalf("Failed to connect to test database: %v", err)
    }
    
    // Run migrations
    driver, err := postgres.WithInstance(db, &postgres.Config{})
    if err != nil {
        t.Fatalf("Failed to create migration driver: %v", err)
    }
    
    m, err := migrate.NewWithDatabaseInstance(
        "file://../../migrations",
        "postgres", driver)
    if err != nil {
        t.Fatalf("Failed to create migrator: %v", err)
    }
    
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        t.Fatalf("Failed to run migrations: %v", err)
    }
    
    return db
}

// CleanupTestDB removes all test data
func CleanupTestDB(t *testing.T, db *sql.DB) {
    t.Helper()
    
    tables := []string{
        "benchmarks",
        "recipes", 
        "sandboxes",
        "transformations",
    }
    
    for _, table := range tables {
        _, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
        if err != nil {
            t.Logf("Failed to truncate table %s: %v", table, err)
        }
    }
}

// SeedTestData populates database with test data
func SeedTestData(t *testing.T, db *sql.DB) {
    t.Helper()
    
    // Insert test recipes
    _, err := db.Exec(`
        INSERT INTO recipes (id, name, language, category)
        VALUES 
            ('test-recipe-1', 'Test Recipe 1', 'java', 'cleanup'),
            ('test-recipe-2', 'Test Recipe 2', 'go', 'security')
    `)
    if err != nil {
        t.Fatalf("Failed to seed test data: %v", err)
    }
}
```

## Implementation Checklist

### Phase 1 Tasks
- ✅ Create `internal/testutil/` package structure (2025-08-25)
- ✅ Implement mock clients for Nomad, Consul, Storage (2025-08-25)
- ✅ Create test fixtures and builders (2025-08-25)
- [ ] Set up Docker Compose stack
- [ ] Write setup scripts for macOS and Linux
- [ ] Test local environment on team machines

### Phase 2 Tasks
- ✅ Write comprehensive testing standards document (Available in docs/TESTING.md - 2025-08-25)
- [ ] Configure GitHub Actions CI pipeline
- ✅ Set up coverage reporting with Codecov (Coverage reporting implemented - 2025-08-26)
- [ ] Implement test data management system
- ✅ Create example tests demonstrating standards (Build and validation tests - 2025-08-26)
- [ ] Conduct team training session

## Success Criteria

### Technical Metrics
- ✅ All external dependencies have mock implementations (Storage, Env, Nomad - 2025-08-26)
- [ ] Local environment starts in < 2 minutes
- [ ] CI pipeline runs in < 5 minutes
- ✅ Test utilities have 100% documentation (MockEnvStore, MockStorageClient documented - 2025-08-26)

### Team Adoption Metrics
- [ ] 100% of team has local environment running
- 🔄 All new PRs include tests (Current work demonstrates this pattern - 2025-08-26)
- ✅ Testing standards reviewed and approved (Available in docs/TESTING.md - 2025-08-25)
- [ ] Team trained on TDD practices

## Risk Mitigation

### Technical Risks
1. **Docker resource usage on developer machines**
   - Mitigation: Provide lightweight alternatives, resource limits
   
2. **Mock complexity becoming unmaintainable**
   - Mitigation: Regular refactoring, shared ownership

3. **CI pipeline becoming slow**
   - Mitigation: Parallel execution, caching, selective testing

### Process Risks
1. **Team resistance to new workflow**
   - Mitigation: Gradual adoption, clear benefits demonstration

2. **Incomplete mock implementations**
   - Mitigation: Prioritize critical paths, iterative improvement

## Next Steps

After completing Phase 1:
1. Begin Phase 2: Unit Testing Infrastructure
2. Start adding tests for critical components
3. Monitor adoption metrics
4. Gather team feedback
5. Adjust approach based on learnings

## References

- [Go Testing Best Practices](https://golang.org/doc/tutorial/add-a-test)
- [testify Documentation](https://github.com/stretchr/testify)
- [gomock Documentation](https://github.com/golang/mock)
- [Docker Compose Documentation](https://docs.docker.com/compose/)
