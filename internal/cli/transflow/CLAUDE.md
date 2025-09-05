# Transflow CLI Module CLAUDE.md

## Purpose
Complete CLI integration for orchestrating multi-step code transformation workflows with comprehensive self-healing capabilities using three distinct branch types (human-step, llm-exec, orw-gen), production Nomad job orchestration, and GitLab merge request integration.

## Narrative Summary
The transflow module provides end-to-end implementation of `ploy transflow run` command supporting complete transformation pipelines with production-ready self-healing capabilities. It applies code transformations via OpenRewrite recipes, validates results through automated builds, creates GitLab merge requests for review, and includes sophisticated self-healing workflows with three distinct healing branch types executed via production Nomad job orchestration.

Core workflow: clone repository → create branch → apply transformations → commit changes → validate build → create/update merge request → on build failures, triggers self-healing via parallel fanout orchestration with first-success-wins semantics. The healing system supports human-step (MR-based manual intervention), llm-exec (LLM-powered code fixes), and orw-gen (OpenRewrite recipe generation) branches. Production orchestration uses SubmitAndWaitTerminal for real Nomad job submission with HCL template rendering and artifact processing.

## Key Files
- `run.go:1-250` - CLI command entry point and flag parsing
- `runner.go:1-650` - Complete orchestration logic with healing integration
- `runner.go:126-172` - Main dependency injection and initialization
- `runner.go:174-280` - Core workflow execution (Run method)
- `runner.go:282-380` - Build validation and error handling
- `runner.go:441-500` - Self-healing workflow integration
- `runner.go:509-553` - GitLab MR creation and updates
- `config.go:1-150` - Configuration loading, validation, and timeout parsing
- `integrations.go:87-200` - Factory pattern for production vs test implementations
- `types.go:1-72` - Complete job submission type system with ProductionBranchRunner interface
- `types.go:17-26` - ProductionBranchRunner interface for asset rendering and dependency access
- `types.go:60-72` - JobSubmissionHelper and FanoutOrchestrator interfaces
- `job_submission.go:1-250` - Production JobSubmissionHelper with HCL rendering and artifact parsing
- `job_submission.go:47-84` - Environment variable substitution for HCL templates
- `job_submission.go:86-98` - JSON artifact retrieval and parsing
- `job_submission.go:100-180` - Real planner/reducer job submission with SubmitAndWaitTerminal
- `fanout_orchestrator.go:1-300` - Production parallel healing branch orchestration with three branch types
- `fanout_orchestrator.go:44-120` - First-success-wins fanout execution with context cancellation and timeout handling
- `fanout_orchestrator.go:127-198` - Branch execution dispatcher and context management
- `fanout_orchestrator.go:200-246` - LLM-exec branch with HCL rendering, environment substitution, and diff.patch processing
- `fanout_orchestrator.go:248-333` - ORW-gen branch with recipe configuration extraction and template substitution
- `fanout_orchestrator.go:335-420` - Human-step branch with MR creation, commit polling, and build validation
- `self_healing.go:1-250` - Self-healing configuration and result tracking
- `mocks.go:1-200` - Complete mock implementation framework
- `integration_test.go:1-300` - End-to-end integration test suite

## Integration Points
### Consumes
- ARF Git Operations: Repository cloning, branching, commits
- ARF Recipe Executor: OpenRewrite recipe execution
- SharedPush Build Checker: Build validation and deployment
- GitLab REST API: Merge request creation/updates (via provider.GitProvider)
- Ploy Orchestration: Production SubmitAndWaitTerminal for real Nomad job submission
- HCL Templates: roadmap/transflow/jobs/*.hcl for planner/reducer/branch job definitions
- Nomad API: Direct job submission with environment variable substitution

### Provides
- CLI Commands: `ploy transflow run -f <config>` with complete flag support
- Workflow Execution: End-to-end transformation pipeline with result tracking
- Self-Healing: Production LangGraph-based healing with real planner/reducer jobs and parallel branch execution
- MR Integration: GitLab merge request creation/updates with rich descriptions
- Test Mode: Complete mock infrastructure for CI/CD and local testing
- Job Orchestration: Production Nomad job submission with HCL template rendering and artifact processing
- Artifact Processing: JSON parsing of plan.json, next.json, and diff.patch from completed jobs
- Branch Type Implementation: Complete support for human-step (Git+MR workflow), llm-exec (HCL job submission), and orw-gen (recipe generation) healing strategies

## Configuration
Required files:
- `transflow.yaml` - Workflow configuration with steps and target repository

Environment variables:
- `GITLAB_URL` - GitLab instance URL (defaults to https://gitlab.com)
- `GITLAB_TOKEN` - GitLab API token for MR operations
- `TRANSFLOW_SUBMIT` - Set to "1" to enable production job submission for healing workflows
- `TRANSFLOW_MODEL` - LLM model for healing jobs (default: gpt-4o-mini@2024-08-06)
- `TRANSFLOW_TOOLS` - Tool configuration JSON for healing jobs (default: file/search allowlist)
- `TRANSFLOW_LIMITS` - Execution limits JSON for healing jobs (default: 8 steps, 12 tool calls, 30m timeout)
- `NOMAD_ADDR` - Nomad cluster address for production job submission
- `RUN_ID` - Automatically generated unique identifier for job runs

CLI flags:
- `--config, -f` - Configuration file path
- `--test-mode` - Enable mock implementations for testing
- `--dry-run` - Show what would be done without execution
- `--verbose` - Detailed output during workflow execution

## Key Patterns
- Complete dependency injection with interface-based design (see runner.go:126-172, integrations.go:87-200)
- Factory pattern for production vs test implementations (see integrations.go)
- Test mode infrastructure with comprehensive mocking (see mocks.go:1-200)
- Production job submission with HCL template rendering and environment substitution (see job_submission.go:47-84)
- Real artifact processing with JSON parsing for job outputs (see job_submission.go:86-98)
- Type-safe job submission interfaces supporting planner/reducer/branch workflows (see types.go:60-72)
- Production fanout orchestration with first-success-wins semantics and real Nomad jobs (see fanout_orchestrator.go:50-120)
- Complete three-branch healing system: human-step (MR+build validation), llm-exec (HCL+diff processing), orw-gen (recipe generation)
- Context-aware cancellation and timeout handling for parallel branch execution
- HCL template processing with environment variable substitution for job definitions
- Graceful error handling with optional MR creation (see runner.go:509-553)
- Self-healing workflow with production LangGraph integration and parallel branch execution via first-success-wins fanout orchestration
- Complete healing branch type system supporting three distinct healing strategies with production Nomad job orchestration
- Context-aware cancellation ensuring resources are freed when first branch succeeds or timeout occurs
- Configuration validation with timeout parsing and comprehensive error reporting

## Related Documentation
- `../git/provider/CLAUDE.md` - GitLab provider implementation
- `../../api/arf/` - Application Recipe Framework integration
- `../../orchestration/CLAUDE.md` - Production job submission and monitoring infrastructure
- `../../../roadmap/transflow/jobs/` - HCL templates for planner, reducer, and healing branch jobs
- `../../../roadmap/transflow/MVP.md` - Complete implementation status and requirements
- Integration test repository: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git