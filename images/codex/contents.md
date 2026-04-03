[Dockerfile](Dockerfile) Builds the Codex container image with required CLI dependencies, shell tooling, and the project entrypoint.
[build.sh](build.sh) Runs `docker buildx` to build and push the `codex:latest` image for a configurable platform and registry.
[entrypoint.sh](entrypoint.sh) Container command wrapper that prepares prompt/context, detects supported `codex exec` flags, runs Codex, and writes logs plus run metadata.
