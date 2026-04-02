[Dockerfile](Dockerfile) Builds the server runtime image with ployd, gate assets, schema bundle, and runtime dependencies.
[entrypoint.sh](entrypoint.sh) Server container entrypoint that materializes PLOY_CA_CERTS into TLS trust env vars before launching ployd.
