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
Ansible playbooks and configuration for deployment environments. See `iac/dev/README.md` for complete setup guide.

```
iac/
├── dev/                            # Development environment
│   ├── site.yml                    # Main orchestration playbook
│   ├── inventory/
│   │   └── hosts.yml              # Target hosts configuration  
│   ├── playbooks/                 # Individual playbooks
│   │   ├── main.yml               # Base system setup
│   │   ├── seaweedfs.yml          # SeaweedFS storage deployment
│   │   ├── hashicorp.yml          # Nomad/Consul/Vault deployment
│   │   ├── controller.yml         # Controller Nomad deployment
│   │   ├── testing.yml            # Test environment setup
│   │   └── freebsd.yml            # FreeBSD VM deployment
│   ├── templates/                 # Jinja2 configuration templates
│   │   ├── nomad-ploy-controller.hcl.j2  # Controller Nomad job
│   │   ├── update-controller.sh.j2       # Controller update script
│   │   ├── rollback-controller.sh.j2     # Controller rollback script
│   │   ├── controller-status.sh.j2       # Controller status script
│   │   ├── migrate-controller.sh.j2      # Controller migration script
│   │   └── *.j2                          # Other configuration templates
│   └── vars/
│       └── main.yml               # Global configuration variables
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

### `/test-scripts/` - Test Automation
Executable test scripts for validation and CI/CD.

```
test-scripts/
├── test-*.sh           # Individual test scenarios
├── test-health-monitoring.sh    # Health endpoint testing
├── test-git-integration.sh      # Git workflow testing
├── test-traefik-integration.sh  # Traefik routing testing
└── test-artifact-integrity.sh   # Storage integrity testing
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
├── TESTS.md            # Test scenarios and validation
└── WASM.md             # WebAssembly compilation and Lane G
```

## Sample Applications

### `/apps/` - Reference Applications
Sample applications demonstrating each deployment lane.

```
apps/
├── node-hello/              # Node.js application (Lane B/C)
├── go-simple/               # Go application (Lane A/B)
├── java-spring/             # Java Spring application (Lane C)
├── python-flask/            # Python Flask application (Lane E)
├── wasm-rust-hello/         # Rust WASM application (Lane G)
├── wasm-go-hello/           # Go WASM application (Lane G)
├── wasm-assemblyscript-hello/  # AssemblyScript WASM application (Lane G)
└── wasm-cpp-hello/          # C++ WASM application (Lane G)
```

Each app contains:
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

### Manifest Files
Application deployment configuration:

```
manifests/
├── app-name.yaml       # Application deployment manifest
└── domains.yaml        # Domain routing configuration
```

## Key File Locations Quick Reference

### Configuration
- Storage config: `/etc/ploy/storage/config.yaml` (external) or `configs/storage-config.yaml` (default)
- Cleanup config: Environment-specified via `PLOY_CLEANUP_CONFIG`
- Ansible vars: `iac/dev/vars/main.yml`

### Health and Monitoring
- Health endpoints: `controller/health/health.go`
- Storage metrics: `internal/storage/metrics.go`
- TTL cleanup: `internal/cleanup/`

### API Endpoints
- Main router: `controller/main.go:35-248`
- Health: `/health`, `/ready`, `/live`, `/health/metrics`
- Apps: `/v1/apps/*`
- Storage: `/v1/storage/*`
- Domains: `/v1/apps/:app/domains/*`

### Build and Deployment
- Lane selection: `tools/lane-pick/main.go`
- Build triggers: `internal/build/build.go`
- Nomad jobs: `controller/nomad/nomad.go`
- Storage operations: `internal/storage/client.go`

## Development Workflow File Locations

1. **Feature Implementation**: Start with `roadmap/README.md` to identify requirements
2. **API Changes**: Update `controller/main.go` and document in `controller/README.md`
3. **CLI Changes**: Modify `cmd/ploy/main.go` and update `cmd/ploy/README.md`
4. **Storage Changes**: Edit files in `internal/storage/`
5. **Infrastructure**: Update `iac/dev/playbooks/` and `platform/`
6. **Testing**: Add tests to `test-scripts/` and scenarios to `docs/TESTS.md`
7. **Documentation**: Update relevant files in `docs/` and `CHANGELOG.md`

This structure enables efficient navigation and quick location of relevant files for any development task.