# Testing Module CLAUDE.md

## Purpose
Centralized testing utilities and E2E validation framework providing comprehensive testing infrastructure for the Ploy platform with VPS production environment support.

## Architecture Overview
The testing module consolidates all test helpers, mocks, builders, and fixtures into a well-organized testing infrastructure. The module includes comprehensive E2E validation capabilities with Makefile build system integration supporting VPS production testing, complete transflow workflow validation, and GitLab integration testing.

**E2E Test Integration with Makefile Build System** - Production-ready end-to-end testing infrastructure integrated with Makefile build system providing comprehensive workflow validation. Supports VPS environment testing with real GitLab operations, transflow workflow validation including Java migration and self-healing, and distributed job orchestration testing with Nomad and Consul backends.

## Module Structure
- `mocks/` - Mock implementations for all platform interfaces
- `builders/` - Test data builders with fluent interfaces for entity creation
- `fixtures/` - Static test data and golden files for validation
- `assertions/` - Custom assertion helpers with detailed error reporting
- `helpers/` - General test helper functions for common operations
- `integration/` - Integration testing framework with VPS support
- `database/` - Database testing utilities for persistence validation
- `README.md` - Comprehensive usage documentation and migration guidelines

## Key Components
### E2E Testing Infrastructure
- Complete transflow workflow validation from CLI to GitLab MR creation
- VPS production environment testing with real service interactions
- Java migration workflow testing with OpenRewrite recipe validation
- Self-healing capability testing with KB learning integration
- GitLab integration testing with merge request operations
- Nomad job orchestration testing with distributed coordination

### Makefile Integration
- `test-e2e`: Complete E2E test suite execution on VPS
- `test-vps-environment`: VPS environment validation tests
- `test-vps-integration`: VPS integration testing with production services
- `test-vps-production`: Production readiness validation testing
- `test-e2e-vps`: E2E tests with production services and real GitLab integration
- `test-e2e-quick`: Quick E2E validation for essential workflows

### Testing Utilities
- Mock implementations for storage, orchestration, and Git providers
- Fluent builder interfaces for creating test entities
- Custom assertions with comprehensive error reporting
- Integration test framework supporting VPS environments
- Database testing utilities with transaction management

## Integration Points
### Consumes
- VPS Environment: Production testing environment for real service validation
- GitLab API: Real repository operations and merge request testing
- Nomad/Consul: Distributed job orchestration and coordination testing
- SeaweedFS: Storage backend validation for KB operations
- ARF Framework: Recipe execution and code transformation testing

### Provides
- E2E Test Framework: Complete workflow validation infrastructure
- VPS Testing Support: Production environment testing capabilities
- Makefile Integration: Build system integration for automated testing
- Mock Infrastructure: Comprehensive mock implementations for all services
- Test Utilities: Builders, assertions, and helpers for all test scenarios
- GitLab Integration Testing: Merge request creation and validation testing
- Workflow Validation: Java migration, self-healing, and KB learning testing

## Configuration
Environment variables for VPS testing:
- `TARGET_HOST` - VPS target host for production testing (default: 45.12.75.241)
- `TEST_FLAGS` - Go test flags for E2E execution
- `COVERAGE_DIR` - Coverage output directory for test results
- `GITLAB_TOKEN` - GitLab API token for integration testing
- `GITLAB_URL` - GitLab instance URL for MR operations

Makefile test targets:
- `make test-e2e` - Complete E2E test suite on VPS
- `make test-vps-integration` - VPS integration testing
- `make test-vps-production` - Production readiness validation
- `make test-e2e-quick` - Essential workflow validation (15m timeout)
- `make test-vps-all` - Complete VPS test suite execution

## Testing Patterns
- Consolidated test utilities with consistent APIs across all modules
- Fluent builder interfaces for entity creation in test scenarios
- Mock implementations following consistent patterns across services
- E2E validation with real service interactions and VPS environments
- Integration testing framework supporting distributed systems validation
- Production environment testing with comprehensive workflow coverage

## Dependencies
- External: Ginkgo framework for E2E test execution
- Internal: All platform services for integration testing
- Production: VPS environment, GitLab, Nomad, Consul, SeaweedFS
- Testing: Mock implementations, builders, assertions, fixtures

## Related Documentation
- `../../tests/e2e/` - E2E test implementations and scenarios
- `../../tests/vps/` - VPS-specific test configurations and validation
- `../cli/transflow/CLAUDE.md` - Transflow workflow testing integration
- `../validation/CLAUDE.md` - Validation framework E2E testing support
- `../../Makefile` - Build system integration and test target definitions