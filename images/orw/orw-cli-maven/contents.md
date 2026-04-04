[Dockerfile](Dockerfile) Builds the Maven-lane OpenRewrite CLI image and embeds the shared `orw-cli-runner` Java launcher.
[orw-cli.sh](orw-cli.sh) Validates runtime inputs, imports Hydra CA bundle into trust stores, runs OpenRewrite CLI, and writes report artifacts.
[rewrite-runner/](rewrite-runner) Maven module (`pom.xml`) that packages the shaded Java runner used as the `rewrite` binary entrypoint.
