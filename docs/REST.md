# Ploy REST API (v1)

## Core Endpoints
- `POST /v1/apps/:app/builds?sha=<sha>&lane=<A..F>&main=<MainClass>` — build & deploy; lane auto-picked if omitted.
- `GET /v1/apps` — list apps (stub).
- `GET /v1/status/:app` — controller status.

## Environment Variables Endpoints (Planned)
- `POST /v1/apps/:app/env` — set environment variable.
  - Body: `{"key": "API_KEY", "value": "secret123", "secret": true}`
- `GET /v1/apps/:app/env` — list all environment variables.
  - Returns: `{"env": [{"key": "NODE_ENV", "value": "production", "secret": false}]}`
- `GET /v1/apps/:app/env/:key` — get specific environment variable.
- `PUT /v1/apps/:app/env/:key` — update environment variable.
  - Body: `{"value": "new_value", "secret": false}`
- `DELETE /v1/apps/:app/env/:key` — delete environment variable.

## Self-Healing Loop Endpoints (Planned)
- `POST /v1/apps/:app/diff?verify=true&branch=<name>` — push diff to verification branch.
  - Body: patch/diff content
  - Returns: `{"branch": "verify-<timestamp>-<hash>", "url": "https://verify-<hash>.<app>.ployd.app"}`
- `POST /v1/apps/:app/webhooks` — configure app webhooks.
  - Body: `{"url": "https://example.com/webhook", "events": ["build.completed", "deploy.failed"], "secret": "..."}`
- `GET /v1/apps/:app/webhooks` — list app webhooks.
- `DELETE /v1/apps/:app/webhooks/:id` — remove webhook.

## Webhook Events
- `build.started`, `build.completed`, `build.failed`
- `deploy.started`, `deploy.completed`, `deploy.failed`
- Payload: `{"event": "build.completed", "app": "myapp", "sha": "abc123", "timestamp": "...", "logs": "...", "metadata": {...}}`

Preview host (`<sha>.<app>.ployd.app`) calls `/v1/apps/:app/builds` and proxies on readiness.
