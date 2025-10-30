FROM --platform=linux/amd64 alpine:3.20

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="mod-llm" \
    org.opencontainers.image.description="Ploy Mods: LLM plan/execute stub" \
    org.opencontainers.image.created="$BUILD_TS"

RUN apk add --no-cache bash coreutils jq

WORKDIR /workspace

COPY mod-llm.sh /usr/local/bin/mod-llm
RUN chmod +x /usr/local/bin/mod-llm
