# Transflow CLI Module CLAUDE.md

## Purpose
Complete CLI integration for orchestrating multi-step code transformation workflows with comprehensive self-healing capabilities using three distinct branch types (human-step, llm-exec, orw-gen), production Nomad job orchestration, and GitLab merge request integration.

## Narrative Summary
The transflow module provides end-to-end implementation of `ploy transflow run` command supporting complete transformation pipelines with production-ready self-healing capabilities. It applies code transformations via OpenRewrite recipes, validates results through automated builds, creates GitLab merge requests for review, and includes sophisticated self-healing workflows with three distinct healing branch types executed via production Nomad job orchestration.

Core workflow: clone repository → create branch → apply transformations → commit changes → validate build → create/update merge request → on build failures, triggers self-healing via parallel fanout orchestration with first-success-wins semantics. The healing system supports human-step (MR-based manual intervention), llm-exec (LLM-powered code fixes), and orw-gen (OpenRewrite recipe generation) branches. Production orchestration uses SubmitAndWaitTerminal for real Nomad job submission with HCL template rendering and artifact processing.

## Key Files
- `run.go:1-250` - CLI command entry point and flag parsing
- `runner.go:1-650` - Complete orchestration logic with healing integration and ProductionBranchRunner interface implementation
- `runner.go:126-172` - Main dependency injection and initialization
- `runner.go:174-280` - Core workflow execution (Run method)
- `runner.go:282-380` - Build validation and error handling
- `runner.go:189-191` - GetTargetRepo() method implementation for human-step branch support
- `runner.go:441-500` - Self-healing workflow integration with complete three-branch fanout support
- `runner.go:509-553` - GitLab MR creation and updates
- `config.go:1-150` - Configuration loading, validation, and timeout parsing
- `integrations.go:87-200` - Factory pattern for production vs test implementations
- `types.go:1-72` - Complete job submission type system with interface definitions
- `fanout_orchestrator.go:17-27` - ProductionBranchRunner interface for asset rendering and dependency access with GetTargetRepo() method
- `types.go:60-72` - JobSubmissionHelper and FanoutOrchestrator interfaces
- `job_submission.go:1-250` - Production JobSubmissionHelper with HCL rendering and artifact parsing
- `job_submission.go:47-84` - Environment variable substitution for HCL templates
- `job_submission.go:86-98` - JSON artifact retrieval and parsing
- `job_submission.go:100-180` - Real planner/reducer job submission with SubmitAndWaitTerminal
- `fanout_orchestrator.go:1-420` - Production parallel healing branch orchestration with complete three branch type implementation
- `fanout_orchestrator.go:50-120` - First-success-wins fanout execution with proper context cancellation and timeout handling
- `fanout_orchestrator.go:127-198` - Branch execution dispatcher with comprehensive error handling and context management
- `fanout_orchestrator.go:200-246` - LLM-exec branch with HCL rendering, environment substitution, and diff.patch artifact processing
- `fanout_orchestrator.go:248-333` - ORW-gen branch with recipe configuration extraction and complete template substitution
- `fanout_orchestrator.go:335-420` - Complete human-step branch implementation with Git MR workflow, commit polling, and build validation
- `self_healing.go:1-250` - Self-healing configuration and result tracking
- `mocks.go:1-200` - Complete mock implementation framework
- `integration_test.go:1-300` - End-to-end integration test suite
- `job_submission_test.go:1-1400` - Comprehensive test coverage for all three healing branch types with complete error handling scenarios
- `job_submission_test.go:32-77` - MockProductionBranchRunner implementation with GetTargetRepo() method support
- `job_submission_test.go:450-1400` - Complete test suites covering human-step, llm-exec, and orw-gen branches with production job submission scenarios

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
- Workflow Execution: End-to-end transformation pipeline with comprehensive result tracking
- Self-Healing: Production LangGraph-based healing with complete three-branch implementation and parallel execution
- MR Integration: GitLab merge request creation/updates with rich descriptions and human-step branch support
- Test Mode: Complete mock infrastructure for CI/CD and local testing with comprehensive branch type coverage
- Job Orchestration: Production Nomad job submission with HCL template rendering and artifact processing
- Artifact Processing: JSON parsing of plan.json, next.json, and diff.patch from completed jobs
- Complete Branch Type System: Full implementation of human-step (Git+MR workflow with commit polling), llm-exec (HCL job submission with diff processing), and orw-gen (recipe generation with template substitution) healing strategies
- Production Interface Implementation: ProductionBranchRunner interface with GetTargetRepo() method for comprehensive dependency access

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
- Test mode infrastructure with comprehensive mocking and complete branch type coverage (see job_submission_test.go:32-77)
- Production job submission with HCL template rendering and environment substitution (see job_submission.go:47-84)
- Real artifact processing with JSON parsing for job outputs (see job_submission.go:86-98)
- Type-safe job submission interfaces supporting planner/reducer/branch workflows (see types.go:60-72)
- Production fanout orchestration with first-success-wins semantics and complete three-branch implementation (see fanout_orchestrator.go:50-420)
- Complete healing branch type system: human-step (Git MR workflow with commit polling and build validation), llm-exec (HCL job submission with diff.patch processing), orw-gen (recipe generation with comprehensive template substitution)
- ProductionBranchRunner interface implementation with GetTargetRepo() method for human-step branch Git operations (see runner.go:189-191, fanout_orchestrator.go:17-27)
- Context-aware cancellation and timeout handling for parallel branch execution with proper resource cleanup
- HCL template processing with environment variable substitution for job definitions
- Graceful error handling with optional MR creation (see runner.go:509-553)
- Comprehensive test coverage with mock implementations supporting all interface methods and error scenarios (see job_submission_test.go:450-1400)
- Self-healing workflow with production LangGraph integration and complete parallel branch execution via first-success-wins fanout orchestration
- Configuration validation with timeout parsing and comprehensive error reporting

## Related Documentation
- `../git/provider/CLAUDE.md` - GitLab provider implementation
- `../../api/arf/` - Application Recipe Framework integration
- `../../orchestration/CLAUDE.md` - Production job submission and monitoring infrastructure
- `../../../roadmap/transflow/jobs/` - HCL templates for planner, reducer, and healing branch jobs
- `../../../roadmap/transflow/MVP.md` - Complete implementation status and requirements
- Integration test repository: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git