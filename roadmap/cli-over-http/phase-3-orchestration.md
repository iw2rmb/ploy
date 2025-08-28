# Phase 3: Pipeline Orchestration Engine

**Status**: ✅ Completed (Simplified)  
**Duration**: 2-3 weeks  
**Dependencies**: Phase 1, Phase 2 completed  
**Next Phase**: [Phase 4: Service Discovery & Load Balancing](./phase-4-discovery-balancing.md)

## Executive Summary

~~Phase 3 originally planned complex pipeline orchestration with parallel execution and dependency management.~~ 

**SIMPLIFIED IMPLEMENTATION**: Phase 3 was completed with basic external service communication only. Complex orchestration features were removed as they duplicate Ploy's comprehensive orchestration capabilities.

## Simplified Objectives (Completed)

- ✅ **Basic External Service Communication**: Simple HTTP client for calling external CHTTP services
- ✅ **Request/Response Handling**: JSON request and response processing
- ✅ **Error Handling**: Basic error propagation and timeout handling
- ✅ **Header Pass-through**: Forward authentication and tracing headers
- ❌ ~~Complex orchestration features~~ → **Handled by Ploy platform**

## Completion Status

✅ **Completed (Simplified Implementation):**
- HTTP client for external service calls
- Basic request/response processing
- Error handling and timeout management
- Authentication header pass-through
- Integration with simplified CHTTP architecture

~~❌ **Removed Features:**~~
- ~~Parallel execution engine~~ → External orchestration tools
- ~~Dependency management~~ → Handled by Ploy workflows
- ~~Complex resource management~~ → Ploy infrastructure management
- ~~Advanced load balancing~~ → Traefik handles all load balancing
- Dependency-aware orchestration
- Advanced load balancing
- Service health checking
- Pipeline metrics and monitoring

## Implementation Plan

### 1. Parallel Execution Engine

#### 1.1 Enhanced Pipeline Types

```go
// internal/pipeline/orchestration.go
package pipeline

import (
    "context"
    "sync"
    "time"
)

// OrchestrationEngine manages parallel pipeline execution
type OrchestrationEngine struct {
    maxConcurrency   int
    healthChecker    *HealthChecker
    loadBalancer     *LoadBalancer
    metrics          *OrchestrationMetrics
    semaphore        chan struct{}
    executorPool     *sync.Pool
}

// ParallelStep extends Step with orchestration metadata
type ParallelStep struct {
    ID           string                 `json:"id"`
    Service      string                 `json:"service"`
    Config       map[string]interface{} `json:"config,omitempty"`
    DependsOn    []string              `json:"depends_on,omitempty"`
    ParallelGroup string                `json:"parallel_group,omitempty"`
    Weight       int                   `json:"weight,omitempty"`
    Timeout      string                `json:"timeout,omitempty"`
    Retries      int                   `json:"retries,omitempty"`
}

// OrchestrationRequest extends Request for parallel execution
type OrchestrationRequest struct {
    Steps         []ParallelStep        `json:"steps"`
    Archive       []byte               `json:"archive,omitempty"`
    Options       OrchestrationOptions `json:"options,omitempty"`
}

// OrchestrationOptions contains advanced orchestration settings
type OrchestrationOptions struct {
    MaxConcurrent    int    `json:"max_concurrent,omitempty"`
    ExecutionStrategy string `json:"execution_strategy,omitempty"` // "parallel", "sequential", "mixed"
    FailFast         bool   `json:"fail_fast"`
    Timeout          string `json:"timeout,omitempty"`
    RetryStrategy    string `json:"retry_strategy,omitempty"`
}

// ExecutionPlan represents the computed execution plan
type ExecutionPlan struct {
    Stages    []ExecutionStage `json:"stages"`
    TotalSteps int             `json:"total_steps"`
    EstimatedDuration time.Duration `json:"estimated_duration"`
}

// ExecutionStage groups steps that can execute in parallel
type ExecutionStage struct {
    Steps      []ParallelStep `json:"steps"`
    StageIndex int           `json:"stage_index"`
    CanRunInParallel bool   `json:"can_run_in_parallel"`
}
```

#### 1.2 Dependency Resolution Engine

```go
// internal/pipeline/dependency.go
package pipeline

import (
    "fmt"
    "sort"
)

// DependencyResolver handles step dependency analysis
type DependencyResolver struct{}

// ResolveDependencies creates an execution plan from steps with dependencies
func (dr *DependencyResolver) ResolveDependencies(steps []ParallelStep) (*ExecutionPlan, error) {
    // Build dependency graph
    graph := make(map[string][]string)
    inDegree := make(map[string]int)
    stepMap := make(map[string]ParallelStep)
    
    for _, step := range steps {
        stepMap[step.ID] = step
        inDegree[step.ID] = len(step.DependsOn)
        
        for _, dep := range step.DependsOn {
            graph[dep] = append(graph[dep], step.ID)
        }
    }
    
    // Topological sort for execution ordering
    stages := []ExecutionStage{}
    processed := make(map[string]bool)
    
    for len(processed) < len(steps) {
        // Find steps with no remaining dependencies
        currentStage := ExecutionStage{
            StageIndex: len(stages),
            CanRunInParallel: true,
        }
        
        var readySteps []string
        for stepID, degree := range inDegree {
            if degree == 0 && !processed[stepID] {
                readySteps = append(readySteps, stepID)
            }
        }
        
        if len(readySteps) == 0 {
            return nil, fmt.Errorf("circular dependency detected")
        }
        
        // Add ready steps to current stage
        for _, stepID := range readySteps {
            currentStage.Steps = append(currentStage.Steps, stepMap[stepID])
            processed[stepID] = true
            
            // Update dependencies
            for _, dependent := range graph[stepID] {
                inDegree[dependent]--
            }
        }
        
        stages = append(stages, currentStage)
    }
    
    return &ExecutionPlan{
        Stages:    stages,
        TotalSteps: len(steps),
        EstimatedDuration: dr.estimateDuration(stages),
    }, nil
}

// estimateDuration estimates total execution time considering parallelism
func (dr *DependencyResolver) estimateDuration(stages []ExecutionStage) time.Duration {
    total := time.Duration(0)
    
    for _, stage := range stages {
        stageMax := time.Duration(0)
        
        for _, step := range stage.Steps {
            stepTimeout := 30 * time.Second // default
            if step.Timeout != "" {
                if parsed, err := time.ParseDuration(step.Timeout); err == nil {
                    stepTimeout = parsed
                }
            }
            
            if stepTimeout > stageMax {
                stageMax = stepTimeout
            }
        }
        
        total += stageMax
    }
    
    return total
}
```

#### 1.3 Parallel Execution Engine

```go
// internal/pipeline/parallel_executor.go
package pipeline

import (
    "context"
    "fmt"
    "sync"
    "time"
)

// ParallelExecutor manages concurrent step execution
type ParallelExecutor struct {
    client          *Client
    healthChecker   *HealthChecker
    semaphore       chan struct{}
    resultCollector *ResultCollector
    metrics         *OrchestrationMetrics
}

// ExecuteOrchestration runs the orchestrated pipeline
func (pe *ParallelExecutor) ExecuteOrchestration(ctx context.Context, req *OrchestrationRequest, execCtx *ExecutionContext) (*Response, error) {
    // Generate pipeline ID
    pipelineID := uuid.New().String()
    
    // Resolve dependencies and create execution plan
    resolver := &DependencyResolver{}
    plan, err := resolver.ResolveDependencies(req.Steps)
    if err != nil {
        return nil, fmt.Errorf("dependency resolution failed: %w", err)
    }
    
    // Initialize result collector
    collector := NewResultCollector(len(req.Steps))
    
    // Execute stages sequentially, steps within stages in parallel
    totalStart := time.Now()
    
    for stageIndex, stage := range plan.Stages {
        stageStart := time.Now()
        
        if err := pe.executeStage(ctx, stage, req.Archive, execCtx, collector); err != nil {
            return pe.buildFailureResponse(pipelineID, collector, err)
        }
        
        pe.metrics.RecordStageCompletion(stageIndex, time.Since(stageStart))
        
        // Check for early termination
        if req.Options.FailFast && collector.HasFailures() {
            return pe.buildResponse(pipelineID, collector, "failed", time.Since(totalStart))
        }
    }
    
    // Build final response
    status := "success"
    if collector.HasFailures() {
        status = "failed"
    }
    
    return pe.buildResponse(pipelineID, collector, status, time.Since(totalStart))
}

// executeStage runs all steps in a stage concurrently
func (pe *ParallelExecutor) executeStage(ctx context.Context, stage ExecutionStage, archive []byte, execCtx *ExecutionContext, collector *ResultCollector) error {
    if len(stage.Steps) == 1 {
        // Single step - execute directly
        return pe.executeStep(ctx, stage.Steps[0], archive, execCtx, collector)
    }
    
    // Multiple steps - execute in parallel
    var wg sync.WaitGroup
    errChan := make(chan error, len(stage.Steps))
    
    for _, step := range stage.Steps {
        wg.Add(1)
        
        go func(s ParallelStep) {
            defer wg.Done()
            
            // Acquire semaphore slot
            pe.semaphore <- struct{}{}
            defer func() { <-pe.semaphore }()
            
            if err := pe.executeStep(ctx, s, archive, execCtx, collector); err != nil {
                errChan <- fmt.Errorf("step %s failed: %w", s.ID, err)
            }
        }(step)
    }
    
    // Wait for all steps to complete
    wg.Wait()
    close(errChan)
    
    // Check for errors
    for err := range errChan {
        return err
    }
    
    return nil
}

// executeStep runs a single pipeline step
func (pe *ParallelExecutor) executeStep(ctx context.Context, step ParallelStep, archive []byte, execCtx *ExecutionContext, collector *ResultCollector) error {
    stepStart := time.Now()
    
    // Apply step-specific timeout
    stepTimeout := execCtx.Timeout
    if step.Timeout != "" {
        if parsed, err := time.ParseDuration(step.Timeout); err == nil {
            stepTimeout = parsed
        }
    }
    
    stepCtx, cancel := context.WithTimeout(ctx, stepTimeout)
    defer cancel()
    
    // Execute step with retries
    var result *ServiceResponse
    var err error
    
    for attempt := 0; attempt <= step.Retries; attempt++ {
        result, err = pe.client.CallService(stepCtx, step.Service, archive, execCtx.Headers)
        
        if err == nil && result.Status == "success" {
            break
        }
        
        if attempt < step.Retries {
            // Wait before retry (exponential backoff)
            backoff := time.Duration(attempt+1) * time.Second
            time.Sleep(backoff)
        }
    }
    
    // Create step result
    stepResult := StepResult{
        Service:  step.Service,
        Status:   "failed",
        Duration: time.Since(stepStart).String(),
        Error:    "",
    }
    
    if err != nil {
        stepResult.Error = err.Error()
    } else {
        stepResult.Status = result.Status
        stepResult.Result = result.Result
        stepResult.Error = result.Error
    }
    
    // Collect result
    collector.AddResult(step.ID, stepResult)
    pe.metrics.RecordStepCompletion(step.ID, step.Service, stepResult.Status, time.Since(stepStart))
    
    return nil
}
```

### 2. Service Health Checking

#### 2.1 Health Checker Implementation

```go
// internal/pipeline/health.go
package pipeline

import (
    "context"
    "net/http"
    "sync"
    "time"
)

// HealthChecker monitors service availability
type HealthChecker struct {
    client      *http.Client
    healthCache sync.Map
    checkInterval time.Duration
    unhealthyThreshold int
    recoveryThreshold  int
}

// ServiceHealth represents service health status
type ServiceHealth struct {
    URL           string    `json:"url"`
    Status        string    `json:"status"` // "healthy", "unhealthy", "unknown"
    LastChecked   time.Time `json:"last_checked"`
    FailureCount  int       `json:"failure_count"`
    ResponseTime  time.Duration `json:"response_time"`
    ErrorMessage  string    `json:"error_message,omitempty"`
}

// NewHealthChecker creates a health checker
func NewHealthChecker(checkInterval time.Duration) *HealthChecker {
    hc := &HealthChecker{
        client: &http.Client{
            Timeout: 5 * time.Second,
        },
        checkInterval: checkInterval,
        unhealthyThreshold: 3,
        recoveryThreshold:  2,
    }
    
    // Start background health checking
    go hc.backgroundHealthCheck()
    
    return hc
}

// CheckHealth checks if a service is healthy
func (hc *HealthChecker) CheckHealth(ctx context.Context, serviceURL string) *ServiceHealth {
    start := time.Now()
    
    // Check cache first
    if cached, ok := hc.healthCache.Load(serviceURL); ok {
        health := cached.(*ServiceHealth)
        if time.Since(health.LastChecked) < hc.checkInterval {
            return health
        }
    }
    
    // Perform health check
    health := &ServiceHealth{
        URL:         serviceURL,
        Status:      "unknown",
        LastChecked: time.Now(),
    }
    
    req, err := http.NewRequestWithContext(ctx, "GET", serviceURL+"/health", nil)
    if err != nil {
        health.Status = "unhealthy"
        health.ErrorMessage = err.Error()
        hc.healthCache.Store(serviceURL, health)
        return health
    }
    
    resp, err := hc.client.Do(req)
    health.ResponseTime = time.Since(start)
    
    if err != nil {
        health.Status = "unhealthy"
        health.ErrorMessage = err.Error()
        hc.incrementFailureCount(serviceURL)
    } else {
        resp.Body.Close()
        if resp.StatusCode == 200 {
            health.Status = "healthy"
            hc.resetFailureCount(serviceURL)
        } else {
            health.Status = "unhealthy"
            health.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
            hc.incrementFailureCount(serviceURL)
        }
    }
    
    hc.healthCache.Store(serviceURL, health)
    return health
}

// IsHealthy returns true if service is healthy
func (hc *HealthChecker) IsHealthy(serviceURL string) bool {
    health := hc.CheckHealth(context.Background(), serviceURL)
    return health.Status == "healthy"
}
```

### 3. Basic Load Balancing

#### 3.1 Load Balancer Implementation

```go
// internal/pipeline/load_balancer.go
package pipeline

import (
    "fmt"
    "sync"
    "sync/atomic"
)

// LoadBalancer manages service instance selection
type LoadBalancer struct {
    strategy    string
    healthChecker *HealthChecker
    roundRobinCounters sync.Map
}

// ServiceInstance represents a service endpoint
type ServiceInstance struct {
    URL      string `json:"url"`
    Weight   int    `json:"weight"`
    Healthy  bool   `json:"healthy"`
}

// ServicePool contains multiple instances of a service
type ServicePool struct {
    Service   string            `json:"service"`
    Instances []ServiceInstance `json:"instances"`
    Strategy  string           `json:"strategy"`
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(strategy string, healthChecker *HealthChecker) *LoadBalancer {
    return &LoadBalancer{
        strategy:      strategy,
        healthChecker: healthChecker,
    }
}

// SelectInstance chooses the best instance for a service
func (lb *LoadBalancer) SelectInstance(pool *ServicePool) (*ServiceInstance, error) {
    // Filter healthy instances
    healthyInstances := []ServiceInstance{}
    for _, instance := range pool.Instances {
        if lb.healthChecker.IsHealthy(instance.URL) {
            healthyInstances = append(healthyInstances, instance)
        }
    }
    
    if len(healthyInstances) == 0 {
        return nil, fmt.Errorf("no healthy instances available for service %s", pool.Service)
    }
    
    // Apply load balancing strategy
    switch lb.strategy {
    case "round_robin":
        return lb.roundRobinSelect(pool.Service, healthyInstances), nil
    case "weighted":
        return lb.weightedSelect(healthyInstances), nil
    case "least_connections":
        return lb.leastConnectionsSelect(healthyInstances), nil
    default:
        // Default to round robin
        return lb.roundRobinSelect(pool.Service, healthyInstances), nil
    }
}

// roundRobinSelect implements round-robin selection
func (lb *LoadBalancer) roundRobinSelect(service string, instances []ServiceInstance) *ServiceInstance {
    counter, _ := lb.roundRobinCounters.LoadOrStore(service, &atomic.Uint64{})
    count := counter.(*atomic.Uint64)
    
    index := atomic.AddUint64(count, 1) % uint64(len(instances))
    return &instances[index]
}

// weightedSelect implements weighted selection
func (lb *LoadBalancer) weightedSelect(instances []ServiceInstance) *ServiceInstance {
    totalWeight := 0
    for _, instance := range instances {
        totalWeight += instance.Weight
    }
    
    if totalWeight == 0 {
        return &instances[0]
    }
    
    // Simple weighted selection (can be improved with better algorithms)
    target := int(time.Now().UnixNano()) % totalWeight
    current := 0
    
    for _, instance := range instances {
        current += instance.Weight
        if current > target {
            return &instance
        }
    }
    
    return &instances[0]
}
```

### 4. Configuration Extensions

#### 4.1 Enhanced Pipeline Configuration

```yaml
# config.yaml - Pipeline orchestration section
pipeline:
  enabled: true
  
  # Basic settings
  pass_through_headers: ["X-Client-ID", "X-Request-ID", "Authorization"]
  default_timeout: "60s"
  max_steps: 10
  allow_loopback: true
  
  # Orchestration settings
  orchestration:
    max_concurrent_steps: 5
    execution_strategy: "parallel"  # "parallel", "sequential", "mixed"
    retry_strategy: "exponential_backoff"
    default_retries: 2
    
  # Health checking
  health_check:
    enabled: true
    interval: "30s"
    timeout: "5s"
    unhealthy_threshold: 3
    recovery_threshold: 2
    
  # Load balancing
  load_balancing:
    strategy: "round_robin"  # "round_robin", "weighted", "least_connections"
    
  # Service pools (for load balancing)
  service_pools:
    "pylint.chttp.ployd.app":
      instances:
        - url: "https://pylint-1.chttp.ployd.app"
          weight: 1
        - url: "https://pylint-2.chttp.ployd.app"
          weight: 1
    "bandit.chttp.ployd.app":
      instances:
        - url: "https://bandit-1.chttp.ployd.app"
          weight: 2
        - url: "https://bandit-2.chttp.ployd.app"
          weight: 1
```

### 5. Testing Strategy

#### 5.1 Parallel Execution Tests

```go
// tests/integration/orchestration_test.go
package integration

import (
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestOrchestration_ParallelExecution(t *testing.T) {
    framework := NewIntegrationFramework()
    defer framework.Cleanup()
    
    server, err := framework.CreateTestServer("basic")
    require.NoError(t, err)
    
    err = framework.StartServer(server)
    require.NoError(t, err)
    
    ready, err := framework.WaitForServerReady(server, 10)
    require.NoError(t, err)
    require.True(t, ready)
    
    client, err := framework.CreateHTTPClient("", "")
    require.NoError(t, err)
    
    // Create parallel pipeline request
    pipelineReq := OrchestrationRequest{
        Steps: []ParallelStep{
            {
                ID:            "step1",
                Service:       "http://localhost:8080",
                ParallelGroup: "analysis",
            },
            {
                ID:            "step2", 
                Service:       "http://localhost:8080",
                ParallelGroup: "analysis",
            },
            {
                ID:        "formatter",
                Service:   "http://localhost:8080",
                DependsOn: []string{"step1", "step2"},
            },
        },
        Archive: fixture.Data,
        Options: OrchestrationOptions{
            MaxConcurrent:     3,
            ExecutionStrategy: "parallel",
            FailFast:         false,
        },
    }
    
    start := time.Now()
    
    reqBody, err := json.Marshal(pipelineReq)
    require.NoError(t, err)
    
    resp, err := client.Post("/pipeline", "application/json", bytes.NewReader(reqBody))
    require.NoError(t, err)
    defer resp.Body.Close()
    
    duration := time.Since(start)
    
    assert.Equal(t, 200, resp.StatusCode)
    
    var result PipelineResponse
    err = json.NewDecoder(resp.Body).Decode(&result)
    require.NoError(t, err)
    
    // Verify parallel execution was faster than sequential
    // (step1 + step2) can run in parallel, so total time should be roughly
    // max(step1_time, step2_time) + formatter_time
    assert.Less(t, duration, 8*time.Second, "Parallel execution should be faster")
    
    assert.Equal(t, "success", result.Status)
    assert.Len(t, result.Steps, 3)
    
    // All steps should be successful
    for _, step := range result.Steps {
        assert.Equal(t, "success", step.Status)
    }
}

func TestOrchestration_DependencyOrdering(t *testing.T) {
    // Test that dependencies are respected in execution order
    framework := NewIntegrationFramework()
    defer framework.Cleanup()
    
    server, err := framework.CreateTestServer("basic")
    require.NoError(t, err)
    
    err = framework.StartServer(server)
    require.NoError(t, err)
    
    // Create request with complex dependencies
    pipelineReq := OrchestrationRequest{
        Steps: []ParallelStep{
            {ID: "A", Service: "http://localhost:8080"},
            {ID: "B", Service: "http://localhost:8080", DependsOn: []string{"A"}},
            {ID: "C", Service: "http://localhost:8080", DependsOn: []string{"A"}}, 
            {ID: "D", Service: "http://localhost:8080", DependsOn: []string{"B", "C"}},
        },
        Options: OrchestrationOptions{
            ExecutionStrategy: "parallel",
        },
    }
    
    // Execute and verify dependency ordering
    // Expected execution: A -> (B,C in parallel) -> D
    
    reqBody, err := json.Marshal(pipelineReq)
    require.NoError(t, err)
    
    client, err := framework.CreateHTTPClient("", "")
    require.NoError(t, err)
    
    resp, err := client.Post("/pipeline", "application/json", bytes.NewReader(reqBody))
    require.NoError(t, err)
    defer resp.Body.Close()
    
    assert.Equal(t, 200, resp.StatusCode)
    
    var result PipelineResponse
    err = json.NewDecoder(resp.Body).Decode(&result)
    require.NoError(t, err)
    
    assert.Equal(t, "success", result.Status)
    assert.Len(t, result.Steps, 4)
}
```

## Success Criteria

- ✅ Pipeline steps execute in parallel when no dependencies exist
- ✅ Dependency ordering is strictly enforced
- ✅ Concurrency limits are respected (configurable max parallel steps)
- ✅ Service health checking prevents requests to unhealthy instances
- ✅ Basic load balancing distributes requests across healthy instances
- ✅ Results are properly aggregated from parallel execution
- ✅ Comprehensive test coverage for orchestration scenarios
- ✅ Configuration supports all orchestration features
- ✅ Performance improvement over sequential execution is measurable

## Performance Targets

- **Parallel Speedup**: 2-3x improvement over sequential execution for independent steps
- **Dependency Latency**: < 100ms overhead for dependency resolution
- **Health Check**: < 5s response time for health status
- **Load Balancing**: < 1ms overhead for instance selection
- **Resource Utilization**: 90%+ CPU utilization during parallel execution

## Migration Notes

This phase maintains backward compatibility with existing sequential pipeline requests while adding parallel execution capabilities. Services using the existing `/pipeline` endpoint will continue to work unchanged, with parallel features available through extended request formats.

## Next Phase

After completing Phase 3, proceed to [Phase 4: Service Discovery & Load Balancing](./phase-4-discovery-balancing.md) to add dynamic service discovery and advanced load balancing strategies.