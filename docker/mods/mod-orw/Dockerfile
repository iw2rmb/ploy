FROM --platform=linux/amd64 eclipse-temurin:17-jdk-alpine

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="mod-orw" \
    org.opencontainers.image.description="Ploy Mods: OpenRewrite apply runner (Maven)" \
    org.opencontainers.image.created="$BUILD_TS"

RUN apk add --no-cache bash curl git maven tar gzip jq

# Workdir is where the step runner mounts the working tree
WORKDIR /workspace

COPY mod-orw.sh /usr/local/bin/mod-orw
RUN chmod +x /usr/local/bin/mod-orw \
 && ln -s /usr/local/bin/mod-orw /usr/local/bin/mods-orw

# Do not set entrypoint to keep job template control over command
# The runner invokes: mods-orw --apply --recipe-json /in/recipe.json --dir /workspace --out /out
