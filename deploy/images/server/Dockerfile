ARG ALPINE_VERSION=3.20

FROM alpine:${ALPINE_VERSION}
RUN apk add --no-cache ca-certificates bash tzdata curl jq git && \
    adduser -D -H -s /sbin/nologin ploy && \
    mkdir -p /etc/ploy /etc/ploy/pki /etc/ploy/gates /etc/ploy/schemas /var/lib/ploy && \
    chown -R ploy:ploy /etc/ploy /var/lib/ploy
COPY dist/ployd-linux /usr/local/bin/ployd
RUN chmod +x /usr/local/bin/ployd
COPY deploy/images/server/entrypoint.sh /usr/local/bin/ployd-entrypoint
RUN chmod +x /usr/local/bin/ployd-entrypoint
COPY --chown=ploy:ploy gates/ /etc/ploy/gates/
COPY --chown=ploy:ploy docs/schemas/gate_profile.schema.json /etc/ploy/schemas/gate_profile.schema.json
USER ploy
EXPOSE 8443 9100
ENTRYPOINT ["/usr/local/bin/ployd-entrypoint"]
CMD ["--config","/etc/ploy/ployd.yaml"]
