[Dockerfile](Dockerfile) Builds the Gradle-lane OpenRewrite CLI image and packages the shared orw-cli-runner Java launcher.
[orw-cli.sh](orw-cli.sh) Validates recipe/runtime inputs, imports optional CA certs, runs OpenRewrite CLI, and writes transform report artifacts.
[rewrite-runner/](rewrite-runner) Maven module descriptor for the shaded Java runner binary that executes OpenRewrite CLI commands.
