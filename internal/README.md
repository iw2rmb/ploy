# Internal Shared Libraries

Reusable modules used by both API and CLI applications. These packages provide core functionality and abstractions that are shared across the Ploy platform.

## Directory Structure

```
internal/
├── analysis/                 # Static analysis components
│   └── models/              # Analysis data models
├── arf/                     # Automated Remediation Framework
│   ├── core/                # Core ARF engine
│   ├── models/              # ARF data models
│   └── recipes/             # Recipe management
├── bluegreen/               # Blue-green deployment
│   ├── bluegreen.go         # Blue-green deployment logic
│   └── traefik.go           # Traefik integration for blue-green
├── build/                   # Build scripts and management (empty directory)
├── builders/                # Build system components
├── cert/                    # Certificate management
│   └── handler.go           # SSL/TLS certificate operations
├── cleanup/                 # TTL cleanup service
│   ├── config.go            # Cleanup configuration
│   ├── handler.go           # Cleanup HTTP handlers
│   └── ttl.go               # TTL-based resource cleanup
├── cli/                     # CLI-specific modules
│   ├── analysis/            # Static analysis CLI commands
│   │   └── handler.go       # Analysis command handlers
│   ├── apps/                # Application management commands
│   │   └── handler.go       # App command handlers
│   ├── arf/                 # ARF CLI commands ✅ Aug 2025
│   │   ├── benchmark.go     # Benchmark testing commands
│   │   ├── composition.go   # Recipe composition utilities
│   │   ├── config.go        # ARF configuration management
│   │   ├── config_handler.go # Configuration command handlers
│   │   ├── errors.go        # ARF-specific error handling
│   │   ├── execution.go     # Recipe execution commands
│   │   ├── formatting.go    # Output formatting utilities
│   │   ├── handler.go       # Main ARF command handlers
│   │   ├── health.go        # Health check commands
│   │   ├── help.go          # Help and documentation commands
│   │   ├── import_export.go # Recipe import/export functionality
│   │   ├── pagination.go    # Result pagination utilities
│   │   ├── recipes.go       # Recipe management commands
│   │   ├── recipes_test.go  # Recipe tests
│   │   ├── sandbox.go       # Sandbox management commands
│   │   ├── templates.go     # Template management commands
│   │   ├── transform.go     # Transformation commands
│   │   ├── utils.go         # ARF utility functions
│   │   └── workflow.go      # Workflow management commands
│   ├── bluegreen/           # Blue-green deployment commands
│   │   └── bluegreen.go     # Blue-green deployment logic
│   ├── certs/               # Certificate management
│   │   └── handler.go       # Certificate command handlers
│   ├── common/              # Common CLI functionality
│   │   ├── deploy.go        # Shared deployment logic
│   │   └── deploy_test.go   # Deployment tests
│   ├── debug/               # Debug operations
│   │   └── handler.go       # Debug command handlers
│   ├── deploy/              # Deployment operations
│   │   ├── handler.go       # Deployment command handlers
│   │   └── handler_test.go  # Deployment handler tests
│   ├── domains/             # Domain operations
│   │   └── handler.go       # Domain command handlers
│   ├── env/                 # Environment management
│   ├── platform/            # Platform management commands
│   │   ├── handler.go       # Platform command handlers
│   │   └── handler_test.go  # Platform handler tests
│   ├── ui/                  # User interface components
│   │   └── interface.go     # CLI UI interface definitions
│   ├── utils/               # CLI utilities
│   │   └── helpers.go       # CLI helper functions
│   └── version/             # Version information
│       └── version.go       # Version command implementation
├── config/                  # Configuration management
├── debug/                   # Debug operations
│   ├── handler.go           # Application debugging utilities
│   └── handler_test.go      # Debug handler tests
├── distribution/            # Binary distribution system
│   ├── binary.go            # Binary management
│   ├── errors.go            # Distribution error types
│   ├── metadata.go          # Binary metadata handling
│   ├── pipeline.go          # Distribution pipeline
│   ├── rollback.go          # Rollback functionality
│   └── system.go            # System integration
├── domain/                  # Domain management
│   └── handler.go           # Domain configuration handlers
├── env/                     # Environment variables (empty directory)
├── envstore/                # Environment variable storage
│   ├── interface.go         # EnvStore interface definitions
│   └── memory.go            # In-memory implementation
├── errors/                  # Error handling utilities
├── git/                     # Git repository integration
│   ├── repository.go        # Git repository analysis
│   ├── repository_test.go   # Repository tests
│   ├── utils.go             # Git utilities
│   ├── utils_test.go        # Git utility tests
│   ├── validator.go         # Repository validation
│   └── validator_test.go    # Validation tests
├── kb/                      # Knowledge Base system ✅ Sep 2025
│   ├── fingerprint/         # Patch analysis and similarity detection
│   │   ├── patch.go         # Semantic fingerprinting and pattern extraction
│   │   └── patch_test.go    # Fingerprint tests
│   ├── learning/            # Learning pipeline orchestration
│   │   ├── learner.go       # Main learning engine and recommendations
│   │   └── learner_test.go  # Learning pipeline tests
│   ├── models/              # Core KB data structures
│   │   ├── case.go          # Learning case with patch and confidence
│   │   ├── case_test.go     # Case model tests
│   │   ├── error.go         # Error pattern representation
│   │   ├── error_test.go    # Error model tests
│   │   ├── summary.go       # Aggregated learning statistics
│   │   └── summary_test.go  # Summary model tests
│   └── storage/             # SeaweedFS-backed persistence
│       ├── config.go        # Storage configuration
│       ├── kb_storage.go    # Main storage operations
│       └── kb_storage_test.go # Storage tests
├── lane/                    # Lane detection system
│   ├── detector.go          # Automatic lane detection
│   └── detector_test.go     # Lane detection tests
├── lifecycle/               # Application lifecycle
│   ├── handler.go           # App creation, destruction, rollback
│   └── handler_test.go      # Lifecycle handler tests
├── monitoring/              # System monitoring
│   ├── health.go            # Health monitoring
│   ├── health_test.go       # Health check tests
│   ├── metrics.go           # Metrics collection
│   ├── metrics_test.go      # Metrics tests
│   ├── tracing.go           # Distributed tracing
│   └── tracing_test.go      # Tracing tests
├── orchestration/           # Orchestration layer
│   ├── allocations.go       # Allocation management
│   ├── client.go            # Orchestration client
│   └── types.go             # Orchestration types
├── policy/                  # Policy enforcement
│   ├── enforcer.go          # Policy enforcer
│   └── interface.go         # Policy interfaces
├── preview/                 # Preview host routing
│   └── router.go            # SHA-based preview URL handling
├── routing/                 # Routing logic
├── security/                # Security components
├── storage/                 # Object storage abstraction
│   ├── client.go            # Enhanced storage client with retry/metrics
│   ├── client_test.go       # Storage client tests
│   ├── errors.go            # Storage error types
│   ├── errors_test.go       # Error handling tests
│   ├── factory/             # Storage factory patterns
│   ├── integrity.go         # Data integrity verification
│   ├── integrity_test.go    # Integrity tests
│   ├── interface.go         # Storage interface definitions
│   ├── interface_test.go    # Interface tests
│   ├── middleware/          # Storage middleware
│   ├── monitoring.go        # Storage operation metrics
│   ├── providers/           # Storage provider implementations
│   ├── retry.go             # Retry logic and backoff
│   ├── retry_test.go        # Retry mechanism tests
│   ├── seaweedfs.go         # SeaweedFS implementation
│   ├── seaweedfs_test.go    # SeaweedFS tests
│   └── storage.go           # Storage provider interface
├── supply/                  # Supply chain components
├── testing/                 # Enhanced testing infrastructure
│   ├── assertions/          # Custom assertions
│   ├── builders/            # Test data builders
│   ├── database/            # Database test utilities
│   ├── fixtures/            # Test fixtures
│   ├── helpers/             # Test helper functions
│   ├── integration/         # Integration test utilities
│   └── mocks/               # Mock implementations
├── utils/                   # Shared utilities
│   ├── helpers.go           # Common utility functions
│   ├── helpers_test.go      # Utility function tests
│   ├── image_size.go        # Container image size utilities
│   └── image_size_test.go   # Image size tests
├── validation/              # Input validation
│   ├── app_name.go          # Application name validation
│   ├── app_name_test.go     # App name validation tests
│   ├── env_vars.go          # Environment variable validation
│   ├── env_vars_test.go     # Environment variable tests
│   ├── resources.go         # Resource constraint validation
│   └── resources_test.go    # Resource validation tests
└── version/                 # Version information
    └── version.go           # Version constants and utilities
```

## Key Packages

### analysis
Static analysis integration providing models and interfaces for code quality tools.

### arf
Automated Remediation Framework core components including the engine, data models, and recipe management.

### storage
Object storage abstraction layer providing a unified interface for different storage backends. Currently implements SeaweedFS with built-in retry logic, monitoring, middleware support, and data integrity verification.

### cli
CLI-specific functionality organized by command groups. Contains handlers for all CLI operations including app management, deployments, domains, certificates, and ARF operations.

### git
Git repository integration providing repository analysis, validation, and utilities for working with git repositories during deployments.

### kb
Knowledge Base system for the transflow MVP providing intelligent error pattern recognition, patch similarity analysis, and automated remediation recommendations. Stores learning data using SeaweedFS and builds confidence scores from historical success rates.

### lane
Automatic lane detection system that analyzes project structure to determine the appropriate deployment lane (A-G) based on technology stack and configuration files.

### monitoring
System monitoring components including health checks, metrics collection, and distributed tracing infrastructure.

### orchestration
Orchestration layer providing abstractions for container orchestration with Nomad, including allocation management and client interfaces.

### policy
Policy enforcement framework for security and compliance requirements.

### validation
Input validation utilities ensuring data integrity across the platform, including app name validation, environment variable validation, and resource constraint checking.

### testing
Comprehensive testing infrastructure with custom assertions, builders, database utilities, fixtures, and mock implementations for all major components.

## Testing

Most packages include comprehensive test coverage with `*_test.go` files. The `testing` package provides shared testing infrastructure including:
- Custom assertions for domain-specific validations
- Test data builders for complex objects
- Database testing utilities
- Fixtures for common test scenarios
- Mock implementations for external dependencies

## Usage

These internal packages are imported by both the API server (`/api/`) and CLI applications (`/cmd/`). They provide the core business logic and abstractions that enable consistent behavior across different entry points to the platform.