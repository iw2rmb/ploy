# CHTTP Server Architecture

**Project**: Simple CLI-to-HTTP Bridge Service  
**Status**: Design Phase  
**Target**: Lightweight CLI wrapper for deployment via Ploy  

## Executive Summary

CHTTP (CLI-over-HTTP) is a minimal service that provides HTTP access to command-line tools. Designed as a simple bridge, CHTTP focuses solely on HTTP-to-CLI translation while relying on Ploy for deployment, security, monitoring, and infrastructure management.

**Core Value Proposition:**
- Convert CLI tools to HTTP endpoints quickly
- Simple HTTP request/response model
- Minimal footprint and dependencies
- Deployed and managed by Ploy's platform
- Focus on CLI execution reliability
- Basic security and logging

## Simplified Architecture

### Core Components

```
┌─────────────────────────────────┐
│         CHTTP Service           │
├─────────────────────────────────┤
│ HTTP Handler                    │
│ ├── Basic Authentication        │
│ ├── Request Parsing             │
│ └── Response Formatting         │
├─────────────────────────────────┤
│ CLI Executor                    │
│ ├── Command Execution           │
│ ├── Output Capture              │
│ └── Error Handling              │
├─────────────────────────────────┤
│ Basic Logging                   │
│ ├── Request Logging             │
│ └── Execution Logging           │
└─────────────────────────────────┘
```

**Managed by Ploy Platform:**
- Deployment & Scaling
- Load Balancing (Traefik)
- Security & TLS
- Monitoring & Alerting
- Infrastructure Management

### Simple Processing Flow

```go
// Minimal service structure
type CHTPService struct {
    config  *Config
    logger  *Logger
    health  *HealthChecker
}

// Basic request processing
HTTP Request → Parse → Execute CLI → Format Response
     ↓           ↓         ↓              ↓
   JSON       Command   Process        JSON
```

## Simple Configuration

### Basic Configuration (YAML)

```yaml
# chttp-config.yaml
server:
  port: 8080
  host: "0.0.0.0"

security:
  api_key: "your-secret-api-key"

commands:
  # Allowed CLI commands
  allowed:
    - "ls"
    - "cat"
    - "grep"
    - "find"
    - "echo"

logging:
  level: "info"      # info, warn, error
  format: "json"     # json, text

health:
  enabled: true
  endpoint: "/health"
```

## API Endpoints

### Execute CLI Command

**POST** `/api/v1/execute`
```

**Request:**
```http
POST /api/v1/execute HTTP/1.1
Content-Type: application/json
X-API-Key: your-secret-api-key

{
  "command": "ls",
  "args": ["-la", "/tmp"],
  "timeout": "30s"
}
```

**Response:**
```json
{
  "success": true,
  "stdout": "total 8\ndrwxr-xr-x  3 user  staff  96 Jan 15 10:30 .\n...",
  "stderr": "",
  "exit_code": 0,
  "duration": "15ms"
}
```

### Health Check

**GET** `/health`

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2025-01-15T10:30:00Z",
  "uptime": "2h30m15s",
  "version": "1.0.0"
}
```

## Deployment with Ploy

CHTTP services are deployed and managed by Ploy's platform:

```yaml
# ploy-app.yaml
name: my-chttp-service
lane: C  # Java/Node.js lane for HTTP services
config:
  port: 8080
  command_allowlist: ["ls", "cat", "grep"]
scaling:
  min_instances: 1
  max_instances: 5
security:
  tls: true
  api_keys: true
```

Ploy handles all the complex infrastructure concerns:
- **Load Balancing**: Traefik automatically load balances requests
- **Security**: TLS termination, authentication, rate limiting
- **Monitoring**: Metrics collection, alerting, health checks  
- **Scaling**: Automatic scaling based on demand
- **Deployment**: Blue-green, canary, rolling deployments
```

## Implementation Phases

### Phase 1-4: Core HTTP-CLI Bridge ✅
- Basic HTTP server and CLI execution
- External service orchestration
- Resilient HTTP client with circuit breakers

### Phase 5: Basic Logging & Health Monitoring 📋
- Simple structured logging
- HTTP health check endpoint  
- Request/response logging

### Phase 6: Documentation & Developer Tools 📋
- Basic API documentation
- Usage guides and examples
- Simple testing utilities

### ~~Removed Phases~~
- ~~Phase 7-8~~: Advanced observability, deployment, security → **Handled by Ploy**

## Architecture Benefits

**CHTTP Focus:**
- Lightweight CLI-to-HTTP bridge
- Simple, reliable command execution
- Minimal dependencies and footprint

**Ploy Platform Advantages:**
- Enterprise-grade deployment automation
- Comprehensive security and monitoring
- Production-ready infrastructure management
- Multi-environment support and scaling

This separation ensures each tool excels in its domain while providing a cohesive development and deployment experience.
```

### Server Core Structure

```go
// internal/server/server.go
package server

import (
    "context"
    "crypto/rsa"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "github.com/gofiber/fiber/v2/middleware/compress"
    "github.com/gofiber/fiber/v2/middleware/recover"
)

type CHTPServer struct {
    config      *config.Config
    app         *fiber.App
    auth        *AuthManager
    sandbox     *SandboxManager
    pipeline    *PipelineEngine
    metrics     *MetricsCollector
}

func NewCHTTPServer(cfg *config.Config) *CHTPServer {
    server := &CHTPServer{
        config:   cfg,
        auth:     NewAuthManager(cfg.Security),
        sandbox:  NewSandboxManager(cfg.Security),
        pipeline: NewPipelineEngine(cfg.Pipeline),
        metrics:  NewMetricsCollector(cfg.Monitoring),
    }

    // Setup Fiber app
    server.app = fiber.New(fiber.Config{
        ReadTimeout:    30 * time.Second,
        WriteTimeout:   30 * time.Second,
        MaxRequestSize: int(cfg.Input.MaxArchiveSize),
        StreamRequestBody: true, // Enable streaming
        DisableKeepalive: false,
    })

    server.setupMiddleware()
    server.setupRoutes()

    return server
}

func (s *CHTPServer) setupMiddleware() {
    s.app.Use(recover.New())
    s.app.Use(compress.New(compress.Config{
        Level: compress.LevelBestSpeed,
    }))
    s.app.Use(cors.New())
    s.app.Use(s.authMiddleware)
    s.app.Use(s.metricsMiddleware)
}

func (s *CHTPServer) setupRoutes() {
    // Core analysis endpoint
    s.app.Post("/analyze", s.handleAnalyze)
    
    // Pipeline chaining
    s.app.Post("/pipeline", s.handlePipeline)
    
    // Health and discovery
    s.app.Get("/health", s.handleHealth)
    s.app.Get("/capabilities", s.handleCapabilities)
    s.app.Get("/metrics", s.handleMetrics)
}
```

### Streaming Analysis Handler

```go
func (s *CHTPServer) handleAnalyze(c *fiber.Ctx) error {
    // Create request context
    reqCtx := &AnalysisContext{
        ID:        generateRequestID(),
        ClientID:  c.Get("X-Client-ID"),
        Timestamp: time.Now(),
        Config:    s.config,
    }

    // Stream input to sandbox
    pr, pw := io.Pipe()
    
    // Start extraction in background
    go func() {
        defer pw.Close()
        if err := s.extractInputStream(c.Context().RequestBodyStream(), pw); err != nil {
            pw.CloseWithError(err)
        }
    }()

    // Create sandbox environment
    sandbox, err := s.sandbox.CreateEnvironment(reqCtx)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "Sandbox creation failed"})
    }
    defer sandbox.Cleanup()

    // Extract to sandbox
    if err := sandbox.ExtractArchive(pr); err != nil {
        return c.Status(400).JSON(fiber.Map{"error": "Archive extraction failed"})
    }

    // Execute CLI tool
    result, err := sandbox.ExecuteCommand(reqCtx)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "Execution failed"})
    }

    // Parse and format output
    parsed, err := s.parseOutput(result, s.config.Output)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "Output parsing failed"})
    }

    // Return structured response
    return c.JSON(parsed)
}
```

### Sandbox Manager

```go
// internal/sandbox/manager.go
package sandbox

import (
    "context"
    "os/exec"
    "path/filepath"
    "syscall"
)

type SandboxManager struct {
    config SecurityConfig
}

type Sandbox struct {
    id      string
    workDir string
    config  SecurityConfig
}

func (sm *SandboxManager) CreateEnvironment(ctx *AnalysisContext) (*Sandbox, error) {
    // Create temporary directory
    workDir := filepath.Join("/tmp", "chttp-"+ctx.ID)
    if err := os.MkdirAll(workDir, 0750); err != nil {
        return nil, err
    }

    // Set up security context
    if err := sm.setupSecurityContext(workDir); err != nil {
        os.RemoveAll(workDir)
        return nil, err
    }

    return &Sandbox{
        id:      ctx.ID,
        workDir: workDir,
        config:  sm.config,
    }, nil
}

func (s *Sandbox) ExecuteCommand(ctx *AnalysisContext) (*ExecutionResult, error) {
    cmd := exec.CommandContext(ctx.Context, ctx.Config.Executable.Path, ctx.Config.Executable.Args...)
    cmd.Dir = s.workDir
    
    // Security: Run with restricted user
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Credential: &syscall.Credential{
            Uid: 1000, // chttp user
            Gid: 1000, // chttp group
        },
    }

    // Resource limits
    cmd.SysProcAttr.Setpgid = true

    // Capture output
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, err
    }
    stderr, err := cmd.StderrPipe()
    if err != nil {
        return nil, err
    }

    // Start command
    startTime := time.Now()
    if err := cmd.Start(); err != nil {
        return nil, err
    }

    // Collect output
    var stdoutBuf, stderrBuf bytes.Buffer
    go io.Copy(&stdoutBuf, stdout)
    go io.Copy(&stderrBuf, stderr)

    // Wait for completion
    err = cmd.Wait()
    executionTime := time.Since(startTime)

    return &ExecutionResult{
        ExitCode:      cmd.ProcessState.ExitCode(),
        Stdout:        stdoutBuf.String(),
        Stderr:        stderrBuf.String(),
        ExecutionTime: executionTime,
        Success:       err == nil,
    }, nil
}
```

## Unix Pipeline Chaining

### Pipeline Engine

```go
// internal/pipeline/engine.go
package pipeline

type PipelineEngine struct {
    config PipelineConfig
    client *http.Client
}

type PipelineStep struct {
    Service string                 `json:"service"`
    Config  map[string]interface{} `json:"config"`
}

func (pe *PipelineEngine) Execute(ctx context.Context, steps []PipelineStep, input io.Reader) (io.Reader, error) {
    var currentInput io.Reader = input

    for i, step := range steps {
        result, err := pe.executeStep(ctx, step, currentInput, i == len(steps)-1)
        if err != nil {
            return nil, fmt.Errorf("step %d (%s) failed: %w", i, step.Service, err)
        }
        currentInput = result
    }

    return currentInput, nil
}

func (pe *PipelineEngine) executeStep(ctx context.Context, step PipelineStep, input io.Reader, isLast bool) (io.Reader, error) {
    // Create HTTP request to next service
    req, err := http.NewRequestWithContext(ctx, "POST", 
        fmt.Sprintf("https://%s/analyze", step.Service), input)
    if err != nil {
        return nil, err
    }

    // Set headers
    req.Header.Set("Content-Type", "application/gzip")
    req.Header.Set("X-Pipeline-Step", "true")
    
    // Add step config as headers
    for k, v := range step.Config {
        req.Header.Set(fmt.Sprintf("X-Config-%s", k), fmt.Sprint(v))
    }

    // Execute request
    resp, err := pe.client.Do(req)
    if err != nil {
        return nil, err
    }

    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("service returned status %d", resp.StatusCode)
    }

    return resp.Body, nil
}
```

### Pipeline Usage Example

```bash
# CLI usage for pipeline chaining
curl -X POST https://analysis.chttp.ployd.app/pipeline \
  -H "Content-Type: application/json" \
  -d '{
    "steps": [
      {"service": "pylint.chttp.ployd.app"},
      {"service": "bandit.chttp.ployd.app"}, 
      {"service": "formatter.chttp.ployd.app", "config": {"format": "sarif"}}
    ]
  }' \
  --data-binary @codebase.tar.gz
```

## Container Configuration

### Multi-stage Dockerfile

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o chttp ./cmd/chttp

# Tool installation stage  
FROM python:3.11-alpine AS tool-builder
RUN pip install --user pylint==3.0.0

# Runtime stage
FROM gcr.io/distroless/python3-debian11

# Copy executable
COPY --from=builder /src/chttp /usr/local/bin/chttp

# Copy Python tool and dependencies
COPY --from=tool-builder /root/.local /usr/local

# Create chttp user
COPY --from=alpine:3.18 /etc/passwd /etc/passwd
USER 1000:1000

# Configuration
COPY configs/pylint-config.yaml /etc/chttp/config.yaml
COPY configs/keys.json /etc/chttp/keys.json

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/chttp", "--health-check"]

ENTRYPOINT ["/usr/local/bin/chttp", "--config", "/etc/chttp/config.yaml"]
```

### Docker Compose for Development

```yaml
# docker-compose.yml
version: '3.8'

services:
  pylint-chttp:
    build: 
      context: .
      dockerfile: Dockerfile.pylint
    image: ployd/pylint-chttp:latest
    ports:
      - "8080:8080"
    volumes:
      - ./configs/pylint-config.yaml:/etc/chttp/config.yaml
      - ./configs/keys.json:/etc/chttp/keys.json
    environment:
      - LOG_LEVEL=debug
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.pylint.rule=Host(`pylint.chttp.localhost`)"
      - "traefik.http.services.pylint.loadbalancer.server.port=8080"
      - "traefik.http.services.pylint.loadbalancer.healthcheck.path=/health"
    deploy:
      replicas: 3
      resources:
        limits:
          memory: 512M
          cpus: '1.0'
        reservations:
          memory: 128M
          cpus: '0.25'

  bandit-chttp:
    build:
      context: .
      dockerfile: Dockerfile.bandit  
    image: ployd/bandit-chttp:latest
    ports:
      - "8081:8080"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.bandit.rule=Host(`bandit.chttp.localhost`)"

  pipeline-chttp:
    build:
      context: .
      dockerfile: Dockerfile.pipeline
    image: ployd/pipeline-chttp:latest
    ports:
      - "8082:8080"
    depends_on:
      - pylint-chttp
      - bandit-chttp
    environment:
      - UPSTREAM_SERVICES=pylint.chttp.localhost,bandit.chttp.localhost
```

## Deployment Architecture

### Traefik Integration

```yaml
# configs/traefik-chttp.yml
http:
  routers:
    pylint-chttp:
      rule: "Host(`pylint.chttp.ployd.app`)"
      service: "pylint-chttp"
      middlewares:
        - "chttp-auth"
        - "chttp-rate-limit"
      tls:
        certResolver: "letsencrypt"
    
    bandit-chttp:
      rule: "Host(`bandit.chttp.ployd.app`)"
      service: "bandit-chttp"
      middlewares:
        - "chttp-auth"
        - "chttp-rate-limit"
      tls:
        certResolver: "letsencrypt"
        
    pipeline-chttp:
      rule: "Host(`analysis.chttp.ployd.app`)"
      service: "pipeline-chttp"
      middlewares:
        - "chttp-auth"
        - "chttp-pipeline-limit"
      tls:
        certResolver: "letsencrypt"

  services:
    pylint-chttp:
      loadBalancer:
        servers:
          - url: "http://pylint-chttp-1:8080"
          - url: "http://pylint-chttp-2:8080"
          - url: "http://pylint-chttp-3:8080"
        healthCheck:
          path: "/health"
          interval: "30s"
          timeout: "5s"
          
  middlewares:
    chttp-auth:
      headers:
        customRequestHeaders:
          X-Forwarded-Proto: "https"
    chttp-rate-limit:
      rateLimit:
        burst: 100
        average: 50
    chttp-pipeline-limit:
      rateLimit:
        burst: 20
        average: 10
```

### Kubernetes Deployment

```yaml
# k8s/pylint-chttp-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pylint-chttp
  namespace: chttp
spec:
  replicas: 3
  selector:
    matchLabels:
      app: pylint-chttp
  template:
    metadata:
      labels:
        app: pylint-chttp
    spec:
      securityContext:
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
      containers:
      - name: pylint-chttp
        image: ployd/pylint-chttp:latest
        ports:
        - containerPort: 8080
        resources:
          requests:
            memory: "128Mi"
            cpu: "250m"
          limits:
            memory: "512Mi" 
            cpu: "1000m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        volumeMounts:
        - name: config
          mountPath: /etc/chttp
        - name: temp-storage
          mountPath: /tmp
      volumes:
      - name: config
        configMap:
          name: pylint-chttp-config
      - name: temp-storage
        emptyDir:
          sizeLimit: 1Gi
---
apiVersion: v1
kind: Service
metadata:
  name: pylint-chttp
  namespace: chttp
spec:
  selector:
    app: pylint-chttp
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
```

## Security Framework

### Authentication Manager

```go
// internal/auth/manager.go
package auth

import (
    "crypto/rsa"
    "crypto/sha256"
    "crypto/x509"
    "encoding/base64"
    "encoding/json"
    "encoding/pem"
)

type AuthManager struct {
    publicKeys map[string]*ClientKey
    config     SecurityConfig
}

type ClientKey struct {
    PublicKey   *rsa.PublicKey `json:"-"`
    Permissions []string       `json:"permissions"`
    RateLimit   string         `json:"rate_limit"`
    Expires     time.Time      `json:"expires"`
}

func (am *AuthManager) ValidateRequest(c *fiber.Ctx) error {
    clientID := c.Get("X-Client-ID")
    signature := c.Get("X-Signature")
    
    if clientID == "" || signature == "" {
        return fiber.ErrUnauthorized
    }

    // Get client key
    clientKey, exists := am.publicKeys[clientID]
    if !exists {
        return fiber.ErrUnauthorized
    }

    // Check expiration
    if time.Now().After(clientKey.Expires) {
        return fiber.ErrUnauthorized
    }

    // Verify signature
    body := c.Body()
    if err := am.verifySignature(clientKey.PublicKey, body, signature); err != nil {
        return fiber.ErrUnauthorized
    }

    // Store client info in context
    c.Locals("client_id", clientID)
    c.Locals("permissions", clientKey.Permissions)
    
    return c.Next()
}

func (am *AuthManager) verifySignature(publicKey *rsa.PublicKey, data []byte, signature string) error {
    sigBytes, err := base64.StdEncoding.DecodeString(signature)
    if err != nil {
        return err
    }

    hash := sha256.Sum256(data)
    return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], sigBytes)
}
```

### Process Isolation

```go
func (s *Sandbox) setupSecurityContext(workDir string) error {
    // Create restricted user namespace
    if err := s.createUserNamespace(); err != nil {
        return err
    }

    // Set filesystem permissions
    if err := os.Chown(workDir, 1000, 1000); err != nil {
        return err
    }

    // Create resource limits
    if err := s.setupCgroups(); err != nil {
        return err
    }

    // Setup network restrictions
    if s.config.DisableNetwork {
        if err := s.disableNetworking(); err != nil {
            return err
        }
    }

    return nil
}

func (s *Sandbox) setupCgroups() error {
    cgroupPath := filepath.Join("/sys/fs/cgroup", "chttp", s.id)
    
    // Memory limit
    if err := s.setCgroupValue(cgroupPath, "memory.max", s.config.MaxMemory); err != nil {
        return err
    }
    
    // CPU limit  
    if err := s.setCgroupValue(cgroupPath, "cpu.max", s.config.MaxCPU); err != nil {
        return err
    }
    
    return nil
}
```

## Monitoring and Observability

### Prometheus Metrics

```go
// internal/metrics/collector.go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

type MetricsCollector struct {
    requestsTotal    prometheus.Counter
    requestDuration  prometheus.Histogram
    activeRequests   prometheus.Gauge
    executionTime    prometheus.Histogram
    errorRate        prometheus.Counter
    archiveSize      prometheus.Histogram
}

func NewMetricsCollector() *MetricsCollector {
    return &MetricsCollector{
        requestsTotal: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "chttp_requests_total",
                Help: "Total number of requests processed",
            },
            []string{"client_id", "endpoint", "status"},
        ),
        requestDuration: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "chttp_request_duration_seconds",
                Help:    "Request processing duration",
                Buckets: prometheus.DefBuckets,
            },
            []string{"endpoint"},
        ),
        activeRequests: promauto.NewGauge(prometheus.GaugeOpts{
            Name: "chttp_active_requests",
            Help: "Number of active requests",
        }),
        executionTime: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "chttp_execution_duration_seconds",
                Help:    "CLI tool execution duration",
                Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0},
            },
            []string{"tool", "exit_code"},
        ),
    }
}
```

### Structured Logging

```go
// internal/logging/logger.go
package logging

import (
    "github.com/sirupsen/logrus"
)

type RequestLogger struct {
    logger *logrus.Logger
}

func (rl *RequestLogger) LogRequest(ctx *AnalysisContext, result *ExecutionResult) {
    fields := logrus.Fields{
        "request_id":    ctx.ID,
        "client_id":     ctx.ClientID,
        "tool":          ctx.Config.Executable.Path,
        "execution_time": result.ExecutionTime,
        "exit_code":     result.ExitCode,
        "input_size":    len(ctx.InputData),
        "output_size":   len(result.Stdout),
        "success":       result.Success,
    }

    if result.Success {
        rl.logger.WithFields(fields).Info("Analysis completed successfully")
    } else {
        fields["error"] = result.Stderr
        rl.logger.WithFields(fields).Error("Analysis failed")
    }
}
```

## Development and Testing

### Unit Testing Framework

```go
// internal/server/server_test.go
package server

import (
    "bytes"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestCHTTPServer_HandleAnalyze(t *testing.T) {
    // Create test configuration
    config := &config.Config{
        Executable: config.ExecutableConfig{
            Path: "echo",
            Args: []string{"test"},
        },
        Security: config.SecurityConfig{
            AuthMethod: "none", // For testing
        },
    }
    
    server := NewCHTTPServer(config)
    
    // Create test request
    body := bytes.NewBufferString("test input")
    req := httptest.NewRequest("POST", "/analyze", body)
    req.Header.Set("Content-Type", "text/plain")
    
    // Execute request
    resp, err := server.app.Test(req)
    require.NoError(t, err)
    
    assert.Equal(t, 200, resp.StatusCode)
}

func TestSandbox_ExecuteCommand(t *testing.T) {
    sandbox := &Sandbox{
        id:      "test-123",
        workDir: "/tmp/test-123",
        config:  SecurityConfig{},
    }
    
    ctx := &AnalysisContext{
        Config: &config.Config{
            Executable: config.ExecutableConfig{
                Path: "echo",
                Args: []string{"hello", "world"},
            },
        },
    }
    
    result, err := sandbox.ExecuteCommand(ctx)
    require.NoError(t, err)
    
    assert.Equal(t, 0, result.ExitCode)
    assert.Contains(t, result.Stdout, "hello world")
    assert.True(t, result.Success)
}
```

### Integration Testing

```bash
#!/bin/bash
# scripts/integration-test.sh

set -e

echo "Starting CHTTP integration tests..."

# Start test server
./bin/chttp --config configs/test-config.yaml &
SERVER_PID=$!

# Wait for server to start
sleep 2

# Test basic analysis
echo "Testing basic analysis..."
tar -czf test-input.tar.gz test-files/
curl -X POST http://localhost:8080/analyze \
  -H "Content-Type: application/gzip" \
  --data-binary @test-input.tar.gz \
  -o test-output.json

# Verify response
if ! jq -e '.status == "success"' test-output.json; then
    echo "❌ Basic analysis test failed"
    exit 1
fi

echo "✅ Basic analysis test passed"

# Test pipeline chaining
echo "Testing pipeline..."
curl -X POST http://localhost:8080/pipeline \
  -H "Content-Type: application/json" \
  -d '{
    "steps": [{"service": "localhost:8080"}],
    "input": {"format": "tar.gz", "source": "inline"},
    "data": "'$(base64 -i test-input.tar.gz)'"
  }' -o pipeline-output.json

if ! jq -e '.status == "success"' pipeline-output.json; then
    echo "❌ Pipeline test failed"
    exit 1
fi

echo "✅ Pipeline test passed"

# Cleanup
kill $SERVER_PID
rm -f test-input.tar.gz test-output.json pipeline-output.json

echo "🎉 All integration tests passed!"
```

## Performance Optimization

### Streaming Optimizations

```go
// Optimized streaming for large archives
func (s *CHTPServer) optimizedStreamingHandler(c *fiber.Ctx) error {
    // Create buffered pipe for better performance
    pr, pw := io.Pipe()
    
    // Use buffer pool to reduce allocations
    bufferPool := &sync.Pool{
        New: func() interface{} {
            return make([]byte, 32*1024) // 32KB buffer
        },
    }
    
    go func() {
        defer pw.Close()
        buffer := bufferPool.Get().([]byte)
        defer bufferPool.Put(buffer)
        
        _, err := io.CopyBuffer(pw, c.Context().RequestBodyStream(), buffer)
        if err != nil {
            pw.CloseWithError(err)
        }
    }()
    
    return s.processStreamingInput(c, pr)
}

// Memory-efficient archive processing
func (s *Sandbox) extractStreamingArchive(reader io.Reader) error {
    tr := tar.NewReader(reader)
    
    for {
        header, err := tr.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
        
        // Process file without loading into memory
        if err := s.processFileStream(header, tr); err != nil {
            return err
        }
    }
    
    return nil
}
```

### Caching Strategy

```go
// Result caching for performance
type ResultCache struct {
    cache sync.Map
    ttl   time.Duration
}

type CachedResult struct {
    Result    *ExecutionResult
    Timestamp time.Time
}

func (rc *ResultCache) Get(key string) (*ExecutionResult, bool) {
    if val, ok := rc.cache.Load(key); ok {
        cached := val.(*CachedResult)
        if time.Since(cached.Timestamp) < rc.ttl {
            return cached.Result, true
        }
        rc.cache.Delete(key)
    }
    return nil, false
}

func (rc *ResultCache) Set(key string, result *ExecutionResult) {
    rc.cache.Store(key, &CachedResult{
        Result:    result,
        Timestamp: time.Now(),
    })
}

// Cache key generation based on content hash
func generateCacheKey(config *config.Config, inputHash string) string {
    h := sha256.New()
    h.Write([]byte(config.Executable.Path))
    h.Write([]byte(strings.Join(config.Executable.Args, " ")))
    h.Write([]byte(inputHash))
    return hex.EncodeToString(h.Sum(nil))
}
```

## Implementation Roadmap

### Phase 1: Core Server ✅ **COMPLETED** 
- ✅ Basic HTTP server with Fiber
- ✅ Configuration system (YAML)
- ✅ Public key authentication
- ✅ Basic CLI execution sandbox
- ✅ Docker containerization
- ✅ Health checks and metrics

### Phase 2: Advanced Features ✅ **COMPLETED** 
- ✅ Streaming archive processing (2025-08-27)
- ✅ Output parsing framework (2025-08-27)
- ✅ Resource limiting and security (2025-08-27)
- ✅ Comprehensive error handling (2025-08-27)
- ✅ Integration testing suite (2025-08-27)
- ✅ Unix pipe-style chaining (2025-08-27)

### Phase 3: Pipeline Orchestration ✅ **COMPLETED** 
**[Detailed Plan: phase-3-orchestration.md](./phase-3-orchestration.md)**
- ✅ **Parallel pipeline execution engine** (2025-08-27)
- ✅ **Dependency resolution with topological sorting** (2025-08-27)
- ✅ **Concurrent execution with semaphore control** (2025-08-27)
- ✅ **Advanced error propagation and fail-fast support** (2025-08-27)
- 🚧 Advanced orchestration and resource management

### Phase 4: Service Discovery & External Service Orchestration ✅ **COMPLETED**
**[Detailed Plan: phase-4-discovery-balancing.md](./phase-4-discovery-balancing.md)**
- ✅ **Service discovery interface foundation** (2025-08-27)
- ✅ **Consul integration with service registration** (2025-08-27) 
- ✅ **Configuration system extended for service discovery** (2025-08-27)
- ✅ **Architectural simplification - Traefik handles all load balancing** (2025-08-28)
- ✅ **Resilient HTTP client with circuit breakers and retry logic** (2025-08-28)
- ✅ **Comprehensive test coverage achieving >60% coverage requirement** (2025-08-28)

### Phase 5: Advanced Monitoring & Observability 📋 **PLANNED**
**[Detailed Plan: phase-5-observability.md](./phase-5-observability.md)**
- Distributed tracing and advanced metrics
- Performance profiling and alerting

### Phase 6: Production Deployment 📋 **PLANNED**
**[Detailed Plan: phase-6-deployment.md](./phase-6-deployment.md)**
- Kubernetes manifests and Helm charts
- Infrastructure as Code and GitOps deployment

### Phase 7: Security & Performance 📋 **PLANNED**
**[Detailed Plan: phase-7-security-performance.md](./phase-7-security-performance.md)**
- Security auditing and performance benchmarking
- Load testing and optimization

### Phase 8: Documentation & Developer Experience 📋 **PLANNED**
**[Detailed Plan: phase-8-documentation.md](./phase-8-documentation.md)**
- Comprehensive documentation and guides
- Developer tooling and examples

## Migration Path for Ploy

### 1. CHTTP Service Deployment
- Deploy Pylint CHTTP service
- Configure Traefik routing
- Test with sample Python projects

### 2. Static Analysis Integration
- Modify Ploy controller to use CHTTP endpoints
- Update analysis engine to use HTTP calls
- **Clean migration approach - no backward compatibility**

### 3. Pipeline Enhancement
- Add pipeline orchestration
- Chain multiple analyzers
- Implement result aggregation

### 4. Complete Migration
- Migrate all analyzers to CHTTP
- **Replace legacy analysis code entirely**
- Update documentation and APIs

## Conclusion

CHTTP provides a robust, scalable foundation for converting CLI tools into production-ready microservices. With its focus on security, performance, and external service orchestration, CHTTP enables Ploy to migrate from in-process static analysis to a distributed, federated architecture while maintaining simplicity and reliability.

**Key Achievements**: 
- **Phase 3**: Parallel pipeline execution engine with dependency resolution and advanced error handling
- **Phase 4**: Simplified architecture leveraging Traefik for all load balancing, focusing on resilient HTTP client for external service orchestration

**Architectural Benefits**:
- **Simplified Design**: Traefik handles all load balancing at service endpoints
- **Federation Ready**: Orchestrate pipelines across different organizations and infrastructure
- **Resilient Communication**: HTTP client with circuit breakers, retries, and per-service configuration
- **Production Ready**: Reduced complexity while maintaining enterprise-grade reliability

The server architecture supports immediate deployment for Ploy's static analysis migration while enabling federation across multiple CHTTP deployments, making it ideal for distributed teams and multi-organization collaborations.