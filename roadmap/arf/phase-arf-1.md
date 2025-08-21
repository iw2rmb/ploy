# Phase ARF-1: Foundation & Core Engine

**Duration**: Foundational phase establishing core infrastructure
**Prerequisites**: Ploy infrastructure (Nomad, Consul, SeaweedFS, FreeBSD)
**Dependencies**: Lane C Java builder pipeline

## Overview

Phase ARF-1 establishes the foundational infrastructure for the Automated Remediation Framework, integrating OpenRewrite's powerful static analysis capabilities with Ploy's existing deployment infrastructure. This phase creates the core transformation engine, sandbox management system, and recipe catalog that will power all subsequent ARF capabilities.

## Technical Architecture

### Core Components
- **OpenRewrite Integration**: Java-based transformation engine with 2,800+ recipes
- **Sandbox Management**: FreeBSD jail-based secure transformation environments
- **Recipe Catalog**: Searchable database of transformation recipes with metadata
- **AST Cache System**: Memory-mapped file caching for 10x performance improvement

### Integration Points
- **Lane C Builder**: Leverages existing Java/Scala build pipeline validation
- **Nomad Scheduler**: Parallel sandbox execution and resource management
- **SeaweedFS Storage**: Transformation artifact and cache storage
- **Consul Service Discovery**: Recipe metadata and execution coordination

## Implementation Tasks

### 1. OpenRewrite Integration Infrastructure

**Objective**: Establish OpenRewrite as the core transformation engine within Ploy's controller architecture.

**Tasks**:
- Install and configure OpenRewrite dependencies in controller module
- Create `controller/arf/` directory structure for ARF components
- Implement `ARFEngine` interface with basic OpenRewrite recipe execution
- Add OpenRewrite Maven dependencies to Java build pipeline
- Create AST cache system using memory-mapped files + LRU cache
- Integrate with existing `controller/builders/java_osv.go` for Lane C validation

**Deliverables**:
```go
// controller/arf/engine.go
type ARFEngine interface {
    ExecuteRecipe(ctx context.Context, recipe Recipe, codebase Codebase) (*TransformationResult, error)
    ValidateRecipe(recipe Recipe) error
    ListAvailableRecipes() ([]Recipe, error)
}

// controller/arf/cache.go
type ASTCache interface {
    Get(key string) (*AST, bool)
    Put(key string, ast *AST)
    Evict(key string)
    Size() int
}
```

**Acceptance Criteria**:
- OpenRewrite engine successfully processes Java/Scala codebases
- AST cache provides 10x performance improvement for repeated operations
- Integration with Lane C validation pipeline completes without conflicts
- Memory-mapped cache persists across controller restarts

### 2. Sandbox Management System

**Objective**: Create secure, isolated environments for code transformations using FreeBSD jails with ZFS snapshot capabilities.

**Tasks**:
- Implement `SandboxManager` using FreeBSD jails for secure transformation environments
- Create ZFS snapshot-based rollback capability for instant restoration
- Integrate with Nomad scheduler for parallel sandbox execution
- Add sandbox validation pipeline (compilation → testing → security scanning)
- Create sandbox cleanup service with configurable TTL (similar to preview cleanup)

**Deliverables**:
```go
// controller/arf/sandbox.go
type SandboxManager interface {
    CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error)
    DestroySandbox(ctx context.Context, sandboxID string) error
    ExecuteInSandbox(ctx context.Context, sandboxID string, command Command) (*ExecutionResult, error)
    CreateSnapshot(ctx context.Context, sandboxID string) (*Snapshot, error)
    RestoreSnapshot(ctx context.Context, snapshotID string) error
}

// platform/nomad/templates/arf-sandbox.hcl.j2
job "arf-sandbox-{{ sandbox_id }}" {
  // Nomad job template for ARF sandboxes
}
```

**Acceptance Criteria**:
- FreeBSD jails provide complete isolation for transformation processes
- ZFS snapshots enable instant rollback in <5 seconds
- Parallel sandbox execution scales to 10+ concurrent transformations
- Sandbox cleanup prevents resource accumulation
- Integration with Nomad handles resource allocation and scheduling

### 3. Recipe Discovery & Management

**Objective**: Build a comprehensive catalog and search system for OpenRewrite's 2,800+ transformation recipes.

**Tasks**:
- Implement static recipe catalog with 2,800+ OpenRewrite recipes
- Create recipe metadata database with success rates and compatibility info
- Build recipe search engine with similarity scoring and filtering
- Add recipe validation system for OpenRewrite YAML syntax checking
- Create recipe performance tracking with historical success metrics

**Deliverables**:
```go
// controller/arf/recipes.go
type RecipeManager interface {
    SearchRecipes(query RecipeQuery) ([]Recipe, error)
    GetRecipe(recipeID string) (*Recipe, error)
    ValidateRecipe(recipe Recipe) (*ValidationResult, error)
    TrackRecipePerformance(recipeID string, result TransformationResult) error
    GetRecipeMetrics(recipeID string) (*RecipeMetrics, error)
}

// Sample recipe metadata structure
type Recipe struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Description string            `json:"description"`
    Category    string            `json:"category"`
    Tags        []string          `json:"tags"`
    Compatibility map[string]string `json:"compatibility"`
    SuccessRate float64           `json:"success_rate"`
    YAML        string            `json:"yaml"`
}
```

**Acceptance Criteria**:
- Recipe search returns relevant results with <100ms response time
- Recipe validation catches syntax errors before execution
- Performance tracking accurately measures success rates
- Recipe catalog includes comprehensive metadata for all 2,800+ recipes
- Search supports filtering by language, framework, and transformation type

### 4. Basic Transformation Engine

**Objective**: Implement the core transformation workflow for single-repository operations with comprehensive error handling, metrics, and disaster recovery.

**Tasks**:
- Implement single-repository transformation workflow
- Create transformation result tracking with success/failure analysis
- Add basic error classification (syntax, compilation, semantic errors)
- Implement simple retry logic with exponential backoff
- Create transformation metrics collection and logging
- Add disaster recovery procedures with atomic rollback
- Implement repository state snapshots before transformations
- Create cost tracking for resource usage

**Deliverables**:
```go
// controller/arf/transformer.go
type TransformationEngine interface {
    Transform(ctx context.Context, req TransformationRequest) (*TransformationResult, error)
    GetTransformationStatus(transformationID string) (*TransformationStatus, error)
    CancelTransformation(transformationID string) error
}

type TransformationRequest struct {
    Repository    Repository    `json:"repository"`
    Recipe        Recipe        `json:"recipe"`
    Configuration map[string]interface{} `json:"configuration"`
    Timeout       time.Duration `json:"timeout"`
}

type TransformationResult struct {
    ID            string                 `json:"id"`
    Status        TransformationStatus   `json:"status"`
    Changes       []FileChange           `json:"changes"`
    Errors        []TransformationError  `json:"errors"`
    Metrics       TransformationMetrics  `json:"metrics"`
    ArtifactURL   string                 `json:"artifact_url"`
}
```

**Acceptance Criteria**:
- Single-repository transformations complete with 90%+ success rate
- Error classification accurately categorizes failure types
- Retry logic handles transient failures automatically
- Transformation artifacts are stored in SeaweedFS with integrity verification
- Comprehensive logging enables debugging and performance analysis
- Disaster recovery procedures restore repository state within 30 seconds
- Cost tracking provides accurate resource usage metrics

### 5. Cost Model Framework

**Objective**: Implement cost tracking and resource planning for transformation operations to enable budgeting and optimization.

**Tasks**:
- Create resource usage tracking for CPU, memory, and storage
- Implement cost calculation models for different transformation types
- Add budgeting controls with spending limits
- Create cost optimization recommendations
- Implement usage reporting and analytics

**Deliverables**:
```go
// controller/arf/cost_model.go
type CostModel interface {
    TrackResourceUsage(ctx context.Context, transformation TransformationID) (*ResourceUsage, error)
    CalculateCost(ctx context.Context, usage ResourceUsage) (*CostBreakdown, error)
    SetBudget(ctx context.Context, budget BudgetConfig) error
    CheckBudgetLimit(ctx context.Context, projected ProjectedCost) (*BudgetStatus, error)
    GenerateCostReport(ctx context.Context, timeframe TimeFrame) (*CostReport, error)
}

type ResourceUsage struct {
    TransformationID string        `json:"transformation_id"`
    CPUSeconds       float64       `json:"cpu_seconds"`
    MemoryGBHours    float64       `json:"memory_gb_hours"`
    StorageGB        float64       `json:"storage_gb"`
    NetworkGB        float64       `json:"network_gb"`
    LLMTokens        int           `json:"llm_tokens,omitempty"`
    Duration         time.Duration `json:"duration"`
}

type CostBreakdown struct {
    ComputeCost    float64            `json:"compute_cost"`
    StorageCost    float64            `json:"storage_cost"`
    NetworkCost    float64            `json:"network_cost"`
    LLMCost        float64            `json:"llm_cost,omitempty"`
    TotalCost      float64            `json:"total_cost"`
    CostPerRepo    float64            `json:"cost_per_repo"`
    Optimizations  []CostOptimization `json:"optimizations"`
}
```

**Acceptance Criteria**:
- Resource tracking accuracy within 5% of actual usage
- Cost predictions enable effective budgeting
- Optimization recommendations reduce costs by 20%+
- Budget controls prevent overspending

### 6. Disaster Recovery & Rollback

**Objective**: Implement comprehensive disaster recovery procedures to ensure safe transformations with quick rollback capabilities.

**Tasks**:
- Create atomic rollback procedures for transformation failures
- Implement repository state snapshots using ZFS
- Add break-glass emergency intervention procedures
- Create recovery playbooks for different failure scenarios
- Implement transformation checkpoint system

**Deliverables**:
```go
// controller/arf/disaster_recovery.go
type DisasterRecovery interface {
    CreateSnapshot(ctx context.Context, repository Repository) (*Snapshot, error)
    RollbackTransformation(ctx context.Context, transformationID string) error
    CreateCheckpoint(ctx context.Context, state TransformationState) (*Checkpoint, error)
    RestoreFromCheckpoint(ctx context.Context, checkpointID string) error
    ExecuteEmergencyStop(ctx context.Context, reason string) error
    GenerateRecoveryReport(ctx context.Context, incident Incident) (*RecoveryReport, error)
}

type Snapshot struct {
    ID           string    `json:"id"`
    Repository   string    `json:"repository"`
    Timestamp    time.Time `json:"timestamp"`
    ZFSSnapshot  string    `json:"zfs_snapshot"`
    GitCommit    string    `json:"git_commit"`
    Size         int64     `json:"size"`
    Checksum     string    `json:"checksum"`
}

type RecoveryPlaybook struct {
    FailureType     string          `json:"failure_type"`
    Severity        string          `json:"severity"`
    Steps           []RecoveryStep  `json:"steps"`
    MaxRecoveryTime time.Duration   `json:"max_recovery_time"`
    Escalation      EscalationPath  `json:"escalation"`
}
```

**Acceptance Criteria**:
- Atomic rollback completes within 30 seconds
- Zero data loss for any transformation failure
- Emergency stop halts all operations within 5 seconds
- Recovery playbooks cover 95% of failure scenarios
- Snapshots consume <10% additional storage

## Configuration Examples

### OpenRewrite Integration
```yaml
# configs/arf-config.yaml
arf:
  openrewrite:
    maven_settings: "/etc/arf/maven-settings.xml"
    recipe_cache_size: 1000
    ast_cache_size: "2GB"
    memory_mapped_cache: true
  sandbox:
    base_image: "freebsd-jail-java"
    resource_limits:
      cpu: "2000m"
      memory: "4Gi"
      disk: "10Gi"
    cleanup_ttl: "2h"
```

### Nomad Job Template
```hcl
# platform/nomad/templates/arf-transformation.hcl.j2
job "arf-transform-{{ transformation_id }}" {
  datacenters = ["{{ datacenter }}"]
  type = "batch"
  
  constraint {
    attribute = "${attr.kernel.name}"
    value     = "freebsd"
  }
  
  group "transformation" {
    task "transform" {
      driver = "jail"
      
      config {
        path = "/zroot/jails/arf-{{ transformation_id }}"
        command = "/usr/local/bin/arf-transform"
        args = [
          "--recipe", "{{ recipe_path }}",
          "--repository", "{{ repository_path }}",
          "--output", "/output"
        ]
      }
      
      resources {
        cpu    = 2000
        memory = 4096
        disk   = 10240
      }
    }
  }
}
```

## Testing Strategy

### Testing Infrastructure
**Dedicated ARF Test Environment**:
- **Test Repository Corpus**: Curated collection of 50+ repositories across different frameworks and complexity levels
- **Lane-Specific Validation**: Utilize Ploy's existing lanes for transformation validation
  - Lane C for Java/Scala transformations
  - Lane B for Node.js transformations
  - Lane A for Go transformations
- **Regression Test Suite**: Automated test harness for recipe evolution validation
- **Performance Benchmarking**: Baseline metrics for transformation operations

```yaml
# configs/arf-test-infrastructure.yaml
test_infrastructure:
  repository_corpus:
    small_repos: 20  # < 1000 files
    medium_repos: 20  # 1000-5000 files
    large_repos: 10   # > 5000 files
  
  validation_lanes:
    java: "lane-c"
    nodejs: "lane-b"
    go: "lane-a"
    python: "lane-b"
  
  regression_suite:
    recipe_versions: 3  # Keep last 3 versions for regression
    test_coverage_threshold: 80
    performance_regression_threshold: 20  # percent
```

### Unit Tests
- OpenRewrite engine initialization and recipe execution
- Sandbox creation, execution, and cleanup
- Recipe search and validation logic
- AST cache performance and correctness
- Cost tracking and resource prediction
- Rollback procedure validation

### Integration Tests
- End-to-end transformation workflows
- Nomad integration with sandbox scheduling
- SeaweedFS artifact storage and retrieval
- ZFS snapshot creation and restoration
- Disaster recovery procedures
- Multi-repository corpus validation

### Performance Tests
- AST cache performance under load
- Parallel sandbox execution scalability
- Recipe search response times
- Memory usage optimization validation
- Cost model accuracy verification
- Resource allocation efficiency

## Success Metrics

- **Recipe Catalog**: 2,800+ OpenRewrite recipes available and searchable
- **Transformation Success**: 90%+ success rate for well-defined transformations
- **Performance**: 10x improvement with AST caching
- **Sandbox Isolation**: Complete security isolation with ZFS rollback in <5s
- **Scalability**: 10+ concurrent transformations supported
- **Integration**: Seamless integration with existing Lane C build pipeline
- **Testing Coverage**: 80%+ test coverage with 50+ repository corpus
- **Cost Tracking**: Resource usage tracked with <5% variance
- **Disaster Recovery**: 30-second rollback capability with zero data loss
- **Emergency Response**: 5-second emergency stop activation

## Risk Mitigation

### Technical Risks
- **OpenRewrite Memory Usage**: Monitor JVM heap usage and implement memory limits
- **Sandbox Resource Exhaustion**: Implement resource quotas and cleanup policies
- **ZFS Snapshot Storage**: Monitor disk usage and implement retention policies

### Operational Risks
- **Recipe Compatibility**: Comprehensive testing with diverse codebases
- **Transformation Failures**: Robust error handling and rollback mechanisms
- **Performance Degradation**: Continuous monitoring and optimization

## Next Phase Dependencies

Phase ARF-1 provides the foundation for:
- **Phase ARF-2**: Multi-repository orchestration and error recovery
- **Phase ARF-3**: LLM integration for enhanced transformation capabilities
- **Phase ARF-4**: Security-focused transformations and production hardening
- **Phase ARF-5**: Enterprise-scale coordination and analytics