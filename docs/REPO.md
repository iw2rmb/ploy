# Repository Structure Guide

Quick reference for navigating Ploy's codebase. This document provides a comprehensive map of the repository structure for efficient development and troubleshooting.

## Root Level

```
ploy/
├── README.md                 # Project overview
├── CHANGELOG.md              # Dated change log with Added/Fixed/Testing sections
├── AGENTS.md                 # LLM agent system configuration
├── CLAUDE.md                 # LLM guidance and development protocols
├── CLAUDE.sessions.md        # Claude AI session management guidance
├── PRODUCTION_READY_STATUS.md # Production readiness status
├── WARP.md                   # Warp-specific deployment instructions
├── Makefile                  # Build automation and test commands
├── go.mod                    # Go module definition
├── go.sum                    # Go module dependencies
├── docker-compose.integration.yml # Integration testing environment
├── test-mod-java11to17.yaml # Mods test configuration
├── .gitignore                # Git ignore rules
└── roadmap/                  # Detailed implementation roadmaps
    ├── README.md             # Main roadmap overview and progress
    ├── arf/                  # Automated Remediation Framework (see roadmap/arf/README.md)
    ├── openrewrite/          # OpenRewrite Service Implementation (see roadmap/openrewrite/README.md)
    ├── static-analysis/      # Static Analysis Integration (see roadmap/static-analysis/README.md)
    ├── tdd/                  # Test-Driven Development (see roadmap/tdd/README.md)
    ├── mods/                 # Mods transformation workflows (see roadmap/mods/README.md)
    ├── transformations/      # Code transformation strategies (see roadmap/transformations/README.md)
    ├── refactor/             # Refactoring initiatives (see roadmap/refactor/README.md)
    └── storage-fix/          # Storage system improvements (see roadmap/storage-fix/README.md)
```

## Core Application Structure

### `/api/` - Backend API Server
Main HTTP API server providing REST endpoints for application deployment and management.

**For detailed structure and component documentation, see [`api/README.md`](../api/README.md).**

Key modules include server architecture, builders for deployment lanes A-G, ARF transformation system, static analysis integration, infrastructure coordination (Nomad, Consul, Traefik), and security components (ACME, DNS, OPA).

### `/cmd/` - Command Line Applications
Command-line interfaces for different aspects of Ploy management.

```
cmd/
├── ploy/                     # Application-focused CLI (see cmd/ploy/README.md)
├── ployman/                  # Infrastructure management CLI (see cmd/ployman/README.md)
├── ploy-wasm-runner/         # WebAssembly runtime HTTP server (see cmd/ploy-wasm-runner/README.md)
├── arf-benchmark/            # ARF benchmarking tool
└── resource-monitor/         # System resource monitoring daemon
```

### `/internal/` - Shared Libraries
Reusable modules used by both API and CLI applications providing core functionality and abstractions.

**For detailed package structure and comprehensive documentation, see [`internal/README.md`](../internal/README.md).**

Key packages include storage abstraction, CLI modules, Git integration, lane detection, monitoring, validation, Knowledge Base (KB) system, and comprehensive testing infrastructure.

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

**For detailed infrastructure structure and environment documentation, see [`iac/README.md`](../iac/README.md).**

Includes common playbooks, development environment (see `iac/dev/README.md`), local development setup (see `iac/local/README.md`), and production configuration (see `iac/prod/README.md`).

### `/platform/` - Platform Configuration
Core platform configurations and job definitions for deployment orchestration.

**For detailed platform structure and deployment lanes, see [`platform/README.md`](../platform/README.md).**

Includes Nomad job definitions for deployment lanes A-G (see `platform/nomad/README.md`), Traefik load balancer configuration, ingress controllers, and Open Policy Agent security policies.

### `/docker/` - Container Configurations
Docker configurations and container definitions.

```
docker/
└── openrewrite/              # OpenRewrite container configurations
```

### `/services/` - External Service Implementations
Standalone service implementations and integrations.

```
services/
└── openrewrite-jvm/          # OpenRewrite JVM service implementation
```

### `/sessions/` - Claude AI Session Management
Claude AI session management and task tracking system.

```
sessions/
├── sessions-config.json      # Session configuration
├── tasks/                    # Task definitions and tracking
├── protocols/                # Session management protocols
└── knowledge/                # Session knowledge base
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

### `/test-results/` - Test Execution Artifacts (Git Ignored)
Test execution results, coverage reports, and benchmark data.

### `/scripts/` - Build and Automation Scripts
Shell scripts for build automation, deployment, and development utilities.

**For detailed script organization and execution guidance, see [`scripts/README.md`](../scripts/README.md).**

Includes main build automation, OpenRewrite container builds, SSL certificate management, DNS configuration, environment validation, phase-based execution scripts, and lane-specific build helpers organized by deployment target.

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
Testing infrastructure organized by scope and environment.

**For detailed test structure and execution guidance, see [`tests/README.md`](../tests/README.md).**

Includes test scripts (see `tests/scripts/README.md`), reference applications for all deployment lanes, unit/integration/e2e/behavioral tests, VPS production validation, and performance benchmarking.

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

### Additional Development Files
Development support and metadata files.

```
ploy/
├── testdata/                   # Test data files
└── test-benchmark-phase1.sh   # Phase 1 benchmark script
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

### Main Entry Points
- **API Server**: `api/main.go`
- **CLI Applications**: `cmd/*/main.go`
- **Lane Selection**: `tools/lane-pick/main.go`, `internal/lane/detector.go`

### Configuration
- **Default Configs**: `configs/*.yaml`
- **Platform Jobs**: `platform/nomad/*.hcl`
- **Infrastructure**: `iac/*/`

### API Endpoints
- Health: `/health`, `/ready`, `/live`
- Apps: `/v1/apps/*`
- ARF: `/v1/arf/*` (recipes, transformations)
- Analysis: `/v1/analysis/*`

## Development Workflow

1. **Feature Planning**: Start with `roadmap/README.md` and specific roadmap directories
2. **API Development**: See `api/README.md` for server architecture and modules
3. **CLI Development**: See `cmd/*/README.md` for command-line interfaces
4. **Internal Libraries**: See `internal/README.md` for shared components
5. **Infrastructure**: See `iac/README.md` for deployment environments
6. **Platform Configuration**: See `platform/README.md` for deployment lanes
7. **Testing**: See `tests/README.md` for comprehensive test execution
8. **Documentation**: Update relevant files in `docs/` and `CHANGELOG.md`

This structure enables efficient navigation through README-driven documentation in each directory, supporting the full platform capabilities including ARF, Mods, and multi-lane deployment architecture.
