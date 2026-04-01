[Dockerfile](Dockerfile) Container image definition for the Codex runner environment used by MIG workflows.
[build.sh](build.sh) Helper script to build and push the Codex container image with configurable platform/registry.
[entrypoint.sh](entrypoint.sh) Runtime launcher that materializes config, runs Codex (or amata compatibility mode), and writes artifacts.
