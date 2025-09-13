# Mock Replacement Analysis and Priority Matrix

## Executive Summary

This document provides a comprehensive analysis of mock usage across the Ploy codebase and establishes priorities for replacing mocks with real service implementations in integration tests and production code.

**Current State:** 11 instances of testify/mock usage, widespread use of `interface{}` types in production code for mock/real service switching, and inappropriate mock references in production files.

**Goal:** Systematic replacement of mocks with real service calls to improve test fidelity, reduce mock-reality drift, and establish clear boundaries between unit tests (mocks) and integration tests (real services).

## Priority Classification

### HIGH PRIORITY - Production Critical Services

These services are used directly in production workflows and have the highest impact on system reliability. Mock replacement is critical for production confidence.

#### 1. Nomad Job Submission (CRITICAL)
- **Current State**: 
  - `fanout_orchestrator.go:31` - Uses `interface{}` for "MockJobSubmitter in tests, real submitter in production"
  - `job_submission.go:26` - Same pattern with interface{} switching
  - Production healing workflows depend on real Nomad job orchestration
- **Real Implementation**: Direct Nomad API client via `nomadapi.Client`
- **Dependencies**: Nomad cluster, HCL template rendering, job monitoring
- **Impact**: Core transflow healing functionality, production orchestration
- **Estimated Effort**: 3-5 days
- **Risk**: HIGH - Critical path for self-healing workflows

#### 2. SeaweedFS Storage Operations (CRITICAL)  
- **Current State**:
  - Storage operations may use mock implementations in integration tests
  - KB (Knowledge Base) persistence layer depends on real storage
  - Artifact storage for job outputs
- **Real Implementation**: SeaweedFS HTTP client via internal/storage package
- **Dependencies**: SeaweedFS master/filer, HTTP client configuration
- **Impact**: Data persistence, KB learning, artifact storage
- **Estimated Effort**: 2-3 days
- **Risk**: HIGH - Data loss potential, KB system integrity

#### 3. GitLab API Integration (HIGH)
- **Current State**:
  - Mods MR creation may use mocks in integration scenarios
  - Human-step healing branches require real GitLab API calls
- **Real Implementation**: GitLab REST API via internal/git/provider
- **Dependencies**: GITLAB_TOKEN, test project access, MR permissions
- **Impact**: MR creation, human intervention workflows, CI integration
- **Estimated Effort**: 2-3 days  
- **Risk**: MEDIUM - Workflow interruption, manual intervention failures

#### 4. KB Service Integration (HIGH)
- **Current State**:
  - KB learning pipeline may use mocks for development
  - Critical for transflow self-healing intelligence
- **Real Implementation**: SeaweedFS + Consul coordination
- **Dependencies**: Storage backend, distributed locking, signature generation
- **Impact**: Learning effectiveness, healing intelligence, duplicate detection
- **Estimated Effort**: 3-4 days
- **Risk**: MEDIUM - Reduced healing effectiveness

### MEDIUM PRIORITY - Integration Testing Enhancement

These services improve integration test quality and catch more bugs but don't directly impact production workflows.

#### 5. Consul KV Operations (MEDIUM)
- **Current State**: 
  - Distributed locking for KB operations may use mocks
  - Configuration coordination across services
- **Real Implementation**: Consul HTTP API via orchestration.KV interface
- **Dependencies**: Consul cluster, key-value permissions
- **Impact**: Distributed locking, configuration sharing, coordination
- **Estimated Effort**: 1-2 days
- **Risk**: LOW - Fallback mechanisms available

#### 6. Build API Client (MEDIUM)
- **Current State**:
  - Build validation may use simplified mocks
  - Critical for transflow build gate validation
- **Real Implementation**: Real build service calls via common.SharedPush
- **Dependencies**: Build service availability, Docker, registry access
- **Impact**: Build validation accuracy, deployment confidence
- **Estimated Effort**: 2-3 days
- **Risk**: MEDIUM - False positives/negatives in build validation

#### 7. External HTTP Clients (MEDIUM)
- **Current State**: Generic HTTP operations may use test doubles
- **Real Implementation**: Real HTTP calls with test servers/fixtures
- **Dependencies**: Test server setup, network access, timeout handling
- **Impact**: HTTP interaction accuracy, timeout behavior
- **Estimated Effort**: 1-2 days
- **Risk**: LOW - Network dependency management

### LOW PRIORITY - Keep Mocks (Appropriate Usage)

These areas should continue using mocks as they serve legitimate testing purposes.

#### 8. Unit Test Dependencies (KEEP MOCKS)
- **Rationale**: Fast, isolated, predictable unit testing
- **Scope**: Individual function/method testing, error path simulation
- **Examples**: MockGitOperations, MockRecipeExecutor, MockBuildChecker in unit tests
- **Pattern**: Test-only mock implementations, never in production code

#### 9. Performance Benchmarking (KEEP MOCKS)  
- **Rationale**: Controlled benchmarking, eliminate network/IO variance
- **Scope**: Performance regression testing, load simulation
- **Pattern**: Dedicated performance test mocks with known response times

#### 10. Error Scenario Simulation (KEEP MOCKS)
- **Rationale**: Reliable error path testing, edge case coverage
- **Scope**: Failure simulation, timeout testing, partial failure scenarios
- **Pattern**: Configurable error injection via mock implementations

## Current Mock Inventory

### Production Code Issues (CRITICAL TO FIX)

| File | Line | Issue | Priority |
|------|------|-------|----------|
| `fanout_orchestrator.go` | 31 | `interface{}` with mock comment | HIGH |
| `job_submission.go` | 26 | `interface{}` with mock comment | HIGH |
| `runner.go` | 133 | `interface{}` jobSubmitter field | HIGH |
| `runner.go` | 165 | SetJobSubmitter accepts `interface{}` | HIGH |

### Mock Implementation Files (APPROPRIATE)

| File | Purpose | Status |
|------|---------|--------|
| `mocks.go` | Test mock implementations | KEEP - Appropriate |
| `internal/testing/mocks/*` | Shared test utilities | KEEP - Appropriate |

## Replacement Strategy

### Phase 1: Interface Definition and Type Safety (Week 1)

1. **Define Real Service Interfaces**
   ```go
   type JobSubmitter interface {
       SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error)
   }
   
   type StorageClient interface {
       Store(ctx context.Context, key string, data io.Reader) error
       Retrieve(ctx context.Context, key string) (io.ReadCloser, error)
       Delete(ctx context.Context, key string) error
   }
   ```

2. **Replace interface{} with Proper Types**
   - Convert `interface{}` fields to typed interfaces
   - Remove mock references from production code comments
   - Establish clear dependency injection patterns

### Phase 2: Real Service Implementation (Week 2-3)

1. **Implement Real Service Clients**
   - NomadJobSubmitter using nomadapi.Client
   - SeaweedStorageClient using HTTP API
   - GitLabProvider using GitLab REST API
   - ConsulKVClient using consulapi.Client

2. **Update Factory Methods**
   - Production constructors use real services only
   - Test constructors support mock injection
   - Clear separation of concerns

### Phase 3: Integration Test Migration (Week 4)

1. **Migrate Existing Integration Tests**
   - Update tests to use RequireServices instead of SkipIfNoServices  
   - Verify real service interactions
   - Add service health validation

2. **Validate Real Service Behavior**
   - Confirm job submission creates actual Nomad jobs
   - Verify storage operations persist data
   - Validate MR creation in GitLab

### Phase 4: Production Validation (Week 5)

1. **VPS Environment Testing**
   - Deploy changes to VPS environment
   - Run integration tests with production services
   - Validate end-to-end workflows

2. **Performance Optimization**
   - Connection pooling for real services
   - Timeout optimization
   - Error handling improvements

## Dependencies and Prerequisites

### Infrastructure Requirements

1. **Docker Compose Services**
   ```yaml
   # docker-compose.integration.yml
   services:
     consul:
       image: hashicorp/consul:latest
       ports: ["8500:8500"]
     nomad:
       image: hashicorp/nomad:latest  
       ports: ["4646:4646"]
     seaweedfs-master:
       image: chrislusf/seaweedfs:latest
       ports: ["9333:9333"]
     seaweedfs-filer:
       image: chrislusf/seaweedfs:latest
       ports: ["8888:8888"]
   ```

2. **Environment Variables**
   ```bash
   export CONSUL_HTTP_ADDR=localhost:8500
   export NOMAD_ADDR=http://localhost:4646  
   export SEAWEEDFS_FILER=http://localhost:8888
   export GITLAB_URL=https://gitlab.com
   export GITLAB_TOKEN=your-integration-test-token
   ```

3. **Test Project Setup**
   - GitLab test project with MR permissions
   - Nomad cluster with job submission permissions
   - SeaweedFS storage with read/write access

### Code Dependencies

1. **Updated Test Utilities**
   - RequireServices function (hard requirement, no skip)
   - Service health validation
   - Real service client factories

2. **Interface Definitions**  
   - Typed interfaces replacing interface{} usage
   - Clear separation between production and test interfaces
   - Dependency injection patterns

## Success Metrics

### Quantitative Goals

- **Mock Reduction**: 80% reduction in inappropriate mock usage in production code
- **Test Coverage**: Maintain >90% test coverage while improving test fidelity  
- **Service Validation**: 100% of integration tests validate real service interactions
- **Type Safety**: 0 remaining interface{} fields for service injection

### Qualitative Improvements

- **Test Fidelity**: Integration tests catch service-specific issues
- **Mock-Reality Alignment**: No drift between mock and real service behavior
- **Production Confidence**: Integration tests mirror production service usage
- **Development Workflow**: Clear boundaries between unit and integration testing

## Risk Mitigation

### High Risk Areas

1. **Nomad Job Submission**
   - **Risk**: Production healing workflows broken
   - **Mitigation**: Comprehensive integration testing, gradual rollout
   - **Rollback**: Maintain interface compatibility during transition

2. **SeaweedFS Storage**
   - **Risk**: Data corruption, KB system failure  
   - **Mitigation**: Backup/restore testing, data validation
   - **Rollback**: Storage interface abstraction enables quick fallback

3. **Service Dependencies**
   - **Risk**: Integration tests require complex infrastructure
   - **Mitigation**: Docker compose setup, clear documentation
   - **Rollback**: Graceful degradation to skip mode if needed

### Medium Risk Areas

1. **Performance Impact**
   - **Risk**: Real services slower than mocks
   - **Mitigation**: Connection pooling, timeout optimization
   - **Monitoring**: Integration test duration tracking

2. **Environment Setup Complexity**
   - **Risk**: Developer friction, CI/CD complexity  
   - **Mitigation**: Automated setup scripts, clear documentation
   - **Support**: Docker-based standardized environment

## Implementation Timeline

| Week | Phase | Deliverables |
|------|-------|-------------|
| 1 | Interface Definition | Typed interfaces, remove interface{} |
| 2-3 | Real Service Implementation | Service clients, factory methods |
| 4 | Integration Test Migration | Updated tests, RequireServices |
| 5 | Production Validation | VPS testing, performance optimization |

## Conclusion

Systematic replacement of mocks with real services will significantly improve test fidelity and production confidence. The phased approach minimizes risk while ensuring comprehensive coverage of all service interactions.

Priority focus on production-critical services (Nomad, SeaweedFS, GitLab) will deliver the highest value, while maintaining appropriate mock usage for unit tests and error simulation ensures continued development velocity.

Success depends on proper infrastructure setup, clear interface definitions, and comprehensive integration testing with real service validation.