# E2E Workflows Testing CLAUDE.md

## Purpose
End-to-end workflow testing infrastructure providing comprehensive validation of complete transflow workflows from CLI invocation through GitLab merge request creation with VPS production environment support.

## Narrative Summary
The E2E workflows testing module provides complete validation infrastructure for transflow workflows including Java migration, self-healing capabilities, KB learning integration, and GitLab merge request operations. Framework supports real VPS production environment testing with actual service interactions, distributed job orchestration validation, and comprehensive workflow outcome verification.

**End-to-End Workflow Testing from CLI to GitLab MR Creation** - Complete testing infrastructure validating entire transflow workflow execution including repository cloning, code transformation via OpenRewrite recipes, build validation, self-healing workflow execution, KB learning integration, and GitLab merge request creation. Framework provides production-ready testing with real service interactions and VPS environment validation.

Core workflow validation: TransflowWorkflow execution → repository operations → code transformation → build validation → self-healing (if needed) → KB learning → GitLab MR creation → outcome verification. Framework supports multiple healing strategies (human-step, llm-exec, orw-gen), production Nomad job orchestration, and comprehensive result tracking.

## Key Files
- `types.go:1-100` - Core E2E workflow data structures and configuration
- `types.go:10-18` - TransflowWorkflow structure with repository, steps, and self-healing configuration
- `types.go:20-25` - WorkflowStep definition for recipe execution and transformation steps
- `types.go:27-31` - SelfHealConfig with KB learning integration and retry limits
- `types.go:41-60` - WorkflowResult with comprehensive outcome tracking and MR integration
- `framework.go:1-500` - E2E test execution framework with VPS environment setup
- `transflow_workflows_test.go:1-200` - Complete Java migration workflow testing with self-healing
- `transflow_workflows_test.go:12-50` - TestTransflowE2E_JavaMigrationComplete with production repository testing
- `vps_e2e_test.go:1-300` - VPS-specific E2E validation with real service interactions

### Test Environment Infrastructure
- Complete VPS environment setup with production service connections
- Real GitLab repository operations with merge request creation and validation
- Distributed job orchestration testing with Nomad and Consul coordination
- KB learning validation with SeaweedFS storage backend testing
- Build validation testing with actual compilation and deployment verification

## Integration Points
### Consumes
- VPS Environment: Production testing environment (45.12.75.241) for real service validation
- GitLab API: Real repository operations using https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
- Transflow CLI: Complete CLI workflow execution via ploy transflow run commands
- OpenRewrite Framework: Java migration recipe execution and code transformation
- Nomad Orchestration: Distributed job execution and monitoring for healing workflows
- Consul KV: Distributed coordination and locking for KB operations
- SeaweedFS Storage: KB persistence and learning integration validation
- Build Systems: Maven/Gradle build validation and deployment testing

### Provides
- Complete Workflow Validation: End-to-end testing from CLI to GitLab MR creation
- Java Migration Testing: OpenRewrite recipe validation with Java 11 to Java 17 migration
- Self-Healing Validation: KB learning integration and healing workflow testing
- VPS Production Testing: Real environment validation with production service interactions
- GitLab Integration Testing: Merge request creation, updates, and validation workflows
- Build Validation Testing: Complete build system integration and deployment verification
- Outcome Verification: Comprehensive result tracking with success/failure/healed classifications
- Performance Validation: Workflow duration tracking and timeout management

## Configuration
Test environment configuration:
- `UseRealServices: true` - Enable production service interactions for E2E validation
- `CleanupAfter: true` - Automatic test environment cleanup after execution
- `TimeoutMinutes: 15` - Maximum execution time for complete workflow validation
- `TARGET_HOST=45.12.75.241` - VPS production environment for testing

Workflow configuration examples:
- Repository: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git (production test repo)
- Recipes: org.openrewrite.java.migrate.Java11toJava17 (Java migration validation)
- Self-healing: Enabled with KB learning and 2 retry maximum
- Expected outcomes: OutcomeSuccess, OutcomeHealedSuccess, OutcomeFailure classifications

## Key Patterns
- Complete workflow execution with real service interactions and VPS environment validation
- Production repository testing with actual GitLab operations and merge request workflows
- Self-healing validation with KB learning integration and multiple healing strategies
- Comprehensive outcome tracking with success, healed success, and failure classifications
- Build validation testing with actual compilation and deployment verification
- Distributed job orchestration validation with Nomad and Consul coordination
- Timeout management with configurable duration limits and graceful failure handling
- Test environment management with automatic setup, execution, and cleanup workflows

## Related Documentation
- `../vps/` - VPS-specific test configurations and production environment validation
- `../../internal/cli/transflow/CLAUDE.md` - Transflow CLI implementation with E2E integration
- `../../internal/testing/CLAUDE.md` - Testing module with E2E framework integration
- `../../internal/validation/CLAUDE.md` - Validation framework supporting E2E infrastructure
- `../../Makefile` - Build system integration with E2E test targets and VPS execution