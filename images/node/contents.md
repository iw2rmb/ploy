[Dockerfile](Dockerfile) Builds the `ployd-node` Alpine image with required CLI tools, runtime directories, gate assets, and container startup defaults.
[entrypoint.sh](entrypoint.sh) Sets TLS-related env vars from the Hydra CA cert path, then execs the node daemon.
