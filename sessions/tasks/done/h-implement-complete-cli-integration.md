---
task: h-implement-complete-cli-integration
branch: feature/complete-cli-integration
status: completed
created: 2025-09-05
started: 2025-09-05
modules: [internal/cli/transflow, cmd/ploy, internal/orchestration]
---

# Complete CLI Integration for Transflow MVP

## Problem/Goal
Complete the end-to-end CLI integration for `ploy transflow run` to make it fully functional per the MVP requirements. While the CLI infrastructure exists, the MVP.md shows "Complete CLI integration (`ploy transflow run`)" as "❌ Not Implemented", indicating missing functionality or integration issues that prevent the full transflow workflow from working properly.

The goal is to ensure `ploy transflow run -f transflow.yaml` executes the complete workflow: recipe execution → build validation → healing (if needed) → GitLab MR creation, with proper error handling and comprehensive testing.

## Success Criteria
- [x] Full end-to-end workflow execution via `ploy transflow run -f transflow.yaml`
- [x] Integration with existing ARF pipeline for recipe execution
- [x] Build validation using `SharedPush` to controller `/v1/apps/:app/builds`
- [x] LangGraph healing integration (planner/reducer jobs) when builds fail
- [x] GitLab MR creation after successful workflow completion
- [x] Comprehensive CLI argument parsing and validation
- [x] Support for all documented CLI flags and options
- [x] Unit tests for CLI command parsing and workflow orchestration
- [x] Create `ploy-orw-java11-maven` test repository, push to GitLab, and update tests to use real repo URL instead of fake test/repo URLs
- [x] Integration tests demonstrating full workflow functionality
- [x] Error handling that follows project patterns (graceful failures)
- [x] Verbose and quiet output modes working correctly
- [x] Working examples using the provided transflow.yaml template
- [x] Documentation updates showing CLI usage and examples
- [x] Mark MVP.md task as completed (✅)
- [x] Update CHANGELOG.md with CLI integration completion
- [x] All tests passing with TDD methodology (RED/GREEN/REFACTOR)

## Context Files
- @roadmap/transflow/MVP.md - Current MVP status and requirements
- @roadmap/transflow/transflow.yaml - Configuration template and examples
- @internal/cli/transflow/run.go - CLI entrypoint and command parsing
- @internal/cli/transflow/runner.go - Core workflow execution engine
- @internal/cli/transflow/config.go - YAML configuration loading and validation
- @cmd/ploy/main.go:47-48 - Main CLI integration point
- @internal/cli/common/deploy.go::SharedPush - Build validation interface
- @internal/orchestration - Job submission and monitoring infrastructure
- @internal/git/provider - GitLab MR integration (recently implemented)

## User Notes
- Follow TDD methodology strictly: RED (failing tests) → GREEN (minimal implementation) → REFACTOR (VPS testing)
- Reuse existing infrastructure where possible (ARF, orchestration, git operations)
- Follow CLI patterns established in other `ploy` commands
- Ensure proper error handling and user feedback for all failure modes
- Test both success and failure scenarios comprehensively
- Validate integration with recently completed GitLab MR functionality
- Use the transflow.yaml template for testing scenarios

## Context Manifest

### How the Complete CLI Integration Currently Works: Transflow MVP Architecture

When a developer runs `ploy transflow run -f transflow.yaml`, the system initiates a complex multi-stage workflow that orchestrates OpenRewrite recipe execution, build validation, self-healing (if enabled), and GitLab merge request creation. This end-to-end integration spans multiple architectural layers and requires careful coordination between existing ARF infrastructure, job orchestration systems, and git provider integrations.

**Current CLI Entry Point and Parsing:**
The main CLI integration begins in `/cmd/ploy/main.go:47-48` where the `transflow` command routes to `transflow.TransflowCmd(os.Args[2:], controllerURL)` in `/internal/cli/transflow/run.go`. The `TransflowCmd` function handles the primary command parsing, supporting only the `run` subcommand which delegates to `runTransflow()`. This function creates a comprehensive flag set supporting multiple execution modes including dry-run validation (`--dry-run`), specialized job rendering (`--render-planner`, `--plan`, `--reduce`), selective execution (`--exec-llm-first`, `--exec-orw-first`, `--apply-first`), and standard workflow execution.

**Configuration Loading and Validation:**
The system loads YAML configuration from `/roadmap/transflow/transflow.yaml` using `LoadConfig()` in `/internal/cli/transflow/config.go`. The configuration structure includes workflow metadata (`id`, `target_repo`, `base_ref`), build parameters (`lane`, `build_timeout`), recipe steps, and optional self-healing configuration. The `SelfHealConfig` struct supports enabling/disabling healing, setting maximum retry attempts (1-5), and configuring cooldown periods. Configuration validation ensures required fields are present and validates duration formats for timeouts and cooldowns.

**Integration Factory Pattern:**
The CLI creates a `TransflowIntegrations` factory in `/internal/cli/transflow/integrations.go` that provides concrete implementations for all workflow components. The factory method `CreateConfiguredRunner()` instantiates a `TransflowRunner` and injects four critical dependencies: `ARFGitOperations` (wrapping existing ARF git operations), `ARFRecipeExecutor` (delegating to `ploy arf transform` commands), `SharedPushBuildChecker` (using existing `SharedPush` for build validation), and `GitLabProvider` (for merge request creation). This dependency injection pattern enables comprehensive testing with mock implementations.

**Workflow Orchestration Engine:**
The `TransflowRunner` in `/internal/cli/transflow/runner.go` executes a seven-step workflow: (1) clone repository using ARF git operations, (2) create timestamped workflow branch (`workflow/{id}/{timestamp}`), (3) execute OpenRewrite recipes by invoking `ploy arf transform` for each recipe ID, (4) commit changes with standardized commit message, (5) run build check using `SharedPush` against controller `/v1/apps/:app/builds` endpoint, (6) push branch to remote repository, and (7) create GitLab merge request if configured. Each step is instrumented with timing and success tracking via `StepResult` objects.

**Build Validation Integration:**
Build checking leverages the existing `SharedPush` function from `/internal/cli/common/deploy.go` configured with `IsPlatform=false` (ploy mode), `Environment=dev` (sandbox), and a generated app name format `tfw-{id}-{timestamp}`. The system uses lane detection if not explicitly specified and respects configurable build timeouts. Build failures trigger the self-healing workflow if enabled, otherwise cause immediate workflow termination.

**Self-Healing Workflow Architecture:**
When builds fail and self-healing is enabled, the system initiates a sophisticated three-phase healing process: planner job submission, parallel fanout execution, and reducer job evaluation. The planner job analyzes the build error and generates a `plan.json` with multiple healing options (human-step, llm-exec, orw-generated). The fanout orchestrator in `/internal/cli/transflow/fanout_orchestrator.go` executes healing branches in parallel with first-success-wins semantics and configurable parallelism limits. The reducer job evaluates all results and determines next actions (typically "stop" if a branch succeeded).

**Job Submission Infrastructure:**
The healing workflow uses Nomad job templates from `/roadmap/transflow/jobs/` including `planner.hcl`, `reducer.hcl`, `llm_exec.hcl`, and `orw_apply.hcl`. Job submission occurs through `/internal/orchestration/submit.go` using `SubmitAndWaitTerminal()` for batch jobs that must reach terminal states. Environment variable substitution handles model configuration (`TRANSFLOW_MODEL`), MCP tools (`TRANSFLOW_TOOLS`), execution limits (`TRANSFLOW_LIMITS`), and run identification. The system supports both local file output and SeaweedFS/HTTP artifact retrieval patterns.

**GitLab Merge Request Integration:**
The recently implemented GitLab integration in `/internal/git/provider/gitlab.go` handles MR creation and updates through GitLab's REST API v4. The system extracts project namespace/name from HTTPS repository URLs, searches for existing MRs with the same source branch for idempotency, and creates descriptive MR content including applied transformations, healing summaries (if applicable), and standardized labels ("ploy", "tfl"). Authentication uses `GITLAB_TOKEN` environment variable with optional `GITLAB_URL` for custom instances.

**Error Handling and Recovery:**
The system implements graceful error handling at multiple levels. Configuration validation catches malformed YAML early. Git operations include proper cleanup of temporary directories. Build failures optionally trigger healing workflows rather than immediate termination. Job submissions include timeout handling and terminal state monitoring. MR creation failures are logged but don't fail the overall workflow since they're considered optional. The `TransflowResult` structure captures comprehensive step-by-step execution details for debugging.

### For Complete CLI Integration Implementation: Current Gaps and Requirements

The MVP roadmap shows "Complete CLI integration (`ploy transflow run`)" as "❌ Not Implemented" despite extensive infrastructure existing. Analysis reveals the integration is substantially complete but has critical gaps preventing full end-to-end functionality.

**Missing Production Job Submission:**
The most significant gap is in `/internal/cli/transflow/job_submission.go` where the `SubmitPlannerJob()` and `SubmitReducerJob()` methods contain placeholder implementations that return "production job submission not implemented yet" errors. The job submission helper needs production implementations that use the existing `orchestration.SubmitAndWaitTerminal()` function with properly rendered HCL templates from the asset rendering methods already implemented in the runner.

**Incomplete Asset Rendering Integration:**
While `RenderPlannerAssets()`, `RenderLLMExecAssets()`, and `RenderORWApplyAssets()` exist in the runner, the job submission helpers don't properly connect these rendered assets to actual job submission. The production implementation needs to: (1) call the appropriate asset rendering method, (2) perform environment variable substitution on the HCL templates, (3) write the rendered HCL to the workspace, and (4) submit the job using `orchestration.SubmitAndWaitTerminal()`.

**Schema Validation Integration:**
The CLI includes sophisticated validation infrastructure in `/internal/cli/transflow/schema.go` using JSON schema validation for plan.json and next.json artifacts, but these validators (`validatePlanJSON`, `validateNextJSON`) are referenced in the CLI but not implemented. The schema validation ensures planner and reducer outputs conform to expected structures before processing.

**Fanout Orchestrator Production Mode:**
The `fanoutOrchestrator` in `/internal/cli/transflow/fanout_orchestrator.go` currently only supports test mode job submission. The production implementation needs to integrate with actual Nomad job submission for parallel healing branch execution.

**Integration Testing Coverage:**
While unit tests exist for individual components, comprehensive integration tests demonstrating the complete end-to-end workflow are missing. The system needs integration tests that validate: configuration loading → asset rendering → job submission → artifact retrieval → result processing.

**Configuration Template Validation:**
The transflow.yaml template in `/roadmap/transflow/transflow.yaml` contains comprehensive configuration options (budgets, network policies, gates) that aren't fully implemented in the current configuration parser. The production system needs to support or gracefully handle all documented configuration options.

### Technical Reference Details

#### Core Interface Signatures

```go
// Primary CLI entry point
func TransflowCmd(args []string, controllerURL string)
func runTransflow(args []string, controllerURL string) error

// Configuration and validation
func LoadConfig(path string) (*TransflowConfig, error)
func (c *TransflowConfig) Validate() error
func (c *TransflowConfig) ParseBuildTimeout() (time.Duration, error)

// Integration factory
func NewTransflowIntegrations(controllerURL, workDir string) *TransflowIntegrations
func (i *TransflowIntegrations) CreateConfiguredRunner(config *TransflowConfig) (*TransflowRunner, error)

// Core workflow execution
func (r *TransflowRunner) Run(ctx context.Context) (*TransflowResult, error)
func (r *TransflowRunner) RenderPlannerAssets() (*PlannerAssets, error)
func (r *TransflowRunner) PrepareRepo(ctx context.Context) (string, string, error)
func (r *TransflowRunner) ApplyDiffAndBuild(ctx context.Context, repoPath, diffPath string) error

// Job submission interfaces (need production implementation)
type JobSubmissionHelper interface {
    SubmitPlannerJob(ctx context.Context, config *TransflowConfig, buildError string, workspace string) (*PlanResult, error)
    SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error)
}

// Fanout orchestration (needs production implementation)  
type FanoutOrchestrator interface {
    RunHealingFanout(ctx context.Context, runCtx interface{}, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error)
}
```

#### Data Structures and Configuration

```go
// Primary configuration structure
type TransflowConfig struct {
    Version      string          `yaml:"version"`
    ID           string          `yaml:"id"`
    TargetRepo   string          `yaml:"target_repo"`
    TargetBranch string          `yaml:"target_branch"`
    BaseRef      string          `yaml:"base_ref"`
    Lane         string          `yaml:"lane"`
    BuildTimeout string          `yaml:"build_timeout"`
    Steps        []TransflowStep `yaml:"steps"`
    SelfHeal     *SelfHealConfig `yaml:"self_heal"`
}

// Workflow execution result
type TransflowResult struct {
    Success        bool
    WorkflowID     string
    BranchName     string
    CommitSHA      string
    BuildVersion   string
    StepResults    []StepResult
    ErrorMessage   string
    Duration       time.Duration
    HealingSummary *TransflowHealingSummary
    MRURL          string
}

// Job submission specifications
type JobSpec struct {
    Name       string                 `json:"name"`
    Type       string                 `json:"type"`
    HCLPath    string                 `json:"hcl_path"`
    EnvVars    map[string]string      `json:"env_vars"`
    Timeout    time.Duration          `json:"timeout"`
    Inputs     map[string]interface{} `json:"inputs"`
}
```

#### API Integration Points

```go
// Build validation endpoint integration
POST /v1/apps/:app/builds?sha=...&env=dev[&lane=...]
// Uses SharedPush with DeployConfig{IsPlatform: false, Environment: "dev"}

// GitLab MR creation endpoints
GET /api/v4/projects/{project}/merge_requests?source_branch={branch}&state=opened
POST /api/v4/projects/{project}/merge_requests
PUT /api/v4/projects/{project}/merge_requests/{id}
```

#### Environment Variables and Configuration

- **GITLAB_URL**: GitLab instance URL (default: "https://gitlab.com")
- **GITLAB_TOKEN**: GitLab API token (required for MR creation)
- **TRANSFLOW_MODEL**: LLM model specification (default: "gpt-4o-mini@2024-08-06")
- **TRANSFLOW_TOOLS**: MCP tools JSON configuration
- **TRANSFLOW_LIMITS**: Execution limits JSON configuration
- **TRANSFLOW_SUBMIT**: Enable job submission ("1" to enable)
- **TRANSFLOW_PLAN_URL/TRANSFLOW_PLAN_PATH**: Plan artifact location
- **TRANSFLOW_DIFF_URL/TRANSFLOW_DIFF_PATH**: Diff artifact location
- **NOMAD_ADDR**: Nomad cluster address for job submission

#### File Locations and Implementation Requirements

**Primary implementation files:**
- CLI entry: `/internal/cli/transflow/run.go` - Command parsing and workflow coordination
- Core engine: `/internal/cli/transflow/runner.go` - Workflow execution logic
- Job submission: `/internal/cli/transflow/job_submission.go` - **NEEDS PRODUCTION IMPLEMENTATION**
- Fanout orchestrator: `/internal/cli/transflow/fanout_orchestrator.go` - **NEEDS PRODUCTION IMPLEMENTATION**
- Schema validation: `/internal/cli/transflow/schema.go` - **NEEDS VALIDATOR IMPLEMENTATION**

**Supporting infrastructure:**
- Configuration: `/internal/cli/transflow/config.go` - YAML parsing and validation
- Integrations: `/internal/cli/transflow/integrations.go` - Dependency injection factory
- Self-healing: `/internal/cli/transflow/self_healing.go` - Healing workflow types
- Git provider: `/internal/git/provider/gitlab.go` - MR creation (recently implemented)
- Orchestration: `/internal/orchestration/submit.go` - Nomad job submission

**Job templates:**
- `/roadmap/transflow/jobs/planner.hcl` - Planner job template
- `/roadmap/transflow/jobs/reducer.hcl` - Reducer job template  
- `/roadmap/transflow/jobs/llm_exec.hcl` - LLM execution job template
- `/roadmap/transflow/jobs/orw_apply.hcl` - OpenRewrite application job template

**Test files requiring completion:**
- `/internal/cli/transflow/runner_test.go` - Core workflow tests
- `/internal/cli/transflow/gitlab_integration_test.go` - GitLab integration tests
- Integration test suite for complete end-to-end workflow validation

**Configuration and documentation:**
- `/roadmap/transflow/transflow.yaml` - Configuration template and examples
- `/roadmap/transflow/MVP.md` - MVP status tracking (needs ✅ update)
- `/CHANGELOG.md` - Requires update upon completion

## Work Log
- [2025-09-05] Created task following MVP requirements and roadmap documentation
- [2025-09-05] Completed: Full CLI integration implemented with production job submission infrastructure, GitLab MR integration, self-healing workflows, comprehensive testing framework, and real test repository