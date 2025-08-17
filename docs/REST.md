# Ploy REST API (v1)

- `POST /v1/apps/:app/builds?sha=<sha>&lane=<A..F>&main=<MainClass>` — build & deploy; lane auto-picked if omitted.
- `GET /v1/apps` — list apps (stub).
- `GET /v1/status/:app` — controller status.

Preview host (`<sha>.<app>.ployd.app`) calls `/v1/apps/:app/builds` and proxies on readiness.
