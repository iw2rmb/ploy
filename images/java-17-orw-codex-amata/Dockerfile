ARG NODE_IMAGE=node:22-bookworm-slim

FROM --platform=$BUILDPLATFORM maven:3.9.11-eclipse-temurin-17 AS rewrite-runner-build

WORKDIR /build
COPY images/install_ploy_ca_bundle.sh /usr/local/lib/ploy/install_ploy_ca_bundle.sh
COPY images/amata/rewrite-runner/pom.xml rewrite-runner/pom.xml
COPY images/orw/shared/orw-cli-runner/src rewrite-runner/src
RUN --mount=type=cache,target=/root/.m2 \
    --mount=type=secret,id=ploy_ca_certs,required=false \
    bash -euxo pipefail -c 'if [[ -s /run/secrets/ploy_ca_certs ]]; then mkdir -p /etc/ploy/certs; cp /run/secrets/ploy_ca_certs /etc/ploy/certs/ca.crt; source /usr/local/lib/ploy/install_ploy_ca_bundle.sh; PLOY_CA_IMPORT_JAVA=1 install_ploy_ca_bundle; fi; cd rewrite-runner && mvn -q -DskipTests package'

FROM --platform=$TARGETOS/$TARGETARCH maven:3.9.11-eclipse-temurin-17 AS maven-tools
FROM --platform=$TARGETOS/$TARGETARCH gradle:8.8-jdk17 AS gradle-tools

FROM ${NODE_IMAGE}

LABEL org.opencontainers.image.title="java-17-orw-codex-amata" \
    org.opencontainers.image.description="Java 17 image with Maven, Gradle, OpenRewrite, Codex CLI, and Amata" \
    org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.licenses="MIT"

COPY images/install_ploy_ca_bundle.sh /usr/local/lib/ploy/install_ploy_ca_bundle.sh
RUN --mount=type=secret,id=ploy_ca_certs,required=false \
    bash -euxo pipefail -c 'if [[ -s /run/secrets/ploy_ca_certs ]]; then \
      mkdir -p /etc/ploy/certs; \
      cp /run/secrets/ploy_ca_certs /etc/ploy/certs/ca.crt; \
      source /usr/local/lib/ploy/install_ploy_ca_bundle.sh; \
      PLOY_CA_IMPORT_JAVA=1 install_ploy_ca_bundle; \
    fi \
    && if [[ -s /run/secrets/ploy_ca_certs ]] && [[ -f /etc/apt/sources.list.d/debian.sources ]]; then \
      sed -i "s|http://deb.debian.org|https://deb.debian.org|g" /etc/apt/sources.list.d/debian.sources; \
    fi \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash ca-certificates coreutils curl fd-find findutils gawk git jq ripgrep unzip \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /workspace /in /out /out/codex /etc/ploy/certs /root/.gradle/init.d \
    && chmod 700 /out/codex \
    && chmod 755 /etc/ploy/certs'

COPY --from=maven-tools /opt/java/openjdk /opt/java/openjdk
COPY --from=maven-tools /usr/share/maven /usr/share/maven
COPY --from=maven-tools /usr/share/maven/ref /usr/share/maven/ref
COPY --from=maven-tools /usr/bin/mvn /usr/bin/mvn
COPY --from=gradle-tools /opt/gradle /opt/gradle
COPY --from=gradle-tools /usr/bin/gradle /usr/bin/gradle

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN ln -sf /usr/bin/fdfind /usr/local/bin/fd \
    && ln -sf /opt/java/openjdk/bin/java /usr/bin/java \
    && ln -sf /opt/java/openjdk/bin/keytool /usr/bin/keytool \
    && if command -v update-ca-certificates >/dev/null 2>&1; then update-ca-certificates || true; fi

RUN echo 'export PATH="/usr/local/bin:$PATH"' >> /root/.bashrc
RUN echo 'if [ -f ~/.bashrc ]; then . ~/.bashrc; fi' >> /root/.bash_profile

ENV JAVA_HOME=/opt/java/openjdk \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    MAVEN_HOME=/usr/share/maven \
    MAVEN_CONFIG=/root/.m2 \
    GRADLE_HOME=/opt/gradle \
    PATH="/opt/java/openjdk/bin:/usr/share/maven/bin:/opt/gradle/bin:/usr/local/bin:${PATH}" \
    WORKSPACE=/workspace \
    OUTDIR=/out \
    PLOY_SERVER_URL="" \
    PLOY_API_TOKEN="" \
    PLOY_CLIENT_CERT_PATH="/etc/ploy/certs/client.crt" \
    PLOY_CLIENT_KEY_PATH="/etc/ploy/certs/client.key"

RUN --mount=type=secret,id=ploy_ca_certs,required=false \
    if [[ -s /run/secrets/ploy_ca_certs ]]; then \
      mkdir -p /etc/ploy/certs; \
      cp /run/secrets/ploy_ca_certs /etc/ploy/certs/ca.crt; \
      source /usr/local/lib/ploy/install_ploy_ca_bundle.sh; \
      PLOY_CA_IMPORT_JAVA=1 install_ploy_ca_bundle; \
    fi \
    && npm install -g @openai/codex @charmland/crush @anthropic-ai/claude-code @musistudio/claude-code-router \
    && npm cache clean --force \
    && rm -rf /root/.npm /tmp/*

COPY images/amata/entrypoint.sh /usr/local/bin/ploy/entrypoint.sh
COPY images/gates/gradle/gradle.properties /root/.gradle/gradle.properties
COPY images/gates/gradle/cache.init.gradle /root/.gradle/init.d/cache.init.gradle
COPY images/amata/amata /usr/local/bin/amata
COPY images/amata/orw-cli.sh /usr/local/bin/orw-cli
COPY images/amata/heal-orw.sh /usr/local/bin/heal-orw
COPY --from=rewrite-runner-build /build/rewrite-runner/target/orw-cli-runner.jar /opt/orw-cli-runner/orw-cli-runner.jar

RUN printf '%s\n' '#!/usr/bin/env bash' \
    'set -euo pipefail' \
    'exec java -jar /opt/orw-cli-runner/orw-cli-runner.jar "$@"' \
    > /usr/local/bin/rewrite \
    && chmod +x /usr/local/bin/rewrite

RUN chmod +x \
    /usr/local/bin/ploy/entrypoint.sh \
    /usr/local/lib/ploy/install_ploy_ca_bundle.sh \
    /usr/local/bin/amata \
    /usr/local/bin/orw-cli \
    /usr/local/bin/heal-orw

WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/ploy/entrypoint.sh"]
