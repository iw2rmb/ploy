# ARF Transformation Status & Healing Workflow

## Overview

Legacy note: The ARF `/v1/arf/transforms/*` HTTP endpoints referenced in this document have been removed. Transflow is now the unified API for transformation workflows under `/v1/transflow/*`. The concepts below (status, healing, nested attempts) live on via Transflow.

The ARF transformation system provided comprehensive status tracking for code transformations with support for nested healing workflows. When a transformation encountered issues during build/test phases, the system automatically spawned healing attempts that could recursively create additional healing transformations as new issues were discovered. This behavior is now modeled in Transflow’s status and artifacts.

## Transform Route Enhancement Plan

### Critical Issue Analysis

**Current `/v1/arf/transforms` Behavior (Async-Only):**
- Executes transformation asynchronously in background goroutines
- Returns immediate response with `transformation_id` and status URL
- Stores status and progress in Consul KV for persistence across restarts
- HTTP connections return within <1 second, no long-lived connections
- Background execution handles timeouts and error recovery

**Previous Synchronous Problems (Resolved):**
1. ✅ **Long-lived Connections**: Eliminated - responses now return immediately
2. ✅ **Timeout Risk**: Eliminated - no client connection timeouts 
3. ✅ **In-Memory Storage**: Replaced with Consul KV persistence
4. ✅ **No Async Pattern**: Implemented - full async workflow with status tracking

### Required Changes

#### 1. Transform Route Response Pattern
**Change from:** Synchronous execution returning full result  
**Change to:** Asynchronous initiation returning status link

**New Response Format:**
```json
{
  "transformation_id": "uuid-1234-5678",
  "status": "initiated", 
  "status_url": "/v1/arf/transforms/uuid-1234-5678/status",
  "message": "Transformation started, use status_url to monitor progress"
}
```

#### 2. Background Execution Implementation
- Start transformation in background goroutine
- Store initial status in Consul immediately
- Return response with status link within <1 second
- Continue transformation execution asynchronously
- Update Consul status as transformation progresses

#### 3. Consul KV Integration
**Replace:** `globalTransformStore` (in-memory)  
**With:** `ConsulHealingStore` for persistent storage with array-based children structure

### Implementation Details

#### Transform Route Code Changes
**File:** `/api/arf/handler_transformation.go`

**Current Implementation:**
```go
func (h *Handler) ExecuteTransformation(c *fiber.Ctx) error {
    // ... validation and setup
    
    // Execute transformation synchronously 
    result, err := h.executeTransformationInternal(ctx, transformID, &req)
    if err != nil {
        // Handle timeout, return 408 or 500
    }
    
    // Store in memory
    globalTransformStore.store(transformID, result)
    
    // Return complete result
    return c.JSON(result)
}
```

**New Implementation:**
```go
func (h *Handler) ExecuteTransformation(c *fiber.Ctx) error {
    // ... validation and setup
    transformID := uuid.New().String()
    
    // Store initial status in Consul immediately
    initialStatus := &TransformationStatus{
        TransformationID: transformID,
        Status: "initiated",
        WorkflowStage: "openrewrite", 
        StartTime: time.Now(),
    }
    h.consulStore.StoreTransformationStatus(transformID, initialStatus)
    
    // Start background execution
    go h.executeTransformationBackground(transformID, &req)
    
    // Return immediately with status link
    return c.JSON(fiber.Map{
        "transformation_id": transformID,
        "status": "initiated",
        "status_url": fmt.Sprintf("/v1/arf/transforms/%s/status", transformID),
        "message": "Transformation started, use status_url to monitor progress",
    })
}

func (h *Handler) executeTransformationBackground(transformID string, req *TransformRequest) {
    // Execute transformation and update Consul status throughout
    result, err := h.executeTransformationInternal(context.Background(), transformID, req)
    // Update final status in Consul
}
```

#### Implementation Complete ✅
**The `/v1/arf/transforms` endpoint has been converted to async-only:**
- **Current behavior**: Asynchronous execution, returns status link immediately
- **Legacy synchronous code**: Removed in Phase 1 cleanup
- **Status tracking**: Use `/v1/arf/transforms/:id/status` to monitor progress
- **No backward compatibility**: All clients use async pattern

## Current Implementation Status

### Existing Components
- **Basic Status Endpoint**: `/v1/arf/transforms/:id/status` (Note: `/v1/arf/transforms/:id` to be removed, keeping only `/status` variant)
- **In-Memory Storage**: Uses `globalTransformStore` for transformation results
- **Simple Workflow**: OpenRewrite transformation with basic success/failure tracking
- **Limited Status Info**: Only shows completed/in_progress states

### Limitations
- **No Persistence**: Transformation statuses lost on API restart
- **Single Operation Tracking**: No support for nested/child transformations
- **Basic Healing**: No automatic build-deploy-test-heal workflow
- **Limited Status Details**: Minimal JSON response without comprehensive progress info
- **Synchronous Execution**: Long-lived HTTP connections during transformation

## Enhanced Architecture

### Consul KV Storage Schema

The transformation system uses Consul KV for persistent, distributed storage of transformation statuses.

#### Key Structure
```
ploy/arf/transforms/{transform_id}/status          # Current workflow status
ploy/arf/transforms/{transform_id}/children         # Complete children hierarchy
ploy/arf/transforms/{transform_id}/sandbox/{id}    # Sandbox deployment info
```

#### Status Document Format
```json
{
  "transformation_id": "uuid-root-1234",
  "workflow_stage": "healing",
  "status": "in_progress",
  "start_time": "2025-01-15T10:00:00Z",
  "children": [
    {
      "transformation_id": "uuid-heal1-5678",
      "attempt_path": "1",
      "trigger_reason": "build_failure",
      "target_errors": ["compilation_error_line_45"],
      "status": "completed",
      "result": "success_but_new_issues",
      "new_issues_discovered": ["test_failure_integration"],
      "children": [
        {
          "transformation_id": "uuid-heal1-1-9012",
          "attempt_path": "1.1",
          "trigger_reason": "test_failure_after_heal", 
          "target_errors": ["test_failure_integration"],
          "status": "in_progress",
          "children": []
        }
      ]
    },
    {
      "transformation_id": "uuid-heal2-3456",
      "attempt_path": "2",
      "trigger_reason": "build_failure",
      "target_errors": ["missing_import"],
      "status": "completed",
      "result": "success",
      "children": []
    }
  ],
  "active_healing_count": 1,
  "total_healing_attempts": 3
}
```

### Data Structures

#### Enhanced TransformationResult
```go
type TransformationResult struct {
    // Existing fields...
    TransformationID string    `json:"transformation_id,omitempty"`
    RecipeID         string    `json:"recipe_id"`
    Success          bool      `json:"success"`
    ChangesApplied   int       `json:"changes_applied"`
    ExecutionTime    time.Duration `json:"execution_time"`
    
    // Enhanced healing workflow fields
    WorkflowStage     string                    `json:"workflow_stage"`      // openrewrite, build, deploy, test, heal
    ChildTransforms   []string                  `json:"child_transforms"`    // IDs of healing transformations
    ParentTransform   string                    `json:"parent_transform"`    // ID of parent transformation
    Children          []HealingAttempt          `json:"children"`
    SandboxID         string                    `json:"sandbox_id"`          // Sandbox used for testing
    DeploymentStatus  *DeploymentMetrics        `json:"deployment_status"`
    
    // Consul metadata
    ConsulKey         string                    `json:"consul_key"`
    LastUpdated       time.Time                 `json:"last_updated"`
}
```

#### Nested Healing Structure
```go
type HealingAttempt struct {
    TransformationID     string                     `json:"transformation_id"`
    AttemptPath         string                     `json:"attempt_path"`        // "1.1.2" for nested attempts
    TriggerReason       string                     `json:"trigger_reason"`      // build_failure, test_failure, etc.
    TargetErrors        []string                   `json:"target_errors"`       // Specific errors this attempt targets
    LLMAnalysis         *LLMAnalysisResult         `json:"llm_analysis"`
    Status              string                     `json:"status"`              // pending, in_progress, completed, failed
    Result              string                     `json:"result"`              // success, partial_success, failed
    StartTime           time.Time                  `json:"start_time"`
    EndTime             time.Time                  `json:"end_time"`
    
    // Recursive healing support
    NewIssuesDiscovered []string                   `json:"new_issues_discovered"`
    Children            []*HealingAttempt          `json:"children"`
    ParentAttempt       string                     `json:"parent_attempt"`      // "1.1" for parent path
}

type HealingTree struct {
    RootTransformID   string                     `json:"root_transform_id"`
    Attempts          []*HealingAttempt          `json:"attempts"`            // Array of attempts
    ActiveAttempts    []string                   `json:"active_attempts"`     // Currently running
    TotalAttempts     int                        `json:"total_attempts"`
    SuccessfulHeals   int                        `json:"successful_heals"`
    FailedHeals       int                        `json:"failed_heals"`
    MaxDepth          int                        `json:"max_depth"`
}

type LLMAnalysisResult struct {
    ErrorType        string   `json:"error_type"`
    Confidence       float64  `json:"confidence"`
    SuggestedFix     string   `json:"suggested_fix"`
    AlternativeFixes []string `json:"alternative_fixes"`
    RiskAssessment   string   `json:"risk_assessment"`
}
```

### Healing Workflow Process

#### 1. Initial Transformation
```
POST /v1/arf/transform
{
  "recipe_id": "upgrade-java-17",
  "type": "openrewrite", 
  "codebase": {
    "repository": "https://github.com/example/project",
    "branch": "main"
  }
}
```

Response includes `transformation_id` for status tracking.

#### 2. Workflow Stages

1. **OpenRewrite Stage** (`workflow_stage: "openrewrite"`)
   - Execute OpenRewrite transformation
   - Capture code changes and diffs
   - Update Consul: transformation in progress

2. **Build Stage** (`workflow_stage: "build"`)
   - Deploy transformed code to sandbox
   - Attempt compilation/build
   - If build fails → trigger healing workflow

3. **Test Stage** (`workflow_stage: "test"`)
   - Run test suites in sandbox
   - If tests fail → trigger healing workflow

4. **Healing Stage** (`workflow_stage: "heal"`)
   - Analyze build/test failures with LLM
   - Generate multiple healing transformation candidates
   - Execute healing attempts in parallel
   - For each healing attempt:
     - Apply suggested fixes
     - Re-run build/test
     - If new issues discovered → spawn child healing attempts

#### 3. Recursive Healing Logic

```go
func (h *Handler) executeHealingWorkflow(transformID string, errors []string, parentPath string) {
    // Generate attempt path (1, 1.1, 1.1.1, etc.)
    attemptPath := h.generateAttemptPath(transformID, parentPath)
    
    // Analyze errors with LLM
    analysis := h.analyzeBuildErrors(errors)
    
    // Create healing attempt
    attempt := &HealingAttempt{
        TransformationID: uuid.New().String(),
        AttemptPath:     attemptPath,
        TriggerReason:   h.determineTriggerReason(errors),
        TargetErrors:    errors,
        LLMAnalysis:     analysis,
        Status:         "in_progress",
        StartTime:      time.Now(),
        Children:       []*HealingAttempt{},
        ParentAttempt:  parentPath,
    }
    
    // Store in Consul
    h.consulStore.AddHealingAttempt(transformID, attemptPath, attempt)
    
    // Execute transformation with suggested fix
    result := h.executeTransformation(attempt.TransformationID, analysis.SuggestedFix)
    
    // Update attempt status
    attempt.Status = "completed"
    attempt.Result = result.Status
    attempt.EndTime = time.Now()
    
    // Check for new issues after healing
    newErrors := h.validateAfterHealing(attempt.TransformationID)
    if len(newErrors) > 0 {
        attempt.NewIssuesDiscovered = newErrors
        
        // Spawn child healing attempts for new issues
        h.executeHealingWorkflow(transformID, newErrors, attemptPath)
    }
    
    // Update Consul with final status
    h.consulStore.UpdateHealingAttempt(transformID, attemptPath, attempt)
}
```

## API Specification

### Enhanced Status Endpoint

#### GET /v1/arf/transforms/{id}/status

Returns comprehensive transformation status with complete healing hierarchy:

```json
{
  "transformation_id": "uuid-root-1234",
  "workflow_stage": "healing",
  "status": "in_progress",
  "start_time": "2025-01-15T10:00:00Z",
  "healing_summary": {
    "total_attempts": 6,
    "active_attempts": 2,
    "successful_heals": 3,
    "failed_heals": 1,
    "max_depth_reached": 3
  },
  "children": [
    {
      "transformation_id": "uuid-heal1-5678",
      "attempt_path": "1",
      "status": "completed",
      "result": "partial_success",
      "trigger_reason": "build_failure",
      "target_errors": ["compilation_error"],
      "new_issues_discovered": ["test_failure"],
      "start_time": "2025-01-15T10:05:00Z",
      "end_time": "2025-01-15T10:08:00Z",
      "llm_analysis": {
        "error_type": "compilation_error",
        "confidence": 0.85,
        "suggested_fix": "Add missing import statements"
      },
      "children": [
        {
          "transformation_id": "uuid-heal1-1-9012",
          "attempt_path": "1.1",
          "status": "in_progress",
          "trigger_reason": "test_failure_after_heal",
          "target_errors": ["test_failure_integration"],
          "start_time": "2025-01-15T10:08:30Z",
          "progress": {
            "stage": "build_validation",
            "percent_complete": 45
          }
        }
      ]
    },
    {
      "transformation_id": "uuid-heal2-3456", 
      "attempt_path": "2",
      "status": "completed",
      "result": "success",
      "trigger_reason": "build_failure",
      "target_errors": ["missing_import"],
      "start_time": "2025-01-15T10:05:00Z",
      "end_time": "2025-01-15T10:07:00Z"
    }
  ],
  "sandbox_info": {
    "primary_sandbox": "sandbox-root-abc123",
    "deployment_url": "https://sandbox-root-abc123.ployd.app",
    "healing_sandboxes": [
      {
        "transformation_id": "uuid-heal1-1-9012",
        "sandbox_id": "sandbox-heal1-1-def456",
        "deployment_url": "https://sandbox-heal1-1-def456.ployd.app",
        "build_status": "in_progress",
        "test_status": "pending"
      }
    ]
  }
}
```


### Status Values

#### Workflow Stages
- `openrewrite` - OpenRewrite transformation in progress
- `build` - Building/compiling transformed code
- `deploy` - Deploying to sandbox environment
- `test` - Running test suites
- `heal` - Healing workflow active with child transformations

#### Status Values
- `pending` - Queued but not started
- `in_progress` - Currently executing
- `completed` - Finished (check result for success/failure)
- `failed` - Failed with no healing attempts
- `timeout` - Exceeded time limits

#### Result Values
- `success` - Completed successfully with no issues
- `partial_success` - Completed but discovered new issues (spawned children)
- `failed` - Failed to resolve target errors

## Consul KV Operations

### ConsulHealingStore Implementation

```go
type ConsulHealingStore struct {
    client    *consulapi.Client
    keyPrefix string
}

// Core operations
func (c *ConsulHealingStore) StoreTransformationStatus(id string, status *TransformationStatus) error
func (c *ConsulHealingStore) GetTransformationStatus(id string) (*TransformationStatus, error)
func (c *ConsulHealingStore) UpdateWorkflowStage(id string, stage string) error

// Healing tree operations
func (c *ConsulHealingStore) AddHealingAttempt(rootID, attemptPath string, attempt *HealingAttempt) error
func (c *ConsulHealingStore) UpdateHealingAttempt(rootID, attemptPath string, attempt *HealingAttempt) error
func (c *ConsulHealingStore) GetHealingTree(rootID string) (*HealingTree, error)
func (c *ConsulHealingStore) GetActiveHealingAttempts(rootID string) ([]string, error)

// Cleanup operations
func (c *ConsulHealingStore) CleanupCompletedTransformations(maxAge time.Duration) error
func (c *ConsulHealingStore) SetTransformationTTL(id string, ttl time.Duration) error
```

### Key Naming Conventions

```
ploy/arf/transforms/{root_id}/status                    # Main transformation status
ploy/arf/transforms/{root_id}/children/{path}           # Individual healing attempts
ploy/arf/transforms/{root_id}/sandbox/{sandbox_id}      # Sandbox deployment info
ploy/arf/transforms/{root_id}/metadata                  # Additional metadata
```

## Configuration & Limits

### Healing Control Parameters

```go
type HealingConfig struct {
    MaxHealingDepth      int           `json:"max_healing_depth"`       // Maximum nesting depth (default: 5)
    MaxParallelAttempts  int           `json:"max_parallel_attempts"`   // Max concurrent healing (default: 3) 
    MaxTotalAttempts     int           `json:"max_total_attempts"`      // Max total attempts per root (default: 20)
    HealingTimeout       time.Duration `json:"healing_timeout"`         // Total healing timeout (default: 2h)
    AttemptTimeout       time.Duration `json:"attempt_timeout"`         // Per-attempt timeout (default: 30m)
    
    // Circuit breaker settings
    FailureThreshold     int           `json:"failure_threshold"`       // Consecutive failures before circuit open
    CircuitOpenDuration  time.Duration `json:"circuit_open_duration"`   // How long circuit stays open
}
```

### Environment Variables

```bash
# Healing workflow configuration
ARF_HEALING_MAX_DEPTH=5                    # Maximum healing depth
ARF_HEALING_MAX_PARALLEL=3                 # Maximum parallel healing attempts
ARF_HEALING_MAX_TOTAL=20                   # Maximum total attempts per transformation
ARF_HEALING_TIMEOUT=2h                     # Total healing workflow timeout
ARF_HEALING_ATTEMPT_TIMEOUT=30m            # Individual attempt timeout

# Consul configuration
ARF_CONSUL_KEY_PREFIX=ploy/arf             # Consul key prefix for ARF data
ARF_CONSUL_TTL=7d                          # TTL for transformation entries
ARF_CONSUL_CLEANUP_INTERVAL=1h             # Cleanup job interval

# LLM integration
ARF_LLM_PROVIDER=openai                    # LLM provider for error analysis
ARF_LLM_MODEL=gpt-4                        # Model to use for healing suggestions
ARF_LLM_MAX_CONTEXT=16k                    # Maximum context window
```

## Implementation Phases

### Phase 1: Transform Route Enhancement & Consul KV Integration (Week 1-2) ✅ COMPLETED
- [x] **RESTful Endpoint Rename**: Rename `/v1/arf/transform` to `/v1/arf/transforms` (plural) to follow RESTful conventions ✅ (Completed 2025-09-02)
  - Update route registration in `api/arf/handler.go`
  - Update handler comment in `api/arf/handler_transformation.go`
  - Update all documentation references (8+ files identified)
  - Update test scripts and test cases
  - Note: This is a breaking change requiring client updates
- [x] **Transform Route Async Conversion**: Modify `/v1/arf/transforms` to return status link instead of waiting
- [x] **Background Execution**: Implement goroutine-based transformation execution
- [x] **Initial Status Storage**: Store transformation initiation status in Consul immediately
- [x] Replace `globalTransformStore` with `ConsulHealingStore`
- [x] Implement basic Consul KV operations for transformation status
- [x] Update existing status endpoints to use Consul
- [x] Add TTL and cleanup mechanisms
- [x] **Breaking Change Implementation**: Complete replacement of synchronous behavior
- [x] **Remove Legacy Synchronous Code**: Remove deprecated synchronous `ExecuteTransformation` method and related code ✅ (Completed 2025-09-02)
  - [x] Remove `api/arf/handler_transformation.go` file containing legacy synchronous implementation
  - [x] Remove synchronous execution logic and timeout handling
  - [x] Update documentation to remove all references to synchronous execution
  - [x] Update tests to remove synchronous execution test cases
  - [x] Clean up any remaining backward compatibility code paths

### Phase 2: Enhanced Data Structures (Week 3-4) ✅ COMPLETED
- [x] Implement nested `HealingAttempt` structure ✅ (Already implemented in consul_types.go)
- [x] Create `HealingTree` management logic ✅ (Already implemented in consul_store.go)
- [x] Extend `TransformationResult` with healing workflow fields ✅ (Completed 2025-09-02)
- [x] Add attempt path generation and management ✅ (Completed 2025-09-02)

### Phase 3: Healing Workflow Logic (Week 5-7) ✅ COMPLETED
- [x] Implement recursive healing workflow execution ✅ (Completed 2025-09-02)
- [x] Add build/test validation in sandbox environments ✅ (Completed 2025-09-02)
- [x] Integrate LLM error analysis for healing suggestions ✅ (Completed 2025-09-02)
- [x] Create parallel healing attempt coordination ✅ (Completed 2025-09-02)

### Phase 4: Enhanced API Response (Week 8-9) ✅ COMPLETED
- [x] Update status endpoint with comprehensive healing tree response ✅ (Completed 2025-09-02)
- [x] Add active attempt tracking and progress reporting ✅ (Completed 2025-09-02)
- [x] Implement healing summary statistics ✅ (Completed 2025-09-02)
- [x] Add sandbox deployment status integration ✅ (Completed 2025-09-02)

### Phase 5: Controls & Optimization (Week 10-11)
- [x] Implement healing depth limits and parallel attempt controls ✅ (Completed 2025-09-02)
- [x] Add healing timeout and circuit breaker logic ✅ (Completed 2025-09-02)
- [x] Create performance metrics for healing success rates ✅ (Completed 2025-09-02)
- [x] Add cost optimization for LLM usage ✅ (Completed 2025-09-02)

### Phase 6: Monitoring & Observability (Week 12)
- [x] Add comprehensive logging for healing workflows ✅ (Completed 2025-09-02)
- [x] Create Prometheus metrics for healing performance ✅ (Completed 2025-09-02)
- [x] Implement alerting for failed healing chains ✅ (Completed 2025-09-02)
- [x] Add debugging endpoints for transformation hierarchies ✅ (Completed 2025-09-02)

## Testing Strategy ✅ COMPLETED

### Unit Tests ✅
- [x] Consul KV store operations and error handling ✅ (Completed 2025-09-02)
- [x] Healing hierarchy construction and navigation ✅ (Completed 2025-09-02)
- [x] Attempt path generation and validation ✅ (Completed 2025-09-02)
- [x] LLM analysis result processing ✅ (Completed 2025-09-02)

### Integration Tests ✅
- [x] End-to-end transformation with healing workflow ✅ (Completed 2025-09-02)
- [x] Consul KV persistence across API restarts ✅ (Completed 2025-09-02)
- [x] Parallel healing attempt coordination ✅ (Completed 2025-09-02)
- [x] Sandbox deployment and validation ✅ (Completed 2025-09-02)

### Load Tests ✅
- [x] Multiple concurrent transformations with healing ✅ (Completed 2025-09-02)
- [x] Consul KV performance under load ✅ (Completed 2025-09-02)
- [x] LLM API rate limiting and cost optimization ✅ (Completed 2025-09-02)
- [x] Memory usage with deep healing hierarchies ✅ (Completed 2025-09-02)

## Monitoring & Alerting ✅ COMPLETED

### Key Metrics ✅
- [x] `arf_transformations_total` - Total transformations started ✅ (Completed 2025-09-02)
- [x] `arf_healing_attempts_total` - Total healing attempts by result ✅ (Completed 2025-09-02)
- [x] `arf_children_tree_depth` - Histogram of children tree depths ✅ (Completed 2025-09-02)
- [x] `arf_healing_duration_seconds` - Duration of healing workflows ✅ (Completed 2025-09-02)
- [x] `arf_consul_operations_total` - Consul KV operation metrics ✅ (Completed 2025-09-02)
- [x] `arf_llm_api_calls_total` - LLM API usage and costs ✅ (Completed 2025-09-02)

### Alert Conditions ✅
- [x] High healing failure rate (> 80% in 1h) ✅ (Completed 2025-09-02)
- [x] Deep healing hierarchies (depth > 8) indicating complex issues ✅ (Completed 2025-09-02)
- [x] Consul KV errors affecting transformation persistence ✅ (Completed 2025-09-02)
- [x] LLM API rate limits or excessive costs ✅ (Completed 2025-09-02)
- [x] Long-running transformations (> 4h) potentially stuck ✅ (Completed 2025-09-02)

## Security Considerations

### Data Protection
- Transformation results may contain sensitive code/configuration
- Consul KV data encryption at rest and in transit
- Access controls for transformation status endpoints
- Audit logging for healing workflow actions

### Resource Limits ✅ COMPLETED
- [x] Prevent resource exhaustion from runaway healing workflows ✅ (Completed 2025-09-02)
  - Implemented via `MaxHealingDepth`, `MaxParallelAttempts`, `MaxTotalAttempts` configuration
  - Circuit breaker logic with `CircuitBreakerState` monitoring
- [x] Sandbox isolation for healing transformations ✅ (Completed 2025-09-02)
  - `DeploymentSandboxManager` with full lifecycle management
  - Automatic `CleanupExpiredSandboxes` functionality
- [x] Rate limiting on LLM API usage ✅ (Completed 2025-09-02)
  - `LLMCostTracker` with per-transform budget controls
  - Cost metrics tracking via `llm_cost_dollars` metric
- [x] Consul KV storage quotas and cleanup ✅ (Completed 2025-09-02)
  - `SetTransformationTTL` and `CleanupCompletedTransformations` methods
  - Configurable TTL and automatic cleanup intervals

## Future Enhancements

### Advanced Healing Strategies
- Machine learning-based error classification and fix suggestion
- Historical analysis of successful healing patterns
- A/B testing of multiple healing approaches
- Integration with static analysis tools for proactive issue detection

### Workflow Optimization
- Caching of successful healing transformations
- Parallel execution of independent healing branches
- Incremental healing that preserves successful changes
- Integration with CI/CD pipelines for automated healing

### Observability Improvements
- Real-time transformation progress visualization
- Healing workflow dependency graphs  
- Cost analysis and optimization recommendations
- Integration with existing monitoring and logging systems
