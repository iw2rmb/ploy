# orw-cli-maven: standalone OpenRewrite CLI runner for Maven repositories.
#
# This image runs OpenRewrite through a single CLI command path and never
# invokes Gradle/Maven project tasks.
ARG JAVA_BASE_TEMURIN_17_IMAGE=java-base-temurin:jdk17
FROM --platform=$BUILDPLATFORM maven:3.9.11-eclipse-temurin-17 AS rewrite-runner-build

WORKDIR /build
COPY images/install_ploy_ca_bundle.sh /usr/local/lib/ploy/install_ploy_ca_bundle.sh
COPY images/orw/orw-cli-maven/rewrite-runner/pom.xml rewrite-runner/pom.xml
RUN --mount=type=cache,target=/root/.m2 \
    --mount=type=secret,id=ploy_ca_certs,required=false \
    bash -euxo pipefail -c 'if [[ -s /run/secrets/ploy_ca_certs ]]; then mkdir -p /etc/ploy/certs; cp /run/secrets/ploy_ca_certs /etc/ploy/certs/ca.crt; source /usr/local/lib/ploy/install_ploy_ca_bundle.sh; PLOY_CA_IMPORT_JAVA=1 install_ploy_ca_bundle; fi; cd rewrite-runner && mvn -q -DskipTests dependency:go-offline'
COPY images/orw/shared/orw-cli-runner/src rewrite-runner/src
RUN --mount=type=cache,target=/root/.m2 \
    --mount=type=secret,id=ploy_ca_certs,required=false \
    bash -euxo pipefail -c 'if [[ -s /run/secrets/ploy_ca_certs ]]; then mkdir -p /etc/ploy/certs; cp /run/secrets/ploy_ca_certs /etc/ploy/certs/ca.crt; source /usr/local/lib/ploy/install_ploy_ca_bundle.sh; PLOY_CA_IMPORT_JAVA=1 install_ploy_ca_bundle; fi; cd rewrite-runner && mvn -q -DskipTests package'

FROM --platform=$TARGETOS/$TARGETARCH ${JAVA_BASE_TEMURIN_17_IMAGE}

LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="orw-cli-maven" \
    org.opencontainers.image.description="Ploy Migs: standalone OpenRewrite CLI runner (Maven lane)" \
    org.opencontainers.image.licenses="MIT"

WORKDIR /workspace

COPY --from=rewrite-runner-build /build/rewrite-runner/target/orw-cli-runner.jar /opt/orw-cli-runner/orw-cli-runner.jar
RUN printf '%s\n' '#!/usr/bin/env bash' \
    'set -euo pipefail' \
    'exec java -jar /opt/orw-cli-runner/orw-cli-runner.jar "$@"' \
    > /usr/local/bin/rewrite && chmod +x /usr/local/bin/rewrite

COPY images/orw/shared/orw-lib.sh /usr/local/lib/ploy/orw-lib.sh
COPY --chmod=755 images/orw/orw-cli-maven/orw-cli.sh /usr/local/bin/orw-cli
RUN ln -s /usr/local/bin/orw-cli /usr/local/bin/migs-orw-cli

ARG BUILD_TS
LABEL org.opencontainers.image.created="$BUILD_TS"

ENTRYPOINT ["migs-orw-cli"]
CMD ["--apply", "--dir", "/workspace", "--out", "/out"]
