# E2E Workflows Testing CLAUDE.md

## Purpose
End-to-end workflow testing infrastructure providing comprehensive validation of complete transflow workflows from CLI invocation through GitLab merge request creation with VPS production environment support. **MVP COMPLETE**: All workflow components validated in production environment with real-world testing.

## Narrative Summary
The E2E workflows testing module provides complete validation infrastructure for transflow workflows including Java migration, self-healing capabilities, KB learning integration, and GitLab merge request operations. Framework supports real VPS production environment testing with actual service interactions, distributed job orchestration validation, and comprehensive workflow outcome verification.

**✅ End-to-End Workflow Testing from CLI to GitLab MR Creation** - Complete testing infrastructure validating entire transflow workflow execution including repository cloning, code transformation via OpenRewrite recipes, build validation, self-healing workflow execution, KB learning integration, and GitLab merge request creation. **VPS PRODUCTION VALIDATED**: Framework provides production-ready testing with real service interactions and VPS environment validation at TARGET_HOST=45.12.75.241.

**✅ MVP Complete workflow validation**: TransflowWorkflow execution → repository operations → code transformation → build validation → self-healing (if needed) → KB learning → GitLab MR creation → outcome verification. Framework supports all three healing strategies (human-step, llm-exec, orw-gen), production Nomad job orchestration, and comprehensive result tracking with VPS production environment validation.

## Key Files
- `types.go:1-100` - Core E2E workflow data structures and configuration
- `types.go:10-18` - TransflowWorkflow structure with repository, steps, and self-healing configuration
- `types.go:20-25` - WorkflowStep definition for recipe execution and transformation steps
- `types.go:27-31` - SelfHealConfig with KB learning integration and retry limits
- `types.go:41-60` - WorkflowResult with comprehensive outcome tracking and MR integration
- `framework.go:1-500` - E2E test execution framework with VPS environment setup
- `transflow_workflows_test.go:1-200` - Complete Java migration workflow testing with self-healing
- `mods_workflows_test.go:12-50` - TestModsE2E_JavaMigrationComplete with production repository testing
- `vps_e2e_test.go:1-300` - VPS-specific E2E validation with real service interactions

### Test Environment Infrastructure (✅ Production Operational)
- **✅ VPS Production Environment**: Complete setup with production service connections (45.12.75.241)
- **✅ GitLab Integration**: Real repository operations with merge request creation and validation using https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
- **✅ Distributed Orchestration**: Job orchestration testing with Nomad and Consul coordination in production
- **✅ KB Learning Validation**: SeaweedFS storage backend testing with real persistence and deduplication
- **✅ Build Validation**: Actual compilation and deployment verification with Java 11 to Java 17 migration testing

## Integration Points
### Consumes (✅ Production Validated)
- **✅ VPS Environment**: Production testing environment (45.12.75.241) for real service validation
- **✅ GitLab API**: Real repository operations using https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
- **✅ Mods CLI**: Complete CLI workflow execution via ploy mod run commands with KB integration
- **✅ OpenRewrite Framework**: Java migration recipe execution and code transformation (Java 11→17 validated)
- **✅ Nomad Orchestration**: Distributed job execution and monitoring for healing workflows in production
- **✅ Consul KV**: Distributed coordination and locking for KB operations with real distributed testing
- **✅ SeaweedFS Storage**: KB persistence and learning integration validation with production backend
- **✅ Build Systems**: Maven/Gradle build validation and deployment testing with actual compilation

### Provides (✅ MVP Complete)
- **✅ Complete Workflow Validation**: End-to-end testing from CLI to GitLab MR creation with production validation
- **✅ Java Migration Testing**: OpenRewrite recipe validation with Java 11 to Java 17 migration in VPS environment
- **✅ Self-Healing Validation**: KB learning integration and all three healing workflow types (human-step, llm-exec, orw-gen) tested
- **✅ VPS Production Testing**: Real environment validation with production service interactions at 45.12.75.241
- **✅ GitLab Integration Testing**: Merge request creation, updates, and validation workflows with real repository operations
- **✅ Build Validation Testing**: Complete build system integration and deployment verification with actual Maven compilation
- **✅ Outcome Verification**: Comprehensive result tracking with success/failure/healed classifications validated
- **✅ Performance Validation**: Workflow duration tracking and timeout management with acceptance criteria met

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

## Production Status

**✅ MVP COMPLETE - All E2E Components Operational:**
- **Workflow Validation**: Complete end-to-end testing from CLI through GitLab MR creation
- **VPS Production Testing**: Full validation in production environment (45.12.75.241) with real service interactions
- **Java Migration Workflows**: OpenRewrite Java 11→17 migration tested and validated with actual compilation
- **Self-Healing Testing**: All three branch types (human-step, llm-exec, orw-gen) validated in production environment
- **KB Learning Integration**: Active learning validation with SeaweedFS + Consul backend in production
- **GitLab Operations**: Real merge request creation and management with production repository validation
- **Performance Benchmarking**: Timeout management, duration tracking, and acceptance criteria met
- **Build System Integration**: Maven/Gradle compilation and deployment verification operational

**Production Test Results:**
- ✅ Workflow execution: End-to-end CLI to GitLab MR creation validated
- ✅ Self-healing workflows: All three branch types operational with real job orchestration
- ✅ KB learning integration: Automatic case recording and learning validated in production
- ✅ VPS environment testing: Complete production service validation with real interactions
- ✅ Performance validation: Acceptance testing completed with production-grade metrics
- ✅ GitLab integration: Real repository operations and merge request workflows validated

## Related Documentation
- `../vps/` - VPS-specific test configurations and production environment validation (✅ operational)
- `../../internal/mods/README.md` - Mods CLI implementation with E2E integration (MVP complete)
- `../../internal/testing/CLAUDE.md` - Testing module with E2E framework integration
- `../../internal/validation/CLAUDE.md` - Validation framework supporting E2E infrastructure
- `../../Makefile` - Build system integration with E2E test targets and VPS execution (✅ validated)
