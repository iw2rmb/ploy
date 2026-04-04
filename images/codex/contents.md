[Dockerfile](Dockerfile) Builds the direct Codex runtime image with CLI tooling and the wrapper entrypoint script.
[build.sh](build.sh) Builds and pushes the `codex:latest` container image for the configured registry.
[entrypoint.sh](entrypoint.sh) Wraps `codex exec` for mig jobs, loading prompt/config inputs and persisting run/session artifacts.
