[README.md](README.md) Defines the runtime contract for mig containers, including `/workspace`, `/in`, `/out`, and diff/artifact semantics.
[amata/](amata) Build context and helper scripts for the `amata` image that runs the amata-based migration workflow.
[build-and-push.sh](build-and-push.sh) Builds/pushes runtime+mig images, publishes `gate-gradle` tags, and mirrors Maven gate base images into the configured registry namespace.
[codex/](codex) Docker image context for the Codex-based mig runner with CLI setup and container entrypoint wiring.
[gates/](gates) Container build contexts and Gradle init/props files for gate images with remote build cache configuration.
[node/](node) Docker image context for the ploy node daemon, including packaged binary, entrypoint, and gate assets.
[orw/](orw) OpenRewrite CLI runner image contexts for Gradle/Maven lanes plus shared Java runner sources.
[server/](server) Docker image context for the ploy server daemon with gate profiles and runtime entrypoint setup.
[shell/](shell) Generic shell-based mig image context for running user-provided scripts in the mounted workspace.
