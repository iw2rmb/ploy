# Build stage - compile Go binary
FROM golang:1.21-alpine AS builder

# Install git for go mod download
RUN apk add --no-cache git

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o openrewrite-service cmd/server/main.go

# Runtime stage - Java/Maven/Git environment
FROM maven:3.9-eclipse-temurin-17

# Install additional tools
RUN apt-get update && \
    apt-get install -y \
        git \
        curl \
        ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Pre-cache OpenRewrite artifacts for faster transformations
RUN mvn dependency:get \
    -DgroupId=org.openrewrite.recipe \
    -DartifactId=rewrite-migrate-java \
    -Dversion=3.15.0 \
    -Dtransitive=true && \
    mvn dependency:get \
    -DgroupId=org.openrewrite \
    -DartifactId=rewrite-maven-plugin \
    -Dversion=5.34.0 \
    -Dtransitive=true

# Create app directory
WORKDIR /app

# Copy the Go binary from builder
COPY --from=builder /build/openrewrite-service .

# Create workspace directory for transformations
RUN mkdir -p /workspace/transformations

# Environment variables (can be overridden)
ENV PORT=8090
ENV WORKSPACE_DIR=/workspace/transformations
ENV CONSUL_ADDRESS=consul.service.consul:8500
ENV SEAWEEDFS_MASTER=seaweedfs.service.consul:9333
ENV WORKER_POOL_SIZE=2
ENV MAX_CONCURRENT_JOBS=5
ENV AUTO_SHUTDOWN_MINUTES=0

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8090/v1/openrewrite/health || exit 1

# Expose port
EXPOSE 8090

# Run the service
CMD ["./openrewrite-service"]