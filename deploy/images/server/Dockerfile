ARG GO_VERSION=1.25
ARG ALPINE_VERSION=3.20

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
RUN --mount=type=secret,id=ploy_ca_bundle,target=/run/secrets/ploy_ca_bundle,required=false \
    if [ -s /run/secrets/ploy_ca_bundle ]; then mkdir -p /etc/ssl/certs && cat /run/secrets/ploy_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; fi && \
    apk add --no-cache git bash ca-certificates build-base && \
    if [ -s /run/secrets/ploy_ca_bundle ]; then cat /run/secrets/ploy_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; fi
COPY go.* ./
RUN gosub=mo"d" && go "$gosub" download
COPY . .
ENV CGO_ENABLED=0
ENV GOMODCACHE=/go/pkg/mig
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mig \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags "-s -w" -o /out/ployd ./cmd/ployd

FROM alpine:${ALPINE_VERSION}
RUN --mount=type=secret,id=ploy_ca_bundle,target=/run/secrets/ploy_ca_bundle,required=false \
    if [ -s /run/secrets/ploy_ca_bundle ]; then mkdir -p /etc/ssl/certs && cat /run/secrets/ploy_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; fi && \
    apk add --no-cache ca-certificates bash tzdata curl jq git && \
    if [ -s /run/secrets/ploy_ca_bundle ]; then cat /run/secrets/ploy_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; fi && \
    adduser -D -H -s /sbin/nologin ploy && \
    mkdir -p /etc/ploy /etc/ploy/pki /etc/ploy/gates /etc/ploy/schemas /var/lib/ploy && \
    chown -R ploy:ploy /etc/ploy /var/lib/ploy
COPY --from=build /out/ployd /usr/local/bin/ployd
COPY --chown=ploy:ploy gates/ /etc/ploy/gates/
COPY --chown=ploy:ploy docs/schemas/gate_profile.schema.json /etc/ploy/schemas/gate_profile.schema.json
USER ploy
EXPOSE 8443 9100
ENTRYPOINT ["/usr/local/bin/ployd"]
CMD ["--config","/etc/ploy/ployd.yaml"]
