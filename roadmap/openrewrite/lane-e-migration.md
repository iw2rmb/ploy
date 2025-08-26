# OpenRewrite Service Migration to Lane E

## Executive Summary

Direct migration of OpenRewrite from embedded controller code to standalone Lane E application deployed via Dockerfile. No backward compatibility required - fast, clean cutover.

**Timeline**: 3 weeks  
**Service Location**: `services/openrewrite/`  
**Platform Subdomain**: `openrewrite.<ploy_apps_domain>` (e.g., `openrewrite.dev.ployd.app`)  
**Deployment Method**: Lane E with Dockerfile  

## Architecture Overview

### Current State
```
controller/
├── openrewrite/        # HTTP handlers
│   ├── handler.go
│   └── types.go
internal/
├── openrewrite/        # Core executor logic
│   ├── executor.go
│   ├── manager.go
│   └── tests...
└── storage/openrewrite/ # Storage interfaces
    ├── consul/
    └── seaweedfs/
```

### Target State
```
services/
└── openrewrite/
    ├── Dockerfile
    ├── go.mod
    ├── go.sum
    ├── cmd/
    │   └── server/
    │       └── main.go
    ├── internal/
    │   ├── executor/
    │   ├── storage/
    │   ├── handlers/
    │   └── jobs/
    └── tests/
        ├── integration/
        └── benchmark/
```

## Week 1: Service Extraction

### Day 1-2: Create Service Structure ✅ COMPLETED 2025-08-26

Create new service directory and move code:
```bash
# Create service structure
mkdir -p services/openrewrite/{cmd/server,internal,tests}

# Move code (no backward compatibility needed)
mv controller/openrewrite/* services/openrewrite/internal/handlers/
mv internal/openrewrite/* services/openrewrite/internal/executor/
mv internal/storage/openrewrite/* services/openrewrite/internal/storage/

# Move tests
find . -name "*openrewrite*test.go" -exec mv {} services/openrewrite/tests/ \;
```

### Day 3-4: Create Standalone Server ✅ COMPLETED 2025-08-26

**services/openrewrite/cmd/server/main.go**:
```go
package main

import (
    "log"
    "os"
    
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/gofiber/fiber/v2/middleware/recover"
    
    "github.com/iw2rmb/ploy/services/openrewrite/internal/executor"
    "github.com/iw2rmb/ploy/services/openrewrite/internal/handlers"
    "github.com/iw2rmb/ploy/services/openrewrite/internal/jobs"
    "github.com/iw2rmb/ploy/services/openrewrite/internal/storage"
)

func main() {
    app := fiber.New(fiber.Config{
        AppName: "OpenRewrite Service",
    })
    
    // Middleware
    app.Use(logger.New())
    app.Use(recover.New())
    
    // Initialize storage clients
    consulAddr := os.Getenv("CONSUL_ADDRESS")
    if consulAddr == "" {
        consulAddr = "consul.service.consul:8500"
    }
    
    seaweedAddr := os.Getenv("SEAWEEDFS_MASTER")
    if seaweedAddr == "" {
        seaweedAddr = "seaweedfs.service.consul:9333"
    }
    
    // Create components
    consulClient := storage.NewConsulClient(consulAddr)
    seaweedClient := storage.NewSeaweedFSClient(seaweedAddr)
    exec := executor.New()
    jobManager := jobs.NewManager(consulClient, seaweedClient)
    handler := handlers.New(exec, jobManager)
    
    // Register routes
    api := app.Group("/v1/openrewrite")
    
    // Health endpoints
    api.Get("/health", handler.Health)
    api.Get("/ready", handler.Ready)
    
    // Transform endpoints (synchronous)
    api.Post("/transform", handler.Transform)
    
    // Job endpoints (asynchronous)
    api.Post("/jobs", handler.CreateJob)
    api.Get("/jobs/:id", handler.GetJob)
    api.Get("/jobs/:id/status", handler.GetJobStatus)
    api.Get("/jobs/:id/diff", handler.GetJobDiff)
    api.Delete("/jobs/:id", handler.CancelJob)
    
    // Metrics endpoint
    api.Get("/metrics", handler.Metrics)
    
    port := os.Getenv("PORT")
    if port == "" {
        port = "8090"
    }
    
    log.Printf("OpenRewrite Service starting on port %s", port)
    log.Fatal(app.Listen(":" + port))
}
```

### Day 5: Create go.mod ✅ COMPLETED 2025-08-26

**services/openrewrite/go.mod**:
```go
module github.com/iw2rmb/ploy/services/openrewrite

go 1.21

require (
    github.com/gofiber/fiber/v2 v2.52.0
    github.com/hashicorp/consul/api v1.28.0
    github.com/google/uuid v1.6.0
)

require (
    // Transitive dependencies will be added by go mod tidy
)
```

## Week 2: Dockerfile and Platform Subdomain

### Day 6-7: Create Multi-stage Dockerfile ✅ COMPLETED 2025-08-26

**services/openrewrite/Dockerfile**:
```dockerfile
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
```

### Day 8-9: Platform Subdomain Support

Add platform subdomain flag to CLI:

**cmd/ploy/commands/apps.go** (modification):
```go
func NewAppsCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "apps",
        Short: "Manage applications",
    }
    
    newCmd := &cobra.Command{
        Use:   "new",
        Short: "Create a new application",
        Run: func(cmd *cobra.Command, args []string) {
            name, _ := cmd.Flags().GetString("name")
            platformSubdomain, _ := cmd.Flags().GetString("platform-subdomain")
            
            if platformSubdomain != "" {
                // Platform apps get special subdomain
                // e.g., openrewrite.dev.ployd.app instead of app-name.ployd.app
                setPlatformSubdomain(name, platformSubdomain)
            }
            
            createApp(name)
        },
    }
    
    newCmd.Flags().String("name", "", "Application name")
    newCmd.Flags().String("platform-subdomain", "", "Platform-specific subdomain (for system services)")
    
    cmd.AddCommand(newCmd)
    return cmd
}
```

**controller/apps/platform.go** (new file):
```go
package apps

import (
    "fmt"
    "github.com/hashicorp/consul/api"
)

// SetPlatformSubdomain configures a platform-specific subdomain for system services
func SetPlatformSubdomain(appName, subdomain string) error {
    client, err := api.NewClient(api.DefaultConfig())
    if err != nil {
        return err
    }
    
    // Store platform subdomain mapping
    key := fmt.Sprintf("ploy/platform-subdomains/%s", subdomain)
    value := []byte(appName)
    
    _, err = client.KV().Put(&api.KVPair{
        Key:   key,
        Value: value,
    }, nil)
    
    return err
}

// GetAppDomain returns the domain for an app (platform or regular)
func GetAppDomain(appName string) string {
    ployDomain := os.Getenv("PLOY_APPS_DOMAIN")
    if ployDomain == "" {
        ployDomain = "ployd.app"
    }
    
    // Check if this is a platform app
    client, _ := api.NewClient(api.DefaultConfig())
    kvPairs, _, _ := client.KV().List("ploy/platform-subdomains/", nil)
    
    for _, pair := range kvPairs {
        if string(pair.Value) == appName {
            // Extract subdomain from key
            subdomain := strings.TrimPrefix(string(pair.Key), "ploy/platform-subdomains/")
            return fmt.Sprintf("%s.%s", subdomain, ployDomain)
        }
    }
    
    // Regular app domain
    return fmt.Sprintf("%s.%s", appName, ployDomain)
}
```

### Day 10: Deploy Script

**services/openrewrite/deploy.sh**:
```bash
#!/bin/bash
set -euo pipefail

APP_NAME="openrewrite-service"
PLATFORM_SUBDOMAIN="openrewrite"

echo "🚀 Deploying OpenRewrite Service to Lane E"

# Create the app with platform subdomain
ploy apps new --name $APP_NAME --platform-subdomain $PLATFORM_SUBDOMAIN

# Set environment variables
ploy env set --app $APP_NAME CONSUL_ADDRESS=consul.service.consul:8500
ploy env set --app $APP_NAME SEAWEEDFS_MASTER=seaweedfs.service.consul:9333
ploy env set --app $APP_NAME WORKER_POOL_SIZE=2
ploy env set --app $APP_NAME MAX_CONCURRENT_JOBS=5

# Create tar archive
tar -czf /tmp/openrewrite.tar.gz \
    Dockerfile \
    go.mod \
    go.sum \
    cmd/ \
    internal/

# Deploy via ploy push
ploy push --app $APP_NAME < /tmp/openrewrite.tar.gz

echo "✅ OpenRewrite Service deployed"
echo "🌐 Available at: https://${PLATFORM_SUBDOMAIN}.${PLOY_APPS_DOMAIN}"
```

## Week 3: Deployment and ARF Integration

### Day 11-12: Update ARF to Use Service

**controller/arf/openrewrite_client.go** (new):
```go
package arf

import (
    "bytes"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "time"
)

type OpenRewriteClient struct {
    baseURL string
    client  *http.Client
}

func NewOpenRewriteClient() *OpenRewriteClient {
    // Get platform domain
    ployDomain := os.Getenv("PLOY_APPS_DOMAIN")
    if ployDomain == "" {
        ployDomain = "ployd.app"
    }
    
    baseURL := fmt.Sprintf("https://openrewrite.%s", ployDomain)
    
    // Allow override for development
    if override := os.Getenv("OPENREWRITE_SERVICE_URL"); override != "" {
        baseURL = override
    }
    
    return &OpenRewriteClient{
        baseURL: baseURL,
        client: &http.Client{
            Timeout: 5 * time.Minute,
        },
    }
}

func (c *OpenRewriteClient) Transform(tarData []byte, recipe RecipeConfig) (*TransformResult, error) {
    req := TransformRequest{
        JobID:       uuid.New().String(),
        TarArchive:  base64.StdEncoding.EncodeToString(tarData),
        RecipeConfig: recipe,
    }
    
    body, err := json.Marshal(req)
    if err != nil {
        return nil, err
    }
    
    resp, err := c.client.Post(
        fmt.Sprintf("%s/v1/openrewrite/transform", c.baseURL),
        "application/json",
        bytes.NewReader(body),
    )
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result TransformResult
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    return &result, nil
}

func (c *OpenRewriteClient) CreateJob(tarData []byte, recipe RecipeConfig) (string, error) {
    req := CreateJobRequest{
        TarArchive:   base64.StdEncoding.EncodeToString(tarData),
        RecipeConfig: recipe,
    }
    
    body, err := json.Marshal(req)
    if err != nil {
        return "", err
    }
    
    resp, err := c.client.Post(
        fmt.Sprintf("%s/v1/openrewrite/jobs", c.baseURL),
        "application/json",
        bytes.NewReader(body),
    )
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    var result JobResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }
    
    return result.JobID, nil
}

func (c *OpenRewriteClient) GetJobStatus(jobID string) (*JobStatus, error) {
    resp, err := c.client.Get(
        fmt.Sprintf("%s/v1/openrewrite/jobs/%s/status", c.baseURL, jobID),
    )
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var status JobStatus
    if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
        return nil, err
    }
    
    return &status, nil
}
```

### Day 13-14: Test Migration

Move all OpenRewrite tests to service directory:

**services/openrewrite/tests/integration/transform_test.go**:
```go
package integration

import (
    "testing"
    "os"
    "net/http"
    "encoding/json"
    "bytes"
)

func TestJava11to17Migration(t *testing.T) {
    // Use deployed service
    serviceURL := os.Getenv("OPENREWRITE_SERVICE_URL")
    if serviceURL == "" {
        serviceURL = "http://localhost:8090"
    }
    
    // Create test tar archive
    tarData := createTestProject(t)
    
    // Send transformation request
    req := TransformRequest{
        JobID:      "test-migration",
        TarArchive: base64.StdEncoding.EncodeToString(tarData),
        RecipeConfig: RecipeConfig{
            Recipe:    "org.openrewrite.java.migrate.Java11to17",
            Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
        },
    }
    
    body, _ := json.Marshal(req)
    resp, err := http.Post(
        serviceURL + "/v1/openrewrite/transform",
        "application/json",
        bytes.NewReader(body),
    )
    require.NoError(t, err)
    defer resp.Body.Close()
    
    var result TransformResult
    err = json.NewDecoder(resp.Body).Decode(&result)
    require.NoError(t, err)
    
    assert.True(t, result.Success)
    assert.NotEmpty(t, result.Diff)
    assert.Contains(t, string(result.Diff), "java.version>17")
}
```

### Day 15: Cutover and Validation

**Cutover Steps**:

1. **Deploy OpenRewrite Service**:
```bash
cd services/openrewrite
./deploy.sh
```

2. **Verify Service Health**:
```bash
curl https://openrewrite.dev.ployd.app/v1/openrewrite/health
```

3. **Update ARF Configuration**:
```bash
ploy env set --app arf OPENREWRITE_SERVICE_URL=https://openrewrite.dev.ployd.app
```

4. **Remove Old Code** (no backward compatibility):
```bash
rm -rf controller/openrewrite/
rm -rf internal/openrewrite/
rm -rf internal/storage/openrewrite/
rm -f platform/nomad/openrewrite-service.hcl
```

5. **Run Integration Tests**:
```bash
cd services/openrewrite
go test ./tests/integration/...
```

## Service Configuration

### Environment Variables
```bash
# Required
CONSUL_ADDRESS=consul.service.consul:8500
SEAWEEDFS_MASTER=seaweedfs.service.consul:9333

# Optional
PORT=8090
WORKER_POOL_SIZE=2
MAX_CONCURRENT_JOBS=5
AUTO_SHUTDOWN_MINUTES=10
JAVA_OPTS=-Xmx3g -Xms1g
MAVEN_OPTS=-Xmx2g -Xms512m
```

### Consul KV Structure
```
ploy/
├── platform-subdomains/
│   └── openrewrite → "openrewrite-service"
├── apps/
│   └── openrewrite-service/
│       └── env/
│           ├── CONSUL_ADDRESS
│           └── SEAWEEDFS_MASTER
└── openrewrite/
    └── jobs/
        └── {jobID}/
            ├── status
            ├── diff_url
            └── metadata
```

### SeaweedFS Collections
```
openrewrite-diffs/     # Transformation diffs
openrewrite-archives/  # Source archives
openrewrite-cache/     # Recipe cache
```

## Testing Strategy

### Unit Tests
```bash
cd services/openrewrite
go test ./internal/...
```

### Integration Tests
```bash
# Start local service
docker build -t openrewrite-test .
docker run -d -p 8090:8090 openrewrite-test

# Run tests
go test ./tests/integration/...
```

### End-to-End Test
```bash
# Deploy to Ploy
./deploy.sh

# Test via ARF
ploy arf benchmark run java11to17_migration \
  --repository https://github.com/spring-projects/spring-petclinic \
  --app test-migration
```

## Success Criteria

- [ ] Service deployed at `openrewrite.dev.ployd.app`
- [ ] All ARF benchmarks pass
- [ ] Consul KV integration working
- [ ] SeaweedFS storage working
- [ ] Job queue functionality preserved
- [ ] All tests migrated and passing
- [ ] Old code removed completely

## Rollback Plan

Since no backward compatibility is required, rollback involves:
1. Restore controller code from git
2. Deploy original Nomad HCL
3. Update ARF to use embedded executor

However, this should not be needed with proper testing.

## Post-Migration Cleanup

1. Remove old Nomad job:
```bash
nomad job stop openrewrite-service
```

2. Clean up Docker images:
```bash
docker rmi localhost:5000/ploy-openrewrite:latest
```

3. Update documentation:
- Remove references to embedded OpenRewrite
- Document new service endpoints
- Update ARF documentation

## Benefits of Lane E Migration

1. **Standard Deployment**: Uses existing Ploy infrastructure
2. **Automatic Updates**: Just `ploy push` for new versions
3. **Environment Management**: Via `ploy env` commands
4. **Health Monitoring**: Standard Nomad health checks
5. **SSL/Routing**: Automatic via Traefik
6. **Rollback Support**: Standard Ploy rollback
7. **Simplified Testing**: Service isolation
8. **Independent Scaling**: Can scale separately from controller

## Summary

This migration transforms OpenRewrite into a first-class Ploy platform service with its own subdomain, deployed via standard Lane E mechanisms. The service maintains full access to Consul KV and SeaweedFS while gaining all benefits of Ploy's application management infrastructure.