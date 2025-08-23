# Ploy Features

## 🎯 Core Purpose
Maximum performance PaaS using unikernels, jails, and VMs with Heroku-like developer experience.

⸻

## 🛠 Build Lanes (A–G)

Auto-classified lanes:
- ✅ **Lane A** – Unikraft Minimal (Go, C)
  - KraftKit unikernel images
  - `<app>-<sha>.img` deterministic naming
  - SBOM + signature generation
- ✅ **Lane B** – Unikraft POSIX (Node, Python)
  - ✅ **Intelligent Node.js Configuration System** (Aug 2025):
    - Specialized B-unikraft-nodejs template for Node.js applications
    - Enhanced V8 runtime support with optimized kernel configuration
    - Threading and memory management for event loop and worker threads
    - Advanced networking with IPv4/IPv6 and HTTP server optimizations
    - Automatic application metadata extraction from package.json
  - ✅ **Node.js Version Detection and Management** (Aug 2025):
    - Automatic Node.js version detection from package.json engines field
    - Support for version ranges (^18.0.0, >=16.0.0, 18.x, ~19.5.0)
    - Download and caching of specific Node.js versions for Unikraft builds
    - Fallback to system Node.js when download fails or network unavailable
    - Version-specific npm and dependency management during build process
    - Integration with Kraft YAML generation and dependency manifests
  - Enhanced Node.js runtime support with libelf, musl, lwip libraries
  - Comprehensive V8/Node.js kconfig for POSIX environment, networking, I/O
  - Musl libc with crypto, locale, networking, and complex math support
  - Optimized lwip networking stack with TCP/UDP, DHCP, threading
  - Dropbear SSH for debug (planned)
- ✅ **Lane C** – OSv Java/Scala
  - ✅ **Java Version Detection** (Aug 2025): Automatic Java version detection from build files
    - Gradle support: `JavaLanguageVersion.of(21)`, `sourceCompatibility = "17"`, `gradle.properties`
    - Maven support: `<maven.compiler.source>21</maven.compiler.source>`, `<java.version>11</java.version>`
    - `.java-version` file support for explicit version specification
    - Intelligent fallback to Java 21 default when detection fails
    - Version validation ensuring reasonable range (8-25) for production builds
    - Enhanced build logging with detected version information and source
  - Jib → Capstan → `<app>-<sha>.qcow2`
  - Custom MainClass support
- ✅ **Lane D** – FreeBSD Jails
  - `<app>-<sha>-jail.tar` rootfs
  - Lightweight isolation for legacy apps
- ✅ **Lane E** – OCI + Kontain
  - `harbor.local/ploy/<app>:<sha>` images
  - `io.kontain` runtime for VM isolation
- ✅ **Lane F** – Full VMs
- ✅ **Lane G** – WebAssembly Runtime (Fully Implemented Aug 2025)
  - ✅ **Complete WASM Implementation** (Aug 2025): Production-ready WebAssembly support with comprehensive feature set
    - **Multi-Language Compilation Support**: Rust (wasm32-wasi), Go (js/wasm), C/C++ (Emscripten), AssemblyScript
    - **wazero Runtime Integration**: Pure Go WebAssembly runtime v1.5.0 with security constraints
    - **WASI Preview 1 Support**: Filesystem access, environment variables, and system interface
    - **Automatic Detection**: Lane picker intelligently detects WASM targets with 95%+ accuracy
    - **Component Model**: Multi-module WASM applications with dependency management and interface validation
    - **Production Templates**: Nomad job templates with health checks, resource limits, and Traefik routing
    - **Security Policies**: OPA policies for production/staging/development environments with WASM-specific constraints
    - **Build Pipeline**: Complete build strategies for different WASM compilation targets
    - **Runtime Features**: HTTP server integration, metrics endpoint, graceful shutdown, health monitoring
    - **Sample Applications**: Working examples for Rust, Go, AssemblyScript, and C++ WASM modules
  - `<app>-<sha>.wasm` module artifacts with wazero runtime
  - Hardware-enforced sandboxing with process isolation  
  - 5–30 MB footprint, 10–50ms boot times
  - Supports Rust, Go, C++, AssemblyScript with extensible compilation detection

⸻

## ⚙️ Builders
- ✅ Per-lane scripts in `build/` directory
- ✅ Auto SBOM (Syft) + signatures (Cosign)
- ✅ Deterministic `<app>-<sha>` naming
- ✅ Standalone or controller invocation
- ✅ **Advanced Node.js Build Pipeline** (Aug 2025):
  - Automatic Node.js application detection via package.json
  - Enterprise dependency management with npm ci and integrity verification
  - Production-optimized package bundling with .unikraft-bundle creation
  - Dependency manifest generation for build optimization and insights
  - Memory-optimized startup scripts for unikernel environments
  - JavaScript syntax validation and main entry point verification
  - Graceful error handling for missing Node.js/npm dependencies

⸻

## 📦 Supply Chain Security
- ✅ **Cryptographic Artifact Signing** (Aug 2025):
  - **Multi-Mode Signing**: Key-based, keyless OIDC, and development dummy signatures
  - **Universal Lane Support**: File-based artifacts (A,B,C,D,F) and Docker images (E)
  - **Automatic Integration**: Seamless signing immediately after successful builds
  - **Smart Prevention**: Avoids duplicate signing by checking existing signatures
  - **Cosign Compatible**: Full support for cosign key management and OIDC flows
- ✅ **Production-Ready SBOM Generation** (Aug 2025):
  - **Comprehensive SBOM Support**: All build scripts generate SBOM files using modern syft scan command
  - **Multi-Format Output**: SPDX-JSON for Unikraft lanes, JSON for other lanes with full metadata
  - **Cross-Lane Coverage**: SBOM generation verified across Unikraft (A/B), jails (D), containers (E), VMs (F)
  - **Source & Artifact Analysis**: Generates SBOMs for both source dependencies and built artifacts
  - **Supply Chain Metadata**: Includes checksums, timestamps, tool versions, and artifact relationships
- ✅ **Enhanced Keyless OIDC Integration** (Aug 2025):
  - **Multi-Provider OIDC Support**: Auto-detection for GitHub Actions, GitLab CI, Buildkite, Google Cloud
  - **Device Flow Authentication**: Interactive and non-interactive signing modes with automatic detection
  - **Certificate Management**: Ephemeral certificate generation from Fulcio with transparency log integration
  - **Environment Adaptability**: Production keyless OIDC, development fallbacks, CI/CD pipeline optimization
  - **Enhanced Error Handling**: Graceful timeout handling, network resilience, comprehensive logging
- ✅ **Comprehensive Signature File Generation** (Aug 2025):
  - **Universal .sig Files**: All build scripts generate signature files for every artifact
  - **Debug Variant Support**: Debug builds include signature generation alongside main builds  
  - **Lane-Specific Implementation**: Optimized signature generation per deployment lane

⸻

## 🌐 DNS Management
- ✅ **Multi-Provider DNS Integration** (Aug 2025):
  - **Cloudflare Provider**: Full DNS API integration with wildcard and individual record support
  - **Namecheap Provider**: Complete DNS management via Namecheap API with sandbox support
  - **Provider Abstraction**: Clean interface enabling easy addition of Route53, DigitalOcean, etc.
- ✅ **Wildcard DNS Configuration** (Aug 2025):
  - **Automatic Subdomain Routing**: Configure `*.ployd.app` for seamless app subdomain access
  - **Multiple Target Support**: IP addresses, CNAME targets, load balancer configurations
  - **DNS Propagation Validation**: Real-time verification of wildcard DNS setup and functionality
- ✅ **Complete DNS Record Management** (Aug 2025):
  - **Full Record Type Support**: A, AAAA, CNAME, TXT, MX records with priority and TTL configuration
  - **CRUD Operations**: Create, read, update, delete individual DNS records via REST API
  - **IPv6 Support**: Dual-stack DNS with automatic AAAA record management
- ✅ **Load Balancer Integration** (Aug 2025):
  - **Multiple IP Configuration**: Support for multiple target IPs in wildcard DNS setup
  - **High Availability**: Automatic DNS-based load balancing for production deployments
- ✅ **Configuration Management** (Aug 2025):
  - **Environment Variables**: Full support for environment-based DNS provider configuration
  - **JSON Configuration**: File-based configuration with sensitive credential protection
  - **Ansible Integration**: Automated DNS setup via infrastructure as code playbooks
  - **Graceful Fallbacks**: Handles missing cosign/syft tools in development environments
- ✅ Vulnerability scans (Grype), advanced keyless signing (Cosign) with full OIDC integration
- ✅ **Comprehensive storage upload** to SeaweedFS with artifact bundles (Aug 2025)
- ✅ **Enhanced OPA Policy Enforcement** (Aug 2025):
  - **Environment-Specific Policy Framework**: Production, staging, and development environments with tailored security policies
  - **Signature & SBOM Requirements**: All deployments must have cryptographic signatures and SBOMs
  - **Production Security Restrictions**: Strict enforcement of key-based/OIDC signing, vulnerability scanning, SSH/debug build controls
  - **Staging Security Balance**: Core security requirements with warning-based degradation for development efficiency
  - **Development Flexibility**: Warning-only enforcement with all signing methods accepted for rapid iteration
  - **Vulnerability Scanning Integration**: Grype-based security analysis for production and staging deployments
  - **Signing Method Detection**: Automatic analysis of signature types (keyless-oidc, key-based, development)
  - **Source Repository Validation**: Trusted repository patterns for supply chain security
  - **Artifact Age Limits**: Maximum 30-day freshness requirements for production deployments
  - **Environment Normalization**: Intelligent handling of environment name variations
  - **Comprehensive Audit Logging**: Detailed logging for all policy decisions with environment context
  - **Break-Glass Approval**: Emergency override mechanism for critical production access with full audit trail
- ✅ **Comprehensive Artifact Integrity Verification** (Aug 2025):
  - **SHA-256 Checksum Verification**: All uploaded artifacts verified with cryptographic checksums to detect corruption
  - **File Size Validation**: Prevents truncated uploads and ensures complete file transfers to storage
  - **SBOM Content Validation**: Validates SPDX-JSON schema compliance and required metadata fields
  - **Bundle Completeness Verification**: Confirms all expected files (artifact, SBOM, signature, certificate) are present
  - **Detailed Error Reporting**: Comprehensive failure analysis with specific reasons for verification failures
  - **Audit Trail Logging**: Complete verification history with timestamps and validation results for compliance
  - **Retry Logic Integration**: Handles temporary storage issues with intelligent retry mechanisms
- ✅ **Lane-Specific Image Size Caps** (Aug 2025):
  - **Optimized Size Limits**: Lane A (50MB), Lane B (100MB), Lane C (500MB), Lane D (200MB), Lane E (1GB), Lane F (5GB)
  - **Multi-Format Size Measurement**: File-based artifacts via filesystem and Docker images via CLI commands
  - **Pre-Deployment Enforcement**: Size caps validated before Nomad deployment to prevent resource waste
  - **Break-Glass Override**: Emergency deployment capability for size cap violations in production environments
  - **Comprehensive Error Reporting**: Detailed size violation messages with actual vs limit comparisons
  - **Performance Optimization**: Size limits aligned with lane performance characteristics and boot requirements
  - **Storage Efficiency**: Prevents oversized deployments while maintaining functionality requirements
- ✅ **Enhanced Lane Detection** (Aug 2025):
  - ✅ **Jib Plugin Detection**: Java/Scala projects with Jib → Lane E (containerless builds)
  - ✅ **Build System Support**: Gradle, Maven, SBT with comprehensive plugin detection
  - ✅ **Language Accuracy**: Proper Scala vs Java identification in mixed projects
  - ✅ **Python C-Extension Detection**: Multi-layered detection for C-extensions → Lane C
    - Source file detection: `.c`, `.cc`, `.cpp`, `.cxx`, `.pyx`, `.pxd` files
    - Library dependencies: numpy, scipy, pandas, psycopg2, lxml, pillow, cryptography
    - Build configuration: `ext_modules`, `Extension()`, `build_ext`, CMake integration
    - Cython support: Import detection and `.pyx` file analysis
  - ✅ **WASM Target Detection** (Aug 2025): Comprehensive automatic detection for WASM compilation targets
    - **Build Configuration**: `wasm32-wasi` target in Cargo.toml, `--target wasm32-wasi` flags, Go build constraints
    - **Direct WASM Files**: `.wasm` and `.wat` file detection with magic byte validation
    - **Language-Specific Dependencies**: wasm-bindgen, js-sys, web-sys, wasi crates for Rust; js/wasm build tags for Go
    - **AssemblyScript Projects**: `.asc` files, AssemblyScript compiler configuration, and package.json scripts
    - **Emscripten Detection**: CMakeLists.txt with Emscripten toolchain and C/C++ WASM compilation flags
    - **Priority Detection**: WASM detection takes priority over standard language detection for Lane G assignment

⸻

## 🚀 Deployment
- ✅ Nomad templates per lane in `platform/nomad/`
- ✅ Jobs include health checks, Vault integration, canary rollouts, Consul registration
- ✅ Controller handles rendering, submission, health polling
- ✅ **Enhanced Health Monitoring** (Aug 2025):
  - **Deployment Progress Tracking**: Real-time monitoring of task group status with healthy/unhealthy allocation counts
  - **Comprehensive Health Checks**: Validates allocation status, deployment health indicators, and Consul service checks
  - **Robust Retry Logic**: Automatic retries with exponential backoff and intelligent error classification
  - **Failure Detection**: Early abort when allocation failure threshold exceeded (3+ failures)
  - **Job Validation**: Pre-submission HCL syntax validation prevents deployment errors
  - **Detailed Error Reporting**: Task event logging with driver failures, exit codes, and actionable debugging information
  - **Concurrent Monitoring**: Background deployment and health check monitoring for faster feedback
  - **Timeout Management**: Prevents indefinite waiting on stuck deployments with configurable deadlines
  - **Log Streaming**: Real-time allocation log following for debugging failed deployments
  - **Network Resilience**: Graceful handling of transient connectivity issues with retry classification

⸻

## 🌐 Routing & Preview
- ✅ **Preview System**: `https://<sha>.<app>.ployd.app` triggers builds
  - ✅ **Nomad Health Monitoring**: Proper allocation health polling before routing
  - ✅ **Smart Readiness**: Replaces naive HTTP checks with Nomad API integration
  - ✅ **Error Handling**: Meaningful feedback for failed/pending deployments
  - ✅ **Dynamic Discovery**: Endpoint detection based on allocation IP/port mapping
- ✅ **Advanced Traefik Load Balancing & SSL** (Aug 2025):
  - ✅ **System Deployment**: Traefik deployed as system job on all Nomad nodes for high availability
  - ✅ **Automatic Service Discovery**: Native Consul integration with Traefik labels for zero-config routing
  - ✅ **Advanced Load Balancing**: Weighted round-robin with configurable health checking and sticky sessions
  - ✅ **Circuit Breaker Patterns**: Fault tolerance with configurable failure thresholds and recovery duration
  - ✅ **Multi-Tier Rate Limiting**: Per-source IP rate limiting with burst and average rate configuration
  - ✅ **Comprehensive Security Headers**: HSTS, CSP, XSS protection, frame options, and permission policies
  - ✅ **SSL/TLS Termination**: Let's Encrypt certificate management with TLS 1.2/1.3 and strong cipher suites
  - ✅ **Dynamic Middleware Configuration**: Service-specific middleware chains with global middleware reuse
  - ✅ **Enhanced Health Checking**: Configurable intervals, timeouts, retries with proper scheme and header support
  - ✅ **Platform Wildcard SSL/TLS** (Aug 2025): Automatic `*.ployd.app` wildcard certificate provisioning
    - **Automatic Platform Certificate**: Single wildcard certificate covers all platform subdomains
    - **DNS-01 Challenge Support**: ACME DNS-01 validation for wildcard certificate issuance
    - **Multi-Provider DNS Integration**: Namecheap and CloudFlare DNS provider support
    - **Intelligent Certificate Selection**: Wildcard for platform subdomains, individual for external domains
    - **Automatic Renewal Service**: Background certificate renewal with configurable thresholds
    - **SeaweedFS Certificate Storage**: Distributed certificate storage for multi-instance access
    - **Health Monitoring Endpoints**: Platform certificate status and expiry tracking
  - ✅ **Blue-Green Deployments** (Aug 2025): Gradual traffic shifting with Traefik weight-based routing
    - **Parallel Version Deployment**: Deploy new version alongside existing version without downtime
    - **Traffic Weight Management**: Manual and automatic traffic shifting between blue and green versions
    - **Health-Based Validation**: Comprehensive health checks before traffic migration steps
    - **Gradual Traffic Migration**: Default strategy: 0% → 10% → 25% → 50% → 75% → 100%
    - **Automatic Rollback**: Instant rollback to previous version on health check failures
    - **CLI Integration**: Complete CLI support for deployment, monitoring, and rollback operations
    - **API-Driven Control**: RESTful endpoints for programmatic blue-green deployment management
    - **Consul State Management**: Deployment state persistence with distributed coordination
  - **Geographic Routing**: Multi-region support with proximity-based traffic direction (planned)
  - ✅ **Minimal Footprint**: ~40MB binary with 50-100MB RAM per instance
  - ✅ **No Single Point of Failure**: Masterless architecture with shared configuration

## 🏗 High Availability Controller Architecture

- ✅ **Zero-SPOF Controller Design**
  - **Nomad-Managed Deployment**: Controller runs as Nomad system job across multiple nodes
  - **Stateless Architecture**: All state externalized to Consul KV, SeaweedFS, and Vault
  - **Load Balancing**: Multiple controller instances behind Traefik with health checking
  - **Horizontal Scaling**: Scale controller instances based on API load and resource requirements
  - ✅ **Enhanced Rolling Updates with Canary Deployment** (Aug 2025): Zero-downtime deployments with canary deployment strategy
    - Nomad update blocks with 1 canary instance and automatic rollback on failures
    - Comprehensive health check integration with stricter validation during updates  
    - Extended health validation timeout (5m) and graceful shutdown coordination (60s)
    - Update progress monitoring with Slack webhook alerts and deployment status tracking
    - Rolling update parallelism control with 30-second stagger delay for stability
  - ✅ **Controller Binary Distribution System** (Aug 2025): Automated controller deployment and version management
    - SeaweedFS-based binary distribution with version management and integrity verification
    - Multi-node binary caching with automatic download and SHA256 hash validation
    - Cross-platform build pipeline with metadata tracking and git commit integration
    - Complete rollback system for controller versions with safety checks and validation
    - Nomad artifact downloads with startup scripts for proper binary selection and execution
    - CLI tools for manual binary operations: upload, download, list, build, and rollback
  - ✅ **Ansible Nomad Controller Integration** (Aug 2025): Infrastructure-as-code deployment automation
    - Complete Ansible playbook integration for Nomad-based controller deployment
    - Automated migration from manual/systemd deployment to high availability Nomad architecture
    - Proper service ordering with dependency validation: SeaweedFS → HashiCorp → Controller → Applications
    - Multi-replica controller deployment (2+ instances) with automatic failover and load balancing
    - Comprehensive management toolchain: update, rollback, status monitoring, and migration scripts
    - Service discovery integration with Consul registration and Traefik load balancer configuration
    - Health check integration with Nomad service discovery for seamless load balancing
    - Process conflict prevention with clean migration paths and validation tools
  - ✅ **Controller Self-Update Capability** (Aug 2025): In-place controller updates with coordination and safety
    - RESTful self-update API endpoints: `/v1/controller/update`, `/update/status`, `/rollback`, `/version`, `/versions`
    - Multiple update strategies: rolling, blue-green, and emergency update approaches
    - Consul-based coordination between controller instances during updates with distributed locking
    - Comprehensive validation: binary integrity (SHA256), platform compatibility, system resource checks
    - Atomic binary replacement to avoid "text file busy" errors with fallback external update scripts
    - Automatic and manual rollback capabilities with last-known-good version detection
    - Update orchestration with proper sequencing, safety checks, and graceful error handling
  - **Auto-Recovery**: Failed instances automatically restarted by Nomad scheduler

- ✅ **External State Management**
  - **Environment Variables**: Consul KV storage (`/ploy/apps/{app}/env/*`)
  - **Build Metadata**: SeaweedFS JSON artifacts with versioning
  - **Application Configuration**: Consul KV with atomic updates and validation
  - **Routing State**: Consul service registry with health checks and load balancer integration
  - **Secrets Management**: Vault integration with dynamic credential management

- ✅ **Production Hardening** (Aug 2025): Complete no-SPOF architecture with operational excellence
  - ✅ **Leader Election System**: Consul-based leader election for coordination-heavy operations with automatic failover
  - ✅ **Graceful Shutdown**: Enhanced SIGTERM handling with connection draining and coordination resource cleanup
  - ✅ **Prometheus Metrics**: Comprehensive metrics collection for leadership, builds, performance, and operational visibility
  - ✅ **TTL Cleanup Coordination**: Leader-only TTL cleanup with automatic task transfer on failover
  - ✅ **Health Monitoring**: `/health/coordination` endpoint for real-time leader election status
  - ✅ **Metrics Observatory**: `/metrics` endpoint with 15+ controller metrics for operational monitoring
- ✅ **Operational Excellence**
  - **99.9% Uptime**: Multiple instances with automatic failover and health monitoring
  - **Self-Healing**: Automatic detection and replacement of unhealthy controller instances
  - **Configuration Management**: Template-driven configuration updates without service interruption
  - **Service Discovery**: Controllers register with Consul for automatic load balancer integration
  - **Health Endpoints**: `/health` and `/ready` endpoints for Nomad health checks
  - **Zero Downtime**: Sub-100ms API responses with <30 second leader failover

- ✅ **TTL Cleanup for Preview Allocations** (Aug 2025):
  - **Automatic Cleanup Service**: Background service with configurable intervals (default: 6h) for preview allocation cleanup
  - **Configurable TTL**: Preview allocations cleaned after TTL expiration (default: 24h) with maximum age limit (7d)
  - **Pattern-Based Detection**: Identifies preview jobs using `{app}-{sha}` naming pattern with SHA validation
  - **Age-Based Cleanup**: Uses Nomad job SubmitTime for accurate age calculation and cleanup decisions
  - **HTTP API Management**: Complete service control via REST endpoints (/cleanup/status, /config, /jobs, /trigger)
  - **Flexible Configuration**: File-based and environment variable configuration with validation
  - **Dry Run Mode**: Safe testing mode for cleanup operations without actual job deletion
  - **Service Control**: Start/stop service management with automatic startup integration
  - **Statistics & Monitoring**: Age distribution analytics and cleanup operation statistics
  - **Error Resilience**: Graceful handling of Nomad API failures and missing jobs
- ✅ **Heroku-Style Domain Management** (Aug 2025): Complete domain and certificate automation
  - **Platform Domain Pattern**: Automatic `{app}.ployd.app` subdomain assignment for all apps
  - **Controller Access Domain**: Controller accessible at `api.ployd.app` via Traefik
  - **Domain API Endpoints**: `POST /v1/apps/{app}/domains` for adding custom domains
  - **Automatic Certificate Provisioning**: `certificate: auto` triggers Let's Encrypt provisioning
  - **Custom Certificate Upload**: Support for uploading custom SSL certificates via API
  - **Domain Type Detection**: Automatic detection of platform vs external domains
  - **Traefik Integration**: Automatic routing configuration for all registered domains
  - **Consul KV Storage**: Persistent domain-to-app mapping storage
- ✅ TLS: Full Let's Encrypt integration with DNS-01 challenges, BYOC supported

⸻

## 👩‍💻 CLI (Go + Bubble Tea)
- ✅ `ploy apps new` – scaffold with /healthz
- ✅ **`ploy apps destroy` – comprehensive app destruction**
  - **Safety First**: Interactive confirmation with detailed resource warnings
  - **Complete Cleanup**: Nomad jobs, environment variables, containers, temp files
  - **Force Mode**: `--force` flag for automated workflows and CI/CD
  - **Status Reporting**: Detailed operation results with per-resource status
  - **Error Resilience**: Continues cleanup even if individual operations fail
- ✅ `ploy push` – tar + stream to controller
  - ✅ **Validated Node.js Lane B Testing** (Aug 2025):
    - Successfully tested with apps/node-hello demonstrating automatic Lane B detection
    - Verified build pipeline progression from tar processing to lane validation
    - Confirmed proper request body handling eliminating EOF errors
    - OPA policy validation triggers correctly for unsigned artifacts
- ✅ `ploy open` – browser launch
- ✅ `ploy env` – manage app environment variables
- ✅ `ploy domains/certs/rollback` – operations
- ✅ **`ploy debug shell` – SSH-enabled debug instances**
  - **Debug Build System**: Lane-specific debug builds with SSH daemon
  - **SSH Key Management**: Automatic RSA key pair generation per session
  - **Debug Isolation**: Nomad debug namespace with 2-hour auto-cleanup
  - **All Lane Support**: Unikraft, OCI, OSv, and jail debug environments
  - **Development Tools**: Pre-installed debuggers, profilers, and network tools
- ✅ Workflow: push → build → deploy → open → destroy

⸻

## 🗄 Storage
- ✅ **SeaweedFS Distributed Storage** (Aug 2025):
  - SeaweedFS cluster with master, volume, and filer servers for optimal small file performance
  - Collection-based organization optimized for artifact types (ploy-artifacts, ploy-metadata, ploy-debug)
  - Automated upload of complete deployment packages (artifact + SBOM + signature + certificate)
  - Upload retry logic with FileID verification for reliable storage operations
  - Enhanced metadata tracking with timestamps and artifact status information
- ✅ **Comprehensive Error Handling & Resilience** (Aug 2025):
  - Advanced error classification system with 10+ error types (network, timeout, corruption, rate limit, etc.)
  - Exponential backoff retry logic with configurable policies and jitter randomization
  - Context-aware timeout handling and graceful operation cancellation
  - File operation retry with automatic seek position reset and stream reopening
  - Circuit breaker pattern to prevent cascading failures during storage outages
- ✅ **Health Monitoring & Metrics** (Aug 2025):
  - Real-time storage operation metrics (uploads, downloads, verifications, success rates)
  - Thread-safe metrics collection with comprehensive performance analytics
  - Health status classification (healthy/degraded/unhealthy) based on failure patterns
  - Deep storage connectivity testing with configuration validation
  - API endpoints `/storage/health` and `/storage/metrics` for monitoring and diagnostics
- ✅ **Enhanced Storage Client** (Aug 2025):
  - Comprehensive wrapper combining error handling, retry logic, and monitoring
  - Operation-level timeout configuration with configurable maximum operation times
  - Graceful fallback to basic storage client when enhanced features unavailable
  - Backward compatibility with existing storage operations and interfaces
- ✅ **External Storage Configuration** (Aug 2025):
  - Per-request storage client initialization for stateless operation and improved reliability
  - External YAML configuration support with fallback to embedded config
  - Configuration validation and hot reload capabilities without service restart
  - API endpoints for configuration management: `/storage/config`, `/storage/config/validate`, `/storage/config/reload`
  - Environment-specific configuration templates with Ansible provisioning to `/etc/ploy/storage/`
- ✅ **Scalable Architecture**: No single point of failure, HTTP-based simple API
- ✅ Config: `configs/storage-config.yaml` with simplified SeaweedFS-only configuration
- ✅ Organization: Collections with proper replication strategies per artifact type
- ✅ **Upload Verification**: Built-in methods to confirm successful storage operations
- ✅ **Enhanced Upload Retry Logic** (Aug 2025): Robust artifact upload with exponential backoff
  - Comprehensive retry mechanism with 3 maximum attempts and progressive delays
  - Integrity verification after each upload attempt with automatic retry on failure
  - Size verification for byte data uploads to detect truncated transfers
  - Proper file handle management and seek position reset for reliable retries
  - Enhanced error reporting with specific failure reasons and attempt counts
  - Independent retry logic for concurrent upload operations
- ✅ **Multi-File Support**: Source SBOMs, container SBOMs, and build artifacts

⸻

## 🔬 Sample Apps
✅ `apps/` directory with Go, Node, Python, .NET, Scala, Java examples.
✅ All include `/healthz` on port 8080.

⸻

## 🧪 CI/CD
- ✅ GitHub Actions: build, SBOM, scan, keyless sign
- ✅ GitLab CI: validate, build, supply-chain, deploy
- ✅ Artifact upload for traceability

⸻


## 🌍 Environment Variables
- ✅ **Management**: `POST/GET/PUT/DELETE /v1/apps/:app/env`
- ✅ **Build-time**: Available during image creation
- ✅ **Runtime**: Injected into deployment environment
- ✅ **Storage**: Consul KV backend with automatic fallback to file-based storage
- ✅ **High Availability**: External state storage eliminates controller SPOF for environment data
- ✅ **CLI**: `ploy env set/get/list/delete` commands
- ✅ **Integration**: All lanes support environment variables in build and deploy phases
- ✅ **Atomic Operations**: Consul KV provides consistency for concurrent environment variable updates

⸻

## 🔀 Git Integration & Repository Validation
- ✅ **Comprehensive Git Repository Analysis** (Aug 2025):
  - **Multi-Source URL Extraction**: Repository URLs from git config, package.json, Cargo.toml, pom.xml, go.mod
  - **URL Normalization**: SSH to HTTPS conversion with .git suffix removal for consistency
  - **Repository Metadata**: Branch detection, commit analysis, contributor statistics, language analysis
  - **Repository Health Scoring**: 0-100 scoring system based on security and validation issues
- ✅ **Security-Focused Repository Validation** (Aug 2025):
  - **Secrets Detection**: AWS keys, private keys, API keys, passwords, tokens in source code
  - **Sensitive File Detection**: .env files, private keys, certificates, SSH keys in repository
  - **GPG Commit Validation**: Signature verification for enhanced security compliance
  - **Comprehensive Validation Results**: Errors, warnings, security issues, and actionable suggestions
- ✅ **Environment-Specific Git Validation** (Aug 2025):
  - **Production Environment**: Clean repo, signed commits, trusted domains, restricted branches, size limits
  - **Staging Environment**: Clean repo with unsigned commit warnings, broader branch support
  - **Development Environment**: Dirty repo warnings only, flexible validation for rapid development
  - **Configurable Validation**: Custom trusted domains, branch restrictions, size limits per environment
- ✅ **Build Pipeline Integration** (Aug 2025):
  - **Enhanced Repository Detection**: Improved `extractSourceRepository` with Git utilities
  - **Build-Time Validation**: Repository validation during build process with environment awareness
  - **Health Score Logging**: Repository health and validation results during deployment pipeline
  - **Multi-Language Support**: Git validation across all project types and deployment lanes

⸻

## 🧬 Automated Remediation Framework (ARF) ✅ IMPLEMENTED

**STATUS: ✅ IMPLEMENTED** - Phases ARF-1 & ARF-2 completed (August 2025). Comprehensive roadmap available in `roadmap/arf/`

ARF represents Ploy's enterprise-grade automated code transformation and self-healing system, designed to automatically remediate common code issues, migrate legacy codebases, and apply security fixes across hundreds of repositories using OpenRewrite and LLM-assisted intelligence.

### ✅ **Implemented Core Transformation Engine**
- ✅ **OpenRewrite Integration**: 2,800+ recipes for framework migrations, security patches, and API upgrades
- ✅ **AST Cache System**: Memory-mapped file caching with 10x performance improvement
- ✅ **Recipe Catalog**: Searchable database with confidence scoring and metadata management  
- ✅ **Single-Repository Workflows**: Complete transformation pipeline for individual repositories
- ✅ **Recipe Discovery & Management**: Static catalog search, validation, and performance tracking

### ✅ **Implemented Self-Healing Loop System**
- ✅ **Circuit Breaker Pattern**: 50% failure threshold with exponential backoff to prevent cascading failures
- ✅ **Error Classification**: Automatic categorization (recipe_mismatch, compilation_failure, semantic_change, incomplete_transformation)
- ✅ **Error-Driven Recipe Evolution**: Automatic recipe modification based on failure analysis with confidence scoring
- ✅ **Parallel Solution Testing**: Fork-join framework for concurrent error remediation attempts with confidence scoring
- ✅ **Multi-Repository Orchestration**: Dependency-aware transformation coordination across multiple repositories

### ✅ **Implemented Sandbox Validation & Testing**
- ✅ **FreeBSD Jail Sandboxes**: Secure isolated environments for code transformations with resource limits
- ✅ **ZFS Snapshot Support**: Instant rollback capability for disaster recovery (< 5 seconds)
- ✅ **Multi-Lane Integration**: Leverages Ploy's existing lanes for language-specific build validation
- ✅ **Sandbox Management**: TTL cleanup, resource monitoring, and automatic environment cleanup

### ✅ **Implemented Intelligence & Learning (ARF Phase 3)**
- ✅ **Error Pattern Learning Database**: PostgreSQL vector similarity for pattern matching and solution caching
- ✅ **Confidence Scoring**: Multi-layered validation with recipe effectiveness tracking
- ✅ **Pattern Matching Algorithms**: Vector embeddings for cross-repository learning and generalization
- ✅ **Monitoring Infrastructure**: Comprehensive metrics, alerting, and distributed tracing for ARF operations
- ✅ **LLM Recipe Generation**: OpenAI/Anthropic integration for dynamic recipe creation based on context
- ✅ **Hybrid Transformation Pipeline**: Intelligent combination of OpenRewrite and LLM approaches
- ✅ **Multi-Language AST Support**: Tree-sitter integration for universal language parsing
- ✅ **A/B Testing Framework**: Statistical validation of recipe improvements with confidence intervals
- ✅ **Continuous Learning System**: Pattern extraction from historical transformations with retraining

### ✅ **Implemented High Availability & Performance**
- ✅ **Distributed Processing**: Consul leader election and state management for multi-controller coordination
- ✅ **AST Caching**: Memory-mapped files with 10x performance improvement and cache persistence
- ✅ **Circuit Breaker Integration**: Distributed coordination across multiple ARF instances
- ✅ **Resource Management**: Nomad scheduler integration for parallel sandbox execution

### ✅ **Implemented API & CLI Integration**
- ✅ **Comprehensive REST API**: Complete `/v1/arf/*` endpoint suite for recipes, transformations, sandboxes, monitoring
- ✅ **ARF Phase 3 Endpoints**: 30+ new endpoints for LLM generation, hybrid pipelines, learning system, A/B testing
- ✅ **Ploy CLI Integration**: `ploy arf` commands for recipe management, transformation, validation, patterns, testing
- ✅ **CLI Integration**: Full `ploy arf` command suite for recipe management, transformations, and health checks
- ✅ **Cache Management**: Cache statistics, clearing, and optimization through API and CLI
- ✅ **System Monitoring**: Health checks, metrics collection, and operational statistics

### 📋 **Planned Security & Vulnerability Management**
- ⏳ **Enhanced Vulnerability Remediation**: Real-time feeds from NVD, GitHub Advisory, Snyk with zero-day response workflows
- ⏳ **SBOM Integration**: Supply chain tracking with comprehensive artifact signing using Cosign integration
- ⏳ **Dynamic Security Recipe Generation**: LLM-generated recipes for specific vulnerabilities with CVE-to-recipe mapping
- ⏳ **Data Retention & GDPR Compliance**: Comprehensive data lifecycle management with configurable retention policies

### 📋 **Planned Human-in-the-Loop Integration**
- ⏳ **Webhook System**: GitHub/Slack/PagerDuty integration for approval workflows with progressive delegation
- ⏳ **Multi-Stage Approval**: Configurable workflows (developer → team lead → architecture → security)
- ⏳ **Error Escalation**: Automated escalation when confidence thresholds not met with intelligent routing
- ⏳ **Diff Visualization**: Comprehensive transformation diffs for human review with security impact analysis

### 📋 **Planned Performance & Scalability**
- ⏳ **High Availability Integration**: Distributed processing with Consul leader election and state management
- ⏳ **AST Caching**: Memory-mapped files + LRU cache for 10x performance improvement with error pattern database
- ⏳ **Parallel Processing**: Nomad scheduler integration for distributed execution with monitoring infrastructure
- ⏳ **Production Optimization**: JVM tuning (G1GC, 4GB+ heap) with operational monitoring and SLI/SLO tracking

### 📋 **Planned Enterprise Features**
- ⏳ **Multi-Repository Campaign Management**: 200-500 repositories per campaign with progress tracking and analytics
- ⏳ **Advanced Analytics & Cost Optimization**: Business impact measurement, ROI calculations, and LLM API cost management
- ⏳ **WASM Integration**: Lane G-specific transformations with size optimization and polyfill injection
- ⏳ **API Ecosystem**: Complete REST API with CLI integration (`ploy arf` commands) and SDK libraries

### 📋 **Planned Use Case Coverage**
- ⏳ **Framework Migrations**: Spring Boot upgrades, JUnit 4→5, Java 8→11→17→21, .NET Framework → .NET Core/5+
- ⏳ **Security Patching**: Log4Shell remediation, dependency upgrades, vulnerability fixes with 4-hour critical response
- ⏳ **API Modernization**: Deprecated API removal, library version upgrades across multiple languages
- ⏳ **Code Quality**: Technical debt reduction, coding standards enforcement with static analysis integration
- ⏳ **Complex Refactoring**: Large-scale architectural changes across multiple repositories with impact analysis

**Integration Points:**
- All Ploy lanes for language-specific validation and testing
- Nomad scheduler for parallel sandbox execution and resource management
- SeaweedFS for AST cache, artifact storage, and transformation tracking
- Consul service mesh for distributed coordination and leader election
- Existing builder pipeline integration with enhanced validation

**Target Performance Metrics:**
- 50-80% time reduction in code migrations vs manual effort
- 95% success rates for well-defined transformations  
- Days to weeks completion vs months manual effort
- 200-500 repositories per transformation campaign
- 300%+ ROI demonstration with measurable business value

⸻

## 🔮 Next Steps
- Per-app Unikraft recipes and custom configurations
- E2E testing suite with full Nomad cluster validation
- Observability stack integration (Loki/Prometheus/Grafana)
- Vault secrets management integration
- Multi-region deployment support
- Cost optimization and resource usage analytics