[Dockerfile](Dockerfile) Builds the Amata runner image with codex/claude/crush tooling and the Amata binary entrypoint.
[build-amata.sh](build-amata.sh) Compiles the sibling `amata` Go CLI and stages the binary into this image build context.
[build.sh](build.sh) Builds and pushes the `amata:latest` container image after staging the Amata binary.
[entrypoint.sh](entrypoint.sh) Runs Amata in mig containers, captures execution logs, and writes codex-compatible output artifacts.
