ARG NODE_IMAGE=node:22-alpine

FROM ${NODE_IMAGE}

LABEL org.opencontainers.image.title="ploy-amata" \
    org.opencontainers.image.description="Ploy Migs: Amata runner image (with codex/cc/cc-router installed)" \
    org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.licenses="Apache-2.0"

RUN apk add --no-cache bash git ca-certificates curl jq && \
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
    CODEX_HOME=/out/codex \
    CODEX_MODEL="" \
    CODEX_AUTH_JSON="" \
    CODEX_CONFIG_TOML="" \
    CRUSH_JSON="" \
    CCR_CONFIG_JSON="" \
    PLOY_SERVER_URL="" \
    PLOY_API_TOKEN="" \
    PLOY_CA_CERTS="/etc/ploy/certs/ca.crt" \
    PLOY_CLIENT_CERT_PATH="/etc/ploy/certs/client.crt" \
    PLOY_CLIENT_KEY_PATH="/etc/ploy/certs/client.key"

COPY images/amata/entrypoint.sh /usr/local/bin/ploy/entrypoint.sh
COPY images/amata/amata /usr/local/bin/amata
RUN chmod +x /usr/local/bin/ploy/entrypoint.sh /usr/local/bin/amata

WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/ploy/entrypoint.sh"]
