# Ploy API Server

The Ploy API server provides REST endpoints for application deployment and management. HTTP endpoints are defined in [`server/server.go`](server/server.go) in the `setupRoutes()` function.

## API Structure

The API is organized into functional modules:

```
api/
├── main.go                   # API server entry point
├── server/                   # HTTP server architecture
│   ├── server.go             # Main server with endpoint definitions (setupRoutes)
│   ├── routes.go             # Route definitions and setup
│   ├── handlers.go           # Primary request handlers
│   ├── handlers_bluegreen.go # Blue-green deployment handlers
│   ├── handlers_certificate.go # Certificate management handlers
│   ├── handlers_health.go    # Health check handlers
│   ├── platform_handlers.go  # Platform service handlers
│   ├── recipes_handlers.go   # Recipe-specific request handlers
│   ├── initializers.go       # Server initialization and setup
│   ├── config.go             # Server configuration management
│   └── storage_resolver.go   # Storage backend resolution
├── config/                   # Configuration management
│   └── config.go             # Configuration loading and validation
├── health/                   # Health checking infrastructure
│   └── health.go             # Health, readiness, liveness endpoints
├── metrics/                  # Metrics collection and monitoring
│   └── metrics.go            # Prometheus metrics integration
├── builders/                 # Lane-specific builders (A-G)
│   ├── debug.go              # Builder debug utilities
│   ├── jail.go               # Lane D - FreeBSD jails
│   ├── java_osv.go           # Lane C - OSv/Hermit VMs for JVM
│   ├── oci.go                # Lane E - OCI containers
│   ├── unikraft.go           # Lanes A/B - Unikraft unikernels
│   ├── vm.go                 # Lane F - Full VMs
│   ├── wasm.go               # Lane G - WebAssembly modules
│   └── utils.go              # Builder utilities
├── nomad/                    # HashiCorp Nomad integration
│   ├── client.go             # Nomad API client
│   ├── health.go             # Nomad cluster health checks
│   ├── render.go             # Job template rendering
│   ├── submit.go             # Job submission to Nomad
│   └── submit_enhanced.go    # Enhanced job submission
├── certificates/             # SSL/TLS certificate management
│   ├── manager.go            # Certificate lifecycle management
│   └── wildcard.go           # Wildcard certificate handling
├── dns/                      # DNS provider integration
│   ├── handler.go            # DNS management endpoints
│   ├── provider.go           # DNS provider interface
│   ├── cloudflare.go         # Cloudflare DNS integration
│   └── namecheap.go          # Namecheap DNS integration
├── acme/                     # ACME protocol for certificates
│   ├── client.go             # ACME client implementation
│   ├── handler.go            # ACME challenge handlers
│   ├── renewal.go            # Automatic certificate renewal
│   └── storage.go            # Certificate storage management
├── domains/                  # Domain management
│   └── handler.go            # Domain configuration handlers
├── routing/                  # Traffic routing management
│   └── traefik.go            # Traefik configuration management
├── envstore/                 # Environment variable storage
│   ├── interface.go          # Storage interface definition
│   ├── store.go              # File-based environment storage
│   └── store_test.go         # Environment store tests
├── consul_envstore/          # Consul KV environment storage
│   └── store.go              # Consul-based environment storage
├── coordination/             # Distributed coordination
│   ├── leader.go             # Distributed leader election logic
│   └── ttl_cleanup.go        # TTL-based resource cleanup
├── selfupdate/               # Self-updating capability
│   ├── executor.go           # Update execution logic
│   ├── handler.go            # Update API endpoints
│   └── utils.go              # Update utilities
├── version/                  # Version management
│   └── handler.go            # Version information endpoints
├── templates/                # Template management
│   └── handler.go            # Template processing endpoints
├── supply/                   # Supply chain security
│   ├── sbom.go               # SBOM generation
│   ├── signing.go            # Artifact signing
│   └── verify.go             # Signature verification
├── opa/                      # Open Policy Agent security
│   └── verify.go             # Security policy verification
├── runtime/                  # Runtime environments
│   └── wasm.go               # WebAssembly runtime integration
├── wasm/                     # WebAssembly components
│   └── components.go         # WASM component management
├── analysis/                 # Static analysis system
│   ├── handler.go            # Analysis API endpoints
│   ├── engine.go             # Analysis engine
│   ├── cache.go              # Analysis result caching
│   ├── types.go              # Analysis type definitions
│   ├── arf_integration.go    # ARF integration
│   ├── nomad_analyzer.go     # Nomad-based distributed analysis
│   ├── nomad_dispatcher.go   # Distributed analysis job dispatch
│   └── analyzers/            # Language-specific analyzers
│       ├── java/             # Java analysis tools (ErrorProne)
│       └── python/           # Python analysis tools (Pylint)
├── platform/                 # Platform service integration
│   ├── handler.go            # Platform API endpoints
│   └── handler_test.go       # Platform handler tests
├── mods/                     # Mods API endpoints
│   └── handler.go            # Mods transformation handlers
├── llms/                     # Large Language Model integration
│   ├── handler.go            # LLM API endpoints
│   └── handler_test.go       # LLM handler tests
├── sbom/                     # SBOM API and analyzer
│   ├── handler.go            # SBOM API endpoints (/v1/sbom/*)
│   ├── analyzer.go           # Minimal Syft-style analyzer
│   └── types.go              # SBOM analysis types
└── arf/                      # Automated Remediation Framework
    ├── handler.go            # Main ARF endpoint handlers
    ├── handler_debug.go      # Debug and analysis endpoints
    ├── handler_recipes.go    # Recipe management endpoints
    ├── handler_sandbox.go    # Sandbox management endpoints
    ├── handler_security.go   # Security analysis endpoints
    ├── handler_transformation_async.go # Async transformation handlers (catalog only)
    ├── multi_language_core.go # Multi-language transformation core
    ├── multi_language_java.go # Java-specific transformations
    ├── multi_language_python.go # Python-specific transformations
    ├── multi_language_go.go  # Go-specific transformations
    ├── multi_language_rust.go # Rust-specific transformations
    ├── multi_language_wasm.go # WebAssembly transformations
    ├── multi_language_javascript.go # JavaScript transformations
    ├── pattern_matcher.go    # Code pattern matching engine
    # Note: recipe_* sources (registry, executor, evolution, types) moved to api/recipes
    ├── sandbox.go            # Sandbox management for transformations
    ├── sandbox_validation.go # Sandbox security validation
    ├── security_engine.go    # Security analysis engine
    ├── storage_service.go    # ARF storage service layer
    ├── unified_service.go    # Unified ARF service interface
    ├── transformation_workflow.go # Transformation orchestration
    ├──
    
    
    ├── nvd_database.go       # National Vulnerability Database integration
    ├── deployment_sandbox.go # Deployment environment sandboxing
    ├── config.go             # ARF configuration management
    ├── common_types.go       # Shared type definitions
    ├── shared_types.go       # Additional shared types
    ├── recipe_types.go       # Recipe-specific type definitions
    ├── transformation_types.go # Transformation type definitions
    ├── llm_types.go          # LLM integration types
    ├── debug_types.go        # Debug system types
    ├── security_engine_types.go # Security engine types
    ├──
    ├── registry_storage_adapter.go # Storage adapter for recipe registry
    ├── db/                   # Database schemas and migrations
    ├── examples/             # ARF recipe examples
    ├── models/               # ARF data models and validation
    └── validation/           # Recipe validation
```

## Key Components

- **Server Core**: `server/server.go` contains the main HTTP server setup and all endpoint route definitions
- **Lane Builders**: `builders/` implements deployment targets for different performance/footprint profiles
- **Infrastructure**: Integration with Nomad, Consul, Traefik, and SeaweedFS storage

### Nomad Wrapper Policy (VPS)

On VPS environments, all Nomad interactions are routed through the job manager wrapper at `/opt/hashicorp/bin/nomad-job-manager.sh`.

- Submission and validation in server code prefer the wrapper and fall back to SDK/CLI only when the wrapper is not present (e.g., non-VPS/local).
- Benefits: unified retries/backoff for 429/5xx, HCL→JSON conversion and validation, consistent logging, and service cleanup before deployments.
- Do not call the raw `nomad` CLI directly in API code when running on the VPS; use the wrapper or the orchestration facade which auto-detects the wrapper.
- **Security**: ACME certificates, DNS validation, supply chain security, and OPA policy enforcement
- **Analysis & Transformation**: Static analysis and automated remediation via ARF system
- **Management**: Self-update, cleanup, monitoring, and coordination services

## SBOM Endpoints

The SBOM module provides endpoints under `/v1/sbom`.

### POST /v1/sbom/generate

Generate a Software Bill of Materials (SBOM) using Syft for a file artifact or a container image. The API delegates to the Syft-based generator in `api/supply/sbom.go`.

- Request (JSON)
  - `artifact` (string, required): Path to a file artifact (e.g., `/path/to/app.bin`) or a container image reference (e.g., `repo/app:1.2.3`).
  - `format` (string, optional): Output format, defaults to `spdx-json`. Accepts Syft-supported formats.
  - `lane` (string, optional): Deployment lane identifier for metadata.
  - `app_name` (string, optional): Application name for metadata.
  - `sha` (string, optional): Build SHA for metadata.

- Response (JSON)
  - `status`: Always `"completed"` on success.
  - `generated_at`: RFC3339 timestamp.
  - `format`: Resolved output format (e.g., `spdx-json`).
  - `location`: Absolute path to the generated SBOM file.

- Behavior
  - If `artifact` contains a colon (`:`), it is treated as a container image reference and the SBOM is written to `/tmp/<sanitized-image>.sbom.json`.
  - Otherwise, the SBOM is generated next to the file with suffix `.sbom.json`.
  - Backward compatibility: If `artifact` is omitted, the endpoint returns a stubbed successful envelope (legacy tests), but no real generation occurs.

- Examples

  File artifact
  - Request
    - `POST /v1/sbom/generate`
    - Body: `{ "artifact": "/opt/builds/app.bin", "format": "spdx-json", "lane": "E", "app_name": "app", "sha": "abc123" }`
  - Response
    - `{ "status": "completed", "generated_at": "2025-09-13T18:30:00Z", "format": "spdx-json", "location": "/opt/builds/app.bin.sbom.json" }`

  Container image
  - Request
    - `POST /v1/sbom/generate`
    - Body: `{ "artifact": "registry.local/app:1.2.3", "format": "spdx-json" }`
  - Response
    - `{ "status": "completed", "generated_at": "2025-09-13T18:30:00Z", "format": "spdx-json", "location": "/tmp/registry.local-app-1.2.3.sbom.json" }`

### POST /v1/sbom/analyze

Analyze an existing SBOM and return a basic risk summary. This endpoint currently performs lightweight parsing and metric computation; vulnerability correlation is a stub in this module and is typically covered by Grype + enrichment pipelines.

- Request (JSON)
  - `sbom_path` (string, required): Path to an existing SBOM file.

- Response (JSON)
  - Includes `summary`, `vulnerabilities` (mock structure), and `generated_at` used by tests.
