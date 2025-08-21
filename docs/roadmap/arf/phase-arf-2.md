# Phase ARF-2: Self-Healing Loop & Error Recovery

**Duration**: Resilience and orchestration phase
**Prerequisites**: Phase ARF-1 completed with sandbox and recipe management
**Dependencies**: Consul service discovery, Nomad job scheduling

## Overview

Phase ARF-2 transforms the basic transformation engine into a resilient, self-healing system capable of handling complex error scenarios and coordinating transformations across multiple repositories. This phase introduces circuit breaker patterns, error-driven recipe evolution, parallel processing capabilities, and sophisticated multi-repository orchestration.

## Technical Architecture

### Core Components
- **Circuit Breaker System**: Prevents cascade failures with automatic recovery
- **Error-Driven Recipe Evolution**: Automatically improves recipes based on failures
- **Fork-Join Framework**: Parallel error resolution with confidence scoring
- **Multi-Repository Orchestrator**: Dependency-aware transformation coordination

### Integration Points
- **Consul Leader Election**: Coordination for multi-repository operations
- **Nomad Resource Management**: Parallel execution scheduling and allocation
- **SeaweedFS Cache System**: Solution caching for similar error patterns
- **ARF-1 Foundation**: Builds upon sandbox and recipe management systems

## Implementation Tasks

### 1. Circuit Breaker Implementation

**Objective**: Implement resilient failure handling to prevent cascade failures and enable automatic recovery from systematic issues.

**Tasks**:
- Add circuit breaker pattern with 50% failure threshold
- Implement exponential backoff with jitter for retry operations
- Create failure threshold monitoring and automatic circuit opening
- Add health monitoring for transformation services
- Integrate circuit breaker state with Consul service discovery

**Deliverables**:
```go
// controller/arf/circuit_breaker.go
type CircuitBreaker interface {
    Execute(ctx context.Context, operation func() error) error
    GetState() CircuitState
    GetMetrics() CircuitMetrics
    Reset() error
}

type CircuitState int
const (
    CircuitClosed CircuitState = iota
    CircuitOpen
    CircuitHalfOpen
)

type CircuitConfig struct {
    FailureThreshold   int           `yaml:"failure_threshold"`
    OpenTimeout        time.Duration `yaml:"open_timeout"`
    MaxRetries         int           `yaml:"max_retries"`
    BackoffMultiplier  float64       `yaml:"backoff_multiplier"`
    JitterEnabled      bool          `yaml:"jitter_enabled"`
}
```

**Acceptance Criteria**:
- Circuit breaker prevents cascade failures when >50% operations fail
- Exponential backoff with jitter reduces system load during recovery
- Automatic circuit opening protects downstream systems
- Health monitoring provides real-time circuit state visibility
- Integration with Consul enables distributed circuit coordination

### 2. Error-Driven Recipe Evolution

**Objective**: Create a self-improving system that automatically enhances recipes based on transformation failures and success patterns.

**Tasks**:
- Implement error classification system (recipe_mismatch, compilation_failure, semantic_change, incomplete_transformation)
- Create automatic recipe modification based on failure analysis
- Add recipe extension system for incomplete transformations
- Implement safety checks and validation for modified recipes
- Create recipe rollback mechanism for problematic changes

**Deliverables**:
```go
// controller/arf/recipe_evolution.go
type RecipeEvolution interface {
    AnalyzeFailure(ctx context.Context, failure TransformationFailure) (*FailureAnalysis, error)
    EvolveRecipe(ctx context.Context, recipe Recipe, analysis FailureAnalysis) (*Recipe, error)
    ValidateEvolution(ctx context.Context, original, evolved Recipe) (*ValidationResult, error)
    RollbackRecipe(ctx context.Context, recipeID string, version int) error
}

type FailureAnalysis struct {
    ErrorType        ErrorType           `json:"error_type"`
    RootCause        string              `json:"root_cause"`
    SuggestedFixes   []RecipeModification `json:"suggested_fixes"`
    Confidence       float64             `json:"confidence"`
    SimilarPatterns  []FailurePattern    `json:"similar_patterns"`
}

type RecipeModification struct {
    Type        ModificationType `json:"type"`
    Target      string          `json:"target"`
    Change      string          `json:"change"`
    Justification string        `json:"justification"`
}
```

**Acceptance Criteria**:
- Error classification accurately categorizes 95% of failure types
- Recipe modifications improve success rates by 20-30%
- Safety validation prevents problematic recipe changes
- Recipe versioning enables rollback to previous working versions
- Pattern matching identifies similar failures across repositories

### 3. Parallel Error Resolution

**Objective**: Implement concurrent error resolution strategies to handle multiple failure scenarios simultaneously and select the best solution.

**Tasks**:
- Implement Fork-Join framework for concurrent error remediation
- Add parallel solution testing with confidence scoring
- Create solution caching system for similar error patterns
- Implement parallel sandbox execution using Nomad job allocation
- Add resource management for concurrent transformation jobs

**Deliverables**:
```go
// controller/arf/parallel_resolver.go
type ParallelResolver interface {
    ResolveError(ctx context.Context, error TransformationError) (*Resolution, error)
    ForkSolutions(ctx context.Context, strategies []ResolutionStrategy) <-chan ResolutionResult
    JoinResults(ctx context.Context, results <-chan ResolutionResult) (*BestResolution, error)
    CacheSolution(pattern ErrorPattern, solution Resolution) error
}

type ResolutionStrategy struct {
    ID          string                 `json:"id"`
    Type        StrategyType           `json:"type"`
    Recipe      Recipe                 `json:"recipe"`
    Parameters  map[string]interface{} `json:"parameters"`
    Confidence  float64               `json:"confidence"`
    Resources   ResourceRequirements   `json:"resources"`
}

type ResolutionResult struct {
    StrategyID    string              `json:"strategy_id"`
    Success       bool                `json:"success"`
    Confidence    float64             `json:"confidence"`
    Result        *TransformationResult `json:"result,omitempty"`
    Error         error               `json:"error,omitempty"`
    Duration      time.Duration       `json:"duration"`
}
```

**Acceptance Criteria**:
- Fork-Join framework executes 3-5 parallel resolution strategies
- Confidence scoring accurately ranks solution quality
- Solution caching reduces resolution time by 60% for similar patterns
- Resource management prevents system overload during parallel execution
- Best solution selection achieves 90%+ accuracy based on confidence scores

### 4. Multi-Repository Orchestration

**Objective**: Coordinate transformations across multiple repositories with dependency awareness, intelligent batching, and high availability integration.

**Tasks**:
- Implement dependency graph construction for repository ordering
- Create complexity-based repository grouping and batching (max 50-100 repos per batch)
- Add topological sort for dependency-aware transformation order
- Implement resource allocation planning for batch execution
- Create execution plan generation with timeout and rollback procedures
- Integrate with Ploy's HA controller architecture for distributed processing
- Implement Consul-based state management and leader election
- Add distributed locking for repository access coordination

**Deliverables**:
```go
// controller/arf/orchestration.go
type MultiRepoOrchestrator interface {
    PlanTransformation(ctx context.Context, repos []Repository, recipe Recipe) (*ExecutionPlan, error)
    ExecutePlan(ctx context.Context, plan ExecutionPlan) (*OrchestrationResult, error)
    GetPlanStatus(planID string) (*PlanStatus, error)
    CancelPlan(planID string) error
}

type ExecutionPlan struct {
    ID              string              `json:"id"`
    Batches         []RepositoryBatch   `json:"batches"`
    Dependencies    map[string][]string `json:"dependencies"`
    ResourcePlan    ResourceAllocation  `json:"resource_plan"`
    Timeout         time.Duration       `json:"timeout"`
    RollbackPlan    RollbackStrategy    `json:"rollback_plan"`
}

type RepositoryBatch struct {
    ID            string       `json:"id"`
    Repositories  []Repository `json:"repositories"`
    Complexity    float64      `json:"complexity"`
    EstimatedTime time.Duration `json:"estimated_time"`
    Dependencies  []string     `json:"dependencies"`
}
```

**Acceptance Criteria**:
- Dependency graph correctly identifies repository relationships
- Batch sizing optimizes resource utilization (50-100 repos per batch)
- Topological sorting ensures dependency-aware execution order
- Resource planning prevents overallocation and contention
- Rollback procedures restore system state after failures
- HA integration enables distributed processing across controller instances
- Leader election ensures single orchestrator for campaigns

### 5. High Availability Integration

**Objective**: Leverage Ploy's HA controller architecture to distribute ARF workloads and ensure resilience.

**Tasks**:
- Distribute transformation workloads across multiple controller instances
- Implement transformation state management in Consul KV
- Add leader election for campaign coordination using Consul
- Create distributed lock mechanisms for repository access
- Implement health checks and automatic failover for ARF components

**Deliverables**:
```go
// controller/arf/ha_integration.go
type HAIntegration interface {
    ElectLeader(ctx context.Context, campaign CampaignID) (*LeaderInfo, error)
    AcquireLock(ctx context.Context, resource string) (*DistributedLock, error)
    ReleaseLock(ctx context.Context, lock *DistributedLock) error
    StoreState(ctx context.Context, key string, state interface{}) error
    GetState(ctx context.Context, key string) (interface{}, error)
    RegisterHealthCheck(ctx context.Context, service ARFService) error
    DistributeWork(ctx context.Context, workItems []WorkItem) (*Distribution, error)
}

type LeaderInfo struct {
    NodeID      string    `json:"node_id"`
    ElectedAt   time.Time `json:"elected_at"`
    TTL         time.Duration `json:"ttl"`
    CampaignID  string    `json:"campaign_id"`
}

type DistributedLock struct {
    LockID      string    `json:"lock_id"`
    Resource    string    `json:"resource"`
    Owner       string    `json:"owner"`
    AcquiredAt  time.Time `json:"acquired_at"`
    TTL         time.Duration `json:"ttl"`
}

type Distribution struct {
    WorkItems   map[string][]WorkItem `json:"work_items"` // NodeID -> WorkItems
    Strategy    string               `json:"strategy"`
    LoadBalance LoadBalanceMetrics   `json:"load_balance"`
}
```

**Acceptance Criteria**:
- Zero-downtime ARF operations during controller failover
- State consistency maintained across distributed instances
- Leader election completes within 5 seconds
- Distributed locks prevent concurrent repository modifications
- Work distribution achieves <20% load variance across nodes

### 6. Monitoring Infrastructure

**Objective**: Implement comprehensive monitoring and alerting for ARF operations to ensure observability and rapid incident response.

**Tasks**:
- Define SLIs/SLOs for transformation success rates and latency
- Create Prometheus metrics for all ARF operations
- Implement PagerDuty integration for critical failures
- Add Grafana dashboards for campaign monitoring
- Create distributed tracing for transformation workflows

**Deliverables**:
```go
// controller/arf/monitoring.go
type MonitoringSystem interface {
    RecordMetric(ctx context.Context, metric Metric) error
    CheckSLO(ctx context.Context, slo SLO) (*SLOStatus, error)
    TriggerAlert(ctx context.Context, alert Alert) error
    CreateTrace(ctx context.Context, operation string) (*Trace, error)
    GenerateDashboard(ctx context.Context, config DashboardConfig) (*Dashboard, error)
}

type Metric struct {
    Name        string              `json:"name"`
    Type        MetricType          `json:"type"`
    Value       float64             `json:"value"`
    Labels      map[string]string   `json:"labels"`
    Timestamp   time.Time           `json:"timestamp"`
}

type SLO struct {
    Name            string          `json:"name"`
    Target          float64         `json:"target"`
    Window          time.Duration   `json:"window"`
    BurnRate        float64         `json:"burn_rate"`
    AlertThreshold  float64         `json:"alert_threshold"`
}

type Alert struct {
    Severity    AlertSeverity       `json:"severity"`
    Title       string              `json:"title"`
    Description string              `json:"description"`
    Runbook     string              `json:"runbook"`
    Labels      map[string]string   `json:"labels"`
    Routing     AlertRouting        `json:"routing"`
}
```

**Prometheus Metrics**:
```yaml
# ARF Metrics
arf_transformation_total{status, repository_type, recipe}
arf_transformation_duration_seconds{repository_type, recipe}
arf_circuit_breaker_state{circuit_name, state}
arf_recipe_evolution_success_rate{recipe_id}
arf_parallel_resolution_attempts{strategy}
arf_campaign_progress{campaign_id, status}
arf_error_pattern_matches{pattern_type}
arf_resource_usage{resource_type, component}
```

**Acceptance Criteria**:
- 99.9% SLO for metric collection reliability
- Alert response time <1 minute for critical issues
- Dashboard refresh rate <10 seconds
- Distributed tracing covers 100% of transformation workflows
- Metrics retention for 90 days minimum

### 7. Error Pattern Learning Database

**Objective**: Create a comprehensive database for storing and analyzing error patterns to improve transformation success rates.

**Tasks**:
- Design schema for error pattern storage with similarity indexing
- Implement pattern matching algorithms using vector embeddings
- Create feedback loop for pattern effectiveness tracking
- Add pattern generalization for cross-repository learning
- Implement pattern decay for outdated solutions

**Deliverables**:
```go
// controller/arf/error_pattern_db.go
type ErrorPatternDB interface {
    StorePattern(ctx context.Context, pattern ErrorPattern) error
    FindSimilarPatterns(ctx context.Context, error TransformationError, threshold float64) ([]ErrorPattern, error)
    UpdatePatternEffectiveness(ctx context.Context, patternID string, outcome Outcome) error
    GeneralizePattern(ctx context.Context, patterns []ErrorPattern) (*GeneralizedPattern, error)
    PruneOutdatedPatterns(ctx context.Context, age time.Duration) (int, error)
}

type ErrorPattern struct {
    ID              string              `json:"id"`
    Signature       string              `json:"signature"`
    ErrorType       string              `json:"error_type"`
    Context         ErrorContext        `json:"context"`
    Solutions       []Solution          `json:"solutions"`
    Effectiveness   float64             `json:"effectiveness"`
    Occurrences     int                 `json:"occurrences"`
    LastSeen        time.Time           `json:"last_seen"`
    Embedding       []float64           `json:"embedding"`
}

type GeneralizedPattern struct {
    BasePatterns    []string            `json:"base_patterns"`
    Generalization  string              `json:"generalization"`
    Confidence      float64             `json:"confidence"`
    Applicability   []string            `json:"applicability"`
}
```

**Database Schema**:
```sql
-- Error patterns table
CREATE TABLE error_patterns (
    id UUID PRIMARY KEY,
    signature TEXT NOT NULL,
    error_type VARCHAR(100),
    context JSONB,
    solutions JSONB,
    effectiveness FLOAT,
    occurrences INT,
    last_seen TIMESTAMP,
    embedding vector(768),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Pattern similarity index
CREATE INDEX idx_pattern_embedding ON error_patterns 
USING ivfflat (embedding vector_cosine_ops);

-- Pattern effectiveness tracking
CREATE TABLE pattern_feedback (
    pattern_id UUID REFERENCES error_patterns(id),
    transformation_id UUID,
    success BOOLEAN,
    confidence FLOAT,
    feedback_time TIMESTAMP DEFAULT NOW()
);
```

**Acceptance Criteria**:
- Pattern matching achieves 85% accuracy for similar errors
- Database handles 100K+ patterns with sub-second queries
- Pattern effectiveness tracking improves solution selection by 30%
- Generalization creates reusable patterns from 5+ similar instances
- Outdated pattern pruning maintains database performance

## Configuration Examples

### Circuit Breaker Configuration
```yaml
# configs/arf-circuit-breaker.yaml
circuit_breaker:
  default:
    failure_threshold: 10
    open_timeout: "30s"
    max_retries: 3
    backoff_multiplier: 2.0
    jitter_enabled: true
  
  recipe_execution:
    failure_threshold: 5
    open_timeout: "60s"
    max_retries: 5
    backoff_multiplier: 1.5
    jitter_enabled: true
```

### Parallel Resolution Configuration
```yaml
# configs/arf-parallel-resolution.yaml
parallel_resolution:
  max_concurrent_strategies: 5
  strategy_timeout: "5m"
  confidence_threshold: 0.7
  resource_limits:
    cpu_per_strategy: "1000m"
    memory_per_strategy: "2Gi"
  caching:
    pattern_cache_size: 1000
    cache_ttl: "24h"
```

### Multi-Repository Orchestration
```yaml
# configs/arf-orchestration.yaml
orchestration:
  batch_size:
    min_repos: 10
    max_repos: 100
    complexity_threshold: 0.8
  
  resource_allocation:
    total_cpu: "20000m"
    total_memory: "40Gi"
    reserve_percentage: 20
  
  execution:
    batch_timeout: "2h"
    dependency_timeout: "30m"
    rollback_timeout: "15m"
```

## Nomad Job Templates

### Parallel Resolution Job
```hcl
# platform/nomad/templates/arf-parallel-resolution.hcl.j2
job "arf-parallel-{{ resolution_id }}" {
  datacenters = ["{{ datacenter }}"]
  type = "batch"
  
  constraint {
    attribute = "${attr.kernel.name}"
    value     = "freebsd"
  }
  
  group "strategies" {
    count = {{ strategy_count }}
    
    restart {
      attempts = 2
      interval = "5m"
      delay    = "15s"
      mode     = "fail"
    }
    
    task "strategy-executor" {
      driver = "jail"
      
      config {
        path = "/zroot/jails/arf-strategy-${NOMAD_ALLOC_INDEX}"
        command = "/usr/local/bin/arf-strategy-executor"
        args = [
          "--strategy-id", "{{ strategy_id }}",
          "--input", "/input/repository.tar.gz",
          "--output", "/output/result.tar.gz"
        ]
      }
      
      template {
        data = <<-EOH
{{ strategy_config | to_json }}
EOH
        destination = "local/strategy.json"
      }
      
      resources {
        cpu    = 1000
        memory = 2048
        disk   = 5120
      }
    }
  }
}
```

### Multi-Repository Batch Job
```hcl
# platform/nomad/templates/arf-batch-execution.hcl.j2
job "arf-batch-{{ batch_id }}" {
  datacenters = ["{{ datacenter }}"]
  type = "batch"
  
  group "batch-coordinator" {
    task "coordinator" {
      driver = "exec"
      
      config {
        command = "/usr/local/bin/arf-batch-coordinator"
        args = [
          "--batch-id", "{{ batch_id }}",
          "--plan", "/local/execution-plan.json",
          "--parallelism", "{{ parallelism }}"
        ]
      }
      
      template {
        data = <<-EOH
{{ execution_plan | to_json }}
EOH
        destination = "local/execution-plan.json"
      }
      
      resources {
        cpu    = 500
        memory = 1024
        disk   = 2048
      }
    }
  }
  
  group "repo-transformers" {
    count = {{ parallelism }}
    
    task "transformer" {
      driver = "jail"
      
      config {
        path = "/zroot/jails/arf-transformer-${NOMAD_ALLOC_INDEX}"
        command = "/usr/local/bin/arf-repo-transformer"
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

### Unit Tests
- Circuit breaker state transitions and failure thresholds
- Recipe evolution logic and safety validation
- Parallel resolution strategy execution and result merging
- Dependency graph construction and topological sorting

### Integration Tests
- End-to-end circuit breaker behavior under load
- Recipe evolution with real transformation failures
- Parallel resolution with multiple concurrent strategies
- Multi-repository orchestration with complex dependency chains

### Performance Tests
- Circuit breaker overhead and recovery performance
- Parallel resolution scalability and resource utilization
- Multi-repository batch processing throughput
- Cache effectiveness for solution reuse

### Chaos Tests
- System behavior under partial failures
- Circuit breaker coordination across distributed instances
- Resource exhaustion scenarios during parallel execution
- Network partition handling during multi-repository operations

## Success Metrics

- **Failure Recovery**: 95% automatic recovery from transformation failures
- **Circuit Protection**: <5% cascade failure rate under high error conditions
- **Recipe Evolution**: 20-30% success rate improvement through recipe modification
- **Parallel Processing**: 3-5x faster error resolution through concurrent strategies
- **Orchestration Scale**: 50-100 repositories per batch with dependency coordination
- **Cache Effectiveness**: 60% reduction in resolution time for similar patterns
- **High Availability**: Zero-downtime operations with <5s leader election
- **Monitoring Coverage**: 100% observability with <1min alert response
- **Pattern Database**: 85% accuracy in error pattern matching
- **Load Distribution**: <20% variance across distributed nodes

## Risk Mitigation

### Technical Risks
- **Resource Exhaustion**: Implement resource quotas and priority-based scheduling
- **Circuit Oscillation**: Add hysteresis and minimum open duration
- **Recipe Drift**: Validate evolved recipes against regression test suites

### Operational Risks
- **Dependency Cycles**: Implement cycle detection in repository dependency graphs
- **Batch Failures**: Create granular rollback strategies for partial batch failures
- **Performance Degradation**: Monitor resource usage and implement adaptive batching

## Next Phase Dependencies

Phase ARF-2 enables:
- **Phase ARF-3**: LLM integration for enhanced error analysis and recipe generation
- **Phase ARF-4**: Security-focused transformations with vulnerability remediation
- **Phase ARF-5**: Enterprise-scale campaigns with advanced analytics and reporting

The resilient foundation provided by ARF-2 is essential for the advanced AI capabilities and enterprise features in subsequent phases.