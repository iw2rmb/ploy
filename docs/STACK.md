# Ploy Technology Stack

## Core Infrastructure
- **FreeBSD** — Host OS providing bhyve hypervisor, ZFS storage, and jail isolation
- **bhyve** — FreeBSD's native hypervisor for VM-based lane execution
- **ZFS** — Copy-on-write filesystem for efficient storage and snapshots
- **Docker** — Container runtime for Lane E (OCI+Kontain) builds

## Orchestration & Service Mesh
- **HashiCorp Nomad** — Job scheduler and workload orchestrator for all lanes
- **HashiCorp Consul** — Service discovery, configuration, and connect mesh
- **HashiCorp Vault** — Secrets management and PKI for secure deployments

## Unikernel & VM Technologies
- **Unikraft** — Modular unikernel framework for Lanes A/B (ultra-fast boot)
- **KraftKit** — Unikraft build tool and package manager
- **OSv** — Java/.NET-optimized unikernel platform for Lane C
- **Hermit** — Rust-based unikernel runtime alternative for Lane C
- **Capstan** — OSv image build and management tool

## WASM Runtime Technologies (Lane G - Fully Implemented)
- **wazero v1.5.0** — Production-deployed pure Go WebAssembly runtime (no CGO dependencies)
- **WASI Preview 1** — Implemented WebAssembly System Interface for controlled filesystem and environment access
- **WebAssembly Component Model** — Multi-module WASM application support with dependency management
- **wasm-bindgen** — Rust to WebAssembly binding generator for browser and WASI targets
- **js-sys/web-sys** — JavaScript and Web API bindings for Rust WASM modules
- **AssemblyScript Compiler** — TypeScript-like syntax compiled to optimized WebAssembly
- **Emscripten SDK** — C/C++ to WebAssembly toolchain with WASI and browser targets

## Container Security & Isolation
- **Kontain** — Lightweight VM isolation for OCI containers (Lane E)
- **Firecracker** — MicroVM technology for secure container execution
- **FreeBSD Jails** — OS-level virtualization for Lane D native apps

## Build & Packaging Tools
- **Go** — Primary language for Ploy api and CLI
- **Jib** — Containerless Java/Scala builds for efficient Lane C/E selection
- **Gradle/Maven** — Java ecosystem build tools with Jib integration
- **NPM/Node.js** — JavaScript runtime and package management
- **Python** — Scripting and application runtime with C-extension detection
- **Cargo** — Rust package manager with production wasm32-wasi target support and cdylib crate types
- **AssemblyScript Compiler** — Production-ready TypeScript-like language compiling to optimized WebAssembly
- **Emscripten SDK** — Complete C/C++ to WebAssembly compilation toolchain with WASI and browser support
- **wasm-pack** — Rust-generated WebAssembly package builder with npm integration
- **wasm-bindgen-cli** — Command-line tool for generating JavaScript bindings for Rust WASM modules
- **Pyodide** — Python scientific stack compiled to WebAssembly (future support planned)

## Automated Remediation Framework (ARF)
- **OpenRewrite** — Semantic-aware Java transformation engine with 2,800+ recipes
- **Error Prone** — Google's compile-time bug detection and prevention system
- **LLM Integration** — Hybrid intelligence for complex transformation patterns
- **Tree-sitter** — Universal parsing infrastructure for multi-language AST support
- **JavaParser** — Java AST manipulation for custom recipe development
- **Lossless Semantic Trees (LST)** — OpenRewrite's format-preserving AST representation
- **Fork-Join Framework** — Java parallel processing for concurrent transformations
- **Circuit Breaker Libraries** — Hystrix/Resilience4j for failure handling patterns
- **AST Caching System** — Memory-mapped files + LRU cache for performance optimization

## Supply Chain Security
- **Syft** — Software Bill of Materials (SBOM) generation
- **Grype** — Vulnerability scanning for container images and artifacts
- **Cosign** — Container image signing and verification with keyless OIDC
- **Open Policy Agent (OPA)** — Policy enforcement for deployment security

## Container Registry & Storage
- **Docker Registry v2** — Lightweight, standards-compliant container image storage
  - **Filesystem Storage** — Local persistence for development environments
  - **Anonymous Access** — No authentication required for development workflows
  - **Traefik Integration** — Automatic SSL termination and reverse proxy routing
  - **Memory Efficiency** — 90% less memory usage vs Harbor (~256MB vs ~2GB)
  - **Nomad Deployment** — Cloud-native deployment with service discovery
  - **Benefits** — Standards-compliant Docker Registry v2 API, simplified RBAC-free access

## Storage & Networking
- **SeaweedFS** — Distributed object storage optimized for small files (artifacts, SBOMs, signatures)
  - **ARF Recipe Storage** — Recipe persistence with retry logic, caching, and deletion markers
  - **Binary Artifact Storage** — API binaries and deployment artifacts
  - **SBOM & Signature Storage** — Distributed storage for security artifacts
- **Consul** — Service discovery, configuration, and distributed coordination
  - **ARF Recipe Indexing** — Full-text search with relevance scoring for recipe discovery
  - **Configuration Management** — Environment-driven backend selection and settings
  - **Service Registration** — Dynamic service discovery and health monitoring
- **Traefik** — Cloud-native reverse proxy and load balancer with automatic service discovery
- **Let's Encrypt** — Automated TLS certificate provisioning with wildcard support
- **Consul Connect** — Service mesh networking and mTLS

## Development & CLI
- **Cobra** — Go CLI framework for `ploy` command structure
- **Bubble Tea** — Terminal UI framework for interactive CLI experiences
- **Fiber** — Go web framework for api REST API
- **Viper** — Configuration management for multi-environment setups

## CI/CD & Automation
- **GitHub Actions** — CI/CD pipelines for automated testing (deployment via ployman CLI)
- **GitLab CI** — Alternative CI/CD platform integration
- **Ansible** — Infrastructure automation and VPS provisioning
- **Packer** — VM image building for Lane F full virtualization

## Monitoring & Observability
- **Prometheus** — Metrics collection and alerting
- **Grafana** — Dashboards and visualization
- **Loki** — Log aggregation for distributed applications
- **OpenTelemetry** — Distributed tracing and observability standards
- **Node Exporter** — System metrics collection for monitoring

## Development Environment
- **QEMU** — Hardware emulation for unikernel testing and development
- **libvirt** — Virtualization management API and tooling
- **cloud-init** — VM initialization and configuration automation
- **rg (ripgrep)** — Fast text search for codebase analysis

## Testing & Quality Assurance
- **ARF Storage Test Suite** — Comprehensive integration testing for recipe storage backends
  - **Storage Integration Tests** — CRUD operations, validation, search functionality testing
  - **Backend Fallback Tests** — Graceful degradation and failover mechanism validation  
  - **Configuration Analysis Tests** — Environment detection and backend configuration verification
  - **Comprehensive Test Runner** — Master test orchestrator with JSON reporting and statistics
- **VPS Runtime Testing** — All functional testing performed on production-like VPS infrastructure
- **Go Testing Framework** — Native Go testing with compilation validation and error checking
- **JSON Test Reporting** — Structured test results with timestamps, statistics, and detailed analysis