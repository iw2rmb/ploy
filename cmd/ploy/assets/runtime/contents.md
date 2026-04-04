[docker-compose.yml](docker-compose.yml) Docker Compose stack that runs runtime server, node, and Gradle build-cache services with required mounts and env wiring.
[gradle-build-cache/](gradle-build-cache) Seed config and bootstrap entrypoint for the local Gradle build-cache service used by runtime workloads.
[node/](node) Node runtime configuration defining server endpoint, node identity, and heartbeat/listen settings.
[run.sh](run.sh) Runtime deploy script that validates prerequisites, resolves image/version env, provisions auth and tokens, and orchestrates compose/bootstrap flow.
[server/](server) Server runtime configuration for HTTP and metrics listeners, admin socket path, auth mode, and Postgres DSN placeholder.
