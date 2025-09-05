# Transflow CLI Module CLAUDE.md

## Purpose
Complete CLI integration for orchestrating multi-step code transformation workflows with automated build validation, self-healing capabilities, and GitLab merge request integration.

## Narrative Summary
The transflow module provides end-to-end implementation of `ploy transflow run` command supporting complete transformation pipelines. It applies code transformations via OpenRewrite recipes, validates results through automated builds, creates GitLab merge requests for review, and includes sophisticated self-healing workflows with LangGraph-based job orchestration.

Core workflow: clone repository → create branch → apply transformations → commit changes → validate build → create/update merge request → optional healing on failures. The module integrates with ARF infrastructure for recipe execution, SharedPush for build validation, and includes comprehensive job submission infrastructure for healing workflows.

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
- `types.go:1-120` - Job submission type system for healing workflows
- `job_submission.go:1-120` - JobSubmissionHelper implementation
- `fanout_orchestrator.go:1-150` - Parallel healing branch orchestration
- `self_healing.go:1-250` - Self-healing configuration and result tracking
- `mocks.go:1-200` - Complete mock implementation framework
- `integration_test.go:1-300` - End-to-end integration test suite

## Integration Points
### Consumes
- ARF Git Operations: Repository cloning, branching, commits
- ARF Recipe Executor: OpenRewrite recipe execution
- SharedPush Build Checker: Build validation and deployment
- GitLab REST API: Merge request creation/updates (via provider.GitProvider)
- Ploy Orchestration: Job submission for healing workflows

### Provides
- CLI Commands: `ploy transflow run -f <config>` with complete flag support
- Workflow Execution: End-to-end transformation pipeline with result tracking
- Self-Healing: LangGraph-based healing with planner/reducer jobs and parallel branch execution
- MR Integration: GitLab merge request creation/updates with rich descriptions
- Test Mode: Complete mock infrastructure for CI/CD and local testing
- Job Orchestration: Nomad job submission and monitoring for healing workflows

## Configuration
Required files:
- `transflow.yaml` - Workflow configuration with steps and target repository

Environment variables:
- `GITLAB_URL` - GitLab instance URL (defaults to https://gitlab.com)
- `GITLAB_TOKEN` - GitLab API token for MR operations
- `TRANSFLOW_SUBMIT` - Set to "1" to enable job submission for healing workflows
- `TRANSFLOW_MODEL` - LLM model for healing (default: gpt-4o-mini@2024-08-06)
- `TRANSFLOW_TOOLS` - Tool configuration JSON for healing jobs
- `TRANSFLOW_LIMITS` - Execution limits JSON for healing jobs
- `NOMAD_ADDR` - Nomad cluster address for job submission

CLI flags:
- `--config, -f` - Configuration file path
- `--test-mode` - Enable mock implementations for testing
- `--dry-run` - Show what would be done without execution
- `--verbose` - Detailed output during workflow execution

## Key Patterns
- Complete dependency injection with interface-based design (see runner.go:126-172, integrations.go:87-200)
- Factory pattern for production vs test implementations (see integrations.go)
- Test mode infrastructure with comprehensive mocking (see mocks.go:1-200)
- Job submission infrastructure with type-safe interfaces (see types.go, job_submission.go)
- Fanout orchestration with first-success-wins semantics (see fanout_orchestrator.go)
- Graceful error handling with optional MR creation (see runner.go:509-553)
- Self-healing workflow with LangGraph integration and parallel branch execution
- Configuration validation with timeout parsing and comprehensive error reporting

## Related Documentation
- `../git/provider/CLAUDE.md` - GitLab provider implementation
- `../../api/arf/` - Application Recipe Framework integration
- `../../orchestration/` - Job submission and monitoring infrastructure
- `../../../roadmap/transflow/MVP.md` - Complete implementation status and requirements
- Integration test repository: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git