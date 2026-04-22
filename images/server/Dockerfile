ARG ALPINE_VERSION=3.20

FROM alpine:${ALPINE_VERSION}
LABEL org.opencontainers.image.title="ploy-server" \
    org.opencontainers.image.description="Ploy control-plane server runtime image" \
    org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.licenses="MIT"

COPY --chmod=755 images/install_ploy_ca_bundle.sh /usr/local/lib/ploy/install_ploy_ca_bundle.sh

RUN --mount=type=secret,id=ploy_ca_certs,required=false set -eux; \
    if [ -s /run/secrets/ploy_ca_certs ]; then \
      mkdir -p /etc/ploy/certs /tmp/ploy-extra-ca; \
      cp /run/secrets/ploy_ca_certs /etc/ploy/certs/ca.crt; \
      cat /etc/ssl/certs/ca-certificates.crt /run/secrets/ploy_ca_certs > /tmp/ploy-extra-ca/ca-bundle.pem; \
      export SSL_CERT_FILE=/tmp/ploy-extra-ca/ca-bundle.pem; \
      export CURL_CA_BUNDLE=/tmp/ploy-extra-ca/ca-bundle.pem; \
      export GIT_SSL_CAINFO=/tmp/ploy-extra-ca/ca-bundle.pem; \
    fi; \
    apk_pkgs="ca-certificates bash tzdata curl jq git"; \
    apk add --no-cache ${apk_pkgs} || { \
      sed -i 's#https://dl-cdn.alpinelinux.org#http://dl-cdn.alpinelinux.org#g' /etc/apk/repositories; \
      apk add --no-cache ${apk_pkgs}; \
    }; \
    if [ -s /run/secrets/ploy_ca_certs ]; then \
      /bin/bash -c 'source /usr/local/lib/ploy/install_ploy_ca_bundle.sh; install_ploy_ca_bundle'; \
    fi; \
    adduser -D -H -s /sbin/nologin ploy && \
    mkdir -p /etc/ploy /etc/ploy/pki /etc/ploy/gates /etc/ploy/schemas /var/lib/ploy && \
    chown -R ploy:ploy /etc/ploy /var/lib/ploy
COPY --chmod=755 dist/ployd-linux /usr/local/bin/ployd
COPY --chmod=755 images/server/entrypoint.sh /usr/local/bin/ployd-entrypoint
COPY --chown=ploy:ploy gates/ /etc/ploy/gates/
COPY --chown=ploy:ploy docs/schemas/gate_profile.schema.json /etc/ploy/schemas/gate_profile.schema.json
USER ploy
EXPOSE 8443 9100
ENTRYPOINT ["/usr/local/bin/ployd-entrypoint"]
CMD ["--config","/etc/ploy/ployd.yaml"]
