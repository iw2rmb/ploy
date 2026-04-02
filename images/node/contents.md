[Dockerfile](Dockerfile) Builds the `ployd-node` Alpine image with required CLI tools, runtime directories, gate assets, and container startup defaults.
[entrypoint.sh](entrypoint.sh) Materializes CA certificates from `PLOY_CA_CERTS` and then execs the node daemon binary.
