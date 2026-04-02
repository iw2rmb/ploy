[Dockerfile](Dockerfile) Builds the Gradle-lane OpenRewrite CLI image and embeds the shared `orw-cli-runner` Java launcher.
[orw-cli.sh](orw-cli.sh) Validates runtime inputs, materializes optional `PLOY_CA_CERTS` trust stores, runs OpenRewrite CLI, and writes report artifacts.
[rewrite-runner/](rewrite-runner) Maven module (`pom.xml`) that packages the shaded Java runner used as the `rewrite` binary entrypoint.
