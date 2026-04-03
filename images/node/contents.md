[Dockerfile](Dockerfile) Builds the `ployd-node` Alpine image with required CLI tools, runtime directories, gate assets, and container startup defaults.
[entrypoint.sh](entrypoint.sh) Sets TLS-related env vars from a readable `PLOY_CA_CERTS` bundle path, then execs the node daemon.
