# syntax=docker/dockerfile:1.7

ARG NODE_IMAGE=node:20-alpine

# --- final image ---
FROM ${NODE_IMAGE}

LABEL org.opencontainers.image.title="mig-codex" \
    org.opencontainers.image.description="Ploy Migs: Codex CLI wrapper" \
    org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.licenses="Apache-2.0"

RUN --mount=type=secret,id=ploy_ca_bundle,required=false \
    if [ -s /run/secrets/ploy_ca_bundle ]; then \
      mkdir -p /usr/local/share/ca-certificates && \
      cp /run/secrets/ploy_ca_bundle /usr/local/share/ca-certificates/ploy-ca.crt && \
      update-ca-certificates || true; \
    fi && \
    apk add --no-cache bash git ca-certificates curl jq && \
    npm install -g @openai/codex && \
    npm cache clean --force && \
    rm -rf /root/.npm /tmp/* && \
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

# Copy only the mig-codex entrypoint; no Build Gate helper binary is included
# to prevent Codex from running the gate inside the container. Build Gate
# verification is performed externally by Ploy (docker gate) or the
# Build Gate HTTP API.
COPY deploy/images/migs/mig-codex/mig-codex.sh /usr/local/bin/mig-codex
RUN chmod +x /usr/local/bin/mig-codex && \
    ln -s /usr/local/bin/mig-codex /usr/local/bin/migs-codex

WORKDIR /workspace
ENTRYPOINT ["mig-codex"]
