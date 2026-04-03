[Dockerfile](Dockerfile) Defines the Codex runtime image with required CLI/tools, cert paths, and the container entrypoint.
[build.sh](build.sh) Builds and pushes the `codex:latest` image via `docker buildx` with configurable platform and registry prefix.
[entrypoint.sh](entrypoint.sh) Parses runtime args, loads Hydra-delivered prompt/config, executes `codex exec`, and writes run logs/manifests.
