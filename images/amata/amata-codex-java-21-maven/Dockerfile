ARG NODE_IMAGE=node:22-bookworm-slim
ARG JAVA_BASE_MAVEN_21_IMAGE=java-base-maven:jdk21

FROM --platform=$TARGETOS/$TARGETARCH ${JAVA_BASE_MAVEN_21_IMAGE} AS maven-tools

FROM ${NODE_IMAGE}
ARG TARGETARCH

LABEL org.opencontainers.image.title="amata-codex-java-21-maven" \
    org.opencontainers.image.description="Java 21 image with Maven, Codex CLI, and Amata"

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash ca-certificates coreutils curl fd-find findutils gawk git jq ripgrep unzip \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /workspace /in /out /out/codex \
    && chmod 700 /out/codex

COPY --from=maven-tools /opt/java/openjdk /opt/java/openjdk
COPY --from=maven-tools /usr/share/maven /usr/share/maven
COPY --from=maven-tools /usr/share/maven/ref /usr/share/maven/ref
COPY --from=maven-tools /usr/bin/mvn /usr/bin/mvn

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
    PATH="/opt/java/openjdk/bin:/usr/share/maven/bin:/usr/local/bin:${PATH}" \
    WORKSPACE=/workspace \
    OUTDIR=/out \
    PLOY_SERVER_URL="" \
    PLOY_CLIENT_CERT_PATH="/etc/ploy/certs/client.crt"

RUN npm install -g @openai/codex \
    && npm cache clean --force \
    && rm -rf /root/.npm /tmp/*

COPY images/amata/entrypoint.sh /usr/local/bin/ploy/entrypoint.sh
COPY images/amata/amata-linux-${TARGETARCH} /usr/local/bin/amata

RUN chmod +x \
    /usr/local/bin/ploy/entrypoint.sh \
    /usr/local/bin/amata

WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/ploy/entrypoint.sh"]
