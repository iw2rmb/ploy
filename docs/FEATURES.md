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
  - Jib → Capstan → `<app>-<sha>.qcow2`
  - Custom MainClass support
- ✅ **Lane D** – FreeBSD Jails
  - `<app>-<sha>-jail.tar` rootfs
  - Lightweight isolation for legacy apps
- ✅ **Lane E** – OCI + Kontain
  - `harbor.local/ploy/<app>:<sha>` images
  - `io.kontain` runtime for VM isolation
- ✅ **Lane F** – Full VMs
  - `<app>-<sha>.img` via Packer
  - Maximum compatibility fallback
- **Lane G** – WASM Runtime (planned)
  - Universal polyglot compilation target
  - `<app>-<sha>.wasm` + runtime bundle
  - Hardware-enforced sandboxing with process isolation
  - 5–30 MB footprint, 10–50ms boot times
  - Supports Rust, Go, C++, AssemblyScript, Python (via Pyodide)

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
  - **WASM Target Detection** (planned): Automatic detection for WASM compilation targets
    - Build configuration: `wasm32-wasi` target in Cargo.toml, `--target wasm32-wasi` flags
    - Direct WASM files: `.wasm` and `.wat` file detection
    - WASM-specific dependencies: wasm-bindgen, js-sys, web-sys, wasi crates
    - AssemblyScript projects: `.asc` files and AssemblyScript compiler configuration

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
- TTL cleanup for previews (planned)
- ✅ Domains: `manifests/<app>.yaml` configuration
- TLS: Certbot integration (planned), BYOC supported

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
- `ploy push --verify --diff` – verification branch testing (planned)
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
- Self-healing loop support for LLM agents

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

## 🤖 Self-Healing Loop (planned)
- **Diff Push**: `POST /v1/apps/:app/diff?verify=true`
  - Temporary branches (`verify-<timestamp>-<hash>`)
  - Isolated verification namespace
  - Auto-cleanup
- **Webhooks**: `POST /v1/apps/:app/webhooks`
  - Real-time events (`build.*`, `deploy.*`)
  - JSON payloads with metadata
  - Retry + auth (Bearer/HMAC)
- **LLM Integration**: Monitor via webhooks, fix via verification branches

## 🌍 Environment Variables
- ✅ **Management**: `POST/GET/PUT/DELETE /v1/apps/:app/env`
- ✅ **Build-time**: Available during image creation
- ✅ **Runtime**: Injected into deployment environment
- ✅ **Storage**: File-based persistence with JSON format
- ✅ **CLI**: `ploy env set/get/list/delete` commands
- ✅ **Integration**: All lanes support environment variables in build and deploy phases

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

## 🔮 Next Steps
- Per-app Unikraft recipes and custom configurations
- TTL cleanup for preview allocations to prevent resource accumulation
- E2E testing suite with full Nomad cluster validation
- Observability stack integration (Loki/Prometheus/Grafana)
- Advanced traffic shifting strategies (blue/green deployments)
- Vault secrets management integration
- Multi-region deployment support
- Cost optimization and resource usage analytics