ARG JAVA_GATE_GRADLE_21_IMAGE=gate-gradle:jdk21

FROM --platform=$TARGETOS/$TARGETARCH ${JAVA_GATE_GRADLE_21_IMAGE}
ARG TARGETARCH

LABEL org.opencontainers.image.title="amata-codex-java-21-gradle" \
    org.opencontainers.image.description="Java 21 image with Gradle, Codex CLI, and Amata"

RUN bash -euxo pipefail -c 'for i in {1..5}; do \
      if apt-get -o Acquire::Retries=5 update \
        && DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        nodejs npm coreutils curl fd-find findutils gawk ripgrep unzip; then \
        break; \
      fi; \
      if [[ "$i" -eq 5 ]]; then exit 1; fi; \
      sleep $((i*5)); \
    done' \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /workspace /in /out /out/codex /root/.gradle/init.d \
    && chmod 700 /out/codex

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN ln -sf /usr/bin/fdfind /usr/local/bin/fd

RUN echo 'export PATH="/usr/local/bin:$PATH"' >> /root/.bashrc
RUN echo 'if [ -f ~/.bashrc ]; then . ~/.bashrc; fi' >> /root/.bash_profile

ENV LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    GRADLE_USER_HOME=/root/.gradle \
    WORKSPACE=/workspace \
    OUTDIR=/out \
    PLOY_SERVER_URL="" \
    PLOY_CLIENT_CERT_PATH="/etc/ploy/certs/client.crt"

RUN npm install -g @openai/codex \
    && npm cache clean --force \
    && rm -rf /root/.npm /tmp/*

COPY images/amata/entrypoint.sh /usr/local/bin/ploy/entrypoint.sh
COPY images/gates/gradle/gradle.properties /root/.gradle/gradle.properties
COPY images/gates/gradle/cache.init.gradle /root/.gradle/init.d/cache.init.gradle
COPY images/amata/amata-linux-${TARGETARCH} /usr/local/bin/amata

RUN chmod +x \
    /usr/local/bin/ploy/entrypoint.sh \
    /usr/local/bin/amata

WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/ploy/entrypoint.sh"]
