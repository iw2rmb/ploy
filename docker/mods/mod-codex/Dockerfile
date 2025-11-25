ARG NODE_IMAGE=node:20-alpine

# --- final image ---
FROM ${NODE_IMAGE}

LABEL org.opencontainers.image.title="mod-codex" \
      org.opencontainers.image.description="Ploy Mods: Codex CLI wrapper" \
      org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
      org.opencontainers.image.licenses="Apache-2.0"

RUN apk add --no-cache bash git ca-certificates curl jq && \
    npm install -g @openai/codex && \
    mkdir -p /workspace /out /root/.codex /etc/ploy/certs && \
    chmod 700 /root/.codex && \
    chmod 755 /etc/ploy/certs

ENV PATH="/usr/local/bin:${PATH}" \
    WORKSPACE=/workspace \
    OUTDIR=/out \
    CODEX_PROMPT="" \
    CODEX_MODEL="" \
    CODEX_EXTRA_DIR="" \
    CODEX_AUTH_JSON="" \
    PLOY_SERVER_URL="" \
    PLOY_API_TOKEN="" \
    PLOY_CA_CERT_PATH="/etc/ploy/certs/ca.crt" \
    PLOY_CLIENT_CERT_PATH="/etc/ploy/certs/client.crt" \
    PLOY_CLIENT_KEY_PATH="/etc/ploy/certs/client.key"

COPY docker/mods/mod-codex/mod-codex.sh /usr/local/bin/mod-codex
COPY docker/mods/mod-codex/buildgate-validate.sh /usr/local/bin/buildgate-validate
RUN chmod +x /usr/local/bin/mod-codex && \
    ln -s /usr/local/bin/mod-codex /usr/local/bin/mods-codex

WORKDIR /workspace
ENTRYPOINT ["mod-codex"]
