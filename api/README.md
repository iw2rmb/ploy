# Ploy REST API (v1)

## Health and Monitoring Endpoints
- `GET /health` — basic service health check
- `GET /ready` — comprehensive readiness probe  
- `GET /live` — simple liveness probe
- `GET /health/metrics` — health check metrics for monitoring
- `GET /health/deployment` — deployment status information
- `GET /health/update` — system update status
- `GET /health/platform-certificates` — platform wildcard certificate health
- `GET /health/coordination` — leader election and coordination status
- `GET /metrics` — Prometheus metrics endpoint
- **Versioned Access**: All health endpoints also available at `/v1/*`

## Core Application Management
- `POST /v1/apps/:app/builds?sha=<sha>&lane=<A..G>&main=<MainClass>` — build & deploy user application
- `GET /v1/apps` — list all user applications
- `GET /v1/apps/:app/status` — get application deployment status
- `GET /v1/apps/:app/logs` — get application logs
- `POST /v1/apps/:app/debug` — create debug instance with SSH access
- `POST /v1/apps/:app/rollback` — rollback app to previous version
- `DELETE /v1/apps/:app` — destroy application and all resources
- `POST /v1/builds/:app` — legacy build endpoint (backward compatibility)

## Platform Service Management
- `POST /v1/platform/:service/deploy?sha=<sha>&env=<dev|staging|prod>` — deploy platform service
- `POST /v1/platform/:service/builds` — build platform service
- `GET /v1/platform/:service/status` — get platform service status
- `POST /v1/platform/:service/rollback?version=<version>` — rollback to specific version
- `GET /v1/platform/:service/logs?lines=<100>&follow=<false>` — stream service logs
- `DELETE /v1/platform/:service` — remove platform service

**Note**: Platform services deploy to `{service}.ployman.app` domains and use higher Nomad priority.

## Domain Management
- `POST /v1/apps/:app/domains` — add domain to app with automatic certificate provisioning
- `GET /v1/apps/:app/domains` — list domains for app with certificate information
- `DELETE /v1/apps/:app/domains/:domain` — remove domain from app

## Certificate Management
- `GET /v1/apps/:app/certificates` — list all certificates for app
- `GET /v1/apps/:app/certificates/:domain` — get certificate details for domain
- `POST /v1/apps/:app/certificates/:domain/provision` — manually provision certificate
- `POST /v1/apps/:app/certificates/:domain/upload` — upload custom certificate bundle
- `DELETE /v1/apps/:app/certificates/:domain` — remove certificate for domain

## Environment Variables Management
- `POST /v1/apps/:app/env` — set multiple environment variables
- `GET /v1/apps/:app/env` — list all environment variables
- `PUT /v1/apps/:app/env/:key` — update single environment variable
- `DELETE /v1/apps/:app/env/:key` — delete environment variable

## Blue-Green Deployment Management
- `POST /v1/apps/:app/deploy/blue-green` — start blue-green deployment with new version
- `GET /v1/apps/:app/blue-green/status` — get current deployment status
- `POST /v1/apps/:app/blue-green/shift` — manually shift traffic between versions
- `POST /v1/apps/:app/blue-green/auto-shift` — automatically shift traffic using default strategy
- `POST /v1/apps/:app/blue-green/complete` — complete deployment (100% green)
- `POST /v1/apps/:app/blue-green/rollback` — rollback to previous version

## Static Analysis System
- `POST /v1/analysis/analyze` — run static analysis on repository
- `GET /v1/analysis/results/:id` — get specific analysis result
- `GET /v1/analysis/results` — list analysis history
- `GET /v1/analysis/config` — get analysis configuration
- `PUT /v1/analysis/config` — update analysis configuration
- `POST /v1/analysis/config/validate` — validate configuration
- `GET /v1/analysis/languages` — list supported languages
- `GET /v1/analysis/languages/:language/info` — get analyzer info
- `GET /v1/analysis/issues/:id/fixes` — get fix suggestions for issue
- `POST /v1/analysis/issues/:id/fix` — apply fix for issue
- `DELETE /v1/analysis/cache` — clear analysis cache
- `GET /v1/analysis/cache/metrics` — get cache performance metrics
- `GET /v1/analysis/health` — check analysis service health

**Note**: OpenRewrite transformations are executed through the unified ARF system using `/v1/arf/transform` with OpenRewrite recipes managed via `/v1/arf/recipes/*` endpoints with `type: "openrewrite"`.

## Storage Management
- `GET /v1/storage/health` — get comprehensive storage system health status
- `GET /v1/storage/metrics` — get detailed storage operation metrics
- `GET /v1/storage/config` — get current storage configuration
- `POST /v1/storage/config/reload` — reload storage configuration without restart
- `POST /v1/storage/config/validate` — validate storage configuration

## TTL Cleanup System
- `GET /v1/cleanup/status` — get cleanup status and statistics
- `GET /v1/cleanup/jobs` — list preview jobs for cleanup
- `GET /v1/cleanup/config` — get cleanup configuration
- `PUT /v1/cleanup/config` — update cleanup configuration
- `GET /v1/cleanup/config/defaults` — get default configuration

## DNS Management System
- `POST /v1/dns/wildcard/setup` — configure wildcard DNS for domain
- `DELETE /v1/dns/wildcard` — remove wildcard DNS configuration
- `GET /v1/dns/wildcard/validate` — validate wildcard DNS propagation
- `GET /v1/dns/records` — list DNS records for domain
- `POST /v1/dns/records` — create DNS record
- `PUT /v1/dns/records` — update DNS record
- `DELETE /v1/dns/records/:hostname/:type` — delete DNS record
- `GET /v1/dns/status` — get DNS system status
- `GET /v1/dns/config` — get DNS configuration
- `POST /v1/dns/config/validate` — validate DNS provider configuration


## System Management

### Self-Update System
- `POST /v1/update` — update controller to latest version
- `GET /v1/update/status` — get update status
- `POST /v1/update/validate` — validate update package
- `POST /v1/rollback` — rollback to previous version
- `GET /v1/versions` — list available versions

### Template Management
- `POST /v1/templates/sync` — sync templates
- `GET /v1/templates/status` — get template status

### Version Information
- `GET /version` — get basic version information
- `GET /version/detailed` — get detailed version information
- `GET /v1/version` — get version (API versioned)
- `GET /v1/version/detailed` — get detailed version (API versioned)

## Automated Remediation Framework (ARF)

### Recipe Management
- `GET /v1/arf/recipes` — list available transformation recipes
- `GET /v1/arf/recipes/:id` — get detailed recipe information
- `POST /v1/arf/recipes` — create new transformation recipe
- `PUT /v1/arf/recipes/:id` — update existing recipe
- `DELETE /v1/arf/recipes/:id` — delete recipe from catalog
- `GET /v1/arf/recipes/search` — search recipes by name or tags
- `POST /v1/arf/recipes/upload` — upload recipe
- `POST /v1/arf/recipes/validate` — validate recipe (supports OpenRewrite recipes with `type: "openrewrite"`)
- `GET /v1/arf/recipes/:id/download` — download recipe
- `GET /v1/arf/recipes/:id/metadata` — get recipe metadata
- `GET /v1/arf/recipes/:id/stats` — get recipe usage statistics
- `POST /v1/arf/recipes/register` — register recipe from runner

### Model Management
- `GET /v1/arf/models` — get available models
- `POST /v1/arf/models` — add new model
- `PUT /v1/arf/models` — import models
- `DELETE /v1/arf/models/:name` — remove model
- `POST /v1/arf/models/:name/set-default` — set default model

### Transformation & Sandbox Operations
- `POST /v1/arf/transform` — execute code transformation (supports OpenRewrite recipes)
- `GET /v1/arf/transforms/:id` — get transformation result
- `GET /v1/arf/sandboxes` — list active sandboxes
- `POST /v1/arf/sandboxes` — create new sandbox
- `DELETE /v1/arf/sandboxes/:id` — destroy sandbox

### Pattern Learning & LLM Integration
- `POST /v1/arf/recipes/generate` — generate LLM recipe
- `POST /v1/arf/transform/hybrid` — execute hybrid transformation
- `POST /v1/arf/strategy/select` — select transformation strategy
- `POST /v1/arf/complexity/analyze` — analyze codebase complexity
- `POST /v1/arf/learning/outcome` — record transformation outcome
- `GET /v1/arf/learning/patterns` — extract learning patterns

### Security & SBOM Analysis
- `POST /v1/arf/security/scan` — security scan
- `POST /v1/arf/security/remediation` — generate remediation plan
- `GET /v1/arf/security/report` — get security report
- `GET /v1/arf/security/report/:id` — get security report by ID
- `GET /v1/arf/security/compliance` — get compliance status
- `POST /v1/arf/sbom/generate` — generate SBOM
- `POST /v1/arf/sbom/analyze` — analyze SBOM
- `GET /v1/arf/sbom/compliance` — get SBOM compliance
- `GET /v1/arf/sbom/report` — get SBOM report
- `GET /v1/arf/sbom/:id` — get SBOM by ID



**Note**: ARF provides comprehensive code transformation, analysis, and remediation capabilities with advanced LLM integration, security scanning, and deployment testing.

## Webhook Events
- `build.started`, `build.completed`, `build.failed`
- `deploy.started`, `deploy.completed`, `deploy.failed`
- Payload: `{"event": "build.completed", "app": "myapp", "sha": "abc123", "timestamp": "...", "logs": "...", "metadata": {...}}`

## WebAssembly Runtime (Lane G)
When deployed to Lane G, WASM applications expose additional runtime endpoints:

- `GET /<app>/health` — standard application health check
- `GET /<app>/wasm-health` — WASM runtime-specific health validation  
- `GET /<app>/metrics` — Prometheus-compatible WASM runtime metrics

**Features**: wazero runtime, WASI Preview 1, automatic detection for Rust/Go/C++/AssemblyScript, hardware-enforced isolation, 10-50ms boot times.
