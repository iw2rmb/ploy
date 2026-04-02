[Dockerfile](Dockerfile) Builds the `ployd` Alpine server image with runtime dependencies, gate assets, schema file, and default command wiring.
[entrypoint.sh](entrypoint.sh) Materializes CA certificates from `PLOY_CA_CERTS` and then execs the server daemon binary.
