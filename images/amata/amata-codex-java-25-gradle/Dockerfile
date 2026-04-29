ARG NODE_IMAGE=node:22-bookworm-slim
ARG JAVA_BASE_GRADLE_25_IMAGE=java-base-gradle:jdk25

FROM --platform=$TARGETOS/$TARGETARCH ${JAVA_BASE_GRADLE_25_IMAGE} AS gradle-tools

FROM ${NODE_IMAGE}
ARG TARGETARCH

LABEL org.opencontainers.image.title="amata-codex-java-25-gradle" \
    org.opencontainers.image.description="Java 25 image with Gradle, Codex CLI, and Amata" \
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
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash ca-certificates coreutils curl fd-find findutils gawk git jq ripgrep unzip \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /workspace /in /out /out/codex /etc/ploy/certs /root/.gradle/init.d \
    && chmod 700 /out/codex \
    && chmod 755 /etc/ploy/certs'

COPY --from=gradle-tools /opt/java/openjdk /opt/java/openjdk
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
    GRADLE_HOME=/opt/gradle \
    GRADLE_USER_HOME=/root/.gradle \
    PATH="/opt/java/openjdk/bin:/opt/gradle/bin:/usr/local/bin:${PATH}" \
    WORKSPACE=/workspace \
    OUTDIR=/out \
    PLOY_SERVER_URL="" \
    PLOY_CLIENT_CERT_PATH="/etc/ploy/certs/client.crt"

RUN --mount=type=secret,id=ploy_ca_certs,required=false \
    if [[ -s /run/secrets/ploy_ca_certs ]]; then \
      mkdir -p /etc/ploy/certs; \
      cp /run/secrets/ploy_ca_certs /etc/ploy/certs/ca.crt; \
      source /usr/local/lib/ploy/install_ploy_ca_bundle.sh; \
      install_ploy_ca_bundle; \
    fi \
    && npm install -g @openai/codex \
    && npm cache clean --force \
    && rm -rf /root/.npm /tmp/*

COPY images/amata/entrypoint.sh /usr/local/bin/ploy/entrypoint.sh
COPY images/gates/gradle/gradle.properties /root/.gradle/gradle.properties
COPY images/gates/gradle/cache.init.gradle /root/.gradle/init.d/cache.init.gradle
COPY images/amata/amata-linux-${TARGETARCH} /usr/local/bin/amata

RUN chmod +x \
    /usr/local/bin/ploy/entrypoint.sh \
    /usr/local/lib/ploy/install_ploy_ca_bundle.sh \
    /usr/local/bin/amata

WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/ploy/entrypoint.sh"]
