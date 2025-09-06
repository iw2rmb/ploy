---
task: 05-integration-test-validation
parent: h-implement-transflow-mvp
branch: feature/transflow-mvp-completion
status: pending
created: 2025-01-09
modules: [testing, integration, transflow, services]
---

# Integration Test Validation

## Problem/Goal
Implement comprehensive integration tests that validate transflow components working together with real services (Docker containers locally, real services on VPS). Move beyond unit test mocks to test actual service interactions and data flows.

## Success Criteria

### RED Phase (Local Integration with Docker)  
- [ ] Write failing integration tests for transflow with real SeaweedFS
- [ ] Write failing integration tests for transflow with real Consul KV
- [ ] Write failing integration tests for transflow with real Nomad cluster
- [ ] Write failing integration tests for KB learning with real storage
- [ ] Write failing integration tests for GitLab MR creation
- [ ] All tests fail initially (integration environment not ready)

### GREEN Phase (Local Integration Success)
- [ ] Implement Docker-based integration test environment
- [ ] All integration tests pass with real local services  
- [ ] KB learning integration tests validate actual storage operations
- [ ] Healing workflow tests use real Nomad job submissions
- [ ] GitLab integration tests create real MRs (test project)
- [ ] Integration test suite completes in <5 minutes
- [ ] `make test-integration` passes

### REFACTOR Phase (VPS Integration Validation)
- [ ] Run all integration tests on VPS with production-like services
- [ ] Validate integration tests with VPS Nomad cluster  
- [ ] Test KB operations with VPS SeaweedFS at scale
- [ ] Validate GitLab MR creation with real repositories
- [ ] Performance validation with realistic workloads
- [ ] All VPS integration tests pass

## TDD Implementation Plan

### 1. RED: Write Failing Integration Tests
```go
// Test files to create:
// tests/integration/transflow/full_workflow_test.go
// tests/integration/kb/learning_integration_test.go  
// tests/integration/healing/nomad_jobs_test.go
// tests/integration/git/gitlab_mr_test.go

func TestTransflowFullWorkflow_Integration(t *testing.T) {
    // Skip if Docker services not available
    testutils.SkipIfNoServices(t)
    
    // Should fail initially - full integration not implemented
    config := testutils.SetupIntegrationEnvironment(t)
    
    // Real transflow with real services (not mocks)
    runner := transflow.NewRunner(config, transflow.Dependencies{
        Nomad:   nomad.NewClient(config.NomadAddr),
        Storage: storage.NewClient(config.SeaweedFSFiler), 
        Git:     git.NewGitLabProvider(config.GitLabURL, config.GitLabToken),
        KB:      kb.NewService(config.KB),
    })
    
    // Test complete Java 11->17 migration workflow
    request := &transflow.Request{
        ID: "test-java-migration",
        TargetRepo: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
        TargetBranch: "refs/heads/main",
        BaseRef: "refs/heads/main", 
        Steps: []Step{
            {Type: "recipe", Engine: "openrewrite", Recipes: []string{
                "org.openrewrite.java.migrate.Java11toJava17",
            }},
        },
        SelfHeal: SelfHealConfig{Enabled: true, MaxRetries: 2},
    }
    
    result, err := runner.Execute(context.Background(), request)
    assert.NoError(t, err)
    assert.True(t, result.Success)
    assert.NotEmpty(t, result.MRUrl) // Should create real GitLab MR
    
    // Validate KB learning occurred  
    history, err := runner.KB.GetErrorHistory(context.Background(), "any-signature")
    assert.NoError(t, err) // Should not fail even if no cases yet
}

func TestKBLearningIntegration(t *testing.T) {
    testutils.SkipIfNoServices(t)
    
    // Should fail - KB integration not complete
    config := testutils.SetupIntegrationEnvironment(t)
    kb := kb.NewService(config.KB)
    
    // Test learning cycle with real storage
    attempt := &models.HealingAttempt{
        TransflowID: "test-transflow-123",
        ErrorSignature: "java-compilation-test-error",
        Patch: []byte("diff --git a/Main.java b/Main.java\n+import java.util.*;"),
        Success: true,
        Duration: 30 * time.Second,
        Timestamp: time.Now(),
    }
    
    // Record learning
    err := kb.RecordHealing(context.Background(), attempt)
    assert.NoError(t, err)
    
    // Validate storage occurred in real SeaweedFS
    cases, err := kb.GetCasesByError(context.Background(), "java-compilation-test-error")
    assert.NoError(t, err)
    assert.Len(t, cases, 1)
    
    // Validate summary generation  
    summary, err := kb.GetSummary(context.Background(), "java-compilation-test-error")
    assert.NoError(t, err)
    assert.Equal(t, 1.0, summary.SuccessRate) // 100% success for single case
}
```

### 2. GREEN: Implement Integration Environment
```go
// internal/testutils/integration.go - Integration test setup
func SetupIntegrationEnvironment(t *testing.T) *Config {
    t.Helper()
    
    // Start Docker services if not already running
    ensureDockerServicesRunning(t)
    
    // Wait for services to be healthy
    waitForServiceHealth(t, []string{
        "http://localhost:8500/v1/status/leader", // Consul
        "http://localhost:4646/v1/status/leader", // Nomad
        "http://localhost:9333/cluster/status",   // SeaweedFS Master
        "http://localhost:8888/",                 // SeaweedFS Filer
    })
    
    return &Config{
        ConsulAddr:      "localhost:8500",
        NomadAddr:       "http://localhost:4646", 
        SeaweedFSMaster: "http://localhost:9333",
        SeaweedFSFiler:  "http://localhost:8888",
        GitLabURL:       getEnvOrSkip(t, "GITLAB_URL", "https://gitlab.com"),
        GitLabToken:     getEnvOrSkip(t, "GITLAB_TOKEN"),
        KB: KBConfig{
            Enabled:    true,
            StorageURL: "http://localhost:8888",
            ConsulAddr: "localhost:8500",
        },
    }
}

func ensureDockerServicesRunning(t *testing.T) {
    // Check if docker-compose services are running
    cmd := exec.Command("docker-compose", "ps", "--services", "--filter", "status=running")
    output, err := cmd.Output()
    if err != nil {
        t.Fatal("Failed to check Docker services:", err)
    }
    
    runningServices := strings.Split(string(output), "\n")
    requiredServices := []string{"consul", "nomad", "seaweedfs-master", "seaweedfs-filer"}
    
    for _, required := range requiredServices {
        if !contains(runningServices, required) {
            t.Fatalf("Required service %s not running. Run: docker-compose up -d", required)
        }
    }
}
```

### 3. REFACTOR: VPS Integration Testing
```bash
# VPS integration test execution
ssh root@$TARGET_HOST 'su - ploy -c "cd /opt/ploy && make test-integration-vps"'

# VPS-specific integration tests with production services
# tests/integration/vps/transflow_production_test.go
```

## Integration Test Architecture

### Local Integration Environment (Docker)
```yaml
# docker-compose.integration.yml
version: '3.8'
services:
  consul:
    image: hashicorp/consul:1.16
    ports: ["8500:8500"]
    command: "consul agent -dev -client=0.0.0.0"
    
  nomad:
    image: hashicorp/nomad:1.6
    ports: ["4646:4646"]
    volumes: 
      - "/var/run/docker.sock:/var/run/docker.sock"
    environment:
      - NOMAD_LOCAL_CONFIG={"server":{"enabled":true},"client":{"enabled":true}}
    
  seaweedfs-master:
    image: chrislusf/seaweedfs:3.57
    ports: ["9333:9333"]
    command: "master -ip=seaweedfs-master"
    
  seaweedfs-filer:
    image: chrislusf/seaweedfs:3.57  
    ports: ["8888:8888"]
    command: "filer -master=seaweedfs-master:9333"
    depends_on: [seaweedfs-master]
    
  # GitLab test instance (optional, can use gitlab.com with test token)
  gitlab:
    image: gitlab/gitlab-ce:latest
    ports: ["8080:80"]
    environment:
      - GITLAB_OMNIBUS_CONFIG="external_url 'http://localhost:8080'"
    volumes:
      - gitlab-data:/var/opt/gitlab
```

### Integration Test Patterns

#### 1. Service Health Validation
```go
func TestServiceHealthChecks(t *testing.T) {
    testutils.SkipIfNoServices(t)
    
    services := []struct {
        name string
        url  string
        timeout time.Duration
    }{
        {"Consul", "http://localhost:8500/v1/status/leader", 5 * time.Second},
        {"Nomad", "http://localhost:4646/v1/status/leader", 5 * time.Second}, 
        {"SeaweedFS", "http://localhost:9333/cluster/status", 5 * time.Second},
    }
    
    for _, service := range services {
        t.Run(service.name, func(t *testing.T) {
            client := &http.Client{Timeout: service.timeout}
            resp, err := client.Get(service.url)
            assert.NoError(t, err, "%s should be healthy", service.name)
            assert.Equal(t, 200, resp.StatusCode)
            resp.Body.Close()
        })
    }
}
```

#### 2. Data Persistence Validation
```go
func TestKBDataPersistence(t *testing.T) {
    testutils.SkipIfNoServices(t)
    
    kb := kb.NewService(testConfig.KB)
    
    // Store learning case
    attempt := &models.HealingAttempt{
        ErrorSignature: "persistent-test-error",
        Patch: []byte("test patch content"),
        Success: true,
    }
    
    err := kb.RecordHealing(context.Background(), attempt)  
    assert.NoError(t, err)
    
    // Restart KB service to test persistence
    kb.Close()
    kb = kb.NewService(testConfig.KB)
    
    // Verify data survived restart
    cases, err := kb.GetCasesByError(context.Background(), "persistent-test-error")
    assert.NoError(t, err)
    assert.Len(t, cases, 1)
    assert.Equal(t, "test patch content", string(cases[0].Patch))
}
```

#### 3. End-to-End Workflow Testing
```go
func TestTransflowE2E_JavaMigration(t *testing.T) {
    testutils.SkipIfNoServices(t)
    
    // Use real test repository
    config := testutils.SetupIntegrationEnvironment(t)
    runner := transflow.NewRunner(config, realDependencies(config))
    
    request := &transflow.Request{
        ID: fmt.Sprintf("integration-test-%d", time.Now().Unix()),
        TargetRepo: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
        TargetBranch: "refs/heads/main", 
        BaseRef: "refs/heads/main",
        Steps: []Step{{
            Type: "recipe",
            Engine: "openrewrite", 
            Recipes: []string{"org.openrewrite.java.migrate.Java11toJava17"},
        }},
        Lane: "C", // Java applications
        BuildTimeout: 10 * time.Minute,
    }
    
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
    defer cancel()
    
    result, err := runner.Execute(ctx, request)
    assert.NoError(t, err)
    
    // Validate workflow completion
    assert.True(t, result.Success, "Transflow should succeed")
    assert.NotEmpty(t, result.WorkflowBranch, "Should create workflow branch")
    assert.NotEmpty(t, result.BuildVersion, "Should complete build")
    
    // If self-healing enabled and build failed, validate healing occurred
    if !result.InitialBuildSuccess && len(result.HealingAttempts) > 0 {
        assert.True(t, result.HealingSuccess, "Healing should succeed")
        
        // Validate KB learning from healing
        lastAttempt := result.HealingAttempts[len(result.HealingAttempts)-1]
        history, err := runner.KB.GetErrorHistory(ctx, lastAttempt.ErrorSignature)
        assert.NoError(t, err)
        assert.True(t, history.TotalCases > 0, "KB should learn from healing")
    }
    
    // Cleanup: delete test branch if created
    if result.WorkflowBranch != "" {
        cleanupTestBranch(t, config, result.WorkflowBranch)
    }
}
```

## Context Files  
- @iac/local/docker-compose.yml - Existing local service setup
- @tests/integration/ - Existing integration test patterns
- @internal/testutils/integration.go - Integration test utilities
- @docs/TESTING.md - Integration testing strategy

## User Notes

**Service Dependencies:**
- All integration tests require Docker services running locally
- Use `testutils.SkipIfNoServices(t)` for graceful skipping
- VPS tests require SSH access to `$TARGET_HOST` environment

**Environment Variables for Integration:**  
```bash
# Required for GitLab integration tests
export GITLAB_URL=https://gitlab.com
export GITLAB_TOKEN=your-test-token
export GITLAB_TEST_PROJECT=your-org/test-repo

# Optional for custom service endpoints
export CONSUL_HTTP_ADDR=localhost:8500
export NOMAD_ADDR=http://localhost:4646
export SEAWEEDFS_FILER=http://localhost:8888

# Test configuration  
export PLOY_TEST_TIMEOUT=15m
export PLOY_INTEGRATION_CLEANUP=true
```

**Test Execution:**
```bash
# Start Docker services
docker-compose -f docker-compose.integration.yml up -d

# Run integration tests  
make test-integration

# Run specific integration test
go test -v ./tests/integration/transflow -run TestTransflowE2E

# Run with increased timeout for slow tests
go test -timeout 20m ./tests/integration/...

# VPS integration testing
TARGET_HOST=45.12.75.241 make test-integration-vps
```

**Performance Requirements:**
- Local integration suite: <5 minutes total execution
- VPS integration suite: <10 minutes total execution  
- Individual workflow tests: <2 minutes per test
- KB learning tests: <30 seconds per test

**Cleanup Strategy:**
- Clean up test branches after workflow tests
- Clean up test KB data after learning tests
- Reset Docker containers between test runs if flaky
- Implement test resource quotas to prevent resource exhaustion

## Work Log
- [2025-01-09] Created integration test validation subtask with comprehensive Docker and VPS testing strategy