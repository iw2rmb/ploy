---
task: h-implement-job-submission-infrastructure
branch: feature/implement-job-submission-infrastructure
status: pending
created: 2025-01-09
modules: [internal/cli/transflow, internal/orchestration, api/arf]
---

# Job Submission Infrastructure for Transflow MVP

## Problem/Goal

Implement the critical job submission and monitoring infrastructure required for transflow's LangGraph healing workflow. This is the foundational piece that enables:
- Planner job submission (LangGraph analysis of build failures)
- Parallel healing branch execution (human-step, llm-exec, orw-generated)
- Reducer job coordination (first-success-wins logic)
- Job status monitoring and artifact collection

Without this infrastructure, the healing workflow cannot function and the MVP cannot be completed.

## Success Criteria

- [ ] Implement `SubmitAndWaitTerminal()` for batch job submission and monitoring
- [ ] Create orchestration helpers for planner/reducer/healing job types  
- [ ] Add job status polling with proper timeout handling
- [ ] Implement artifact collection from completed jobs
- [ ] Write comprehensive unit tests for all job submission logic
- [ ] Integration test with mock Nomad to validate workflow
- [ ] Update `TransflowRunner` to use job submission for healing cycle
- [ ] Document usage patterns and error handling
- [ ] Pass all existing tests + new test coverage > 80%
- [ ] Build compiles successfully (`go build ./...`)
- [ ] Update CHANGELOG.md with implementation details

## Context Files

<!-- Core requirements from roadmap -->
- @roadmap/transflow/MVP.md                           # MVP specification and requirements
- @roadmap/transflow/orchestrator_submit_wait_terminal.md  # Job submission guidance
- @roadmap/transflow/orchestrator_fanout_sketch.md        # Parallel execution pattern
- @roadmap/transflow/jobs/planner.hcl                    # Planner job template
- @roadmap/transflow/jobs/reducer.hcl                    # Reducer job template
- @roadmap/transflow/jobs/llm_exec.hcl                   # LLM execution job template

<!-- Current implementation -->
- @internal/cli/transflow/runner.go                   # Main runner that needs job integration
- @internal/cli/transflow/run.go                      # CLI entry point with job flags
- @internal/orchestration/                            # Existing orchestration package
- @api/arf/multi_repo_orchestrator.go               # Reference for job patterns

<!-- Test patterns -->
- @internal/cli/transflow/runner_test.go             # Existing test structure
- @AGENTS.md                                         # TDD requirements and testing guidelines

## User Notes

**Critical MVP Blocker**: This task represents the most critical missing piece for transflow MVP. All healing functionality depends on reliable job submission and monitoring.

**TDD Approach Required**: Follow @AGENTS.md mandatory TDD cycle:
1. RED: Write failing tests for job submission scenarios
2. GREEN: Implement minimal working job submission logic  
3. REFACTOR: Test integration on VPS with real Nomad

**Architecture Constraints**:
- Must integrate with existing `internal/orchestration` package
- Follow established patterns from ARF multi-repo orchestrator
- Support both synchronous (wait for completion) and asynchronous job execution
- Handle job timeouts, failures, and cancellation gracefully

**Reference Implementation**: Use `orchestrator_submit_wait_terminal.md` and `orchestrator_fanout_sketch.md` as design guidance - these contain the architectural patterns already validated for the platform.

## Context Manifest

### How This Currently Works: Transflow Orchestration Architecture

When a user initiates a transflow run via `ploy transflow run -f transflow.yaml`, the request flows through a sophisticated multi-layer architecture that's designed around healing workflows and job orchestration.

**Current Entry Point and Configuration Flow:**
The command enters through `internal/cli/transflow/run.go::runTransflow()`, which parses flags and loads the YAML configuration via `internal/cli/transflow/config.go::LoadConfig()`. The configuration system supports:
- Basic workflow definition (id, target_repo, base_ref, steps)
- Lane-specific deployment configuration (lane A-G auto-detection)
- Build timeout configuration with validation
- Self-healing configuration (max_retries, cooldown, enabled flag)

**Current Implementation Status:**
The transflow system is **partially implemented** with several critical gaps:
- ✅ OpenRewrite recipe execution via ARF integration (`internal/cli/transflow/integrations.go`)
- ✅ Git operations wrapper around `api/arf/git_operations.go`
- ✅ Build checking via `SharedPush` from `internal/cli/common/deploy.go`  
- ✅ Basic runner structure with dependency injection (`internal/cli/transflow/runner.go`)
- ❌ **JOB SUBMISSION AND MONITORING INFRASTRUCTURE** (this task's focus)
- ❌ Healing workflow orchestration (planner → fanout → reducer)
- ❌ Artifact collection from completed jobs

**Existing Orchestration Infrastructure:**
The system leverages `internal/orchestration/` package which provides:

1. **Submit Functions** (`internal/orchestration/submit.go`):
   - `Submit(jobPath)` - Basic HCL job registration
   - `SubmitAndWaitHealthy()` - For services (not applicable to batch jobs)
   - **`SubmitAndWaitTerminal()`** - **ALREADY EXISTS** for batch jobs but needs integration
   - `ValidateJob()` and `PlanJob()` utilities

2. **Monitoring Infrastructure** (`internal/orchestration/monitor.go`):
   - `HealthMonitor` with SDK adapter pattern
   - Job status polling with allocation state tracking
   - Terminal state detection (complete/failed) 
   - Endpoint discovery for running services

3. **Nomad SDK Integration** (`internal/orchestration/monitor_sdk_adapter.go`):
   - Clean abstraction over Nomad API client
   - Environment-driven configuration (`NOMAD_ADDR`)
   - Allocation status parsing with deployment health checks

**Current Runner Architecture:**
The `TransflowRunner` in `internal/cli/transflow/runner.go` follows dependency injection pattern:
- Interface-based design (`GitOperationsInterface`, `RecipeExecutorInterface`, `BuildCheckerInterface`)
- Factory pattern via `TransflowIntegrations` in `integrations.go`
- Comprehensive test coverage with mock implementations in `runner_test.go`

**Existing Job Templates and Patterns:**
The healing workflow architecture is defined through HCL job templates in `roadmap/transflow/jobs/`:

1. **Planner Job** (`planner.hcl`):
   - LangGraph-based Docker container execution
   - Input: `inputs.json` with language, lane, last_error, deps
   - Output: `plan.json` with parallel healing options
   - Environment variables: MODEL, TOOLS_JSON, LIMITS_JSON, RUN_ID
   - Host volume mounts for context, kb (knowledge base), and output

2. **Reducer Job** (`reducer.hcl`):  
   - Processes branch results and determines next actions
   - Input: `history.json` with plan_id, branches, winner
   - Output: `next.json` with action and notes
   - Lighter resource requirements (300 CPU, 512 MB memory)

3. **Healing Branch Jobs**:
   - **LLM Exec** (`llm_exec.hcl`): Direct LLM-generated patch application
   - **ORW Apply** (`orw_apply.hcl`): OpenRewrite recipe execution

All job templates use:
- Batch type with restart attempts = 0, mode = "fail"
- Host volume mounts for workspace isolation
- Environment variable placeholders for runtime substitution
- Consistent timeout patterns (5m kill_timeout, job-specific execution timeout)

### For Job Submission Implementation: What Needs to Connect

Since we're implementing the job submission infrastructure that enables healing workflows, this connects to the existing architecture at several critical integration points:

**The TransflowRunner Integration Point:**
After a build failure in `runner.go::Run()`, the healing cycle needs to be triggered. Currently, the runner stops execution on build failure (lines 381-398), but the MVP requires planner job submission at this point. The architecture calls for:

1. **Planner Submission** after build failure detection
2. **Plan Parsing** to extract healing options 
3. **Fanout Execution** of parallel healing branches (human-step, llm-exec, orw-generated)
4. **First-Success-Wins** cancellation logic
5. **Reducer Job** to determine final actions

**Command-Line Interface Extensions:**
The `run.go` already has partial job submission flags:
- `--plan` flag renders and optionally submits planner job
- `--exec-llm-first` and `--exec-orw-first` for branch execution
- `--apply-first` for diff application and validation
- Environment-driven submission control via `TRANSFLOW_SUBMIT=1`

**ARF System Integration Patterns:**
The existing `api/arf/openrewrite_dispatcher.go` demonstrates the job submission pattern we need to follow:
- Nomad client initialization with environment configuration
- Job template rendering with placeholder substitution  
- Error handling and retry logic
- Infrastructure validation before job submission

**Volume and Artifact Management:**
The job templates expect host volume sources:
- `transflow-context` - Input data for jobs
- `transflow-kb` - Knowledge base for planner
- `transflow-out` - Artifact collection from completed jobs
- `transflow-history` - Branch execution results for reducer

**Environment Variable Patterns:**
Jobs expect consistent environment configuration:
- `TRANSFLOW_MODEL` (default: "gpt-4o-mini@2024-08-06")
- `TRANSFLOW_TOOLS` (JSON allowlisting tools)  
- `TRANSFLOW_LIMITS` (JSON with max_steps, max_tool_calls, timeout)
- Recipe-specific vars for ORW jobs (RECIPE_CLASS, RECIPE_COORDS)

**Current Integration Gaps:**
The existing `SubmitAndWaitTerminal()` function provides the core job submission capability, but we need:
1. **Helper functions** for each job type (planner, reducer, llm-exec, orw-apply)
2. **Asset rendering** functions (already partially implemented in `runner.go`)
3. **Fanout orchestration** following the pseudo-code in `orchestrator_fanout_sketch.md`
4. **Artifact collection** after job completion
5. **Integration with TransflowRunner** healing cycle

### Technical Reference Details

#### Core Functions to Implement

**Job Submission Helpers:**
```go
// In internal/cli/transflow/job_submission.go
func SubmitPlannerJob(ctx context.Context, run *RunContext) (*PlannerResult, error)
func SubmitReducerJob(ctx context.Context, run *RunContext, history *BranchHistory) (*ReducerResult, error) 
func SubmitLLMExecJob(ctx context.Context, run *RunContext, option BranchSpec) (*BranchResult, error)
func SubmitORWApplyJob(ctx context.Context, run *RunContext, option BranchSpec) (*BranchResult, error)
```

**Fanout Orchestration:**
```go
// Following orchestrator_fanout_sketch.md pattern
func RunHealingFanout(ctx context.Context, run *RunContext, plan []BranchSpec, maxParallel int) (winner BranchResult, results []BranchResult, err error)
func executeBranch(ctx context.Context, run *RunContext, spec BranchSpec, cancelCh <-chan struct{}) BranchResult
```

#### Data Structures

**Job Execution Context:**
```go
type RunContext struct {
    Config       *TransflowConfig
    WorkspaceDir string
    RepoPath     string
    OutDir       string
    JobTimeout   time.Duration
    BuildTimeout time.Duration
    Allowlist    []string
}
```

**Branch Specifications (from fanout sketch):**
```go
type BranchSpec struct {
    ID     string
    Type   string // "human" | "llm-exec" | "orw-gen" 
    Inputs map[string]any
}

type BranchResult struct {
    ID         string
    Status     string // success|failed|canceled|timeout
    Artifact   string // diff.patch path
    JobName    string  
    JobID      string
    StartedAt  time.Time
    FinishedAt time.Time
    Notes      string
}
```

#### Configuration Requirements

**Environment Variables:**
- `NOMAD_ADDR` - Nomad cluster address (orchestration package)
- `TRANSFLOW_MODEL` - LLM model specification
- `TRANSFLOW_TOOLS` - Tool allowlist JSON
- `TRANSFLOW_LIMITS` - Execution limits JSON
- `TRANSFLOW_SUBMIT` - Enable actual job submission (vs dry-run)

**Validation Schema (existing in transflow/schema.go):**
- `validatePlanJSON()` - Validates planner output structure
- `validateNextJSON()` - Validates reducer output structure

#### File Locations

**Implementation Structure:**
- New file: `internal/cli/transflow/job_submission.go` - Core job submission helpers
- New file: `internal/cli/transflow/fanout_orchestration.go` - Healing workflow orchestration
- Extension: `internal/cli/transflow/runner.go` - Integrate healing cycle into main Run()
- Extension: `internal/orchestration/submit.go` - Add job-specific utilities if needed

**Test Implementation:**
- New file: `internal/cli/transflow/job_submission_test.go` - Unit tests for submission logic
- New file: `internal/cli/transflow/fanout_orchestration_test.go` - Integration tests for healing flow
- Extend: `internal/cli/transflow/runner_test.go` - Add healing workflow test cases

**Job Template Integration:**
- Use existing: `roadmap/transflow/jobs/*.hcl` - Job templates with placeholder substitution
- Workspace structure: `${workspace}/planner/`, `${workspace}/llm-exec/${option-id}/`, etc.

**Volume Mount Points:**
- Host volumes require Nomad cluster configuration
- Local development: Workspace subdirectories mounted as host volumes
- Production: Shared storage backend (SeaweedFS) integration

#### Key Integration Points

**With Existing Systems:**
1. **orchestration.SubmitAndWaitTerminal()** - Already implemented, needs integration
2. **TransflowRunner.ApplyDiffAndBuild()** - Already handles diff validation and build gates
3. **TransflowRunner.RenderPlannerAssets()** - Asset preparation (partially implemented)
4. **SharedPush build checking** - Build gate validation after diff application

**Error Handling Patterns:**
- Follow existing patterns in `api/arf/openrewrite_dispatcher.go`
- Timeout handling consistent with `internal/orchestration/submit.go`
- Job cancellation via context and channel patterns (fanout sketch)
- Artifact validation before declaring success

## Work Log

- [2025-01-09] Created task, ready to begin implementation following TDD cycle