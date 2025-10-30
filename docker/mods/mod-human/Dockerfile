FROM --platform=linux/amd64 alpine:3.20

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="mod-human" \
    org.opencontainers.image.description="Ploy Mods: Human-in-the-loop stub" \
    org.opencontainers.image.created="$BUILD_TS"

RUN apk add --no-cache bash

WORKDIR /workspace

COPY mod-human.sh /usr/local/bin/mod-human
RUN chmod +x /usr/local/bin/mod-human
