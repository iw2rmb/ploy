# Ploy REST API (v1)

## Health and Readiness Endpoints
- `GET /health` — basic service health check.
  - Returns 200 if service is healthy (critical dependencies OK)
  - Returns 503 if service is unhealthy
  - Response includes dependency status for Consul, Nomad, Vault, SeaweedFS
- `GET /ready` — comprehensive readiness probe.
  - Returns 200 if service is ready (all critical dependencies OK)
  - Returns 503 if service is not ready
  - Critical dependencies: storage_config, consul, nomad
- `GET /live` — simple liveness probe.
  - Always returns 200 with alive status
- `GET /health/metrics` — health check metrics for monitoring.
  - Returns counts, failure rates, and timing information
- **Versioned Access**: All health endpoints also available at `/v1/health`, `/v1/ready`, `/v1/live`, `/v1/health/metrics`

## Core Application Endpoints
- `POST /v1/apps/:app/builds?sha=<sha>&lane=<A..G>&main=<MainClass>` — build & deploy; lane auto-picked if omitted.
  - **Lane G Support**: WebAssembly applications automatically detected and routed to wazero runtime
- `GET /v1/apps` — list all applications.
- `GET /v1/status/:app` — get application deployment status.
- `DELETE /v1/apps/:app` — destroy application and all associated resources.

## Domain Management Endpoints (Heroku-style with Automatic Certificate Provisioning)
- `POST /v1/apps/:app/domains` — add domain to app with automatic certificate provisioning.
  - Body: `{"domain": "example.com", "certificate": "auto", "cert_provider": "letsencrypt"}`
  - Certificate options: `"auto"` (default, automatic Let's Encrypt), `"manual"` (user-managed), `"none"` (no certificate)
  - Returns: `{"status": "added", "app": "myapp", "domain": "example.com", "message": "Domain registered successfully, certificate provisioning started", "certificate": {"domain": "example.com", "status": "provisioning", "provider": "letsencrypt", "auto_renew": true}}`
- `GET /v1/apps/:app/domains` — list domains for app with certificate information.
  - Returns: `{"status": "success", "app": "myapp", "domains": ["myapp.ployd.app", "example.com"], "certificates": [{"domain": "example.com", "status": "active", "provider": "letsencrypt", "issued_at": "2025-08-21 10:30:00", "expires_at": "2025-11-19 10:30:00", "auto_renew": true}]}`
- `DELETE /v1/apps/:app/domains/:domain` — remove domain from app (automatically removes associated certificate).
  - Returns: `{"status": "removed", "app": "myapp", "domain": "example.com", "message": "Domain removed successfully"}`

## Certificate Management Endpoints (Heroku-style, Domain-based)
- `GET /v1/apps/:app/certificates` — list all certificates for app.
  - Returns: `{"status": "success", "app": "myapp", "certificates": [{"domain": "example.com", "status": "active", "provider": "letsencrypt", "issued_at": "2025-08-21 10:30:00", "expires_at": "2025-11-19 10:30:00", "auto_renew": true}]}`
- `GET /v1/apps/:app/certificates/:domain` — get certificate details for domain.
  - Returns: `{"status": "success", "app": "myapp", "domain": "example.com", "certificate": {"domain": "example.com", "status": "active", "provider": "letsencrypt", "issued_at": "2025-08-21 10:30:00", "expires_at": "2025-11-19 10:30:00", "auto_renew": true}}`
- `POST /v1/apps/:app/certificates/:domain/provision` — manually provision certificate for domain.
  - Returns: `{"status": "provisioning", "app": "myapp", "domain": "example.com", "message": "Certificate provisioning started", "certificate": {"domain": "example.com", "status": "provisioning", "provider": "letsencrypt", "auto_renew": true}}`
- `POST /v1/apps/:app/certificates/:domain/upload` — upload custom certificate bundle (multipart/form-data: certificate, private_key, ca_certificate).
  - CLI: `ploy domains certificates myapp upload example.com --cert-file=cert.pem --key-file=key.pem --ca-file=ca.pem`
  - Returns: `{"status": "uploaded", "app": "myapp", "domain": "example.com", "certificate": {"domain": "example.com", "status": "active", "provider": "custom", "auto_renew": false}, "message": "Custom certificate uploaded successfully"}`
- `DELETE /v1/apps/:app/certificates/:domain` — remove certificate for domain.
  - Returns: `{"status": "removed", "app": "myapp", "domain": "example.com", "message": "Certificate removed successfully"}`

## Legacy Certificate Endpoints (Deprecated)
- `POST /v1/certs/issue` — **DEPRECATED** - Use domain-based certificate management instead.
- `GET /v1/certs` — **DEPRECATED** - Use `/v1/apps/:app/certificates` instead.

## Debug & Operations Endpoints (Implemented)
- `POST /v1/apps/:app/debug` — create debug instance with SSH.
  - Query params: `?lane=<A-G>` (optional, includes Lane G for WASM debugging)
  - Body: `{"ssh_enabled": true}`
  - Returns: `{"status": "debug_created", "app": "myapp", "instance": "debug-myapp-123", "ssh_enabled": true, "ssh_command": "ssh debug@debug-myapp-123.debug.ployd.app"}`
  - **WASM Debug Support**: Lane G debug instances provide SSH access to wazero runtime environment
- `POST /v1/apps/:app/rollback` — rollback app to previous version.
  - Body: `{"sha": "abc123def456"}`
  - Returns: `{"status": "rolled_back", "app": "myapp", "sha": "abc123def456", "message": "Application rolled back successfully"}`

## Environment Variables Endpoints (Implemented)
- `POST /v1/apps/:app/env` — set multiple environment variables.
  - Body: `{"NODE_ENV": "production", "DATABASE_URL": "postgres://localhost", "DEBUG": "true"}`
  - Returns: `{"status": "updated", "app": "myapp", "count": 3, "message": "Environment variables updated successfully"}`
- `GET /v1/apps/:app/env` — list all environment variables.
  - Returns: `{"app": "myapp", "env": {"NODE_ENV": "production", "DATABASE_URL": "postgres://localhost"}}`
- `PUT /v1/apps/:app/env/:key` — update single environment variable.
  - Body: `{"value": "new_value"}`
  - Returns: `{"status": "updated", "app": "myapp", "key": "NODE_ENV", "message": "Environment variable updated successfully"}`
- `DELETE /v1/apps/:app/env/:key` — delete environment variable.
  - Returns: `{"status": "deleted", "app": "myapp", "key": "NODE_ENV", "message": "Environment variable deleted successfully"}`

**Features:**
- Environment variables available during build phase (all lanes)
- Environment variables injected into Nomad job templates for runtime
- Consul KV storage with automatic fallback to file-based storage
- Full CRUD operations with proper error handling

## Storage Management Endpoints (Implemented)
- `GET /v1/storage/health` — get comprehensive storage system health status.
  - Returns: `{"timestamp": "2025-08-20T19:35:10Z", "status": "degraded", "checks": {...}, "summary": "...", "metrics": {...}}`
- `GET /v1/storage/metrics` — get detailed storage operation metrics.
  - Returns: `{"total_uploads": 42, "successful_uploads": 40, "failed_uploads": 2, ...}`
- `GET /v1/storage/config` — get current storage configuration.
  - Returns: `{"storage": {"provider": "seaweedfs", "master": "localhost:9333", ...}}`
- `POST /v1/storage/config/reload` — reload storage configuration without restart.
  - Returns: `{"reloaded": true, "config": {...}, "message": "Configuration reload completed"}`
- `POST /v1/storage/config/validate` — validate storage configuration.
  - Returns: `{"valid": true, "message": "Configuration is valid"}`

**Features:**
- SeaweedFS distributed storage with health monitoring
- Real-time configuration reload capabilities
- Comprehensive storage metrics and monitoring
- External YAML configuration management

## TTL Cleanup Endpoints (Implemented)
- `POST /v1/ttl/cleanup` — trigger manual TTL cleanup of preview allocations.
- `GET /v1/ttl/config` — get current TTL cleanup configuration.
- `POST /v1/ttl/config` — update TTL cleanup configuration.
- `GET /v1/ttl/stats` — get TTL cleanup statistics and history.

**Features:**
- Automatic preview allocation cleanup with configurable TTL
- Manual cleanup triggers for immediate resource recovery
- Comprehensive cleanup statistics and monitoring

## DNS Management Endpoints (Implemented)
- `POST /v1/dns/wildcard/setup` — configure wildcard DNS for domain.
  - Body: `{"target_ip": "192.168.1.100", "target_cname": "load-balancer.example.com", "ttl": 300, "load_balancer": ["192.168.1.100", "192.168.1.101"]}`
  - Returns: `{"status": "success", "message": "Wildcard DNS configured for *.ployd.app", "config": {...}}`
- `DELETE /v1/dns/wildcard` — remove wildcard DNS configuration.
  - Returns: `{"status": "success", "message": "Wildcard DNS removed for *.ployd.app"}`
- `GET /v1/dns/wildcard/validate` — validate wildcard DNS propagation.
  - Returns: `{"status": "valid", "message": "Wildcard DNS is properly configured for *.ployd.app"}`
- `GET /v1/dns/records` — list DNS records for domain.
  - Query params: `?domain=ployd.app` (optional)
  - Returns: `{"domain": "ployd.app", "records": [...], "count": 5}`
- `POST /v1/dns/records` — create DNS record.
  - Body: `{"hostname": "api.ployd.app", "type": "A", "value": "192.168.1.100", "ttl": 300}`
  - Returns: `{"status": "created", "record": {...}}`
- `PUT /v1/dns/records` — update DNS record.
  - Body: `{"hostname": "api.ployd.app", "type": "A", "value": "192.168.1.101", "ttl": 600}`
  - Returns: `{"status": "updated", "record": {...}}`
- `DELETE /v1/dns/records/:hostname/:type` — delete DNS record.
  - Returns: `{"status": "deleted", "hostname": "api.ployd.app", "type": "A"}`
- `GET /v1/dns/config` — get current DNS configuration.
  - Returns: `{"domain": "ployd.app", "target_ip": "192.168.1.100", "ttl": 300, ...}`
- `POST /v1/dns/config/validate` — validate DNS provider configuration.
  - Returns: `{"status": "valid", "message": "DNS provider configuration is valid"}`

**Features:**
- Multi-provider support (Cloudflare, Namecheap)
- Wildcard DNS configuration for automatic subdomain routing
- Individual DNS record management (A, AAAA, CNAME, TXT, MX)
- Load balancer IP configuration for high availability
- IPv6 support with AAAA records
- DNS propagation validation and testing
- Provider-agnostic configuration via JSON or environment variables

## Automated Remediation Framework Endpoints (Phase ARF-1 - Implemented)

### Recipe Management
- `GET /v1/arf/recipes` — list available transformation recipes with filtering.
  - Query params: `?language=java&category=cleanup&min_confidence=0.8`
  - Returns: `{"recipes": [...], "count": 42}`
- `GET /v1/arf/recipes/:id` — get detailed recipe information.
  - Returns: Recipe object with metadata, options, and usage statistics
- `POST /v1/arf/recipes` — create new transformation recipe.
  - Body: `{"id": "custom-recipe", "name": "...", "source": "org.openrewrite.java.cleanup.Custom", ...}`
  - Returns: `{"message": "Recipe created successfully", "recipe_id": "custom-recipe"}`
- `PUT /v1/arf/recipes/:id` — update existing recipe.
- `DELETE /v1/arf/recipes/:id` — delete recipe from catalog.
- `GET /v1/arf/recipes/search?q=<query>` — search recipes by name, description, or tags.
- `GET /v1/arf/recipes/:id/metadata` — get comprehensive recipe metadata.
- `GET /v1/arf/recipes/:id/stats` — get recipe usage statistics and performance metrics.

### Transformation Execution  
- `POST /v1/arf/transform` — execute code transformation with OpenRewrite.
  - Body: `{"recipe_id": "cleanup.unused-imports", "codebase": {"repository": "...", "branch": "main", "language": "java"}, "options": {...}}`
  - Returns: `{"recipe_id": "...", "success": true, "changes_applied": 5, "files_modified": ["Main.java"], "execution_time": "2s", "validation_score": 0.95}`
- `GET /v1/arf/transforms/:id` — get transformation result (future implementation).

### Sandbox Management
- `GET /v1/arf/sandboxes` — list active FreeBSD jail sandboxes.
  - Returns: `{"sandboxes": [...], "count": 3}`
- `POST /v1/arf/sandboxes` — create new isolated sandbox environment.
  - Body: `{"repository": "...", "branch": "main", "language": "java", "ttl": "30m", "memory_limit": "2G"}`
  - Returns: Sandbox object with jail details and expiration time
- `DELETE /v1/arf/sandboxes/:id` — destroy sandbox and cleanup resources.

### System Operations
- `GET /v1/arf/health` — comprehensive ARF system health check.
  - Returns: `{"status": "healthy", "components": {"engine": {...}, "cache": {...}}}`
- `GET /v1/arf/cache/stats` — AST cache performance metrics.
  - Returns: `{"hits": 1250, "misses": 200, "hit_rate": 0.86, "size": 1500, "memory_usage": 524288000}`
- `POST /v1/arf/cache/clear` — clear AST cache (maintenance operation).

## Webhook Events
- `build.started`, `build.completed`, `build.failed`
- `deploy.started`, `deploy.completed`, `deploy.failed`
- Payload: `{"event": "build.completed", "app": "myapp", "sha": "abc123", "timestamp": "...", "logs": "...", "metadata": {...}}`

## WebAssembly Runtime Endpoints (Lane G - Implemented)

### WASM Application Health and Metrics
When deployed to Lane G, WASM applications expose additional runtime endpoints via the ploy-wasm-runner service:

- `GET /<app>/health` — standard application health check
  - Returns: `{"status": "success", "message": "WASM module executed successfully", "runtime": "wazero", "timestamp": "..."}`
- `GET /<app>/wasm-health` — WASM runtime-specific health validation  
  - Returns: `{"status": "healthy", "wasm_runtime": "wazero", "module_loaded": true, "max_memory_mb": 64, "timeout": "30s"}`
- `GET /<app>/metrics` — Prometheus-compatible WASM runtime metrics
  - Returns: WASM execution counts, duration histograms, memory usage, and runtime statistics

### WASM Build Process
- **Automatic Detection**: Lane picker detects WASM compilation targets (Rust wasm32-wasi, Go js/wasm, AssemblyScript, Emscripten)
- **Multi-Strategy Builds**: Automatic build strategy selection based on project structure and language
- **Component Model**: Support for multi-module WASM applications with dependency management
- **Security Validation**: OPA policies with WASM-specific constraints for production deployments

### WASM Runtime Features
- **wazero Runtime**: Pure Go WebAssembly runtime v1.5.0 with no CGO dependencies
- **WASI Preview 1**: WebAssembly System Interface for controlled filesystem and environment access
- **Resource Limits**: Memory (64MB default, 128MB max), execution time (30s default), CPU constraints
- **Sandboxing**: Hardware-enforced isolation with process-level separation
- **Performance**: 10-50ms boot times, 5-30MB footprint

### Supported Languages and Compilation
- **Rust**: `cargo build --target wasm32-wasi` with wasm-bindgen integration
- **Go**: `GOOS=js GOARCH=wasm go build` with syscall/js support
- **C/C++**: Emscripten toolchain with WASI and browser targets  
- **AssemblyScript**: TypeScript-like syntax compiled to optimized WebAssembly
- **Component Model**: Multi-module applications with interface validation

Preview host (`<sha>.<app>.ployd.app`) calls `/v1/apps/:app/builds` and proxies on readiness.
