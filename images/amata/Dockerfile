ARG NODE_IMAGE=node:22-alpine
FROM --platform=$BUILDPLATFORM maven:3.9.11-eclipse-temurin-17 AS rewrite-runner-build

WORKDIR /build
COPY images/amata/rewrite-runner/pom.xml rewrite-runner/pom.xml
COPY images/orw/shared/orw-cli-runner/src rewrite-runner/src
RUN --mount=type=cache,target=/root/.m2 \
    bash -euxo pipefail -c 'cd rewrite-runner && mvn -q -DskipTests package'

FROM ${NODE_IMAGE}

LABEL org.opencontainers.image.title="ploy-amata" \
    org.opencontainers.image.description="Ploy Migs runner image with Codex, Crush, Claude Code, and Claude Code Router installed" \
    org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache bash git ca-certificates curl jq openjdk17-jre && \
    if command -v update-ca-certificates >/dev/null 2>&1; then \
      update-ca-certificates || true; \
    fi && \
    npm install -g @openai/codex @charmland/crush @anthropic-ai/claude-code @musistudio/claude-code-router && \
    npm cache clean --force && \
    rm -rf /root/.npm /tmp/* && \
    mkdir -p /workspace /out /out/codex /etc/ploy/certs && \
    chmod 700 /out/codex && \
    chmod 755 /etc/ploy/certs

RUN apk update
RUN apk add --no-cache bash ripgrep fd grep findutils gawk coreutils
SHELL ["/bin/bash", "-euo", "pipefail", "-c"]
RUN echo 'export PATH="/usr/local/bin:$PATH"' >> /root/.bashrc
RUN echo 'if [ -f ~/.bashrc ]; then . ~/.bashrc; fi' >> /root/.bash_profile

ENV PATH="/usr/local/bin:${PATH}" \
    WORKSPACE=/workspace \
    OUTDIR=/out \
    PLOY_SERVER_URL="" \
    PLOY_API_TOKEN="" \
    PLOY_CLIENT_CERT_PATH="/etc/ploy/certs/client.crt" \
    PLOY_CLIENT_KEY_PATH="/etc/ploy/certs/client.key"

COPY images/amata/entrypoint.sh /usr/local/bin/ploy/entrypoint.sh
COPY images/install_ploy_ca_bundle.sh /usr/local/lib/ploy/install_ploy_ca_bundle.sh
COPY images/amata/amata /usr/local/bin/amata
COPY images/amata/orw-cli.sh /usr/local/bin/orw-cli
COPY images/amata/heal-orw.sh /usr/local/bin/heal-orw
COPY --from=rewrite-runner-build /build/rewrite-runner/target/orw-cli-runner.jar /opt/orw-cli-runner/orw-cli-runner.jar
RUN printf '%s\n' '#!/usr/bin/env bash' \
    'set -euo pipefail' \
    'exec java -jar /opt/orw-cli-runner/orw-cli-runner.jar "$@"' \
    > /usr/local/bin/rewrite && chmod +x /usr/local/bin/rewrite
RUN chmod +x \
    /usr/local/bin/ploy/entrypoint.sh \
    /usr/local/lib/ploy/install_ploy_ca_bundle.sh \
    /usr/local/bin/amata \
    /usr/local/bin/orw-cli \
    /usr/local/bin/heal-orw

WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/ploy/entrypoint.sh"]
