# CHTTP Server Architecture

**Project**: Generic CLI-to-HTTP Wrapper Service  
**Status**: Design Phase  
**Target**: Production-ready microservice foundation for Ploy static analysis migration  

## Executive Summary

CHTTP (CLI-over-HTTP) is a generic wrapper service that converts any command-line tool into a secure, scalable HTTP microservice. Built for Ploy's static analysis migration, CHTTP provides sandboxed execution, streaming file processing, and Unix pipe-style composability.

**Core Value Proposition:**
- Convert any CLI tool to HTTP microservice in minutes
- Production-ready security with public key authentication  
- Container-native with 25-35MB footprint
- Streaming support for large codebases
- Unix pipe-style chaining for complex workflows

## Architecture Overview

### System Components

```
┌─────────────────────────────────────────────┐
│               CHTTP Service                 │
├─────────────────────────────────────────────┤
│ HTTP API Layer                              │
│ ├── Authentication (Public Key)             │
│ ├── Request Validation                      │
│ ├── Rate Limiting                           │
│ └── Middleware Stack                        │
├─────────────────────────────────────────────┤
│ Stream Processing Engine                    │
│ ├── Tar/Zip Extraction                     │
│ ├── File Streaming                         │
│ ├── Output Capture                         │
│ └── Result Serialization                   │
├─────────────────────────────────────────────┤
│ Execution Sandbox                           │
│ ├── Process Isolation                      │
│ ├── Resource Limits                        │
│ ├── Filesystem Jail                        │
│ └── Security Context                       │
├─────────────────────────────────────────────┤
│ CLI Tool Integration                        │
│ ├── Executable Wrapper                     │
│ ├── Argument Management                     │
│ ├── Environment Control                     │
│ └── Output Parsing                         │
└─────────────────────────────────────────────┘
```

### Service Architecture Pattern

```go
// Core service structure
type CHTPService struct {
    config     *Config
    auth       *AuthenticationManager
    sandbox    *SandboxManager
    pipeline   *PipelineEngine
    monitoring *MetricsCollector
}

// Request processing pipeline
HTTP Request → Auth → Validation → Extraction → Execution → Response
     ↓             ↓         ↓           ↓           ↓         ↓
   TLS/JWT    Public Key   Schema    Streaming    Sandbox   JSON/Stream
```

## Configuration System

### Primary Configuration (YAML)

```yaml
# chttp-config.yaml
service:
  name: "pylint-chttp"
  version: "1.0.0" 
  port: 8080
  host: "0.0.0.0"
  
  # Health and monitoring
  health_check_path: "/health"
  metrics_path: "/metrics"
  log_level: "info"

executable:
  # Primary executable configuration
  path: "pylint"
  args: ["--output-format=json", "--reports=no"]
  timeout: "5m"
  working_dir: ""
  
  # Input/Output configuration
  stdin_enabled: false
  capture_stdout: true
  capture_stderr: true
  exit_code_success: [0]

security:
  # Authentication method
  auth_method: "public_key"  # public_key, jwt, none (dev only)
  public_keys_file: "/etc/chttp/keys.json"
  require_signature: true
  
  # Process security
  run_as_user: "chttp"
  run_as_group: "chttp"
  disable_network: false
  readonly_filesystem: false
  
  # Resource limits
  max_memory: "512MB"
  max_cpu: "1.0"
  max_execution_time: "10m"
  max_file_size: "100MB"
  temp_dir_size: "1GB"

input:
  # Supported input formats
  formats: ["tar.gz", "tar", "zip", "raw"]
  max_archive_size: "500MB"
  max_files_count: 10000
  
  # File filtering
  allowed_extensions: [".py", ".pyw"]
  excluded_patterns:
    - "__pycache__/**"
    - "*.pyc"
    - ".git/**"
    - "node_modules/**"
    - "*.log"
  
  # Extraction settings
  extract_to_temp: true
  preserve_permissions: false
  follow_symlinks: false

output:
  # Output processing
  format: "json"           # json, text, stream
  parser: "pylint_json"    # Built-in parsers
  streaming: true          # Enable streaming responses
  compression: "gzip"      # Response compression
  
  # Custom parsing (if parser: "custom")
  custom_parser:
    type: "regex"
    patterns:
      error: "ERROR: (.*)"
      warning: "WARNING: (.*)"

pipeline:
  # Unix pipe-style chaining
  enabled: true
  next_services: []
  
  # Pipeline configuration
  pass_through_headers: ["X-Client-ID", "X-Request-ID"]
  aggregate_results: false
  fail_fast: true

monitoring:
  # Observability
  enabled: true
  prometheus_metrics: true
  request_logging: true
  performance_tracking: true
  
  # Health checks
  health_check_interval: "30s"
  ready_check_command: ["pylint", "--version"]
```

### Security Keys Configuration

```json
{
  "keys": {
    "ploy-controller": {
      "public_key": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA...",
      "permissions": ["analyze", "health"],
      "rate_limit": "100/minute",
      "expires": "2025-12-31T23:59:59Z"
    },
    "ci-system": {
      "public_key": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA...",
      "permissions": ["analyze"],
      "rate_limit": "50/minute",
      "expires": "2025-06-30T23:59:59Z"
    }
  },
  "default_permissions": ["health"],
  "signature_algorithm": "RS256"
}
```

## API Specification

### Core Endpoints

#### 1. Analysis Endpoint

```http
POST /analyze HTTP/1.1
Host: pylint.chttp.ployd.app
Content-Type: application/gzip
X-Client-ID: ploy-controller  
X-Signature: <RSA-SHA256-signature>
X-Request-ID: uuid-v4

[gzipped tar archive of codebase]
```

**Response:**
```json
{
  "id": "analysis-uuid",
  "timestamp": "2025-08-26T10:30:00Z",
  "status": "success",
  "executable": {
    "path": "pylint",
    "version": "3.0.0",
    "args": ["--output-format=json", "--reports=no"],
    "exit_code": 0,
    "execution_time": "2.5s"
  },
  "input": {
    "format": "tar.gz",
    "size_bytes": 52428800,
    "files_processed": 247,
    "files_skipped": 15
  },
  "output": {
    "format": "json",
    "size_bytes": 15420,
    "compression": "gzip",
    "streaming": true
  },
  "result": {
    "issues": [
      {
        "file": "src/main.py",
        "line": 10,
        "column": 4,
        "severity": "error",
        "rule": "syntax-error",
        "message": "invalid syntax",
        "suggestion": "Check syntax around line 10"
      }
    ],
    "summary": {
      "total_issues": 23,
      "by_severity": {
        "error": 2,
        "warning": 8,
        "info": 13
      }
    }
  },
  "metrics": {
    "execution_time": "2.5s",
    "memory_used": "45MB",
    "cpu_time": "1.2s"
  }
}
```

#### 2. Pipeline Chaining Endpoint

```http
POST /pipeline HTTP/1.1
Host: analysis.chttp.ployd.app
Content-Type: application/json
X-Client-ID: ploy-controller

{
  "steps": [
    {
      "service": "pylint.chttp.ployd.app",
      "config": {"min_score": 8.0}
    },
    {
      "service": "bandit.chttp.ployd.app", 
      "config": {"severity": "medium"}
    },
    {
      "service": "formatter.chttp.ployd.app",
      "config": {"format": "sarif"}
    }
  ],
  "input": {
    "format": "tar.gz",
    "source": "inline"
  },
  "data": "<base64-encoded-archive>"
}
```

#### 3. Health and Status

```http
GET /health HTTP/1.1
Host: pylint.chttp.ployd.app

# Response
{
  "status": "healthy",
  "version": "1.0.0",
  "executable": {
    "available": true,
    "path": "/usr/local/bin/pylint",
    "version": "3.0.0"
  },
  "resources": {
    "memory_used": "25MB",
    "cpu_load": "0.1",
    "disk_used": "120MB"
  },
  "uptime": "5h32m15s"
}
```

#### 4. Capabilities Discovery

```http
GET /capabilities HTTP/1.1
Host: pylint.chttp.ployd.app

# Response
{
  "service": "pylint-chttp",
  "version": "1.0.0",
  "executable": {
    "name": "pylint",
    "version": "3.0.0",
    "supported_formats": ["python"]
  },
  "input_formats": ["tar.gz", "tar", "zip"],
  "output_formats": ["json", "text", "sarif"],
  "pipeline_compatible": true,
  "streaming_support": true,
  "security_features": ["public_key_auth", "process_isolation", "resource_limits"],
  "supported_extensions": [".py", ".pyw"]
}
```

## Implementation Architecture

### Core Server Implementation

```go
// cmd/chttp/main.go
package main

import (
    "context"
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/ployd/chttp/internal/server"
    "github.com/ployd/chttp/internal/config"
)

func main() {
    var configFile = flag.String("config", "/etc/chttp/config.yaml", "Configuration file path")
    flag.Parse()

    // Load configuration
    cfg, err := config.LoadConfig(*configFile)
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Create server
    srv := server.NewCHTTPServer(cfg)

    // Graceful shutdown
    ctx, cancel := signal.NotifyContext(context.Background(), 
        syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    if err := srv.Run(ctx); err != nil {
        log.Fatalf("Server error: %v", err)
    }
}
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
./build/chttp --config configs/test-config.yaml &
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

### Phase 1: Core Server (Weeks 1-2)
- ✅ Basic HTTP server with Fiber
- ✅ Configuration system (YAML)
- ✅ Public key authentication
- ✅ Basic CLI execution sandbox
- ✅ Docker containerization
- ✅ Health checks and metrics

### Phase 2: Advanced Features (Weeks 3-4)
- 🚧 Streaming archive processing
- 🚧 Output parsing framework
- 🚧 Resource limiting and security
- 🚧 Comprehensive error handling
- 🚧 Integration testing suite

### Phase 3: Pipeline System (Weeks 5-6)
- ❌ Unix pipe-style chaining
- ❌ Pipeline orchestration engine
- ❌ Service discovery integration
- ❌ Load balancing optimization
- ❌ Advanced monitoring

### Phase 4: Production Ready (Weeks 7-8)
- ❌ Kubernetes deployment manifests
- ❌ Traefik integration
- ❌ Performance benchmarking
- ❌ Security audit and hardening
- ❌ Documentation completion

## Migration Path for Ploy

### 1. CHTTP Service Deployment
- Deploy Pylint CHTTP service
- Configure Traefik routing
- Test with sample Python projects

### 2. Static Analysis Integration
- Modify Ploy controller to use CHTTP endpoints
- Update analysis engine to use HTTP calls
- Maintain backward compatibility

### 3. Pipeline Enhancement
- Add pipeline orchestration
- Chain multiple analyzers
- Implement result aggregation

### 4. Complete Migration
- Migrate all analyzers to CHTTP
- Remove legacy analysis code
- Update documentation and APIs

## Conclusion

CHTTP provides a robust, scalable foundation for converting CLI tools into production-ready microservices. With its focus on security, performance, and Unix-philosophy composability, CHTTP enables Ploy to migrate from in-process static analysis to a distributed, sandboxed architecture while maintaining simplicity and reliability.

The server architecture supports immediate deployment for Ploy's static analysis migration while offering potential as a standalone product for the broader developer community seeking to modernize CLI tools for cloud-native environments.