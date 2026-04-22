FROM --platform=$TARGETOS/$TARGETARCH docker:27-cli AS docker-cli

FROM debian:bookworm-slim
LABEL org.opencontainers.image.title="ploy-node" \
    org.opencontainers.image.description="Ploy node runtime image for executing jobs and gates" \
    org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.licenses="MIT"

COPY --chmod=755 images/install_ploy_ca_bundle.sh /usr/local/lib/ploy/install_ploy_ca_bundle.sh

RUN --mount=type=secret,id=ploy_ca_certs,required=false \
    bash -euxo pipefail -c 'if [[ -s /run/secrets/ploy_ca_certs ]]; then \
      mkdir -p /etc/ploy/certs; \
      cp /run/secrets/ploy_ca_certs /etc/ploy/certs/ca.crt; \
    fi \
    && if [[ -s /run/secrets/ploy_ca_certs ]] && [[ -f /etc/apt/sources.list.d/debian.sources ]]; then \
      sed -i "s|http://deb.debian.org|https://deb.debian.org|g" /etc/apt/sources.list.d/debian.sources; \
    fi \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash ca-certificates curl git jq rsync tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && if [[ -s /run/secrets/ploy_ca_certs ]]; then \
      source /usr/local/lib/ploy/install_ploy_ca_bundle.sh; \
      install_ploy_ca_bundle; \
    fi \
    && useradd --system --create-home --home-dir /var/lib/ploy --shell /usr/sbin/nologin ploy \
    && mkdir -p /etc/ploy /etc/ploy/pki /etc/ploy/gates /var/lib/ploy \
    && chown -R ploy:ploy /etc/ploy /var/lib/ploy'
COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
COPY --chmod=755 dist/ployd-node-linux /usr/local/bin/ployd-node
COPY --chmod=755 images/node/entrypoint.sh /usr/local/bin/ployd-node-entrypoint
COPY --chown=ploy:ploy gates/ /etc/ploy/gates/
USER root
# Needs access to Docker socket; run as root inside container for docker CLI
VOLUME ["/var/run/docker.sock"]
USER ploy
ENTRYPOINT ["/usr/local/bin/ployd-node-entrypoint"]
CMD ["--config","/etc/ploy/ployd-node.yaml"]
