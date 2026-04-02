[Dockerfile](Dockerfile) Builds the node runtime image with ployd-node binary, base tools, gate assets, and non-root runtime defaults.
[entrypoint.sh](entrypoint.sh) Node container entrypoint that materializes PLOY_CA_CERTS into TLS trust env vars before launching ployd-node.
