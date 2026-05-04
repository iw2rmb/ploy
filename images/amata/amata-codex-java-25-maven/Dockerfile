ARG JAVA_BASE_MAVEN_25_IMAGE=java-base-maven:jdk25

FROM --platform=$TARGETOS/$TARGETARCH ${JAVA_BASE_MAVEN_25_IMAGE}
ARG TARGETARCH

LABEL org.opencontainers.image.title="amata-codex-java-25-maven" \
    org.opencontainers.image.description="Java 25 image with Maven, Codex CLI, and Amata"

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
    && mkdir -p /workspace /in /out /out/codex \
    && chmod 700 /out/codex

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN ln -sf /usr/bin/fdfind /usr/local/bin/fd

RUN echo 'export PATH="/usr/local/bin:$PATH"' >> /root/.bashrc
RUN echo 'if [ -f ~/.bashrc ]; then . ~/.bashrc; fi' >> /root/.bash_profile

ENV LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    MAVEN_CONFIG=/root/.m2 \
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
