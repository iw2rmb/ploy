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

**For detailed API structure and folder organization, see [`api/README.md`](../api/README.md).**

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
Reusable modules used by both API and CLI applications. These packages provide core functionality and abstractions shared across the Ploy platform.

**For detailed internal package structure and documentation, see [`internal/README.md`](../internal/README.md).**

Key packages include:
- **storage/** - Object storage abstraction (SeaweedFS implementation)
- **cli/** - CLI-specific modules and command handlers
- **git/** - Git repository integration and validation
- **lane/** - Automatic lane detection system
- **monitoring/** - Health checks, metrics, and tracing
- **validation/** - Input validation utilities
- **testutil/** & **testutils/** - Testing infrastructure and mocks

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

**For detailed infrastructure structure and documentation, see [`iac/README.md`](../iac/README.md).**

Key components:
- **common/** - Shared playbooks, scripts, and 40+ Jinja2 templates
- **dev/** - Development environment with 11 playbooks including ARF and OpenRewrite support
- **local/** - Local development setup with Docker Compose
- **prod/** - Production environment configuration

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
10. **OpenRewrite Services**: Use `services/openrewrite/` for transformation services

This structure enables efficient navigation and quick location of relevant files for any development task while supporting the expanded ARF, CHTTP, and OpenRewrite capabilities.