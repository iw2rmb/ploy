# Ploy REST API (v1)

## Core Endpoints
- `POST /v1/apps/:app/builds?sha=<sha>&lane=<A..F>&main=<MainClass>` — build & deploy; lane auto-picked if omitted.
- `GET /v1/apps` — list apps (stub).
- `GET /v1/status/:app` — controller status.

## Domain Management Endpoints (Implemented)
- `POST /v1/apps/:app/domains` — add domain to app.
  - Body: `{"domain": "example.com"}`
  - Returns: `{"status": "added", "app": "myapp", "domain": "example.com", "message": "Domain registered successfully"}`
- `GET /v1/apps/:app/domains` — list domains for app.
  - Returns: `{"app": "myapp", "domains": ["myapp.ployd.app", "example.com"]}`
- `DELETE /v1/apps/:app/domains/:domain` — remove domain from app.
  - Returns: `{"status": "removed", "app": "myapp", "domain": "example.com", "message": "Domain removed successfully"}`

## Certificate Management Endpoints (Implemented)
- `POST /v1/certs/issue` — issue TLS certificate.
  - Body: `{"domain": "example.com"}`
  - Returns: `{"status": "issued", "domain": "example.com", "message": "Certificate issued successfully", "expires": "2025-11-18"}`
- `GET /v1/certs` — list all managed certificates.
  - Returns: `{"certificates": [{"domain": "example.com", "status": "valid", "expires": "2025-11-18"}]}`

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
- File-based storage with JSON persistence
- Full CRUD operations with proper error handling

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
