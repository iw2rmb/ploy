# syntax=docker/dockerfile:1.7
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
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags "-s -w" -o /out/ployd-node ./cmd/ployd-node

FROM alpine:${ALPINE_VERSION}
RUN --mount=type=secret,id=ploy_ca_bundle,target=/run/secrets/ploy_ca_bundle,required=false \
    if [ -s /run/secrets/ploy_ca_bundle ]; then mkdir -p /etc/ssl/certs && cat /run/secrets/ploy_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; fi && \
    apk add --no-cache ca-certificates bash tzdata curl jq docker-cli git rsync && \
    if [ -s /run/secrets/ploy_ca_bundle ]; then cat /run/secrets/ploy_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; fi && \
    adduser -D -H -s /sbin/nologin ploy && \
    mkdir -p /etc/ploy /etc/ploy/pki /etc/ploy/gates /var/lib/ploy && \
    chown -R ploy:ploy /etc/ploy /var/lib/ploy
COPY --from=build /out/ployd-node /usr/local/bin/ployd-node
COPY --chown=ploy:ploy gates/ /etc/ploy/gates/
USER root
# Needs access to Docker socket; run as root inside container for docker CLI
VOLUME ["/var/run/docker.sock"]
USER ploy
ENTRYPOINT ["/usr/local/bin/ployd-node"]
CMD ["--config","/etc/ploy/ployd-node.yaml"]
