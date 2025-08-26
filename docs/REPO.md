# Repository Structure Guide

Quick reference for navigating Ploy's codebase. This document provides a comprehensive map of the repository structure for efficient development and troubleshooting.

## Root Level

```
ploy/
├── CHANGELOG.md         # Dated change log with Added/Fixed/Testing sections
├── CLAUDE.md            # LLM guidance and development protocols
├── go.mod               # Go module definition
├── go.sum               # Go module dependencies
├── .gitignore           # Git ignore rules
├── README.md            # Project overview
└── roadmap/             # Detailed implementation roadmaps
    ├── arf/            # Automated Remediation Framework roadmap
    │   ├── README.md           # ARF overview and phase summary
    │   ├── phase-arf-1.md      # Foundation & Core Engine
    │   ├── phase-arf-2.md      # Self-Healing Loop & Error Recovery
    │   ├── phase-arf-3.md      # LLM Integration & Hybrid Intelligence
    │   ├── phase-arf-4.md      # Security & Production Hardening
    │   └── phase-arf-5.md      # Production Features & Scale
    └── static-analysis/        # Static Analysis Integration Framework
        ├── README.md           # Framework overview and roadmap
        ├── phase-1.md          # Core Framework & Java Integration
        ├── phase-2.md          # Multi-Language Support
        ├── phase-3.md          # Advanced Integration & Enterprise
        └── phase-4.md          # Production Features & Team Collaboration
```

## Core Application Structure

### `/controller/` - Backend API Server
Main HTTP API server providing REST endpoints for application deployment and management.

```
controller/
├── main.go              # Stateless entry point with dependency injection
├── server/              # Stateless server architecture
│   ├── server.go        # Server struct with dependency injection and graceful shutdown
│   └── handlers.go      # Request handlers with injected dependencies
├── config/              # Configuration management
│   └── config.go        # Storage config loading, validation, hot reload
├── consul_envstore/     # Consul KV environment storage
│   └── consul_envstore.go
├── envstore/            # File-based environment storage
│   └── envstore.go
├── health/              # Health checking infrastructure
│   └── health.go        # Health, readiness, liveness endpoints
├── builders/            # Lane-specific image builders
│   ├── unikraft.go      # Lanes A/B - Unikraft unikernels
│   ├── java_osv.go      # Lane C - OSv/Hermit VMs for JVM
│   ├── freebsd_jail.go  # Lane D - FreeBSD jails
│   ├── oci.go           # Lane E - OCI containers
│   ├── vm.go            # Lane F - Full VMs
│   └── wasm.go          # Lane G - WebAssembly modules
├── nomad/               # HashiCorp Nomad integration
│   └── nomad.go         # Job scheduling and deployment
├── opa/                 # Open Policy Agent security
│   └── opa.go           # Security policy verification
├── supply/              # Supply chain security
│   └── supply.go        # SBOM generation, signatures
├── domains/             # Domain management (new Traefik-based)
│   └── domains.go       # Traefik service discovery integration
└── routing/             # Traffic routing management
    └── routing.go       # Traefik configuration management
```

### `/cmd/` - Command Line Applications
Command-line interfaces for different aspects of Ploy management.

```
cmd/
├── ploy/                # Application-focused CLI
│   └── main.go          # App management commands (apps, push, open, etc.)
├── ploy-wasm-runner/    # WebAssembly runtime HTTP server
│   └── main.go          # wazero-based WASM module execution
└── ployman/             # Infrastructure management CLI  
    ├── main.go          # Controller and infrastructure management
    └── controller.go    # Controller binary management commands
```

### `/internal/` - Shared Libraries
Reusable modules used by both controller and CLI.

```
internal/
├── storage/             # Object storage abstraction
│   ├── storage.go       # Storage provider interface
│   ├── client.go        # Enhanced storage client with retry/metrics
│   ├── seaweedfs.go     # SeaweedFS implementation
│   ├── s3.go            # S3-compatible storage
│   ├── retry.go         # Retry logic and backoff
│   ├── metrics.go       # Storage operation metrics
│   └── health.go        # Storage health checking
├── cli/                 # CLI-specific modules
│   ├── apps.go          # Application management commands
│   ├── deploy.go        # Deployment operations
│   ├── env.go           # Environment variable management
│   ├── domains.go       # Domain operations
│   ├── certs.go         # Certificate management
│   ├── debug.go         # Debug operations
│   ├── ui.go            # User interface helpers
│   └── utils.go         # CLI utilities
├── preview/             # Preview host routing
│   └── preview.go       # SHA-based preview URL handling
├── build/               # Build management
│   └── build.go         # Build orchestration and lane selection
├── domain/              # Domain management (legacy)
│   └── domain.go        # Domain configuration
├── cert/                # Certificate management
│   └── cert.go          # SSL/TLS certificate operations
├── env/                 # Environment variables
│   └── env.go           # Environment variable operations
├── debug/               # Debug operations
│   └── debug.go         # Application debugging
├── lifecycle/           # Application lifecycle
│   └── lifecycle.go     # App creation, destruction, rollback
├── cleanup/             # TTL cleanup service
│   ├── cleanup.go       # Main cleanup service
│   ├── config.go        # Cleanup configuration
│   └── handlers.go      # HTTP handlers for cleanup endpoints
├── git/                 # Git repository integration
│   ├── repository.go    # Git repository analysis
│   ├── validator.go     # Repository validation
│   └── utils.go         # Git utilities
├── openrewrite/         # OpenRewrite Java transformation service
│   ├── types.go         # Type definitions and interfaces
│   ├── manager.go       # Git repository manager for transformations
│   ├── executor.go      # OpenRewrite executor for Maven/Gradle
│   └── *_test.go        # Comprehensive unit tests
└── utils/               # Shared utilities
    └── utils.go         # Common utility functions
```

## Configuration and Infrastructure

### `/configs/` - Configuration Files
Application configuration templates and defaults.

```
configs/
└── storage-config.yaml  # Default storage configuration
```

### `/iac/` - Infrastructure as Code
Ansible playbooks and configuration for deployment environments. Uses unified template system for consistency between dev and prod.

```
iac/
├── common/                         # Shared infrastructure components
│   ├── playbooks/                  # Reusable playbooks
│   │   ├── controller.yml          # Controller deployment logic
│   │   ├── seaweedfs.yml          # SeaweedFS storage deployment
│   │   └── hashicorp.yml          # Nomad/Consul/Vault deployment
│   └── templates/                  # Unified Jinja2 templates
│       ├── consul-server.hcl.j2   # Linux Consul server configuration
│       ├── consul-freebsd.hcl.j2  # FreeBSD Consul client configuration
│       ├── nomad-server.hcl.j2    # Linux Nomad server configuration
│       ├── nomad-freebsd.hcl.j2   # FreeBSD Nomad client configuration
│       ├── nomad-ploy-controller.hcl.j2  # Controller Nomad job
│       ├── seaweedfs-*.service.j2  # SeaweedFS systemd services
│       ├── update-controller.sh.j2 # Controller management scripts
│       └── *.j2                    # Platform service templates
├── dev/                            # Development environment
│   ├── site.yml                    # Main orchestration playbook
│   ├── inventory/hosts.yml         # Target hosts configuration  
│   ├── playbooks/                  # Environment-specific playbooks
│   │   ├── main.yml               # Dev system setup with wildcard SSL
│   │   ├── seaweedfs.yml          # Dev SeaweedFS (mode 000)
│   │   ├── hashicorp.yml          # Dev HashiCorp stack
│   │   ├── controller.yml         # Dev controller deployment
│   │   ├── testing.yml            # Test environment setup
│   │   └── freebsd.yml            # FreeBSD VM deployment
│   └── vars/
│       ├── main.yml               # Dev configuration variables
│       └── dev-wildcard.yml       # Dev wildcard certificate config
└── prod/                           # Production environment
    ├── site.yml                    # Production orchestration playbook
    ├── inventory/hosts.yml         # Production hosts configuration
    ├── playbooks/main.yml          # Production system setup
    └── vars/
        ├── main.yml               # Production configuration variables
        └── prod-wildcard.yml      # Production wildcard certificate config
```

### `/platform/` - Platform Configuration
Platform-specific deployment configurations.

```
platform/
├── nomad/                          # Nomad job definitions
│   ├── ploy-controller.hcl         # Production system job for Ploy Controller
│   ├── ploy-controller-simple.hcl  # Simplified service job for testing
│   ├── traefik-simple.hcl          # Basic Traefik configuration
│   ├── traefik-system.hcl          # System Traefik with Docker
│   ├── traefik-system-rawexec.hcl  # System Traefik with raw exec
│   └── templates/                  # Nomad job templates for deployment lanes
│       ├── lane-a-unikraft.hcl.j2  # Lane A - Unikraft minimal
│       ├── lane-b-unikraft.hcl.j2  # Lane B - Unikraft POSIX
│       ├── lane-c-osv.hcl.j2       # Lane C - OSv JVM
│       ├── lane-d-jail.hcl.j2      # Lane D - FreeBSD jails
│       ├── lane-e-oci.hcl.j2       # Lane E - OCI containers
│       ├── lane-f-vm.hcl.j2        # Lane F - Full VMs
│       └── wasm-app.hcl.j2         # Lane G - WebAssembly applications
```

## Development and Testing

### `/build/` - Binary Build Output (Git Ignored)
Compiled binaries and build artifacts.

```
build/                      # Created during build process
├── controller              # Controller binary
├── ploy                    # CLI binary
└── kraft/                  # Unikraft build tools
    ├── gen_kraft_yaml.sh   # Kraft YAML generator
    └── build_unikraft.sh   # Unikraft build script
```

### `/scripts/` - Build and Automation Scripts
Shell scripts for build automation and deployment.

```
scripts/
└── build/              # Lane-specific build scripts
    ├── kraft/          # Unikraft build scripts (Lanes A, B)
    ├── osv/            # OSv build scripts (Lane C)
    ├── jail/           # FreeBSD jail scripts (Lane D)
    ├── oci/            # OCI container scripts (Lane E)
    ├── packer/         # VM build scripts (Lane F)
    └── wasm/           # WebAssembly build scripts (Lane G)
        ├── rust-wasm32.sh      # Rust WASM compilation
        ├── go-js-wasm.sh       # Go WASM compilation
        ├── assemblyscript.sh   # AssemblyScript compilation
        └── emscripten.sh       # C/C++ Emscripten compilation
```


### `/tools/` - Development Tools
Standalone tools for development and debugging.

```
tools/
└── lane-pick/          # Automated lane selection
    └── main.go         # Lane selection algorithm
```

### `/research/` - Research and Documentation
Research materials and proof-of-concept implementations.

```
research/               # Research and experimental code
```

## Documentation

### `/docs/` - Project Documentation
Comprehensive project documentation and specifications.

```
docs/
├── REPO.md             # This file - repository structure guide
├── STACK.md            # Technology stack and dependencies
├── REST.md             # REST API implementation details
├── STORAGE.md          # Storage abstraction and configuration
├── SCENARIOS.md        # Test scenarios and use cases
├── FEATURES.md         # Feature list and capabilities
└── WASM.md             # WebAssembly compilation and Lane G
```

## Sample Applications

### `/tests/` - Testing Infrastructure
Comprehensive testing assets including scripts and reference applications.

```
tests/
├── scripts/                 # Test scripts for various scenarios
│   ├── test-*.sh           # Individual test scripts
│   └── README.md           # Test script documentation
└── apps/                   # Reference applications for testing
    ├── node-hello/         # Node.js application (Lane B/C)
    ├── go-hellosvc/        # Go application (Lane A/B)
    ├── java-ordersvc/      # Java Spring application (Lane C)
    ├── dotnet-ordersvc/    # .NET application (Lane C)
    ├── python-api/         # Python Flask application (Lane E)
    └── nginx-edge/         # Nginx static content (Lane E)
```

Each test app contains:
```
app-name/
├── src/                # Application source code
├── Dockerfile          # Container definition (if applicable)
├── manifest.yaml       # Ploy deployment manifest
└── README.md           # App-specific documentation
```

## Lane-Specific File Patterns

### Lane Detection Patterns
Files that influence automatic lane selection:

- **Lane A/B (Unikraft)**: `kraft.yaml`, `kraft.yml`, `.unikraft/`
- **Lane C (OSv/Hermit)**: `pom.xml`, `build.gradle`, `.csproj`, `project.json`
- **Lane D (FreeBSD Jail)**: `jail.conf`, `.freebsd/`, native binaries
- **Lane E (OCI Container)**: `Dockerfile`, `container.yaml`
- **Lane F (VM)**: `Vagrantfile`, `vm.yaml`, `packer.json`
- **Lane G (WASM)**: `*.wasm`, `*.wat`, `Cargo.toml` (wasm32-wasi), `package.json` (AssemblyScript), `CMakeLists.txt` (Emscripten)

### `/lanes/` - Lane-Specific Configurations
Lane-specific build configurations and templates.

```
lanes/
├── A-unikraft-minimal/  # Lane A - Minimal Unikraft
│   └── kraft.yaml       # Kraft configuration
├── B-unikraft-nodejs/   # Lane B - Node.js Unikraft
│   └── kraft.yaml       # Kraft configuration
└── B-unikraft-posix/    # Lane B - POSIX Unikraft
    └── kraft.yaml       # Kraft configuration
```

### `/manifests/` - Application Manifests
Application deployment configuration:

```
manifests/
└── java-ordersvc.yaml   # Java order service manifest
```

### `/policies/` - Security Policies
OPA security policies.

```
policies/
└── wasm.rego            # WebAssembly security policy
```

### `/test-results/` - Test Execution Results
Stored test execution results and reports.

```
test-results/
└── arf-phase4/          # ARF Phase 4 test results
    ├── compliance-status.json
    ├── container-scan.json
    ├── security-report.json
    ├── sbom-*.json
    └── [other test results]
```

### `/vscode-arf-extension/` - VS Code Extension
ARF VS Code extension for development.

```
vscode-arf-extension/
├── package.json         # Extension manifest
└── src/
    └── extension.ts     # Extension entry point
```

## Key File Locations Quick Reference

### Configuration
- Storage config: `/etc/ploy/storage/config.yaml` (external) or `configs/storage-config.yaml` (default)
- Cleanup config: Environment-specified via `PLOY_CLEANUP_CONFIG`
- Ansible vars: `iac/dev/vars/main.yml`

### Health and Monitoring
- Health endpoints: `controller/health/health.go`
- Storage monitoring: `internal/storage/monitoring.go`
- TTL cleanup: `internal/cleanup/ttl.go`, `controller/coordination/ttl_cleanup.go`

### API Endpoints
- Main router: `controller/main.go:35-248`
- Health: `/health`, `/ready`, `/live`, `/health/metrics`
- Apps: `/v1/apps/*`
- Storage: `/v1/storage/*`
- Domains: `/v1/apps/:app/domains/*`

### Build and Deployment
- Lane selection: `tools/lane-pick/main.go`
- Build triggers: Handled through controller builders
- Nomad jobs: `controller/nomad/client.go`, `controller/nomad/submit.go`
- Storage operations: `internal/storage/client.go`, `internal/storage/seaweedfs.go`

## Development Workflow File Locations

1. **Feature Implementation**: Start with `roadmap/README.md` to identify requirements
2. **API Changes**: Update `controller/main.go` and document in `controller/README.md`
3. **CLI Changes**: Modify `cmd/ploy/main.go` and update `cmd/ploy/README.md`
4. **Storage Changes**: Edit files in `internal/storage/`
5. **Infrastructure**: Update `iac/dev/playbooks/` and `platform/`
6. **Testing**: Add tests to `tests/scripts/` and update `tests/scripts/README.md`
7. **Documentation**: Update relevant files in `docs/` and `CHANGELOG.md`

This structure enables efficient navigation and quick location of relevant files for any development task.