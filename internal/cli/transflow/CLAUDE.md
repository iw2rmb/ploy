# Transflow Service CLAUDE.md

## Purpose
Orchestrates multi-step code transformation workflows with automated build validation and GitLab merge request integration.

## Narrative Summary
The transflow service provides a complete pipeline for applying code transformations (via OpenRewrite recipes) to target repositories, validating the results through automated builds, and optionally creating GitLab merge requests for review. The service supports self-healing workflows that can automatically retry failed transformations with alternative strategies.

Core workflow: clone repository → create branch → apply transformations → commit changes → validate build → create/update merge request. The service integrates with existing ARF (Application Recipe Framework) infrastructure for recipe execution and SharedPush for build validation.

## Key Files
- `runner.go:126-597` - Main transflow orchestration logic
- `runner.go:509-553` - GitLab MR integration implementation
- `types.go:1-75` - Core data structures and configuration
- `config.go:1-120` - Configuration loading and validation
- `integrations.go:87-140` - Dependency injection and service factories
- `integrations.go:117-120` - GitLab provider factory method
- `integrations.go:133` - GitLab provider injection in runner configuration
- `job_submission.go:1-95` - Healing workflow job submission
- `self_healing.go:1-200` - Self-healing orchestration logic

## Integration Points
### Consumes
- ARF Git Operations: Repository cloning, branching, commits
- ARF Recipe Executor: OpenRewrite recipe execution
- SharedPush Build Checker: Build validation and deployment
- GitLab REST API: Merge request creation/updates (via provider.GitProvider)
- Ploy Orchestration: Job submission for healing workflows

### Provides
- CLI Commands: `ploy transflow run -f <config>`
- Workflow Execution: Complete transformation pipeline
- Self-Healing: Automatic retry with alternative strategies
- MR Integration: Automated GitLab merge request management

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

## Key Patterns
- Dependency injection pattern for testability (see runner.go:149-172)
- Factory pattern for service integration (see integrations.go:87-137)
- Optional failure pattern for MR creation (see runner.go:509-553)
- Self-healing orchestration with planner/fanout/reducer workflow

## Related Documentation
- `../git/provider/CLAUDE.md` - GitLab provider service
- `../../api/arf/` - Application Recipe Framework
- `../orchestration/` - Job submission infrastructure