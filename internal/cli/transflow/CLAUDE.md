# Transflow CLI Module CLAUDE.md

## Purpose
Complete CLI integration for orchestrating multi-step code transformation workflows with automated build validation, production-ready self-healing capabilities, and GitLab merge request integration.

## Narrative Summary
The transflow module provides end-to-end implementation of `ploy transflow run` command supporting complete transformation pipelines with production Nomad job submission. It applies code transformations via OpenRewrite recipes, validates results through automated builds, creates GitLab merge requests for review, and includes sophisticated self-healing workflows with real LangGraph-based job orchestration via Nomad.

Core workflow: clone repository → create branch → apply transformations → commit changes → validate build → create/update merge request → production healing workflows on failures. The module integrates with ARF infrastructure for recipe execution, SharedPush for build validation, and production orchestration infrastructure using SubmitAndWaitTerminal for real job submission with artifact retrieval.

**NEW: KB Persistence Layer** - Implements cross-run learning system that persists error signatures, healing attempts, successful patches, and statistical summaries. The KB system enables intelligent fix recommendations based on historical success rates and provides distributed coordination via Consul locking. Error signatures are normalized using regex patterns to handle temporal/environmental variations, while summaries use weighted scoring (recency, frequency, success rate) to promote the most effective fixes.

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
- `types.go:1-72` - Complete job submission type system with interfaces
- `job_submission.go:1-250` - Production JobSubmissionHelper with HCL rendering and artifact parsing
- `job_submission.go:47-84` - Environment variable substitution for HCL templates
- `job_submission.go:86-98` - JSON artifact retrieval and parsing
- `job_submission.go:100-180` - Real planner/reducer job submission with SubmitAndWaitTerminal
- `fanout_orchestrator.go:1-300` - Production parallel healing branch orchestration
- `fanout_orchestrator.go:44-120` - First-success-wins fanout execution with real job submission
- `self_healing.go:1-250` - Self-healing configuration and result tracking
- `mocks.go:1-200` - Complete mock implementation framework
- `integration_test.go:1-300` - End-to-end integration test suite

### KB Persistence Layer
- `kb_storage.go:1-310` - KB storage interface and SeaweedFS implementation for healing cases
- `kb_storage.go:17-50` - Data structures (CaseContext, HealingAttempt, HealingOutcome)
- `kb_storage.go:129-310` - SeaweedFSKBStorage with CRUD operations for cases/summaries/patches
- `kb_signatures.go:1-200` - Error signature generation and patch fingerprinting
- `kb_signatures.go:17-30` - DefaultSignatureGenerator with regex-based normalization
- `kb_locks.go:1-180` - Distributed locking via Consul for concurrent KB access
- `kb_locks.go:29-50` - ConsulKBLockManager implementation
- `kb_summary.go:1-350` - Summary computation and fix promotion logic
- `kb_summary.go:12-30` - SummaryConfig with scoring weights and thresholds
- `kb_integration.go:1-200` - Main KB integration orchestrator
- `kb_integration.go:12-36` - KBConfig and EnhancedSelfHealConfig structures

## Integration Points
### Consumes
- ARF Git Operations: Repository cloning, branching, commits
- ARF Recipe Executor: OpenRewrite recipe execution
- SharedPush Build Checker: Build validation and deployment
- GitLab REST API: Merge request creation/updates (via provider.GitProvider)
- Ploy Orchestration: Production SubmitAndWaitTerminal for real Nomad job submission
- HCL Templates: roadmap/transflow/jobs/*.hcl for planner/reducer/branch job definitions
- Nomad API: Direct job submission with environment variable substitution
- Storage Interface: SeaweedFS backend for KB case/summary/patch persistence (via storage.Storage)
- Orchestration KV: Consul key-value store for distributed locking (via orchestration.KV)

### Provides
- CLI Commands: `ploy transflow run -f <config>` with complete flag support
- Workflow Execution: End-to-end transformation pipeline with result tracking
- Self-Healing: Production LangGraph-based healing with real planner/reducer jobs and parallel branch execution
- MR Integration: GitLab merge request creation/updates with rich descriptions
- Test Mode: Complete mock infrastructure for CI/CD and local testing
- Job Orchestration: Production Nomad job submission with HCL template rendering and artifact processing
- Artifact Processing: JSON parsing of plan.json, next.json, and diff.patch from completed jobs
- KB Learning System: Cross-run persistence of healing cases with intelligent fix recommendations
- KB API: Storage/retrieval of error signatures, patches, and statistical summaries

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
- `CONSUL_ADDR` - Consul address for KB distributed locking
- SeaweedFS configuration via storage package environment variables

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
- Production fanout orchestration with first-success-wins semantics and real Nomad jobs (see fanout_orchestrator.go:44-120)
- Branch type support for llm-exec, orw-gen, and human-step healing strategies
- Graceful error handling with optional MR creation (see runner.go:509-553)
- Self-healing workflow with production LangGraph integration and parallel branch execution
- Configuration validation with timeout parsing and comprehensive error reporting
- KB persistence with content-addressed storage and distributed locking (see kb_storage.go, kb_locks.go)
- Error signature normalization with regex-based cleanup (see kb_signatures.go:31-100)
- Weighted scoring system for fix promotion with recency/frequency/success factors (see kb_summary.go:80-150)

## Related Documentation
- `../git/provider/CLAUDE.md` - GitLab provider implementation
- `../../api/arf/` - Application Recipe Framework integration
- `../../orchestration/CLAUDE.md` - Production job submission and monitoring infrastructure
- `../../../roadmap/transflow/jobs/` - HCL templates for planner, reducer, and healing branch jobs
- `../../../roadmap/transflow/MVP.md` - Complete implementation status and requirements
- Integration test repository: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git