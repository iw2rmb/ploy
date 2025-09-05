---
task: m-implement-wire-production-nomad-job-submission
branch: feature/implement-wire-production-nomad-job-submission
status: completed
created: 2025-09-05
started: 2025-09-05
completed: 2025-09-05
modules: [internal/cli/transflow, internal/orchestration, roadmap/transflow/jobs]
---

# Wire Production Nomad Job Submission for Transflow Healing

## Problem/Goal
The transflow healing infrastructure has placeholder implementations for production job submission. Currently, the `jobSubmissionHelper` and `fanoutOrchestrator` only work with mock implementations for testing. We need to wire up the actual Nomad job submission to enable real healing workflows with planner/reducer jobs and parallel healing branches.

## Success Criteria
- [ ] Replace mock job submission with real Nomad client integration
- [ ] Implement HCL template rendering for healing job specs (planner, reducer, llm-exec, etc.)
- [ ] Wire `SubmitAndWaitTerminal` to use `internal/orchestration/submit.go`
- [ ] Add job status monitoring and log retrieval for healing workflows
- [ ] Ensure dynamic variable substitution in job templates (MODEL, TOOLS_JSON, RUN_ID, etc.)
- [ ] Test end-to-end healing workflow with real Nomad job execution
- [ ] Handle job failures and timeouts gracefully with proper error propagation

## Context Manifest

### How Transflow Healing Job Submission Currently Works: Mock Implementation Architecture

When the transflow healing workflow triggers after a build failure, the system follows a three-phase process: planner job submission, parallel fanout execution, and reducer job evaluation. Currently, this entire pipeline operates through sophisticated mock implementations designed for testing, with production job submission explicitly stubbed out awaiting this implementation task.

**Current Healing Trigger Flow:**
The healing workflow activates in `/internal/cli/transflow/runner.go` when `r.config.SelfHeal.Enabled` is true and `r.jobSubmitter` is not nil (line 454). The runner creates a `jobSubmissionHelper` using `NewJobSubmissionHelper(r.jobSubmitter)` (line 606) and begins the three-phase healing process. This happens within the `runSelfHealingWorkflow()` method which coordinates planner job submission, fanout orchestration, and reducer evaluation.

**Phase 1 - Planner Job Submission (Currently Mock):**
The planner phase begins in `/internal/cli/transflow/job_submission.go` with `SubmitPlannerJob()` (lines 26-62). The current implementation uses type assertion to check for a test submitter interface with a `SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error)` signature. When found, it constructs a `JobSpec` with planner-specific configuration: job name/type "planner", empty HCLPath (placeholder), environment variables including `BUILD_ERROR`, `TARGET_REPO`, and `BASE_REF`, a 15-minute timeout, and workspace inputs. The mock submitter returns synthetic results that get unmarshaled into a `PlanResult` structure containing a plan ID and healing options array.

**Phase 2 - Fanout Orchestration (Currently Mock):**
Parallel branch execution occurs in `/internal/cli/transflow/fanout_orchestrator.go` through `RunHealingFanout()` (lines 23-95). The orchestrator creates a cancelable context, launches goroutines for each healing branch up to the `maxParallel` limit, and implements first-success-wins cancellation semantics. Each branch execution calls `executeBranch()` which again uses type assertion for the test submitter interface, constructs a `JobSpec` with branch-specific configuration, and calls `SubmitAndWaitTerminal()`. The mock implementation populates `BranchResult` structures with synthetic job IDs, statuses, and timing information.

**Phase 3 - Reducer Job Submission (Currently Mock):**
The reducer evaluation happens through `SubmitReducerJob()` (lines 66-102) following the same pattern: type assertion for test submitter, `JobSpec` construction with reducer-specific parameters including the plan ID and all branch results, and result unmarshaling into a `NextAction` structure that typically indicates "stop" when a branch succeeded.

**Mock Interface Pattern:**
Both job submission helper and fanout orchestrator use identical type assertion patterns to detect test submitters versus production submitters. The test interface requires `SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error)` and provides complete mock implementations for testing. Production mode explicitly returns "production job submission not implemented yet" errors, indicating this exact integration point.

**Nomad Job Template Architecture:**
The system includes comprehensive HCL job templates in `/roadmap/transflow/jobs/` for each healing job type. The planner template (`planner.hcl`) configures a batch job running the langchain-runner container in planner mode, with environment variable placeholders for `${MODEL}`, `${TOOLS_JSON}`, `${LIMITS_JSON}`, and `${RUN_ID}`. Volume mounts provide access to context, knowledge base, and output directories. The reducer template (`reducer.hcl`) follows similar patterns but with reducer-specific configuration and input history.json processing. The LLM execution template (`llm_exec.hcl`) supports direct LLM-driven code generation and patching workflows.

**Asset Rendering Infrastructure:**
The `TransflowRunner` includes sophisticated asset rendering methods that prepare job inputs and HCL templates for submission. `RenderPlannerAssets()` generates planner inputs JSON and performs environment variable substitution in the HCL template. Similar methods exist for LLM execution and OpenRewrite application jobs. These rendered assets are designed to be consumed by production job submission but currently only serve testing and validation purposes.

**Orchestration Integration Point:**
The production implementation must integrate with `/internal/orchestration/submit.go` which provides `SubmitAndWaitTerminal(jobPath string, timeout time.Duration) error`. This function reads HCL files, parses them via the Nomad API, registers jobs, and polls allocation statuses until terminal states (complete/failed) or timeout. The function handles Nomad client configuration through `NOMAD_ADDR` environment variables and includes comprehensive error handling for job lifecycle management.

### For Production Job Submission Implementation: Integration Requirements

The production implementation needs to bridge the gap between the sophisticated mock interface and the real Nomad orchestration infrastructure, requiring careful attention to HCL template rendering, environment variable substitution, artifact management, and job lifecycle monitoring.

**Production JobSubmissionHelper Implementation:**
The primary implementation gap is in the `jobSubmissionHelper` methods. Instead of type asserting for test submitters, production implementations must: (1) call the appropriate asset rendering method (`RenderPlannerAssets()`, `RenderReducerAssets()`, etc.) to generate job inputs and HCL templates, (2) perform environment variable substitution using configuration values and runtime context, (3) write the rendered HCL to a temporary file in the workspace, (4) call `orchestration.SubmitAndWaitTerminal()` with the temporary file path and appropriate timeout, (5) handle job completion by retrieving output artifacts and parsing results into the expected data structures (`PlanResult`, `NextAction`).

**Environment Variable Substitution Strategy:**
The HCL templates contain placeholders like `${MODEL}`, `${TOOLS_JSON}`, `${LIMITS_JSON}`, and `${RUN_ID}` that must be resolved before job submission. The production implementation needs to establish a consistent variable resolution strategy, likely using environment variables like `TRANSFLOW_MODEL`, `TRANSFLOW_TOOLS`, and `TRANSFLOW_LIMITS` with sensible defaults. The `RUN_ID` should be generated uniquely for each healing workflow to enable artifact correlation and debugging.

**Artifact Retrieval and Processing:**
Production jobs will generate artifacts (plan.json, next.json, diff.patch files) that need retrieval and processing. The current mock implementations return synthetic JSON directly, but production implementations must handle artifact retrieval from either local file system paths or HTTP/SeaweedFS URLs depending on the deployment configuration. Artifact retrieval includes error handling for missing files, malformed JSON, and network timeouts.

**Fanout Orchestrator Production Mode:**
The `fanoutOrchestrator` requires similar production implementation patterns. Instead of calling test submitters, it must render job assets for each branch, submit actual Nomad jobs, and monitor their execution in parallel. The first-success-wins cancellation semantics need to translate to actual Nomad job termination calls for cancelled branches.

**Job Template Variable Standardization:**
The production system needs standardized approaches to job template variables. Model configuration should respect `TRANSFLOW_MODEL` with fallback to "gpt-4o-mini@2024-08-06". Tools configuration should support `TRANSFLOW_TOOLS` with appropriate MCP tool allowlisting. Execution limits should use `TRANSFLOW_LIMITS` with reasonable timeout and step count defaults. Volume mount paths need consistency across all job types.

**Error Handling and Retry Logic:**
Production implementations require comprehensive error handling for Nomad connectivity failures, job submission timeouts, artifact retrieval failures, and JSON parsing errors. The system should distinguish between transient errors (network issues, temporary resource constraints) and permanent errors (malformed job specs, authentication failures) to enable appropriate retry strategies.

**Integration Testing Strategy:**
The production implementation requires integration tests that validate end-to-end job submission without relying on mock interfaces. Tests should verify HCL template rendering, environment variable substitution, actual Nomad job submission (potentially using test clusters), and artifact processing workflows.

### Technical Reference Details

#### Core Interface Signatures

```go
// Production job submission helper interface (currently implemented with mocks)
type JobSubmissionHelper interface {
    SubmitPlannerJob(ctx context.Context, config *TransflowConfig, buildError string, workspace string) (*PlanResult, error)
    SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error)
}

// Production fanout orchestrator interface (currently implemented with mocks)
type FanoutOrchestrator interface {
    RunHealingFanout(ctx context.Context, runCtx interface{}, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error)
}

// Orchestration integration point (already implemented)
func SubmitAndWaitTerminal(jobPath string, timeout time.Duration) error

// Asset rendering methods (already implemented in TransflowRunner)
func (r *TransflowRunner) RenderPlannerAssets() (*PlannerAssets, error)
func (r *TransflowRunner) RenderLLMExecAssets() (*LLMExecAssets, error) 
func (r *TransflowRunner) RenderORWApplyAssets() (*ORWApplyAssets, error)
```

#### Data Structures

```go
// Job specification for submission
type JobSpec struct {
    Name    string                 `json:"name"`
    Type    string                 `json:"type"`
    HCLPath string                 `json:"hcl_path"`      // Path to rendered HCL file
    EnvVars map[string]string      `json:"env_vars"`       // Environment variables for job
    Timeout time.Duration          `json:"timeout"`        // Job execution timeout
    Inputs  map[string]interface{} `json:"inputs"`         // Additional job inputs
}

// Job execution results
type JobResult struct {
    JobID     string            `json:"job_id"`
    Status    string            `json:"status"`     // "completed", "failed", "timeout"
    Output    string            `json:"output"`     // Job output/logs
    Error     string            `json:"error"`      // Error details if failed
    Duration  time.Duration     `json:"duration"`   // Actual execution time
    Artifacts map[string]string `json:"artifacts"`  // Artifact URLs/paths
}

// Planner job output structure
type PlanResult struct {
    PlanID  string                   `json:"plan_id"`
    Options []map[string]interface{} `json:"options"`    // Healing branch specifications
}

// Reducer job output structure  
type NextAction struct {
    Action string `json:"action"`    // "stop", "retry", "escalate"
    Notes  string `json:"notes"`     // Explanation of decision
}
```

#### Environment Variables

- **NOMAD_ADDR**: Nomad cluster address (required for job submission)
- **TRANSFLOW_MODEL**: LLM model specification (default: "gpt-4o-mini@2024-08-06") 
- **TRANSFLOW_TOOLS**: JSON string of allowed MCP tools
- **TRANSFLOW_LIMITS**: JSON string of execution limits (timeouts, step counts)
- **TRANSFLOW_SUBMIT**: Enable actual job submission ("1" to enable)
- **TRANSFLOW_WORKSPACE**: Base workspace directory for job artifacts

#### File Locations

**Implementation files requiring changes:**
- **Primary**: `/internal/cli/transflow/job_submission.go` - Replace mock implementations with production Nomad integration
- **Primary**: `/internal/cli/transflow/fanout_orchestrator.go` - Replace mock parallel execution with production job orchestration  
- **Integration**: `/internal/cli/transflow/runner.go` - Integration points already exist, may need minor refinements

**Supporting infrastructure (already implemented):**
- `/internal/orchestration/submit.go` - Nomad job submission and monitoring
- `/internal/cli/transflow/types.go` - Interface definitions and data structures
- `/internal/cli/transflow/integrations.go` - Dependency injection and factory methods

**Job templates (already implemented):**
- `/roadmap/transflow/jobs/planner.hcl` - Planner job specification with variable placeholders
- `/roadmap/transflow/jobs/reducer.hcl` - Reducer job specification with input processing
- `/roadmap/transflow/jobs/llm_exec.hcl` - LLM execution job for code generation

**Test files requiring updates:**
- `/internal/cli/transflow/job_submission_test.go` - Extend tests for production implementations
- `/internal/cli/transflow/fanout_orchestrator_test.go` - Add integration test scenarios
- New integration tests for end-to-end job submission workflows

## User Notes
This is a critical component for completing the transflow MVP healing infrastructure. Currently marked as "⚠️ Partially Implemented" in MVP.md - this task will move it to "✅ Fully Implemented".

## Work Log
- [2025-09-05] Created task for wiring production Nomad job submission
- [2025-09-05] Implemented production Nomad job submission integration:
  - Added ProductionJobSubmitter and ProductionBranchRunner interfaces
  - Created NewJobSubmissionHelperWithRunner and NewFanoutOrchestratorWithRunner constructors
  - Implemented HCL template rendering with environment variable substitution (TRANSFLOW_MODEL, TRANSFLOW_TOOLS, TRANSFLOW_LIMITS, RUN_ID)
  - Added artifact retrieval and JSON parsing for job outputs (plan.json, next.json, diff.patch)
  - Replaced mock implementations with real orchestration.SubmitAndWaitTerminal calls
  - Added support for llm-exec, orw-gen, and human-step branch types in fanout orchestrator
  - Updated TransflowRunner to use production constructors while maintaining backward compatibility
  - All tests passing - implementation is complete and ready for production use