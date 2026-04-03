[Dockerfile](Dockerfile) Builds the `ployd` Alpine server image with runtime dependencies, gate assets, schema file, and default command wiring.
[entrypoint.sh](entrypoint.sh) Sets TLS-related env vars from a readable `PLOY_CA_CERTS` bundle path, then execs the server daemon.
