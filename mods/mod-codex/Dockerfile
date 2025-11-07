ARG GO_IMAGE=golang:1.25-alpine
ARG NODE_IMAGE=node:20-alpine

# --- build ploy-buildgate (reuses the exact Build Gate executor) ---
FROM ${GO_IMAGE} AS gobuild
WORKDIR /src
# Expect build context at repo root; copy minimal tree to build the CLI
COPY go.mod go.sum ./
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod go mod download
RUN --mount=type=cache,target=/go/pkg/mod CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags='-s -w' -o /out/ploy-buildgate ./internal/cmd/ploy-buildgate

# --- final image ---
FROM ${NODE_IMAGE}

LABEL org.opencontainers.image.title="mod-codex" \
      org.opencontainers.image.description="Ploy Mods: Codex CLI wrapper" \
      org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
      org.opencontainers.image.licenses="Apache-2.0"

RUN apk add --no-cache bash git ca-certificates && \
    npm install -g @openai/codex && \
    mkdir -p /workspace /out /root/.codex && chmod 700 /root/.codex

ENV PATH="/usr/local/bin:${PATH}" \
    WORKSPACE=/workspace \
    OUTDIR=/out \
    CODEX_PROMPT="" \
    CODEX_MODEL="" \
    CODEX_EXTRA_DIR="" \
    CODEX_AUTH_JSON=""

COPY --from=gobuild /out/ploy-buildgate /usr/local/bin/ploy-buildgate
COPY mods/mod-codex/mod-codex.sh /usr/local/bin/mod-codex
RUN chmod +x /usr/local/bin/mod-codex && \
    ln -s /usr/local/bin/mod-codex /usr/local/bin/mods-codex

WORKDIR /workspace
ENTRYPOINT ["mod-codex"]
