FROM alpine:3.20

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="mod-plan" \
    org.opencontainers.image.description="Ploy Mods: Planner stub for E2E" \
    org.opencontainers.image.created="$BUILD_TS"

RUN apk add --no-cache bash jq

WORKDIR /workspace

COPY mod-plan.sh /usr/local/bin/mod-plan
RUN chmod +x /usr/local/bin/mod-plan \
 && ln -s /usr/local/bin/mod-plan /usr/local/bin/mods-plan

ENTRYPOINT ["mods-plan"]
CMD []
