ARG GO_VERSION=1.25
ARG ALPINE_VERSION=3.20

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
RUN apk add --no-cache git bash ca-certificates build-base
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags "-s -w" -o /out/ployd-node ./cmd/ployd-node

FROM alpine:${ALPINE_VERSION}
RUN apk add --no-cache ca-certificates bash tzdata curl jq docker-cli git && \
    adduser -D -H -s /sbin/nologin ploy && \
    mkdir -p /etc/ploy /etc/ploy/pki /var/lib/ploy && \
    chown -R ploy:ploy /etc/ploy /var/lib/ploy
COPY --from=build /out/ployd-node /usr/local/bin/ployd-node
USER root
# Needs access to Docker socket; run as root inside container for docker CLI
VOLUME ["/var/run/docker.sock"]
USER ploy
ENTRYPOINT ["/usr/local/bin/ployd-node"]
CMD ["--config","/etc/ploy/ployd-node.yaml"]
