# syntax=docker/dockerfile:1.7

# orw-cli: standalone OpenRewrite CLI runner.
#
# This image runs OpenRewrite through a single CLI command path and never
# invokes Gradle/Maven project tasks.
FROM --platform=$TARGETOS/$TARGETARCH eclipse-temurin:17-jdk

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="orw-cli" \
    org.opencontainers.image.description="Ploy Migs: standalone OpenRewrite CLI runner" \
    org.opencontainers.image.created="$BUILD_TS"

RUN --mount=type=secret,id=ploy_ca_bundle,required=false \
    if [ -s /run/secrets/ploy_ca_bundle ]; then \
      cp /run/secrets/ploy_ca_bundle /usr/local/share/ca-certificates/ploy-ca.crt && \
      if command -v update-ca-certificates >/dev/null 2>&1; then \
        update-ca-certificates; \
      fi; \
    fi \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash ca-certificates git jq \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

COPY --chmod=755 orw-cli.sh /usr/local/bin/orw-cli
RUN ln -s /usr/local/bin/orw-cli /usr/local/bin/migs-orw-cli

ENTRYPOINT ["migs-orw-cli"]
CMD ["--apply", "--dir", "/workspace", "--out", "/out"]
