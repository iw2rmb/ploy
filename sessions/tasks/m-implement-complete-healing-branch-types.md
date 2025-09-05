---
task: m-implement-complete-healing-branch-types
branch: feature/implement-complete-healing-branch-types
status: pending
created: 2025-09-05
modules: [internal/cli/transflow, roadmap/transflow, tests]
---

# Complete Healing Branch Types Implementation

## Problem/Goal
The transflow healing infrastructure has production Nomad job submission working, but the healing branch types need completion. Currently, human-step branches are just placeholders, and comprehensive testing coverage is needed for all branch types (llm-exec, orw-gen, human-step). Additionally, the roadmap needs to be updated to reflect completed tasks and follow TDD principles from @AGENTS.md.

## Success Criteria
- [ ] Implement complete human-step branch handler with Git-based manual intervention workflow
- [ ] Ensure llm-exec branch type is fully functional with MCP tool integration
- [ ] Verify orw-gen branch type works with OpenRewrite recipe generation and application
- [ ] Add comprehensive test coverage for all three healing branch types
- [ ] Update roadmap/transflow/MVP.md to mark completed tasks and current status
- [ ] Follow TDD framework from @AGENTS.md: RED (failing tests) → GREEN (minimal code) → REFACTOR (VPS testing)
- [ ] Deploy to VPS for integration testing following mandatory update protocol
- [ ] Update FEATURES.md and CHANGELOG.md as needed

## Context Manifest

### How This Currently Works: Transflow Self-Healing Architecture

The transflow healing infrastructure is a sophisticated multi-phase workflow system that automatically recovers from build failures using parallel healing strategies. When a transflow execution encounters a build failure, it triggers a three-phase healing process: **planner → fanout → reducer**.

#### Phase 1: Planner Job Analysis

When a build fails during the normal transflow workflow (clone → branch → recipes → commit → **build failure**), the `attemptHealing` method in `runner.go` is called with the build error message. The healing process begins by creating a `TransflowHealingSummary` to track the entire healing attempt.

The system first submits a **planner job** via `JobSubmissionHelper.SubmitPlannerJob()`. This planner is a Nomad batch job that analyzes the build error context and generates healing options. The planner job runs in a Docker container with the LangGraph planner model, receiving:
- Build error details and context
- Repository metadata and state
- Knowledge base snapshots (future enhancement)

The planner writes a `plan.json` artifact containing an array of healing options. Each option specifies a different healing strategy with its own type and confidence score.

#### Phase 2: Fanout Orchestration (Parallel Branch Execution)

Once the planner completes, the system converts the planner options into `BranchSpec` structures and submits them to the `FanoutOrchestrator`. This orchestrator implements a **first-success-wins** parallel execution model with configurable parallelism limits.

The fanout orchestrator launches multiple healing "branches" simultaneously:
- **llm-exec**: LLM-powered code generation that analyzes the error and generates a patch
- **orw-gen**: OpenRewrite recipe generation that suggests and applies automated refactoring recipes  
- **human-step**: Manual intervention workflow that creates Git branches/MRs for human review

Each branch executes as an independent Nomad job with its own resource allocation and timeout. The orchestrator uses Go channels and context cancellation to coordinate the parallel execution. When the first branch succeeds, it cancels all remaining branches to prevent resource waste.

#### Phase 3: Reducer Decision Making

After branch execution completes (either with a winner or all failures), the system submits a **reducer job** via `JobSubmissionHelper.SubmitReducerJob()`. The reducer analyzes all branch results and the winning solution to determine the next action.

The reducer typically returns `{"action": "stop", "notes": "healing succeeded"}` when a branch successfully fixes the build, allowing the main transflow workflow to continue with the healed changes.

#### Branch Type Implementations (Current State)

**llm-exec branches** (`executeLLMExecBranch` in `fanout_orchestrator.go`):
- Renders LLM execution assets using `RenderLLMExecAssets()` 
- Substitutes environment variables in the HCL template (MODEL, TOOLS_JSON, LIMITS_JSON, RUN_ID)
- Submits as Nomad job using `orchestration.SubmitAndWaitTerminal()`
- Expects the job to produce a `diff.patch` artifact in the output directory
- Validates success by checking for the diff.patch file existence

**orw-gen branches** (`executeORWGenBranch`):
- Renders OpenRewrite application assets using `RenderORWApplyAssets()`
- Extracts recipe configuration from branch inputs (class, coords, timeout)
- Performs recipe-specific HCL variable substitution (RECIPE_CLASS, RECIPE_COORDS, RECIPE_TIMEOUT)
- Submits as Nomad job and waits for completion
- Validates success by checking for generated diff.patch artifact

**human-step branches** (`executeHumanStepBranch`):
- Currently returns immediate failure with "not yet implemented" message
- Intended to create Git branches/MRs for manual intervention
- Should poll for manual commits and validate build success after human changes

### For New Feature Implementation: Completing the Missing Pieces

Since we're implementing complete healing branch types with comprehensive test coverage, several critical integration points need completion:

#### Human-Step Branch Implementation Requirements

The current human-step implementation is a placeholder that immediately fails. The complete implementation needs to:

1. **Git Branch Creation**: Create a dedicated Git branch for human intervention, separate from the main workflow branch
2. **Issue/MR Creation**: Use the existing `GitProvider` interface to create GitLab merge requests with detailed error context
3. **Polling Mechanism**: Implement a polling loop that checks for manual commits on the human intervention branch
4. **Build Validation**: After detecting manual changes, re-run the build check to verify the fix
5. **Timeout Handling**: Respect branch-level timeouts and gracefully fail if human intervention doesn't occur

The human-step workflow should integrate with the existing `r.gitProvider` (GitLab integration) and `r.buildChecker` interfaces already available in the `TransflowRunner`.

#### Test Coverage Completion

The existing test infrastructure in `job_submission_test.go` and `self_healing_test.go` uses a `MockJobSubmitter` pattern that simulates job execution. The missing test coverage needs to address:

1. **Branch Type Validation**: Tests for each branch type (llm-exec, orw-gen, human-step) with both success and failure scenarios
2. **Timeout Handling**: Tests that verify proper timeout behavior and context cancellation
3. **Error Recovery**: Tests that validate error handling when branches fail at different stages
4. **Integration Testing**: End-to-end tests that combine planner → fanout → reducer workflows

The test infrastructure already provides `MockJobSubmitter` with configurable `JobResults` mapping and error injection capabilities.

#### Production Job Submission Integration

The healing workflow integrates with the existing Nomad job submission infrastructure through the `orchestration.SubmitAndWaitTerminal()` function. The production flow:

1. **Asset Rendering**: Uses `RenderLLMExecAssets()`, `RenderORWApplyAssets()` to prepare HCL job templates
2. **Variable Substitution**: Performs environment variable substitution in HCL templates using `substituteHCLTemplate()`
3. **Job Submission**: Calls `orchestration.SubmitAndWaitTerminal()` with rendered HCL paths and timeout values
4. **Artifact Collection**: Reads job output artifacts (diff.patch files) from expected output directories
5. **Result Processing**: Parses artifacts and determines branch success/failure

The existing HCL templates in `roadmap/transflow/jobs/` define the Nomad job specifications with placeholder variables that get substituted at runtime.

### Technical Reference Details

#### Component Interfaces & Signatures

```go
// Main healing orchestration entry point
func (r *TransflowRunner) attemptHealing(ctx context.Context, repoPath string, buildError string) (*TransflowHealingSummary, error)

// Job submission interfaces
type JobSubmissionHelper interface {
    SubmitPlannerJob(ctx context.Context, config *TransflowConfig, buildError string, workspace string) (*PlanResult, error)
    SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error)
}

// Fanout orchestration
type FanoutOrchestrator interface {
    RunHealingFanout(ctx context.Context, runCtx interface{}, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error)
}

// Branch execution methods
func (o *fanoutOrchestrator) executeLLMExecBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult
func (o *fanoutOrchestrator) executeORWGenBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult
func (o *fanoutOrchestrator) executeHumanStepBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult
```

#### Data Structures

```go
// Healing summary tracking
type TransflowHealingSummary struct {
    Enabled       bool                       `json:"enabled"`
    AttemptsCount int                        `json:"attempts_count"`
    MaxRetries    int                        `json:"max_retries"`
    Attempts      []*TransflowHealingAttempt `json:"attempts"`
    FinalSuccess  bool                       `json:"final_success"`
    PlanID        string                     `json:"plan_id,omitempty"`
    Winner        *BranchResult              `json:"winner,omitempty"`
    AllResults    []BranchResult             `json:"all_results,omitempty"`
}

// Branch specifications and results
type BranchSpec struct {
    ID     string                 `json:"id"`
    Type   string                 `json:"type"`   // "llm-exec", "orw-gen", "human-step"
    Inputs map[string]interface{} `json:"inputs"`
}

type BranchResult struct {
    ID         string        `json:"id"`
    Status     string        `json:"status"`     // "completed", "failed", "timeout", "cancelled"
    JobID      string        `json:"job_id"`
    Notes      string        `json:"notes"`
    StartedAt  time.Time     `json:"started_at"`
    FinishedAt time.Time     `json:"finished_at"`
    Duration   time.Duration `json:"duration"`
}
```

#### Configuration Requirements

```yaml
# transflow.yaml self-healing configuration
self_heal:
  enabled: true
  max_retries: 2      # Maximum healing attempts (1-5)
  cooldown: 30s       # Optional cooldown between attempts
```

Environment variables for healing jobs:
- `TRANSFLOW_MODEL`: LLM model for healing (default: gpt-4o-mini@2024-08-06)
- `TRANSFLOW_TOOLS`: MCP tools configuration JSON
- `TRANSFLOW_LIMITS`: Execution limits configuration JSON

#### File Locations

- **Main Implementation**: 
  - `internal/cli/transflow/runner.go` - Main healing orchestration in `attemptHealing()`
  - `internal/cli/transflow/fanout_orchestrator.go` - Branch execution logic (needs human-step completion)
  - `internal/cli/transflow/job_submission.go` - Nomad job submission helpers

- **Test Files**:
  - `internal/cli/transflow/self_healing_test.go` - Healing workflow tests (needs branch-specific coverage)
  - `internal/cli/transflow/job_submission_test.go` - Job submission tests (needs human-step tests)
  - `internal/cli/transflow/integration_test.go` - End-to-end integration tests

- **Nomad Job Templates**:
  - `roadmap/transflow/jobs/llm_exec.hcl` - LLM execution job template
  - `roadmap/transflow/jobs/orw_apply.hcl` - OpenRewrite application job template
  - Need to create: `roadmap/transflow/jobs/human_step.hcl` (if job-based approach is used)

- **Documentation Updates**:
  - `roadmap/transflow/MVP.md` - Update implementation status from "Partially Implemented" to "Fully Implemented"
  - `FEATURES.md` - Document completed healing branch types
  - `CHANGELOG.md` - Record implementation completion

## User Notes
- Follow @AGENTS.md TDD framework strictly - write failing tests first, minimal implementation, then VPS refactor phase
- Human-step branch should involve creating Git branches/MRs and polling for manual commits
- All branch types should have proper error handling and timeout management
- Tests should cover both success and failure scenarios for each branch type
- Integration with existing Nomad job submission infrastructure is already complete

## Work Log
- [2025-09-05] Created task for completing healing branch types implementation