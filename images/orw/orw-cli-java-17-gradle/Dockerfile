# orw-cli-java-17-gradle: standalone OpenRewrite CLI runner for Gradle repositories.
#
# This image runs OpenRewrite through a single CLI command path and never
# invokes Gradle/Maven project tasks.
ARG JAVA_BASE_TEMURIN_17_IMAGE=java-base-temurin:jdk17
ARG JAVA_BASE_MAVEN_17_IMAGE=java-base-maven:jdk17
FROM --platform=linux/amd64 ${JAVA_BASE_MAVEN_17_IMAGE} AS rewrite-runner-build

WORKDIR /build
COPY images/orw/orw-cli-java-17-gradle/rewrite-runner/pom.xml rewrite-runner/pom.xml
RUN --mount=type=cache,target=/root/.m2 \
    bash -euxo pipefail -c 'cd rewrite-runner && mvn -q -DskipTests dependency:go-offline'
COPY images/orw/shared/orw-cli-runner/src rewrite-runner/src
RUN --mount=type=cache,target=/root/.m2 \
    bash -euxo pipefail -c 'cd rewrite-runner && mvn -q -DskipTests package'

FROM --platform=$TARGETOS/$TARGETARCH ${JAVA_BASE_TEMURIN_17_IMAGE}

LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="orw-cli-java-17-gradle" \
    org.opencontainers.image.description="Ploy Migs: standalone OpenRewrite CLI runner (Gradle lane)" \
    org.opencontainers.image.licenses="MIT"

WORKDIR /workspace

COPY --from=rewrite-runner-build /build/rewrite-runner/target/orw-cli-runner.jar /opt/orw-cli-runner/orw-cli-runner.jar
RUN printf '%s\n' '#!/usr/bin/env bash' \
    'set -euo pipefail' \
    'exec java -jar /opt/orw-cli-runner/orw-cli-runner.jar "$@"' \
    > /usr/local/bin/rewrite && chmod +x /usr/local/bin/rewrite

COPY images/orw/shared/orw-lib.sh /usr/local/lib/ploy/orw-lib.sh
COPY --chmod=755 images/orw/orw-cli-java-17-gradle/orw-cli.sh /usr/local/bin/orw-cli
RUN ln -s /usr/local/bin/orw-cli /usr/local/bin/migs-orw-cli

ARG BUILD_TS
LABEL org.opencontainers.image.created="$BUILD_TS"

ENTRYPOINT ["migs-orw-cli"]
CMD ["--apply", "--dir", "/workspace", "--out", "/out"]
