# syntax=docker/dockerfile:1.6

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/traefik-sync ./cmd/traefik-sync

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/traefik-sync /usr/local/bin/traefik-sync
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/traefik-sync"]
