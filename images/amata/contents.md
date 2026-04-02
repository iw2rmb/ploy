[Dockerfile](Dockerfile) Container image definition for the amata runner environment and bundled CLI tooling.
[build-amata.sh](build-amata.sh) Builds the amata binary from sibling source and stages it into this image context.
[build.sh](build.sh) Wrapper script that builds/stages amata and runs `docker buildx` to publish the image.
[entrypoint.sh](entrypoint.sh) Runtime entrypoint that prepares config, executes amata, and writes `/out` run artifacts.
