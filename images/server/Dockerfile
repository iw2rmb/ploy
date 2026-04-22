FROM debian:bookworm-slim
LABEL org.opencontainers.image.title="ploy-server" \
    org.opencontainers.image.description="Ploy control-plane server runtime image" \
    org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.licenses="MIT"

COPY --chmod=755 images/install_ploy_ca_bundle.sh /usr/local/lib/ploy/install_ploy_ca_bundle.sh

RUN --mount=type=secret,id=ploy_ca_certs,required=false \
    bash -euxo pipefail -c 'if [[ -s /run/secrets/ploy_ca_certs ]]; then \
      mkdir -p /etc/ploy/certs; \
      cp /run/secrets/ploy_ca_certs /etc/ploy/certs/ca.crt; \
    fi \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash ca-certificates curl git jq tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && if [[ -s /run/secrets/ploy_ca_certs ]]; then \
      source /usr/local/lib/ploy/install_ploy_ca_bundle.sh; \
      install_ploy_ca_bundle; \
    fi \
    && useradd --system --create-home --home-dir /var/lib/ploy --shell /usr/sbin/nologin ploy \
    && mkdir -p /etc/ploy /etc/ploy/pki /etc/ploy/gates /etc/ploy/schemas /var/lib/ploy \
    && chown -R ploy:ploy /etc/ploy /var/lib/ploy'
COPY --chmod=755 dist/ployd-linux /usr/local/bin/ployd
COPY --chmod=755 images/server/entrypoint.sh /usr/local/bin/ployd-entrypoint
COPY --chown=ploy:ploy gates/ /etc/ploy/gates/
COPY --chown=ploy:ploy docs/schemas/gate_profile.schema.json /etc/ploy/schemas/gate_profile.schema.json
USER ploy
EXPOSE 8443 9100
ENTRYPOINT ["/usr/local/bin/ployd-entrypoint"]
CMD ["--config","/etc/ploy/ployd.yaml"]
