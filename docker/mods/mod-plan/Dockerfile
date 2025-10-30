FROM --platform=linux/amd64 alpine:3.20

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
      org.opencontainers.image.title="mods-plan" \
      org.opencontainers.image.description="Ploy Mods: Planner stub for E2E" \
      org.opencontainers.image.created="$BUILD_TS"

RUN apk add --no-cache bash jq

WORKDIR /workspace

COPY mods-plan.sh /usr/local/bin/mods-plan
RUN chmod +x /usr/local/bin/mods-plan

