# Ploy API Server

The Ploy API server provides REST endpoints for application deployment and management. HTTP endpoints are defined in [`server/server.go`](server/server.go) in the `setupRoutes()` function.

## API Structure

The API is organized into functional modules:

```
api/
├── main.go                   # API server entry point
├── server/                   # HTTP server architecture
│   ├── server.go             # Main server with endpoint definitions (setupRoutes)
│   ├── handlers.go           # Primary request handlers  
│   └── platform_handlers.go  # Platform service handlers
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
│   └── analyzers/            # Language-specific analyzers
│       ├── java/             # Java analysis tools (ErrorProne)
│       └── python/           # Python analysis tools (Pylint)
└── arf/                      # Automated Remediation Framework
    ├── handler_*.go          # ARF endpoint handlers
    ├── catalog.go            # Recipe catalog management
    ├── sandbox.go            # Sandbox management for transformations
    ├── openrewrite_engine.go # OpenRewrite integration
    ├── hybrid_pipeline.go    # Hybrid transformation pipeline
    ├── examples/             # ARF recipe examples
    ├── models/               # ARF data models and validation
    ├── storage/              # ARF storage backends
    ├── validation/           # Recipe validation
    └── sql/                  # Database integration for learning system
```

## Key Components

- **Server Core**: `server/server.go` contains the main HTTP server setup and all endpoint route definitions
- **Lane Builders**: `builders/` implements deployment targets for different performance/footprint profiles
- **Infrastructure**: Integration with Nomad, Consul, Traefik, and SeaweedFS storage
- **Security**: ACME certificates, DNS validation, supply chain security, and OPA policy enforcement
- **Analysis & Transformation**: Static analysis and automated remediation via ARF system
- **Management**: Self-update, cleanup, monitoring, and coordination services
