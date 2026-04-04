[docker-compose.yml](docker-compose.yml) Docker Compose stack for runtime server, node, and Gradle cache services with required env, mounts, and health checks.
[gradle-build-cache/](gradle-build-cache) Gradle build-cache seed config and entrypoint that boots a writable local cache node for gate workloads.
[node/](node) Node runtime YAML defining control-plane endpoint, node identity, and heartbeat/listener settings.
[run.sh](run.sh) Cluster deploy helper that validates prerequisites, provisions tokens/auth state, and orchestrates compose startup and seed actions.
[server/](server) Server runtime YAML configuring HTTP/metrics listeners, admin socket path, bearer-token auth mode, and Postgres DSN field.
