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
- `POST /v1/apps/:app/builds?sha=<sha>&lane=<A..F>&main=<MainClass>` — build & deploy; lane auto-picked if omitted.
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
  - Query params: `?lane=<A-F>` (optional)
  - Body: `{"ssh_enabled": true}`
  - Returns: `{"status": "debug_created", "app": "myapp", "instance": "debug-myapp-123", "ssh_enabled": true, "ssh_command": "ssh debug@debug-myapp-123.debug.ployd.app"}`
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

## Automated Remediation Framework Endpoints (Planned)
- `POST /v1/arf/transform` — execute code transformation on repositories.
  - Body: `{"repositories": ["repo1", "repo2"], "recipe": "spring-boot-2-to-3", "strategy": "hybrid"}`
  - Returns: `{"job_id": "arf-123", "status": "started", "estimated_time": "2h"}`
- `GET /v1/arf/jobs/:id` — check transformation job status.
  - Returns: `{"job_id": "arf-123", "status": "running", "progress": 65, "repositories_completed": 13, "repositories_total": 20}`
- `POST /v1/arf/recipes` — create or update transformation recipe.
  - Body: OpenRewrite recipe YAML content
- `GET /v1/arf/recipes` — list available transformation recipes.
- `POST /v1/apps/:app/webhooks` — configure app webhooks for ARF events.
  - Body: `{"url": "https://example.com/webhook", "events": ["transform.completed", "transform.failed"], "secret": "..."}`

## Webhook Events
- `build.started`, `build.completed`, `build.failed`
- `deploy.started`, `deploy.completed`, `deploy.failed`
- Payload: `{"event": "build.completed", "app": "myapp", "sha": "abc123", "timestamp": "...", "logs": "...", "metadata": {...}}`

Preview host (`<sha>.<app>.ployd.app`) calls `/v1/apps/:app/builds` and proxies on readiness.
