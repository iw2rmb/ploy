[Dockerfile](Dockerfile) Builds the amata runner image with codex/crush/cc tools and ploy runtime defaults.
[build-amata.sh](build-amata.sh) Builds the amata binary from sibling source and stages it into this image context.
[build.sh](build.sh) Wrapper script that builds/stages amata and runs `docker buildx` to publish the image.
[entrypoint.sh](entrypoint.sh) Container entrypoint that activates CCR when configured, runs amata, and emits codex artifacts to `/out`.
