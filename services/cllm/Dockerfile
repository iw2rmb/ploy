# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o bin/cllm-server \
    ./cmd/server

# Final stage - use distroless for security
FROM gcr.io/distroless/static-debian11:nonroot

# Copy binary from builder
COPY --from=builder /app/bin/cllm-server /usr/local/bin/cllm-server

# Copy default config
COPY --from=builder /app/configs/ /etc/cllm/

# Create sandbox work directory
USER nonroot:nonroot
WORKDIR /tmp

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/cllm-server", "--health-check"] || exit 1

# Expose port
EXPOSE 8082

# Run the binary
ENTRYPOINT ["/usr/local/bin/cllm-server"]