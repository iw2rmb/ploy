# Transflow CLI Module CLAUDE.md

## Purpose
Complete CLI integration for orchestrating multi-step code transformation workflows with comprehensive self-healing capabilities using three distinct branch types (human-step, llm-exec, orw-gen), production Nomad job orchestration, and GitLab merge request integration.

## Narrative Summary
The transflow module provides end-to-end implementation of `ploy transflow run` command supporting complete transformation pipelines with production-ready self-healing capabilities. It applies code transformations via OpenRewrite recipes, validates results through automated builds, creates GitLab merge requests for review, and includes sophisticated self-healing workflows with three distinct healing branch types executed via production Nomad job orchestration.

Core workflow: clone repository → create branch → apply transformations → commit changes → validate build → create/update merge request → on build failures, triggers self-healing via parallel fanout orchestration with first-success-wins semantics. The healing system supports human-step (MR-based manual intervention), llm-exec (LLM-powered code fixes), and orw-gen (OpenRewrite recipe generation) branches. Production orchestration uses SubmitAndWaitTerminal for real Nomad job submission with HCL template rendering and artifact processing.

**NEW: Model Context Protocol (MCP) Integration** - Extends LLM-exec healing branches with Model Context Protocol tool support, enabling enhanced context gathering during code transformation workflows. The system supports file system tools (mcp://fs), search tools (mcp://rg), and HTTP/HTTPS URL context sources. MCP configuration is declaratively specified in transflow YAML files and automatically converted to environment variables for containerized job execution. Context prefetching system pre-loads file patterns and web resources to improve LLM context quality during healing operations.

**NEW: KB Persistence Layer** - Implements cross-run learning system that persists error signatures, healing attempts, successful patches, and statistical summaries. The KB system enables intelligent fix recommendations based on historical success rates and provides distributed coordination via Consul locking. Error signatures are normalized using regex patterns to handle temporal/environmental variations, while summaries use weighted scoring (recency, frequency, success rate) to promote the most effective fixes.

## Key Files
- `run.go:1-250` - CLI command entry point and flag parsing
- `runner.go:1-650` - Complete orchestration logic with healing integration and ProductionBranchRunner interface implementation
- `runner.go:126-172` - Main dependency injection and initialization
- `runner.go:174-280` - Core workflow execution (Run method)
- `runner.go:282-380` - Build validation and error handling
- `runner.go:189-191` - GetTargetRepo() method implementation for human-step branch support
- `runner.go:441-500` - Self-healing workflow integration with complete three-branch fanout support
- `runner.go:509-553` - GitLab MR creation and updates
- `config.go:1-149` - Configuration loading, validation, and timeout parsing with MCP integration
- `config.go:20-25` - Extended TransflowStep with MCP fields (MCPTools, Context, Budgets)
- `config.go:91-103` - Integrated MCP configuration validation in transflow step validation
- `integrations.go:87-200` - Factory pattern for production vs test implementations
- `types.go:1-72` - Complete job submission type system with interface definitions
- `fanout_orchestrator.go:17-27` - ProductionBranchRunner interface for asset rendering and dependency access with GetTargetRepo() method
- `types.go:60-72` - JobSubmissionHelper and FanoutOrchestrator interfaces
- `job_submission.go:1-250` - Production JobSubmissionHelper with HCL rendering and artifact parsing
- `job_submission.go:47-84` - Environment variable substitution for HCL templates
- `job_submission.go:51-89` - MCP-enhanced HCL template substitution with context prefetching
- `job_submission.go:86-98` - JSON artifact retrieval and parsing
- `job_submission.go:100-180` - Real planner/reducer job submission with SubmitAndWaitTerminal
- `fanout_orchestrator.go:1-300` - Production parallel healing branch orchestration
- `fanout_orchestrator.go:44-120` - First-success-wins fanout execution with real job submission
- `fanout_orchestrator.go:209-220` - MCP configuration parsing and integration for healing branches
- `fanout_orchestrator.go:256-290` - MCP input parsing from healing branch configurations
- `self_healing.go:1-250` - Self-healing configuration and result tracking
- `mcp_integration.go:1-349` - Complete Model Context Protocol integration system with validation framework
- `mcp_integration.go:14-46` - Core MCP data structures (MCPTool, MCPConfig, MCPBudgets, MCPEnvironmentConfig)
- `mcp_integration.go:66-107` - Comprehensive validation system with endpoint, timeout, and configuration validation
- `mcp_integration.go:109-167` - MCP configuration to environment variable conversion with JSON marshaling
- `mcp_integration.go:195-349` - Context prefetching system supporting file patterns and HTTP/HTTPS URLs
- `mcp_integration.go:202-230` - MCPContextPrefetcher with workspace and context directory management
- `mcp_integration.go:232-349` - URL fetching, file pattern processing, and context manifest generation
- `mocks.go:1-200` - Complete mock implementation framework
- `integration_test.go:1-300` - End-to-end integration test suite
- `mcp_integration_test.go:1-400` - Comprehensive MCP integration test suite with performance benchmarks

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
- HCL Templates: roadmap/transflow/jobs/*.hcl for planner/reducer/branch job definitions with MCP environment variables
- Nomad API: Direct job submission with environment variable substitution and MCP configuration
- Storage Interface: SeaweedFS backend for KB case/summary/patch persistence (via storage.Storage)
- Orchestration KV: Consul key-value store for distributed locking (via orchestration.KV)
- MCP Endpoints: File system tools (mcp://fs), search tools (mcp://rg), and HTTP/HTTPS context sources
- Context Prefetching: File pattern matching and URL content retrieval for LLM context enhancement

### Provides
- CLI Commands: `ploy transflow run -f <config>` with complete flag support
- Workflow Execution: End-to-end transformation pipeline with comprehensive result tracking
- Self-Healing: Production LangGraph-based healing with complete three-branch implementation and parallel execution
- MR Integration: GitLab merge request creation/updates with rich descriptions and human-step branch support
- Test Mode: Complete mock infrastructure for CI/CD and local testing with comprehensive branch type coverage
- Job Orchestration: Production Nomad job submission with HCL template rendering and artifact processing
- Artifact Processing: JSON parsing of plan.json, next.json, and diff.patch from completed jobs
- KB Learning System: Cross-run persistence of healing cases with intelligent fix recommendations
- KB API: Storage/retrieval of error signatures, patches, and statistical summaries
- MCP Integration: Model Context Protocol tool orchestration for LLM-exec healing branches
- Context Enhancement: File and URL content prefetching for improved LLM context awareness
- MCP Configuration: Dynamic environment variable generation from YAML configuration with validation
- Default MCP Configuration: Pre-configured file-system and search tools for enhanced LLM capabilities

## Configuration
Required files:
- `transflow.yaml` - Workflow configuration with steps, target repository, and MCP tool configuration

Extended YAML Configuration (MCP fields are optional):
```yaml
steps:
  - type: llm-exec
    id: healing-step
    model: gpt-4o-mini@2024-08-06   # Optional model override
    prompts:                      # Optional additional prompts
      - "Apply best practices for error handling"
    mcp_tools:                    # Optional MCP tools configuration
      - name: file-system
        endpoint: mcp://fs
        config:                   # Optional tool-specific configuration
          max_file_size: "1MB"
      - name: search  
        endpoint: mcp://rg
        config:
          max_results: "100"
      - name: web-scraper
        endpoint: https://api.example.com/mcp/web
        config:
          timeout: "30s"
    context:                      # Optional context patterns/URLs
      - "src/**/*.java"
      - "pom.xml"
      - "https://docs.example.com/api/v2"
    budgets:                      # Optional resource budgets
      max_tokens: 8000
      max_cost: 10
      timeout: "20m"
```

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

MCP Environment Variables (automatically generated from configuration):
- `MCP_TOOLS_JSON` - JSON serialized MCP tool definitions for LLM-exec jobs
- `MCP_CONTEXT_JSON` - JSON array of context patterns and URLs for prefetching
- `MCP_ENDPOINTS_JSON` - JSON mapping of tool names to MCP endpoint URLs
- `MCP_BUDGETS_JSON` - JSON serialized resource budgets (tokens, cost, timeout)
- `MCP_PROMPTS_JSON` - JSON array of additional prompts for MCP-enhanced workflows
- `MCP_TIMEOUT` - Timeout duration for MCP operations (default: 30m)
- `MCP_SECURITY_MODE` - Security mode for MCP tools (default: allowlist)

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
- MCP-enhanced template substitution with context prefetching (see job_submission.go:51-89)
- Real artifact processing with JSON parsing for job outputs (see job_submission.go:86-98)
- Type-safe job submission interfaces supporting planner/reducer/branch workflows (see types.go:60-72)
- Production fanout orchestration with first-success-wins semantics and real Nomad jobs (see fanout_orchestrator.go:44-120)
- Branch type support for llm-exec, orw-gen, and human-step healing strategies
- MCP configuration parsing and validation with comprehensive error handling (see mcp_integration.go:66-107)
- Context prefetching system supporting file patterns and HTTP/HTTPS URLs (see mcp_integration.go:195-349)
- Environment variable generation from structured MCP configuration (see mcp_integration.go:109-167)
- MCP tool endpoint validation with protocol support (mcp://, http://, https://) (see mcp_integration.go:66-86)
- Default MCP configuration with file-system and search tools (see mcp_integration.go:169-193)
- Context manifest creation for containerized job execution (see mcp_integration.go:321-348)
- URL content fetching with timeout and error handling (see mcp_integration.go:243-281)
- File pattern processing with manifest generation (see mcp_integration.go:283-308)
- Graceful error handling with optional MR creation (see runner.go:509-553)
- Comprehensive test coverage with mock implementations supporting all interface methods and error scenarios (see job_submission_test.go:450-1400)
- Self-healing workflow with production LangGraph integration and complete parallel branch execution via first-success-wins fanout orchestration
- Configuration validation with timeout parsing and comprehensive error reporting
- KB persistence with content-addressed storage and distributed locking (see kb_storage.go, kb_locks.go)
- Error signature normalization with regex-based cleanup (see kb_signatures.go:31-100)
- Weighted scoring system for fix promotion with recency/frequency/success factors (see kb_summary.go:80-150)
- Backward compatibility maintained for non-MCP workflows with optional MCP fields in YAML configuration

## Related Documentation
- `../git/provider/CLAUDE.md` - GitLab provider implementation
- `../../api/arf/` - Application Recipe Framework integration
- `../../orchestration/CLAUDE.md` - Production job submission and monitoring infrastructure
- `../../../roadmap/transflow/jobs/` - HCL templates for planner, reducer, and healing branch jobs
- `../../../roadmap/transflow/jobs/llm_exec.hcl` - MCP-enhanced HCL template with environment variables for LLM-exec jobs
- `../../../roadmap/transflow/jobs/MCP_INTEGRATION.md` - Comprehensive MCP integration documentation and usage examples
- `../../../roadmap/transflow/MVP.md` - Complete implementation status and requirements
- Integration test repository: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git