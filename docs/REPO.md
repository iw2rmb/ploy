# Repository Structure Guide

Quick reference for navigating Ploy's codebase. This document provides a comprehensive map of the repository structure for efficient development and troubleshooting.

## Root Level

```
ploy/
├── CHANGELOG.md              # Dated change log with Added/Fixed/Testing sections
├── CLAUDE.md                 # LLM guidance and development protocols
├── README.md                 # Project overview
├── WARP.md                   # Warp-specific deployment instructions
├── Makefile                  # Build automation and test commands
├── go.mod                    # Go module definition
├── go.sum                    # Go module dependencies
├── .gitignore                # Git ignore rules
├── coverage.yml              # Code coverage configuration
└── roadmap/                  # Detailed implementation roadmaps
    ├── arf/                  # Automated Remediation Framework roadmap
    │   ├── README.md                 # ARF overview and phase summary
    │   ├── phase-arf-1.md            # Foundation & Core Engine
    │   ├── phase-arf-2.md            # Self-Healing Loop & Error Recovery
    │   ├── phase-arf-3.md            # LLM Integration & Hybrid Intelligence
    │   ├── phase-arf-4.md            # Security & Production Hardening
    │   ├── phase-arf-5.md            # Production Features & Scale
    │   ├── phase-arf-6.md            # Advanced Capabilities
    │   ├── phase-arf-7.md            # Enterprise Features
    │   └── phase-arf-8.md            # Future Roadmap
    ├── openrewrite/          # OpenRewrite Service Implementation ✅ Aug 2025
    │   ├── README.md                 # Service roadmap and three-stream approach
    │   ├── api-specification.md      # HTTP API specification
    │   ├── benchmark-java11.md       # Java 11→17 migration test scenarios
    │   ├── stream-a-core.md          # Core transformation pipeline
    │   ├── stream-b-infrastructure.md # Distributed infrastructure
    │   └── stream-c-production.md    # Production readiness features
    ├── static-analysis/      # Static Analysis Integration Framework
    │   ├── README.md                 # Framework overview and roadmap
    │   ├── phase-1.md                # Core Framework & Java Integration
    │   ├── phase-2.md                # Multi-Language Support
    │   ├── phase-3.md                # Advanced Integration & Enterprise
    │   ├── phase-4.md                # Production Features & Team Collaboration
    │   └── migrate-to-chttp.md       # CHTTP migration strategy
    ├── tdd/                  # Test-Driven Development roadmap
    │   ├── README.md                 # TDD framework overview
    │   ├── phase-tdd-1-foundation.md # Foundation & Setup
    │   ├── phase-tdd-2-unit-testing.md # Unit Testing Framework
    │   ├── phase-tdd-3-integration.md # Integration Testing
    │   └── phase-tdd-4-behavioral.md # Behavioral Testing
    └── cli-over-http/        # CLI-over-HTTP architecture
        └── server.md                 # Server implementation details
```

## Core Application Structure

### `/api/` - Backend API Server
Main HTTP API server providing REST endpoints for application deployment and management.

```
api/
├── main.go                   # API server entry point with dependency injection
├── README.md                 # API documentation and endpoints
├── server/                   # HTTP server architecture
│   ├── server.go             # Server struct with graceful shutdown
│   ├── handlers.go           # Primary request handlers
│   ├── handlers_test.go      # Handler unit tests
│   └── performance_handlers.go # Performance monitoring endpoints
├── config/                   # Configuration management
│   └── config.go             # Configuration loading and validation
├── health/                   # Health checking infrastructure
│   └── health.go             # Health, readiness, liveness endpoints
├── metrics/                  # Metrics collection and monitoring
│   └── metrics.go            # Prometheus metrics integration
├── builders/                 # Lane-specific image builders
│   ├── debug.go              # Debug utilities for builders
│   ├── jail.go               # Lane D - FreeBSD jails
│   ├── jail_test.go          # FreeBSD jail builder tests
│   ├── java_osv.go           # Lane C - OSv/Hermit VMs for JVM
│   ├── java_osv_test.go      # OSv builder tests
│   ├── oci.go                # Lane E - OCI containers
│   ├── oci_test.go           # OCI container builder tests
│   ├── unikraft.go           # Lanes A/B - Unikraft unikernels
│   ├── unikraft_test.go      # Unikraft builder tests
│   ├── utils.go              # Builder utilities
│   ├── utils_test.go         # Utility function tests
│   ├── vm.go                 # Lane F - Full VMs
│   ├── vm_test.go            # VM builder tests
│   ├── wasm.go               # Lane G - WebAssembly modules
│   └── wasm_test.go          # WebAssembly builder tests
├── nomad/                    # HashiCorp Nomad integration
│   ├── client.go             # Nomad API client
│   ├── health.go             # Nomad cluster health checks
│   ├── render.go             # Job template rendering
│   ├── render_test.go        # Template rendering tests
│   ├── submit.go             # Job submission to Nomad
│   └── submit_enhanced.go    # Enhanced job submission with features
├── opa/                      # Open Policy Agent security
│   └── verify.go             # Security policy verification
├── supply/                   # Supply chain security
│   ├── sbom.go               # SBOM (Software Bill of Materials) generation
│   ├── signing.go            # Artifact signing
│   └── verify.go             # Signature verification
├── domains/                  # Domain management
│   └── handler.go            # Domain configuration handlers
├── routing/                  # Traffic routing management
│   └── traefik.go            # Traefik configuration management
├── certificates/             # SSL/TLS certificate management
│   ├── manager.go            # Certificate lifecycle management
│   └── wildcard.go           # Wildcard certificate handling
├── dns/                      # DNS provider integration
│   ├── cloudflare.go         # Cloudflare DNS integration
│   ├── handler.go            # DNS handler endpoints
│   ├── namecheap.go          # Namecheap DNS integration
│   └── provider.go           # DNS provider interface
├── acme/                     # ACME protocol implementation
│   ├── client.go             # ACME client for certificate management
│   ├── handler.go            # ACME HTTP challenge handlers
│   ├── renewal.go            # Automatic certificate renewal
│   └── storage.go            # Certificate storage management
├── envstore/                 # Environment variable storage
│   ├── interface.go          # Storage interface definition
│   ├── store.go              # File-based environment storage
│   └── store_test.go         # Environment store tests
├── consul_envstore/          # Consul KV environment storage
│   └── store.go              # Consul-based environment storage
├── coordination/             # Distributed coordination
│   ├── leader.go             # Leader election and coordination
│   └── ttl_cleanup.go        # TTL-based resource cleanup
├── selfupdate/               # Self-updating capability
│   ├── executor.go           # Update execution logic
│   ├── handler.go            # Update API endpoints
│   └── utils.go              # Update utilities
├── version/                  # Version management
│   └── handler.go            # Version information endpoints
├── templates/                # Template management
│   └── handler.go            # Template processing endpoints
├── performance/              # Performance optimization
│   ├── balancer.go           # Load balancing
│   ├── cache.go              # Response caching
│   └── pool.go               # Resource pooling
├── runtime/                  # Runtime environments
│   └── wasm.go               # WebAssembly runtime integration
├── wasm/                     # WebAssembly components
│   └── components.go         # WASM component management
├── analysis/                 # Static analysis integration ✅ Aug 2025
│   ├── analyzers/            # Language-specific analyzers
│   │   ├── java/             # Java analysis tools
│   │   │   └── errorprone.go # ErrorProne integration
│   │   └── python/           # Python analysis tools
│   │       ├── pylint.go     # Pylint integration
│   │       └── pylint_test.go # Pylint tests
│   ├── arf_integration.go    # ARF integration
│   ├── cache.go              # Analysis result caching
│   ├── chttp_adapter.go      # CHTTP service adapter
│   ├── chttp_adapter_test.go # CHTTP adapter tests
│   ├── engine.go             # Analysis engine
│   ├── handler.go            # Analysis API endpoints
│   └── types.go              # Analysis type definitions
└── arf/                      # Automated Remediation Framework ✅ Aug 2025
    ├── BENCHMARK_STATUS.md           # ARF benchmark status and results
    ├── benchmark_configs/            # Benchmark configuration files
    │   ├── java11to17_migration.yaml # Java 11→17 migration benchmarks
    │   └── minimal_test.yaml         # Minimal test configuration
    ├── examples/                     # ARF recipe examples
    │   ├── code-cleanup.yaml         # Code cleanup recipes
    │   ├── java11to17-migration.yaml # Java migration recipes
    │   ├── security-patches.yaml     # Security patch recipes
    │   ├── spring-boot3-upgrade.yaml # Spring Boot upgrade recipes
    │   ├── template-composite.yaml   # Composite transformation templates
    │   ├── template-openrewrite.yaml # OpenRewrite integration templates
    │   └── template-shell.yaml       # Shell command templates
    ├── models/                       # ARF data models
    │   ├── execution_config.go       # Execution configuration models
    │   ├── recipe.go                 # Recipe data structures
    │   ├── recipe_metadata.go        # Recipe metadata handling
    │   ├── recipe_step.go            # Recipe step definitions
    │   ├── validation_rules.go       # Recipe validation rules
    │   └── validation_rules_test.go  # Validation rule tests
    ├── storage/                      # ARF storage backends
    │   ├── consul_index.go           # Consul-based indexing
    │   ├── recipe_storage.go         # Recipe storage interface
    │   └── seaweedfs_storage.go      # SeaweedFS integration
    ├── validation/                   # Recipe validation
    │   ├── recipe_validator.go       # Recipe structure validation
    │   └── schema_validator.go       # Schema validation
    ├── sql/                          # Database integration
    │   ├── queries/                  # SQL queries for learning system
    │   │   ├── failure_patterns.sql  # Failure pattern queries
    │   │   ├── strategy_weights.sql  # Strategy weight queries
    │   │   ├── success_patterns.sql  # Success pattern queries
    │   │   └── transformation_outcomes.sql # Outcome tracking queries
    │   ├── schema/                   # Database schema
    │   │   └── 001_learning_system.sql # Learning system tables
    │   └── sqlc.yaml                 # SQLC configuration
    ├── db/                           # Database access layer
    │   └── learning_db.go            # Learning database operations
    └── [50+ additional ARF files]    # Core ARF implementation files
```

### `/chttp/` - CLI-over-HTTP Service ✅ Aug 2025
Standalone HTTP service for CLI command execution with enhanced security and isolation.

```
chttp/
├── README.md                 # CHTTP service documentation
├── go.mod                    # CHTTP module dependencies
├── go.sum                    # CHTTP dependency checksums
├── Dockerfile.client         # Client container definition
├── Dockerfile.pylint         # Pylint service container
├── docker-compose.yml        # Local development stack
├── cmd/                      # CHTTP command implementations
│   ├── chttp/                # Main CHTTP server
│   │   ├── main.go           # CHTTP server entry point
│   │   └── main_test.go      # Server tests
│   └── pylint-chttp/         # Pylint service implementation
│       ├── main.go           # Pylint service entry point
│       ├── main_test.go      # Pylint service tests
│       ├── docker_test.go    # Docker integration tests
│       ├── integration_test.go # Integration tests
│       └── service_test.go   # Service behavior tests
├── configs/                  # CHTTP configuration files
│   └── pylint-chttp-config.yaml # Pylint service configuration
├── internal/                 # CHTTP internal packages
│   ├── analyzers/            # Code analyzers
│   │   ├── pylint.go         # Pylint analyzer implementation
│   │   ├── pylint_test.go    # Pylint analyzer tests
│   │   └── types.go          # Analyzer type definitions
│   ├── auth/                 # Authentication and authorization
│   │   ├── manager.go        # Auth manager implementation
│   │   └── manager_test.go   # Auth manager tests
│   ├── config/               # Configuration management
│   │   ├── config.go         # Configuration loading
│   │   └── config_test.go    # Configuration tests
│   ├── errors/               # Error handling framework
│   │   ├── errors.go         # Custom error types
│   │   ├── errors_test.go    # Error handling tests
│   │   ├── middleware.go     # Error middleware
│   │   └── middleware_test.go # Middleware tests
│   ├── parsers/              # Output parsers
│   │   ├── parser.go         # Parser interface
│   │   ├── parser_test.go    # Parser tests
│   │   ├── pylint_parser.go  # Pylint output parser
│   │   ├── bandit_parser.go  # Bandit security scanner parser
│   │   ├── eslint_parser.go  # ESLint parser
│   │   ├── generic_json_parser.go # Generic JSON parser
│   │   ├── regex_parser.go   # Regex-based parser
│   │   ├── regex_parser_test.go # Regex parser tests
│   │   └── types.go          # Parser type definitions
│   ├── sandbox/              # Sandboxed execution
│   │   ├── manager.go        # Sandbox manager
│   │   └── manager_test.go   # Sandbox tests
│   ├── security/             # Security controls
│   │   ├── limiter.go        # Rate limiting
│   │   └── limiter_test.go   # Rate limiter tests
│   └── server/               # HTTP server implementation
│       ├── server.go         # CHTTP server
│       └── server_test.go    # Server tests
├── scripts/                  # CHTTP scripts
│   ├── build-docker.sh       # Docker build automation
│   └── test-chttp-client.sh  # Client testing script
├── tests/                    # CHTTP test infrastructure
│   └── integration/          # Integration testing
│       ├── framework.go      # Test framework
│       └── framework_test.go # Framework tests
├── bin/                      # Built binaries (git ignored)
└── build/                    # Build artifacts (git ignored)
```

### `/services/` - Microservices
Standalone microservices for specialized functionality.

```
services/
├── cllm/                     # Code LLM microservice ✅ Aug 2025
│   ├── cmd/                  # Service command implementations
│   │   └── server/           # CLLM server
│   │       ├── main.go       # Service entry point
│   │       └── main_test.go  # Server tests
│   ├── internal/             # Service internal packages
│   │   ├── api/              # HTTP handlers and routing
│   │   │   ├── handlers.go   # API request handlers
│   │   │   └── handlers_test.go # Handler tests
│   │   ├── config/           # Configuration management
│   │   │   ├── config.go     # Configuration structures
│   │   │   └── config_test.go # Configuration tests
│   │   ├── sandbox/          # Sandboxed execution (planned)
│   │   ├── providers/        # LLM provider implementations (planned)
│   │   ├── analysis/         # Code analysis and context (planned)
│   │   └── diff/            # Git diff generation (planned)
│   ├── configs/              # Configuration templates
│   │   ├── cllm-config.yaml # Default configuration
│   │   ├── development.yaml  # Development environment
│   │   └── production.yaml   # Production environment
│   ├── tests/                # Service-specific tests
│   │   ├── integration/      # Integration test suites
│   │   ├── fixtures/         # Test data and mock responses
│   │   └── performance/      # Basic performance tests
│   ├── Dockerfile           # Service container definition
│   ├── docker-compose.yml   # Development stack with Ollama
│   ├── Makefile            # TDD and build automation
│   ├── go.mod              # Service module dependencies
│   ├── go.sum              # Service dependency checksums
│   └── README.md           # Service documentation
└── openrewrite/              # OpenRewrite transformation service ✅ Aug 2025
    ├── Dockerfile            # Service container definition
    ├── go.mod                # Service module dependencies
    ├── go.sum                # Service dependency checksums
    ├── cmd/                  # Service command implementations
    │   └── server/           # OpenRewrite server
    │       └── main.go       # Service entry point
    ├── internal/             # Service internal packages
    │   ├── executor/         # Transformation execution
    │   │   ├── executor.go   # OpenRewrite executor
    │   │   └── types.go      # Execution type definitions
    │   ├── handlers/         # HTTP request handlers
    │   │   └── handlers.go   # Service API handlers
    │   ├── jobs/             # Job management
    │   │   ├── manager.go    # Job queue manager
    │   │   └── types.go      # Job type definitions
    │   └── storage/          # Distributed storage
    │       ├── client.go     # Storage client interface
    │       ├── consul.go     # Consul KV integration
    │       └── seaweedfs.go  # SeaweedFS integration
    └── tests/                # Service tests
        └── integration/      # Integration tests
            └── transform_test.go # Transformation tests
```

### `/cmd/` - Command Line Applications
Command-line interfaces for different aspects of Ploy management.

```
cmd/
├── ploy/                     # Application-focused CLI
│   ├── main.go               # CLI entry point and command routing
│   └── README.md             # CLI documentation and usage
├── ployman/                  # Infrastructure management CLI  
│   ├── main.go               # Infrastructure management entry point
│   └── api.go                # API binary management commands
├── ploy-wasm-runner/         # WebAssembly runtime HTTP server
│   ├── main.go               # WASM runtime server
│   └── README.md             # WASM runner documentation
├── arf-benchmark/            # ARF benchmarking tool ✅ Aug 2025
│   └── main.go               # Benchmark execution and reporting
└── resource-monitor/         # System resource monitoring
    └── main.go               # Resource monitoring daemon
```

### `/internal/` - Shared Libraries
Reusable modules used by both API and CLI applications.

```
internal/
├── storage/                  # Object storage abstraction
│   ├── storage.go            # Storage provider interface
│   ├── client.go             # Enhanced storage client with retry/metrics
│   ├── client_test.go        # Storage client tests
│   ├── seaweedfs.go          # SeaweedFS implementation
│   ├── seaweedfs_test.go     # SeaweedFS tests
│   ├── interface.go          # Storage interface definitions
│   ├── interface_test.go     # Interface tests
│   ├── retry.go              # Retry logic and backoff
│   ├── retry_test.go         # Retry mechanism tests
│   ├── monitoring.go         # Storage operation metrics
│   ├── errors.go             # Storage error types
│   ├── errors_test.go        # Error handling tests
│   ├── integrity.go          # Data integrity verification
│   └── integrity_test.go     # Integrity tests
├── cli/                      # CLI-specific modules
│   ├── apps/                 # Application management commands
│   │   └── handler.go        # App command handlers
│   ├── common/               # Common CLI functionality
│   │   ├── deploy.go         # Shared deployment logic
│   │   └── deploy_test.go    # Deployment tests
│   ├── deploy/               # Deployment operations
│   │   ├── handler.go        # Deployment command handlers
│   │   └── handler_test.go   # Deployment handler tests
│   ├── platform/             # Platform management commands
│   │   ├── handler.go        # Platform command handlers
│   │   └── handler_test.go   # Platform handler tests
│   ├── domains/              # Domain operations
│   │   └── handler.go        # Domain command handlers
│   ├── certs/                # Certificate management
│   │   └── handler.go        # Certificate command handlers
│   ├── debug/                # Debug operations
│   │   └── handler.go        # Debug command handlers
│   ├── bluegreen/            # Blue-green deployment commands
│   │   └── bluegreen.go      # Blue-green deployment logic
│   ├── analysis/             # Static analysis CLI commands ✅ Aug 2025
│   │   └── handler.go        # Analysis command handlers
│   ├── analyze/              # Code analysis commands
│   │   └── analyze.go        # Analysis execution logic
│   ├── ui/                   # User interface components
│   │   └── interface.go      # CLI UI interface definitions
│   ├── utils/                # CLI utilities
│   │   └── helpers.go        # CLI helper functions
│   ├── version/              # Version information
│   │   └── version.go        # Version command implementation
│   └── arf/                  # ARF CLI commands ✅ Aug 2025
│       ├── benchmark.go      # Benchmark testing commands
│       ├── composition.go    # Recipe composition utilities
│       ├── config.go         # ARF configuration management
│       ├── config_handler.go # Configuration command handlers
│       ├── errors.go         # ARF-specific error handling
│       ├── execution.go      # Recipe execution commands
│       ├── formatting.go     # Output formatting utilities
│       ├── handler.go        # Main ARF command handlers
│       ├── health.go         # Health check commands
│       ├── help.go           # Help and documentation commands
│       ├── import_export.go  # Recipe import/export functionality
│       ├── pagination.go     # Result pagination utilities
│       ├── recipes.go        # Recipe management commands
│       ├── sandbox.go        # Sandbox management commands
│       ├── templates.go      # Template management commands
│       ├── transform.go      # Transformation commands
│       ├── utils.go          # ARF utility functions
│       └── workflow.go       # Workflow management commands
├── preview/                  # Preview host routing
│   └── router.go             # SHA-based preview URL handling
├── build/                    # Build scripts and management (empty directory)
├── domain/                   # Domain management
│   └── handler.go            # Domain configuration handlers
├── cert/                     # Certificate management
│   └── handler.go            # SSL/TLS certificate operations
├── env/                      # Environment variables (empty directory)
├── debug/                    # Debug operations
│   └── handler.go            # Application debugging utilities
├── lifecycle/                # Application lifecycle
│   ├── handler.go            # App creation, destruction, rollback
│   └── handler_test.go       # Lifecycle handler tests
├── cleanup/                  # TTL cleanup service
│   ├── config.go             # Cleanup configuration
│   ├── handler.go            # Cleanup HTTP handlers
│   └── ttl.go                # TTL-based resource cleanup
├── git/                      # Git repository integration
│   ├── repository.go         # Git repository analysis
│   ├── repository_test.go    # Repository tests
│   ├── utils.go              # Git utilities
│   ├── utils_test.go         # Git utility tests
│   ├── validator.go          # Repository validation
│   └── validator_test.go     # Validation tests
├── lane/                     # Lane detection system
│   ├── detector.go           # Automatic lane detection
│   └── detector_test.go      # Lane detection tests
├── bluegreen/                # Blue-green deployment
│   ├── bluegreen.go          # Blue-green deployment logic
│   └── traefik.go            # Traefik integration for blue-green
├── chttp/                    # CHTTP client integration
│   ├── client.go             # CHTTP service client
│   └── client_test.go        # CHTTP client tests
├── distribution/             # Binary distribution system
│   ├── binary.go             # Binary management
│   ├── errors.go             # Distribution error types
│   ├── metadata.go           # Binary metadata handling
│   ├── pipeline.go           # Distribution pipeline
│   ├── rollback.go           # Rollback functionality
│   └── system.go             # System integration
├── monitoring/               # System monitoring
│   ├── health.go             # Health monitoring
│   ├── health_test.go        # Health check tests
│   ├── metrics.go            # Metrics collection
│   ├── metrics_test.go       # Metrics tests
│   ├── tracing.go            # Distributed tracing
│   └── tracing_test.go       # Tracing tests
├── openrewrite/              # OpenRewrite integration (empty directory)
├── testutil/                 # Test utilities
│   ├── api/                  # API testing utilities
│   │   ├── client.go         # Test API client
│   │   ├── client_test.go    # API client tests
│   │   └── scenarios.go      # Test scenarios
│   ├── builders.go           # Test builders
│   ├── fixtures.go           # Test fixtures
│   ├── helpers.go            # Test helper functions
│   ├── mocks.go              # Mock implementations
│   ├── resource_monitor.go   # Resource monitoring for tests
│   └── testutil_test.go      # Test utility tests
├── testutils/                # Enhanced test utilities
│   ├── assertions.go         # Custom assertions
│   ├── builders/             # Test data builders
│   │   └── builders.go       # Builder implementations
│   ├── database.go           # Database test utilities
│   ├── fixtures/             # Test fixtures
│   │   └── fixtures.go       # Fixture implementations
│   ├── integration/          # Integration test utilities
│   │   └── integration.go    # Integration test helpers
│   ├── mocks/                # Mock implementations
│   │   ├── consul_mock.go    # Consul mock
│   │   ├── nomad_mock.go     # Nomad mock
│   │   └── storage_mock.go   # Storage mock
│   └── testutils.go          # Main test utilities
├── utils/                    # Shared utilities
│   ├── helpers.go            # Common utility functions
│   ├── helpers_test.go       # Utility function tests
│   ├── image_size.go         # Container image size utilities
│   └── image_size_test.go    # Image size tests
├── validation/               # Input validation
│   ├── app_name.go           # Application name validation
│   ├── app_name_test.go      # App name validation tests
│   ├── env_vars.go           # Environment variable validation
│   ├── env_vars_test.go      # Environment variable tests
│   ├── resources.go          # Resource constraint validation
│   └── resources_test.go     # Resource validation tests
└── version/                  # Version information
    └── version.go            # Version constants and utilities
```

## Configuration and Infrastructure

### `/configs/` - Configuration Files
Application configuration templates and defaults.

```
configs/
├── storage-config.yaml           # Default storage configuration
├── arf-hybrid-pipeline.yaml      # ARF hybrid pipeline configuration ✅ Aug 2025
├── arf-learning-config.yaml      # ARF learning system configuration
├── arf-llm-config.yaml           # ARF LLM integration configuration
├── java-errorprone-config.yaml   # Java ErrorProne analyzer configuration
├── python-pylint-config.yaml     # Python Pylint configuration
├── static-analysis-config.yaml   # Static analysis framework configuration
└── webhooks-config.yaml          # Webhook configuration
```

### `/iac/` - Infrastructure as Code
Ansible playbooks and configuration for deployment environments.

```
iac/
├── README.md                       # Infrastructure documentation
├── common/                         # Shared infrastructure components
│   ├── playbooks/                  # Reusable playbooks
│   │   ├── api.yml                 # API deployment logic
│   │   ├── seaweedfs.yml           # SeaweedFS storage deployment
│   │   └── hashicorp.yml           # Nomad/Consul/Vault deployment
│   └── templates/                  # Unified Jinja2 templates
│       ├── consul-server.hcl.j2    # Linux Consul server configuration
│       ├── consul-freebsd.hcl.j2   # FreeBSD Consul client configuration
│       ├── nomad-server.hcl.j2     # Linux Nomad server configuration
│       ├── nomad-freebsd.hcl.j2    # FreeBSD Nomad client configuration
│       ├── nomad-ploy-api.hcl.j2   # API Nomad job
│       ├── nomad-traefik-system.hcl.j2 # Traefik system job
│       ├── seaweedfs-*.service.j2  # SeaweedFS systemd services
│       ├── api-status.sh.j2        # API status monitoring script
│       ├── migrate-api.sh.j2       # API migration scripts
│       ├── rollback-api.sh.j2      # API rollback scripts
│       ├── setup-env.sh.j2         # Environment setup
│       ├── test-*.sh.j2            # Various test scripts
│       ├── update-api.sh.j2        # API update scripts
│       ├── chttp-*.j2              # CHTTP service templates
│       ├── traefik-*.yml.j2        # Traefik configuration templates
│       ├── validate-dns-records.sh.j2 # DNS validation script
│       └── [additional templates]  # Platform service templates
├── dev/                            # Development environment
│   ├── site.yml                    # Main orchestration playbook
│   ├── ansible.cfg                 # Ansible configuration
│   ├── README.md                   # Development environment documentation
│   ├── inventory/hosts.yml         # Target hosts configuration  
│   ├── playbooks/                  # Environment-specific playbooks
│   │   ├── main.yml                # Dev system setup with wildcard SSL
│   │   ├── seaweedfs.yml           # Dev SeaweedFS (mode 000)
│   │   ├── hashicorp.yml           # Dev HashiCorp stack
│   │   ├── api.yml                 # Dev API deployment
│   │   ├── chttp.yml               # CHTTP service deployment ✅ Aug 2025
│   │   ├── traefik.yml             # Traefik configuration
│   │   ├── testing.yml             # Test environment setup
│   │   └── freebsd.yml             # FreeBSD VM deployment
│   ├── scripts/                    # Development scripts
│   │   └── validate-deployment.sh  # Deployment validation
│   └── vars/
│       ├── main.yml                # Dev configuration variables
│       └── dev-wildcard.yml        # Dev wildcard certificate config
├── local/                          # Local development environment
│   ├── README.md                   # Local setup documentation
│   ├── ansible.cfg                 # Local Ansible configuration
│   ├── docker-compose.yml          # Local service stack
│   ├── inventory/localhost.yml     # Local host inventory
│   ├── playbooks/setup-macos.yml   # macOS development setup
│   ├── config/                     # Local service configurations
│   │   ├── consul.hcl              # Local Consul configuration
│   │   ├── dynamic.yml             # Dynamic configuration
│   │   ├── nomad.hcl               # Local Nomad configuration
│   │   ├── postgres-init.sql       # PostgreSQL initialization
│   │   └── traefik.yml             # Local Traefik configuration
│   └── scripts/                    # Local development scripts
│       ├── cleanup.sh              # Local cleanup
│       ├── setup.sh                # Local environment setup
│       └── wait-for-services.sh    # Service startup coordination
└── prod/                           # Production environment
    ├── site.yml                    # Production orchestration playbook
    ├── README.md                   # Production documentation
    ├── inventory/hosts.yml         # Production hosts configuration
    ├── playbooks/main.yml          # Production system setup
    └── vars/
        ├── main.yml                # Production configuration variables
        └── prod-wildcard.yml       # Production wildcard certificate config
```

### `/platform/` - Platform Configuration
Platform-specific deployment configurations.

```
platform/
├── nomad/                          # Nomad job definitions
│   ├── README.md                   # Nomad platform documentation
│   ├── ploy-api.hcl                # Production API job
│   ├── ploy-api-dynamic.hcl        # Dynamic API job configuration
│   ├── traefik.hcl                 # Traefik load balancer job
│   ├── lane-*.hcl                  # Lane-specific job templates
│   ├── debug-*.hcl                 # Debug job configurations
│   ├── validate-openrewrite-service.sh # OpenRewrite validation script
│   └── templates/                  # Nomad job templates
│       ├── arf-llm-transformation.hcl.j2 # ARF LLM transformation jobs
│       ├── arf-parallel-transformation.hcl.j2 # Parallel transformation jobs
│       └── wasm-app.hcl.j2         # WebAssembly application jobs
├── opa/                            # Open Policy Agent policies
│   └── policy.rego                 # Main security policy
├── traefik/                        # Traefik configurations
│   ├── api-load-balancer.yml       # API load balancer configuration
│   └── middlewares.yml             # Traefik middleware definitions
└── ingress/                        # Ingress configurations
    ├── certbot-hook.sh             # Certificate automation hook
    └── haproxy.cfg                 # HAProxy configuration
```

## Development and Testing

### `/bin/` - Binary Build Output (Git Ignored)
Compiled binaries and build artifacts.

```
bin/                            # Created during build process
├── api                         # API server binary
├── ploy                        # CLI binary
├── ployman                     # Infrastructure management binary
├── arf-benchmark               # ARF benchmarking tool
└── resource-monitor            # Resource monitoring daemon
```

### `/scripts/` - Build and Automation Scripts
Shell scripts for build automation, deployment, and utilities.

```
scripts/
├── build.sh                    # Main build script
├── build-openrewrite-container.sh # OpenRewrite container build
├── diagnose-ssl.sh             # SSL certificate diagnostics
├── get-api-url.sh              # API URL retrieval utility
├── setup-dev-dns.sh            # Development DNS setup
├── test-ssl-certificate.sh     # SSL certificate testing
├── update-dev-dns.sh           # DNS record updates
├── update-test-scripts.sh      # Test script maintenance
├── validate-phase1-setup.sh    # Phase 1 validation
└── build/                      # Build-specific scripts (empty)
```

### `/tools/` - Development Tools
Standalone tools for development and debugging.

```
tools/
├── lane-pick/                  # Automated lane selection
│   ├── main.go                 # Lane selection algorithm
│   ├── main_test.go            # Lane picker tests
│   ├── go.mod                  # Lane picker module
│   ├── go.sum                  # Lane picker dependencies
│   └── coverage.out            # Test coverage data
├── debug-config/               # Configuration debugging
│   └── main.go                 # Config debug utility
└── test-upload/                # Upload testing
    └── main.go                 # Upload test utility
```

## Testing Infrastructure

### `/tests/` - Comprehensive Testing Assets
Testing infrastructure including scripts, behavioral tests, and reference applications.

```
tests/
├── scripts/                        # Test execution scripts
│   ├── README.md                   # Test documentation
│   ├── test-*.sh                   # Individual test scripts (50+ files)
│   ├── test-arf-*.sh               # ARF-specific test scripts
│   ├── test-chttp-*.sh             # CHTTP service tests
│   ├── test-openrewrite-*.sh       # OpenRewrite integration tests
│   └── benchmark-*.sh              # Performance benchmark scripts
├── apps/                           # Reference applications for testing
│   ├── node-hello/                 # Node.js application (Lane B/C)
│   ├── go-hellosvc/                # Go application (Lane A/B)
│   ├── java-ordersvc/              # Java Spring application (Lane C)
│   ├── dotnet-ordersvc/            # .NET application (Lane C)
│   ├── python-apisvc/              # Python Flask application (Lane E)
│   ├── rust-hellosvc/              # Rust application
│   ├── scala-catalogsvc/           # Scala application
│   ├── wasm-*-hello/               # WebAssembly applications (Lane G)
│   │   ├── wasm-rust-hello/        # Rust WebAssembly
│   │   ├── wasm-go-hello/          # Go WebAssembly
│   │   ├── wasm-cpp-hello/         # C++ WebAssembly
│   │   └── wasm-assemblyscript-hello/ # AssemblyScript WebAssembly
│   └── test-nomad-enhanced/        # Enhanced Nomad testing
├── behavioral/                     # Behavioral/E2E tests
│   ├── suite_test.go               # Test suite configuration
│   ├── app_deployment_test.go      # Application deployment tests
│   ├── domain_certificate_test.go  # Domain and certificate tests
│   ├── e2e_lifecycle_test.go       # End-to-end lifecycle tests
│   ├── environment_management_test.go # Environment management tests
│   ├── performance_regression_test.go # Performance regression tests
│   └── bin/                        # Test binaries
├── integration/                    # Integration tests
│   ├── api_integration_test.go     # API integration tests
│   ├── build_integration_test.go   # Build system integration tests
│   ├── test-dev-deployment.sh      # Development deployment tests
│   ├── test-prod-deployment.sh     # Production deployment tests
│   ├── contract/                   # Contract testing
│   │   └── contract_test.go        # API contract tests
│   └── performance/                # Performance tests
│       ├── chttp_performance_test.go # CHTTP performance tests
│       └── load_test.go            # Load testing
├── unit/                           # Unit tests
│   ├── cleanup_test.go             # Cleanup functionality tests
│   └── integration_test_validation_test.go # Test validation
└── performance-data/               # Performance test data
    └── README.md                   # Performance data documentation
```

## Results and Artifacts

### `/test-results/` - Test Execution Results
Stored test execution results and reports.

```
test-results/
└── arf-phase4/                 # ARF Phase 4 test results
    ├── compliance-status.json
    ├── container-scan.json
    ├── security-report.json
    ├── sbom-*.json
    ├── optimization-*.json
    ├── workflow-*.json
    ├── results-*.log
    └── summary-*.txt
```

### `/benchmark_results/` - Performance Benchmarks
ARF and system performance benchmark results.

```
benchmark_results/
├── Phase1-Test-Report-20250827.md # Phase 1 comprehensive test report
└── phase1/                     # Phase 1 specific results
    ├── Investigation-Report-OpenRewrite-Issues-20250826.md
    └── Phase1-ARF-Migration-Report-20250826.md
```

### `/coverage/` - Code Coverage Reports
Test coverage data and reports.

```
coverage/
└── unit-coverage.out           # Unit test coverage data
```

## Lane-Specific Configurations

### `/lanes/` - Lane-Specific Build Configurations
Lane-specific build configurations and templates.

```
lanes/
├── A-unikraft-minimal/         # Lane A - Minimal Unikraft
│   └── kraft.yaml              # Kraft configuration
├── B-unikraft-nodejs/          # Lane B - Node.js Unikraft
│   └── kraft.yaml              # Kraft configuration
└── B-unikraft-posix/           # Lane B - POSIX Unikraft
    └── kraft.yaml              # Kraft configuration
```

### `/manifests/` - Application Manifests
Application deployment configuration examples.

```
manifests/
└── java-ordersvc.yaml          # Java order service manifest example
```

### `/policies/` - Security Policies
Open Policy Agent security policies.

```
policies/
└── wasm.rego                   # WebAssembly security policy
```

## Research and Extensions

### `/research/` - Research and Documentation
Research materials and architectural investigations.

```
research/
├── auth.md                     # Authentication research
├── cli-over-http.md            # CLI-over-HTTP architecture
├── code-transformation.md      # Code transformation research
├── distributed-paas.md         # Distributed PaaS architecture
├── http-to-protobuf.md         # Protocol buffer integration
├── paas-openrewrite.md         # OpenRewrite PaaS integration
├── protobuf-on-the-fly.md      # Dynamic protocol buffer generation
└── self-debugging-algo.md      # Self-debugging algorithm research
```

### `/vscode-arf-extension/` - VS Code Extension
ARF VS Code extension for development workflow integration.

```
vscode-arf-extension/
├── package.json                # Extension manifest and dependencies
└── src/
    └── extension.ts            # Extension entry point and functionality
```

## Documentation

### `/docs/` - Project Documentation
Comprehensive project documentation and specifications.

```
docs/
├── REPO.md                     # This file - repository structure guide
├── STACK.md                    # Technology stack and dependencies
├── STORAGE.md                  # Storage abstraction and configuration
├── FEATURES.md                 # Feature list and capabilities
├── WASM.md                     # WebAssembly compilation and Lane G
├── CERTIFICATES.md             # Certificate management documentation
└── TESTING.md                  # Testing framework and best practices
```

## Support Files

### Additional Files
Development support and metadata files.

```
ploy/
├── Dockerfile.openrewrite      # OpenRewrite service container
├── .ploy.yaml                  # Ploy deployment configuration
├── test-simple/                # Simple test application
│   ├── index.js
│   └── package.json
├── testdata/                   # Test data files
│   └── sample.json
├── test-benchmark-phase1.sh    # Phase 1 benchmark script
└── controller/                 # Legacy controller directory
    └── analysis/               # Analysis components
        └── analyzers/
```

## Lane Detection Patterns

Files that influence automatic lane selection:

- **Lane A/B (Unikraft)**: `kraft.yaml`, `kraft.yml`, `.unikraft/`
- **Lane C (OSv/Hermit)**: `pom.xml`, `build.gradle`, `.csproj`, `project.json`
- **Lane D (FreeBSD Jail)**: `jail.conf`, `.freebsd/`, native binaries
- **Lane E (OCI Container)**: `Dockerfile`, `container.yaml`
- **Lane F (VM)**: `Vagrantfile`, `vm.yaml`, `packer.json`
- **Lane G (WASM)**: `*.wasm`, `*.wat`, `Cargo.toml` (wasm32-wasi), `package.json` (AssemblyScript), `CMakeLists.txt` (Emscripten)

## Key File Locations Quick Reference

### Configuration
- Storage config: `/etc/ploy/storage/config.yaml` (external) or `configs/storage-config.yaml` (default)
- ARF config: `configs/arf-*.yaml`
- CHTTP config: `chttp/configs/pylint-chttp-config.yaml`

### Health and Monitoring
- Health endpoints: `api/health/health.go`
- Storage monitoring: `internal/storage/monitoring.go`
- ARF monitoring: `api/arf/monitoring.go`
- CHTTP monitoring: `chttp/internal/server/server.go`

### API Endpoints
- Main router: `api/main.go`
- Health: `/health`, `/ready`, `/live`, `/health/metrics`
- Apps: `/v1/apps/*`
- Storage: `/v1/storage/*`
- Domains: `/v1/apps/:app/domains/*`
- ARF: `/v1/arf/*` (recipes, benchmarks, transformations)
- Analysis: `/v1/analysis/*` (static analysis integration)

### Build and Deployment
- Lane selection: `tools/lane-pick/main.go`, `internal/lane/detector.go`
- Build triggers: `api/builders/`
- Nomad jobs: `api/nomad/client.go`, `platform/nomad/`
- Storage operations: `internal/storage/client.go`

## Development Workflow File Locations

1. **Feature Implementation**: Start with `roadmap/README.md` to identify requirements
2. **API Changes**: Update `api/main.go` and document in `api/README.md`
3. **CLI Changes**: Modify `cmd/ploy/main.go` and update `cmd/ploy/README.md`
4. **Storage Changes**: Edit files in `internal/storage/`
5. **Infrastructure**: Update `iac/dev/playbooks/` and `platform/`
6. **Testing**: Add tests to `tests/scripts/` and update `tests/scripts/README.md`
7. **Documentation**: Update relevant files in `docs/` and `CHANGELOG.md`
8. **ARF Development**: Follow `roadmap/arf/README.md` phase guidelines
9. **CHTTP Integration**: Work in `chttp/` directory with separate module
10. **OpenRewrite Services**: Use `services/openrewrite/` for transformation services

This structure enables efficient navigation and quick location of relevant files for any development task while supporting the expanded ARF, CHTTP, and OpenRewrite capabilities.