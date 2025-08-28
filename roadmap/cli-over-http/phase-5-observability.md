# Phase 5: Basic Logging & Health Monitoring

**Status**: ✅ Completed  
**Duration**: 1 week  
**Dependencies**: Phase 4 completed  
**Next Phase**: [Phase 6: Documentation & Developer Tools](./phase-6-documentation.md)

## Executive Summary

Phase 5 implements minimal logging and health monitoring for CHTTP as a simple CLI-to-HTTP bridge. This phase focuses only on basic operational visibility without duplicating Ploy's comprehensive observability capabilities.

## Objectives

- **Basic Logging**: Simple structured logging for CLI execution tracking
- **Health Endpoint**: Basic HTTP health check endpoint
- **Error Tracking**: Log CLI command failures and HTTP request errors
- **Request Logging**: Track incoming HTTP requests and CLI command executions

**Note**: Advanced observability (tracing, metrics, alerting) is handled by Ploy's comprehensive monitoring system when CHTTP services are deployed.

## Current Status

**Prerequisites from Phase 4:**
- ✅ Resilient HTTP client
- ✅ Circuit breaker patterns
- ✅ Basic external service communication

**Phase 5 Implementation (Completed 2025-08-28):**
- ✅ **Basic structured logging** with JSON and text format support
- ✅ **HTTP health check endpoint** with uptime and configuration summary
- ✅ **CLI execution logging** with duration, success status, and output metrics
- ✅ **Request/response logging** with client IP, status codes, and timing
- ✅ **Authentication logging** for security audit trails
- ✅ **Error logging** with contextual metadata for troubleshooting

## Implementation Plan

### 1. Basic Structured Logging

#### 1.1 Simple Logger Setup

```go
// internal/logging/logger.go
package logging

import (
    "context"
    "log/slog"
    "os"
    "time"
)

// Logger provides basic structured logging for CHTTP
type Logger struct {
    *slog.Logger
}

// NewLogger creates a simple structured logger
func NewLogger() *Logger {
    opts := &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }
    handler := slog.NewJSONHandler(os.Stdout, opts)
    return &Logger{
        Logger: slog.New(handler),
    }
}

// LogCLIExecution logs CLI command execution
func (l *Logger) LogCLIExecution(ctx context.Context, command string, args []string, duration time.Duration, success bool, output string) {
    l.Info("CLI command executed",
        "command", command,
        "args", args,
        "duration_ms", duration.Milliseconds(),
        "success", success,
        "output_length", len(output),
    )
}

// LogHTTPRequest logs HTTP request processing
func (l *Logger) LogHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
    l.Info("HTTP request processed",
        "method", method,
        "path", path,
        "status_code", statusCode,
        "duration_ms", duration.Milliseconds(),
    )
}
```

### 2. Health Check Endpoint

```go
// internal/health/checker.go
package health

import (
    "context"
    "time"
)

// HealthChecker provides basic health checking
type HealthChecker struct {
    startTime time.Time
}

// NewHealthChecker creates a new health checker
func NewHealthChecker() *HealthChecker {
    return &HealthChecker{
        startTime: time.Now(),
    }
}

// CheckHealth performs basic health check
func (hc *HealthChecker) CheckHealth(ctx context.Context) HealthStatus {
    return HealthStatus{
        Status:    "healthy",
        Timestamp: time.Now(),
        Uptime:    time.Since(hc.startTime),
        Version:   "1.0.0",
    }
}

// HealthStatus represents health check response
type HealthStatus struct {
    Status    string        `json:"status"`
    Timestamp time.Time     `json:"timestamp"`
    Uptime    time.Duration `json:"uptime"`
    Version   string        `json:"version"`
}
```

## Configuration

```yaml
# config.yaml - Minimal logging configuration
logging:
  level: "info"          # info, warn, error
  format: "json"         # json, text
  output: "stdout"       # stdout, stderr, file
  
health:
  enabled: true
  endpoint: "/health"
```

## Success Criteria

- ✅ Basic structured logging for CLI execution
- ✅ HTTP health check endpoint responds correctly
- ✅ Request/response logging captures essential information
- ✅ Error logging helps with basic troubleshooting
- ✅ Minimal performance overhead (<1ms per request)

## Next Phase

After completing Phase 5, proceed to [Phase 6: Documentation & Developer Tools](./phase-6-documentation.md) to add basic documentation and simple developer utilities.

**Note**: Comprehensive observability, monitoring, alerting, and performance optimization are handled by Ploy when CHTTP services are deployed to production environments.