[docker-compose.yml](docker-compose.yml) Docker Compose stack defining runtime server, node, and shared cache services for local/runtime deploy.
[gradle-build-cache/](gradle-build-cache) Gradle build-cache service seed config and entrypoint used by the runtime compose stack.
[node/](node) Runtime node service configuration consumed by the node container startup.
[run.sh](run.sh) Runtime deployment helper that validates env, prepares state, and orchestrates docker compose operations.
[server/](server) Runtime server service configuration consumed by the server container startup.
