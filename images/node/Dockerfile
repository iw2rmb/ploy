ARG ALPINE_VERSION=3.20

FROM alpine:${ALPINE_VERSION}
LABEL org.opencontainers.image.title="ploy-node" \
    org.opencontainers.image.description="Ploy node runtime image for executing jobs and gates" \
    org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.licenses="MIT"

RUN set -eux; \
    apk_pkgs="ca-certificates bash tzdata curl jq docker-cli git rsync"; \
    apk add --no-cache ${apk_pkgs} || { \
      sed -i 's#https://dl-cdn.alpinelinux.org#http://dl-cdn.alpinelinux.org#g' /etc/apk/repositories; \
      apk add --no-cache ${apk_pkgs}; \
    } && \
    adduser -D -H -s /sbin/nologin ploy && \
    mkdir -p /etc/ploy /etc/ploy/pki /etc/ploy/gates /var/lib/ploy && \
    chown -R ploy:ploy /etc/ploy /var/lib/ploy
COPY --chmod=755 dist/ployd-node-linux /usr/local/bin/ployd-node
COPY --chmod=755 images/node/entrypoint.sh /usr/local/bin/ployd-node-entrypoint
COPY --chown=ploy:ploy gates/ /etc/ploy/gates/
USER root
# Needs access to Docker socket; run as root inside container for docker CLI
VOLUME ["/var/run/docker.sock"]
USER ploy
ENTRYPOINT ["/usr/local/bin/ployd-node-entrypoint"]
CMD ["--config","/etc/ploy/ployd-node.yaml"]
