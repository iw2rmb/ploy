# orw-cli-gradle: standalone OpenRewrite CLI runner for Gradle repositories.
#
# This image runs OpenRewrite through a single CLI command path and never
# invokes Gradle/Maven project tasks.
FROM --platform=$BUILDPLATFORM maven:3.9.11-eclipse-temurin-17 AS rewrite-runner-build

WORKDIR /build
COPY deploy/images/orw/orw-cli-gradle/rewrite-runner/pom.xml rewrite-runner/pom.xml
COPY deploy/images/orw/shared/orw-cli-runner/src rewrite-runner/src
RUN --mount=type=cache,target=/root/.m2 \
    bash -euxo pipefail -c 'cd rewrite-runner && mvn -q -DskipTests package'

FROM --platform=$TARGETOS/$TARGETARCH eclipse-temurin:17-jdk

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="orw-cli-gradle" \
    org.opencontainers.image.description="Ploy Migs: standalone OpenRewrite CLI runner (Gradle lane)" \
    org.opencontainers.image.created="$BUILD_TS"

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash ca-certificates git jq \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
ENV ORW_BUILD_SYSTEM=gradle

COPY --from=rewrite-runner-build /build/rewrite-runner/target/orw-cli-runner.jar /opt/orw-cli-runner/orw-cli-runner.jar
RUN printf '%s\n' '#!/usr/bin/env bash' \
    'set -euo pipefail' \
    'exec java -jar /opt/orw-cli-runner/orw-cli-runner.jar "$@"' \
    > /usr/local/bin/rewrite && chmod +x /usr/local/bin/rewrite

COPY --chmod=755 deploy/images/orw/orw-cli-gradle/orw-cli.sh /usr/local/bin/orw-cli
RUN ln -s /usr/local/bin/orw-cli /usr/local/bin/migs-orw-cli

ENTRYPOINT ["migs-orw-cli"]
CMD ["--apply", "--dir", "/workspace", "--out", "/out"]
