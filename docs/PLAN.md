# PLAN.md — Instructions

Changes implemented:
- **Lane builds**: A/B (Unikraft), C (OSv Java), D (Jail), E (OCI+Kontain), F (VM).
- **Supply chain**: CI produces SBOM (Syft), scans (Grype), signs (Cosign); controller **OPA** check before deploy.
- **Preview**: `https://<sha>.<app>.ployd.app` triggers build; naive readiness proxy.
- **CLI**: `apps new`, `.gitignore`-aware `push`, `open`.
- **Storage**: S3 abstraction (SeaweedFS) + automatic artifact uploads.

Next steps to implement:

**Phase 1: Critical Missing Basic Functionality**
1. ✅ **COMPLETED (2025-08-18)** Complete missing CLI commands: domains add, certs issue, debug shell, rollback.
2. ✅ **COMPLETED (2025-08-18)** Fix lane picker: Add Jib detection for Java/Scala Lane E vs C selection.
3. ✅ **COMPLETED (2025-08-18)** Fix Python C-extension detection in lane picker (should force Lane C).
4. ✅ **COMPLETED (2025-08-18)** App environment variables: `POST/GET/PUT/DELETE /v1/apps/:app/env` API and `ploy env` CLI commands to manage per-app environment variables that are available during build and deploy phases.
5. ✅ **COMPLETED (2025-08-19)** Replace naive readiness with Nomad API polling of alloc health, then proxy.
6. ✅ **COMPLETED (2025-08-19)** Implement debug build with SSH support: Complete implementation of `POST /v1/apps/:app/debug` with SSH key generation, debug builds for all lanes, and Nomad debug namespace deployment.
7. ✅ **COMPLETED (2025-08-19)** Implement app destroy command: `ploy apps destroy --name <app>` CLI command and `DELETE /v1/apps/:app` API endpoint to completely remove all app resources including services, storage, environment variables, domains, certificates, and debug instances.

**Phase 2: Lane B (Node.js Unikraft) Enhancement**
1. ✅ **COMPLETED (2025-08-19)** Enhance `lanes/B-unikraft-posix/kraft.yaml` with Node.js runtime libraries and configuration.
2. ✅ **COMPLETED (2025-08-19)** Extend `build/kraft/build_unikraft.sh` with Node.js detection and build steps.
3. ✅ **COMPLETED (2025-08-19)** Add Node.js dependency handling (npm install, package bundling) to build process.
4. ✅ **COMPLETED (2025-08-19)** Create Node.js-specific Unikraft configuration within existing template system.
5. ✅ **COMPLETED (2025-08-19)** Test `ploy push` with `apps/node-hello` example using enhanced Lane B detection.

**Phase 3: Supply Chain Security Implementation**
1. ✅ **COMPLETED (2025-08-19)** Implement cryptographic signing of build artifacts during build process.
2. ✅ **COMPLETED (2025-08-19)** Generate signature files (`.sig`) for all built artifacts.
3. ✅ **COMPLETED (2025-08-19)** Implement SBOM (Software Bill of Materials) generation during builds.
4. ✅ **COMPLETED (2025-08-19)** Create SBOM files (`.sbom.json`) with actual dependency information.
5. ✅ **COMPLETED (2025-08-19)** Integrate cosign keyless OIDC flow and key management.
6. ✅ **COMPLETED (2025-08-19)** Ensure artifacts and signatures are properly uploaded to SeaweedFS storage.

**Phase 4: Policy Enforcement & Validation**
1. ✅ **COMPLETED (2025-08-19)** Implement OPA policies requiring signature/SBOM for deployments.
2. ✅ **COMPLETED (2025-08-19)** Add artifact integrity verification after storage upload.
3. ✅ **COMPLETED (2025-08-19)** Implement image size caps per lane in OPA policies.
4. ✅ **COMPLETED (2025-08-19)** Enhance policy enforcement for production vs development environments.

**Phase 5: Build Process Enhancements**
1. ✅ **COMPLETED (2025-08-20)** Enhance Nomad job health monitoring with robust status checking.
2. ✅ **COMPLETED (2025-08-20)** Improve Git integration with proper repository validation.
3. ✅ **COMPLETED (2025-08-20)** Add comprehensive error handling for storage operations.
4. ✅ **COMPLETED (2025-08-20)** Implement Node.js version detection and management for Unikraft builds.
5. ✅ **COMPLETED (2025-08-20)** Enhance build artifact upload with retry logic and verification.

**Phase 6: Platform Enhancement Features**
1. ✅ **COMPLETED (2025-08-20)** Implement Java version detection for Gradle and Maven projects with fallback to default version.
2. ✅ **COMPLETED (2025-08-20)** Add TTL cleanup for preview allocations to prevent resource accumulation.
3. Enrich Nomad templates with Vault/Consul/env/volumes and canary rollout.

**Phase Networking: Production Domain Routing**
1. ✅ **COMPLETED (2025-08-20)** Deploy Traefik as system job on all Nomad nodes for load balancing and SSL termination.
2. Implement wildcard DNS configuration for `*.ployd.app` domain routing.
3. Create Consul service registration with Traefik labels for automatic route discovery.
4. Add domain management API endpoints: `POST/GET/DELETE /v1/apps/:app/domains`.
5. Implement Let's Encrypt wildcard certificate management with automatic renewal.
6. Create health checking system for routed applications with traffic management.
7. Add blue-green deployment support with gradual traffic shifting via Traefik weights.
8. Implement CLI commands for domain management: `ploy domains add/remove/list`.
9. Create SeaweedFS-backed domain mapping storage for persistence and recovery.
10. Add geographic routing support for multi-region deployments.

**Phase WASM: WebAssembly Runtime Support**
1. **WASM Runtime Integration**: Integrate wazero (pure Go) WebAssembly runtime for Lane G deployment.
2. **Lane G Builder Implementation**: Create `controller/builders/wasm.go` with WASM module detection and bundling.
3. **WASM Detection Logic**: Implement automatic detection of WASM compilation targets in lane picker:
   - Direct `.wasm` and `.wat` file detection
   - Rust `wasm32-wasi` target in Cargo.toml
   - AssemblyScript `.asc` files and compiler configuration
   - WASM-specific dependencies (wasm-bindgen, js-sys, web-sys, wasi)
   - Go with `GOOS=js GOARCH=wasm` build tags
   - C/C++ with Emscripten toolchain detection
4. **WASM Build Pipeline**: Create `scripts/build/wasm/` directory with build scripts for different WASM compilation paths.
5. **Nomad WASM Driver**: Configure Nomad job templates for WASM runtime execution with proper resource limits and networking.
6. **WASI Support**: Implement WASI Preview 1 filesystem and networking interfaces for WASM modules.
7. **Component Model Integration**: Add support for linking multiple WASM modules using the WebAssembly Component Model.
8. **WASM Security Policies**: Extend OPA policies for WASM-specific security requirements and resource constraints.
9. **WASM Testing**: Create sample WASM applications in `apps/` directory for Rust, Go, AssemblyScript, and C++ targets.
10. **Lane G Documentation**: Complete WASM compilation detection analysis and integration documentation.

**Phase Automated Remediation Framework (ARF): Enterprise Code Transformation**

**Phase ARF-1: Foundation & Core Engine**
1. **OpenRewrite Integration Infrastructure**:
   - Install and configure OpenRewrite dependencies in controller module
   - Create `controller/arf/` directory structure for ARF components
   - Implement `ARFEngine` interface with basic OpenRewrite recipe execution
   - Add OpenRewrite Maven dependencies to Java build pipeline
   - Create AST cache system using memory-mapped files + LRU cache
   - Integrate with existing `controller/builders/java_osv.go` for Lane C validation

2. **Sandbox Management System**:
   - Implement `SandboxManager` using FreeBSD jails for secure transformation environments
   - Create ZFS snapshot-based rollback capability for instant restoration
   - Integrate with Nomad scheduler for parallel sandbox execution
   - Add sandbox validation pipeline (compilation → testing → security scanning)
   - Create sandbox cleanup service with configurable TTL (similar to preview cleanup)

3. **Recipe Discovery & Management**:
   - Implement static recipe catalog with 2,800+ OpenRewrite recipes
   - Create recipe metadata database with success rates and compatibility info
   - Build recipe search engine with similarity scoring and filtering
   - Add recipe validation system for OpenRewrite YAML syntax checking
   - Create recipe performance tracking with historical success metrics

4. **Basic Transformation Engine**:
   - Implement single-repository transformation workflow
   - Create transformation result tracking with success/failure analysis
   - Add basic error classification (syntax, compilation, semantic errors)
   - Implement simple retry logic with exponential backoff
   - Create transformation metrics collection and logging

**Phase ARF-2: Self-Healing Loop & Error Recovery**
1. **Circuit Breaker Implementation**:
   - Add circuit breaker pattern with 50% failure threshold
   - Implement exponential backoff with jitter for retry operations
   - Create failure threshold monitoring and automatic circuit opening
   - Add health monitoring for transformation services
   - Integrate circuit breaker state with Consul service discovery

2. **Error-Driven Recipe Evolution**:
   - Implement error classification system (recipe_mismatch, compilation_failure, semantic_change, incomplete_transformation)
   - Create automatic recipe modification based on failure analysis
   - Add recipe extension system for incomplete transformations
   - Implement safety checks and validation for modified recipes
   - Create recipe rollback mechanism for problematic changes

3. **Parallel Error Resolution**:
   - Implement Fork-Join framework for concurrent error remediation
   - Add parallel solution testing with confidence scoring
   - Create solution caching system for similar error patterns
   - Implement parallel sandbox execution using Nomad job allocation
   - Add resource management for concurrent transformation jobs

4. **Multi-Repository Orchestration**:
   - Implement dependency graph construction for repository ordering
   - Create complexity-based repository grouping and batching (max 50-100 repos per batch)
   - Add topological sort for dependency-aware transformation order
   - Implement resource allocation planning for batch execution
   - Create execution plan generation with timeout and rollback procedures

**Phase ARF-3: LLM Integration & Hybrid Intelligence (Months 4-6)**
1. **LLM-Assisted Recipe Creation**:
   - Integrate LLM API for dynamic recipe generation from error contexts
   - Implement LLM prompt engineering for OpenRewrite recipe creation
   - Add LLM response parsing into valid OpenRewrite YAML format
   - Create LLM-generated recipe validation and testing system
   - Implement fallback handling when LLM generation fails

2. **Hybrid Transformation Pipeline**:
    - Create hybrid execution workflow: OpenRewrite → LLM enhancement → validation
    - Implement confidence scoring system (token confidence + build success + test coverage)
    - Add intelligent strategy selection based on transformation complexity
    - Create context-aware prompting with surrounding code and build logs
    - Implement solution confidence ranking and selection

3. **Continuous Learning System**:
    - Add success pattern extraction from completed transformations
    - Implement failure pattern analysis and cataloging
    - Create recipe performance tracking by repository type and complexity
    - Add pattern generalization for new recipe template creation
    - Implement model retraining for strategy selection algorithms

4. **Transformation Strategy Selection**:
    - Create strategy selection matrix based on issue type and complexity
    - Implement historical performance analysis for confidence scoring
    - Add resource availability assessment for strategy decisions
    - Create strategy escalation logic (recipe → LLM → human intervention)
    - Implement strategy performance monitoring and optimization

**Phase ARF-4: Security & Production Hardening**
1. **Security Vulnerability Remediation**:
    - Create security-specific recipe repository for CVE fixes
    - Implement vulnerability analysis and severity assessment
    - Add dynamic security recipe generation for specific vulnerabilities
    - Create security-focused transformation validation with enhanced scanning
    - Implement rapid remediation workflows for critical vulnerabilities

2. **SBOM Integration & Supply Chain Security**:
    - Integrate SBOM tracking for transformation artifacts
    - Add supply chain security validation during transformations
    - Create transformation artifact signing with Cosign integration
    - Implement transformation audit trails with comprehensive logging
    - Add compliance validation for security best practices

3. **Human-in-the-Loop Integration**:
    - Implement webhook system for GitHub/Slack/PagerDuty integration
    - Create progressive delegation workflows (developer → team lead → architecture → security)
    - Add approval workflow configuration based on risk assessment
    - Implement diff visualization for comprehensive transformation review
    - Create error escalation system when confidence thresholds not met

4. **Production Performance Optimization**:
    - Optimize JVM configuration (G1GC, 4GB+ heap) for codebase processing
    - Implement distributed processing coordination using Consul service mesh
    - Add AST caching optimization with memory-mapped files for 10x performance improvement
    - Create resource usage monitoring and optimization
    - Implement load balancing for concurrent transformation requests

**Phase ARF-5: Production Features & Scale**
1. **Multi-Repository Scale Management**:
    - Implement mid-scale multi-repository coordination (hundreds of repos)
    - Add cross-repository dependency analysis and impact assessment
    - Create organization-wide transformation campaigns with progress tracking
    - Implement repository prioritization and resource allocation
    - Add transformation scheduling and queue management

2. **Advanced Analytics & Reporting**:
    - Create comprehensive transformation analytics dashboard
    - Implement business impact measurement (time savings, error reduction)
    - Add transformation success rate tracking and trend analysis
    - Create executive reporting with ROI calculations
    - Implement predictive analysis for transformation success probability

3. **Integration & API Ecosystem**:
    - Create REST API endpoints for external integration (`/v1/arf/*`)
    - Add CLI commands: `ploy arf transform`, `ploy arf status`, `ploy arf recipes`
    - Implement webhook system for real-time transformation events
    - Create basic SDK libraries for external system integration
    - Add transformation result export capabilities

4. **Production Security & Compliance**:
    - Implement basic authentication and authorization (API keys)
    - Add audit logging for transformation activities
    - Create transformation approval workflows with webhook integration
    - Implement data privacy controls for sensitive code transformations
    - Add basic compliance reporting for code transformation activities

**ARF Success Metrics & Targets**:
- 50-80% time reduction in code migrations (baseline: manual migration time)
- 95% success rates for well-defined transformations
- Support for hundreds of repositories per transformation campaign
- Days to weeks completion vs months manual effort
- Mid-scale processing capability suitable for most organizations
- Integration with existing Ploy infrastructure (Lane C validation, Nomad scheduling, SeaweedFS storage)

**Phase no-SPOF Controller: High Availability & Horizontal Scaling**

**Phase no-SPOF-1: State Externalization**
1. ✅ **COMPLETED (2025-08-20)** **Move envStore to Consul KV Storage**:
   - Replace file-based envStore with Consul KV backend
   - Implement `consul_envstore` package with same interface as file-based store
   - Add atomic operations for environment variable updates
   - Implement key-value mapping: `/ploy/apps/{app}/env` → JSON document
   - Add comprehensive error handling and connection retries for Consul operations
   - Create automatic fallback to file-based store when Consul unavailable

2. **Move Storage Client to External Configuration**:
   - Replace singleton storage client with per-request initialization
   - Move storage configuration from embedded config to external YAML files
   - Add configuration validation and reload capabilities without service restart
   - Implement configuration templates for different environments (dev/staging/prod)
   - Add storage client pooling and connection management for improved performance

3. **Add Health and Readiness Endpoints**:
   - Implement `/health` endpoint for basic service health checking
   - Implement `/ready` endpoint for readiness probes with dependency validation
   - Add comprehensive health checks for Consul, Nomad, SeaweedFS, and Vault connectivity
   - Implement graceful degradation when non-critical dependencies are unavailable
   - Add health check metrics and logging for operational monitoring

4. **Implement Stateless Initialization Patterns**:
   - Remove all global state variables and singleton patterns from controller
   - Implement request-scoped dependency injection for all external services
   - Add configuration-driven initialization for all controller components
   - Implement clean shutdown procedures with graceful connection draining
   - Add comprehensive logging for initialization and shutdown procedures

**Phase no-SPOF-2: Nomad Job Creation**
1. **Create Nomad System Job Definition**:
   - Create `platform/nomad/ploy-controller.hcl` with system job configuration
   - Configure multi-instance deployment with proper resource allocation
   - Add restart policies, update strategies, and failure handling
   - Implement rolling update configuration with health check integration
   - Add proper constraints and affinity rules for optimal node placement

2. **Service Discovery Integration**:
   - Add Consul service registration with health checks and metadata
   - Configure service discovery tags for Traefik load balancer integration
   - Implement service mesh connectivity for secure inter-service communication
   - Add service versioning and blue-green deployment support
   - Configure automatic service deregistration on instance failure

3. **Traefik Load Balancing Configuration**:
   - Configure Traefik routing rules for controller API load balancing
   - Add health-based routing with automatic failover to healthy instances
   - Implement sticky sessions for stateful operations if needed
   - Add rate limiting and security headers for API protection
   - Configure SSL/TLS termination and certificate management

4. **Rolling Update Strategy Implementation**:
   - Configure Nomad update blocks with canary deployments
   - Implement health check integration for update validation
   - Add automatic rollback on failed updates or health check failures
   - Configure update parallelism and timing for zero-downtime deployments
   - Add monitoring and alerting for update progress and failures

**Phase no-SPOF-3: Bootstrap Integration**
1. **Controller Binary Distribution**:
   - Implement controller binary distribution via SeaweedFS artifact storage
   - Add version management and artifact integrity verification
   - Create automated build pipeline for controller releases
   - Implement binary caching and distribution across multiple nodes
   - Add rollback capability to previous controller versions

2. **Ansible Playbook Integration**:
    - Modify Ansible playbooks to deploy controller as Nomad job instead of system service
    - Add controller binary deployment and version management to playbooks
    - Implement proper ordering: Consul/Vault → Controller → Application jobs
    - Add validation steps for controller deployment success
    - Create migration scripts from current systemd-based deployment

3. **Controller Self-Update Capability**:
    - Implement controller API endpoints for self-update operations
    - Add validation and safety checks for controller updates
    - Implement coordination between controller instances during updates
    - Add rollback mechanisms for failed self-updates
    - Create update orchestration logic with proper sequencing

4. **Migration Scripts and Procedures**:
    - Create comprehensive migration plan from current architecture
    - Implement data migration scripts for environment variables and configuration
    - Add validation procedures for post-migration system health
    - Create rollback procedures for migration failure scenarios
    - Document operational procedures for controller management

**Phase no-SPOF-4: Production Hardening**
1. **Leader Election for Coordination Operations**:
    - Implement Consul-based leader election for coordination-heavy operations
    - Add leader election for TTL cleanup service coordination
    - Implement shared state management with leader coordination
    - Add follower instance coordination and workload distribution
    - Create leader election monitoring and automatic failover

2. **Graceful Shutdown and Connection Draining**:
    - Implement SIGTERM handling for graceful shutdown procedures
    - Add connection draining with configurable timeout periods
    - Implement in-flight request completion before shutdown
    - Add proper cleanup of resources and external connections
    - Create shutdown coordination between multiple controller instances

3. **Monitoring and Alerting Integration**:
    - Add comprehensive metrics collection for controller cluster health
    - Implement alerting for controller instance failures and recoveries
    - Add performance monitoring for API response times and throughput
    - Create dashboards for controller cluster status and resource utilization
    - Implement log aggregation and structured logging for operational visibility

4. **Performance Optimization for Multi-Instance Coordination**:
    - Optimize Consul KV operations for high-frequency environment variable access
    - Implement caching strategies for frequently accessed configuration data
    - Add connection pooling and retry logic for external service interactions
    - Optimize controller startup time and resource utilization
    - Implement load balancing algorithms for optimal request distribution

**no-SPOF Success Metrics & Targets**:
- 99.9% controller uptime with automatic failover in <30 seconds
- Zero-downtime controller updates with <5 second request interruption
- Horizontal scaling capability supporting 3-10 controller instances
- Sub-100ms API response times with load balancing across multiple instances
- Automatic recovery from individual controller failures within 60 seconds
- Configuration changes deployed without service interruption
- Complete elimination of single points of failure in Ploy infrastructure
