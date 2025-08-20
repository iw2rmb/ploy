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

## WASM Runtime Technologies
- **Wazero** — Pure Go WebAssembly runtime for Lane G (no CGO dependencies)
- **Wasmtime** — Fast and secure WebAssembly runtime with WASI support
- **Wasmer** — Universal WebAssembly runtime with multiple execution engines
- **WASI Preview 1** — WebAssembly System Interface for filesystem and network access
- **Component Model** — Future standard for linking multiple WASM modules efficiently

## Container Security & Isolation
- **Kontain** — Lightweight VM isolation for OCI containers (Lane E)
- **Firecracker** — MicroVM technology for secure container execution
- **FreeBSD Jails** — OS-level virtualization for Lane D native apps

## Build & Packaging Tools
- **Go** — Primary language for Ploy controller and CLI
- **Jib** — Containerless Java/Scala builds for efficient Lane C/E selection
- **Gradle/Maven** — Java ecosystem build tools with Jib integration
- **NPM/Node.js** — JavaScript runtime and package management
- **Python** — Scripting and application runtime with C-extension detection
- **Cargo** — Rust package manager with wasm32-wasi target support
- **AssemblyScript** — TypeScript-like language that compiles to WebAssembly
- **Emscripten** — C/C++ to WebAssembly compilation toolchain
- **wasm-pack** — Rust-generated WebAssembly package builder
- **Pyodide** — Python scientific stack compiled to WebAssembly

## Supply Chain Security
- **Syft** — Software Bill of Materials (SBOM) generation
- **Grype** — Vulnerability scanning for container images and artifacts
- **Cosign** — Container image signing and verification with keyless OIDC
- **Open Policy Agent (OPA)** — Policy enforcement for deployment security

## Storage & Networking
- **SeaweedFS** — Distributed object storage optimized for small files (artifacts, SBOMs, signatures)
- **Traefik/Envoy** — Ingress controllers and load balancing
- **Let's Encrypt** — Automated TLS certificate provisioning
- **Consul Connect** — Service mesh networking and mTLS

## Development & CLI
- **Cobra** — Go CLI framework for `ploy` command structure
- **Bubble Tea** — Terminal UI framework for interactive CLI experiences
- **Fiber** — Go web framework for controller REST API
- **Viper** — Configuration management for multi-environment setups

## CI/CD & Automation
- **GitHub Actions** — CI/CD pipelines for automated testing and deployment
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