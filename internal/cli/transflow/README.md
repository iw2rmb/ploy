<!-- moved from CLAUDE.md: Transflow CLI documentation -->
# Transflow CLI Module

## Purpose
Production-ready CLI integration for orchestrating multi-step code transformation workflows with comprehensive self-healing capabilities using three distinct branch types (human-step, llm-exec, orw-gen), production Nomad job orchestration, GitLab merge request integration, and active Knowledge Base learning from healing attempts. MVP COMPLETE: All components now operational in production environment.

## Narrative Summary
The transflow module provides end-to-end implementation of `ploy transflow run` command supporting complete transformation pipelines with production-ready self-healing capabilities. It applies code transformations via OpenRewrite recipes, validates results through automated builds, creates GitLab merge requests for review, and includes sophisticated self-healing workflows with three distinct healing branch types executed via production Nomad job orchestration.

Core workflow: clone repository → create branch → apply transformations → commit changes → validate build → create/update merge request → on build failures, triggers self-healing via parallel fanout orchestration with first-success-wins semantics. The healing system supports human-step (MR-based manual intervention), llm-exec (LLM-powered code fixes), and orw-gen (OpenRewrite recipe generation) branches. Production orchestration uses SubmitAndWaitTerminal for real Nomad job submission with HCL template rendering and artifact processing.

NEW: Model Context Protocol (MCP) Integration - Extends LLM-exec healing branches with Model Context Protocol tool support, enabling enhanced context gathering during code transformation workflows. The system supports file system tools (mcp://fs), search tools (mcp://rg), and HTTP/HTTPS URL context sources. MCP configuration is declaratively specified in transflow YAML files and automatically converted to environment variables for containerized job execution. Context prefetching system pre-loads file patterns and web resources to improve LLM context quality during healing operations.

✅ ACTIVE: KB Learning Pipeline - Production-ready Knowledge Base learning system now actively integrated in the main transflow workflow via `KBTransflowRunner`. Every healing attempt (success or failure) is automatically recorded, analyzed, and added to the KB for future recommendations. The system provides intelligent fix suggestions based on historical success patterns, fuzzy error signature matching, and confidence scoring. Features comprehensive deduplication with Hamming distance similarity, multi-factor patch similarity detection, automated storage compaction, and distributed coordination via Consul locking. VPS VALIDATED: System operational in production environment with real-world validation.

✅ E2E Test Framework - Complete end-to-end validation framework providing comprehensive testing of entire transflow workflows from CLI invocation through GitLab MR creation. Framework validates Java migration workflows, self-healing capabilities, KB learning integration, and production Nomad job orchestration. VPS PRODUCTION VALIDATED: Supports VPS production environment testing with real GitLab integration at https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git, enabling full workflow validation including repository cloning, code transformation, build validation, healing branch execution, and merge request operations.

## Key Files
- `run.go` - CLI command entry point and flag parsing
- `runner.go` - Complete orchestration logic with healing integration and ProductionBranchRunner interface implementation
- `config.go` - Configuration loading, validation, and timeout parsing with MCP integration
- `integrations.go` - Factory pattern for production vs test implementations with KB integration
- `types.go` - Job submission type system with interface definitions
- `fanout_orchestrator.go` - ProductionBranchRunner interface for asset rendering and dependency access with GetTargetRepo() method
- `job_submission.go` - Production JobSubmissionHelper with HCL rendering and artifact parsing
- `mcp_integration.go` - MCP configuration parsing, context prefetching, and env generation

## Key Patterns
- Complete dependency injection with interface-based design
- Factory pattern for production vs test implementations
- Test mode infrastructure with comprehensive mocking
- Production job submission with HCL template rendering and environment substitution
- MCP-enhanced template substitution with context prefetching
- Real artifact processing with JSON parsing for job outputs
- Type-safe job submission interfaces supporting planner/reducer/branch workflows
- Production fanout orchestration with first-success-wins semantics and real Nomad jobs
- Branch type support for llm-exec, orw-gen, and human-step healing strategies
- MCP configuration parsing and validation with comprehensive error handling
- Context prefetching system supporting file patterns and HTTP/HTTPS URLs
- Environment variable generation from structured MCP configuration
- MCP tool endpoint validation with protocol support (mcp://, http://, https://)
- Default MCP configuration with file-system and search tools
- Context manifest creation for containerized job execution
- URL content fetching with timeout and error handling
- File pattern processing with manifest generation
- Graceful error handling with optional MR creation
- Comprehensive test coverage with mock implementations supporting all interface methods and error scenarios
- Self-healing workflow with production LangGraph integration and complete parallel branch execution via first-success-wins fanout orchestration
- Configuration validation with timeout parsing and comprehensive error reporting
- ✅ Production KB learning integration via KBTransflowRunner with automatic healing case recording - MVP complete and VPS validated
- KB persistence with content-addressed storage and distributed locking
- Production KB integration with SeaweedFS and Consul backends
- Advanced deduplication with fuzzy matching algorithms and Hamming distance similarity
- Multi-factor patch similarity detection using lexical, structural, and semantic analysis
- Automated storage compaction with intelligent case merging and retention policies
- Maintenance job orchestration with Nomad-based scheduling and resource management
- Performance monitoring with real-time deduplication metrics and query optimization tracking
- Backward compatibility preservation with comprehensive performance validation
- Weighted scoring system for fix promotion with recency/frequency/success factors
- Backward compatibility maintained for non-MCP workflows with optional MCP fields in YAML configuration

## Production Status

✅ MVP COMPLETE - All Components Operational:
- Self-Healing System: All three branch types (human-step, llm-exec, orw-gen) operational with production Nomad orchestration
- KB Learning Integration: Active learning from every healing attempt via KBTransflowRunner integration
- VPS Production Validation: Complete workflows tested and validated in production environment
- E2E Testing Framework: Full workflow validation from CLI through GitLab MR creation
- Performance Benchmarking: Acceptance testing completed with production-grade performance
- GitLab Integration: Production merge request operations with real repository validation
- Storage Backends: SeaweedFS + Consul operational for KB persistence and distributed coordination
- Job Orchestration: Nomad-based healing workflows with HCL template rendering and artifact processing

## References
- `../../../platform/nomad/transflow/` - HCL templates for planner, reducer, and healing branch jobs
- `../../../platform/nomad/transflow/llm_exec.hcl` - MCP-enhanced HCL template with environment variables for LLM-exec jobs
- `../../../platform/nomad/transflow/MCP_INTEGRATION.md` - MCP integration documentation and usage examples
- `../git/provider/README.md` - GitLab provider implementation
- `../../orchestration` - Production job submission and monitoring infrastructure
