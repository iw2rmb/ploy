# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.25
ARG ALPINE_VERSION=3.20

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
RUN --mount=type=secret,id=ploy_extra_ca,required=false \
    if [ -s /run/secrets/ploy_extra_ca ]; then mkdir -p /etc/ssl/certs && cat /run/secrets/ploy_extra_ca >> /etc/ssl/certs/ca-certificates.crt; fi && \
    apk add --no-cache git bash ca-certificates build-base && \
    if [ -s /run/secrets/ploy_extra_ca ]; then cat /run/secrets/ploy_extra_ca >> /etc/ssl/certs/ca-certificates.crt; fi
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags "-s -w" -o /out/ployd ./cmd/ployd

FROM alpine:${ALPINE_VERSION}
RUN --mount=type=secret,id=ploy_extra_ca,required=false \
    if [ -s /run/secrets/ploy_extra_ca ]; then mkdir -p /etc/ssl/certs && cat /run/secrets/ploy_extra_ca >> /etc/ssl/certs/ca-certificates.crt; fi && \
    apk add --no-cache ca-certificates bash tzdata curl jq && \
    if [ -s /run/secrets/ploy_extra_ca ]; then cat /run/secrets/ploy_extra_ca >> /etc/ssl/certs/ca-certificates.crt; fi && \
    adduser -D -H -s /sbin/nologin ploy && \
    mkdir -p /etc/ploy /etc/ploy/pki /var/lib/ploy && \
    chown -R ploy:ploy /etc/ploy /var/lib/ploy
COPY --from=build /out/ployd /usr/local/bin/ployd
USER ploy
EXPOSE 8443 9100
ENTRYPOINT ["/usr/local/bin/ployd"]
CMD ["--config","/etc/ploy/ployd.yaml"]
