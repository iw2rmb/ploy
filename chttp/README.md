# CHTTP - Simple CLI-to-HTTP Bridge

CHTTP is a lightweight service that provides HTTP access to command-line tools. It serves as a simple bridge between HTTP requests and CLI command execution, designed to be deployed and managed by Ploy's comprehensive platform.

## Features

- **Simple HTTP-to-CLI Bridge**: Execute command-line tools via HTTP requests
- **Basic Security**: API key authentication and command allow-listing  
- **Structured Logging**: JSON-formatted logging for operations tracking
- **Health Monitoring**: Basic health check endpoint
- **Lightweight**: Minimal dependencies and resource footprint
- **Ploy Integration**: Designed for deployment via Ploy's platform

## Quick Start

### 1. Configuration

Create a `config.yaml` file:

```yaml
server:
  host: "0.0.0.0"
  port: 8080

security:
  api_key: "your-secret-api-key"

commands:
  allowed:
    - "echo"
    - "ls" 
    - "cat"
    - "grep"
    - "find"
  default_timeout: "30s"

logging:
  level: "info"      # info, warn, error
  format: "json"     # json, text

health:
  enabled: true
  endpoint: "/health"
```

### 2. Start the Server

```bash
go build -o chttp ./cmd/chttp
./chttp -config config.yaml
```

### 3. Make Requests

```bash
# Execute a command
curl -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret-api-key" \
  -d '{
    "command": "echo",
    "args": ["Hello, World!"],
    "timeout": "10s"
  }'

# Check health
curl http://localhost:8080/health
```

## API Reference

### Execute CLI Command

**POST** `/api/v1/execute`

Execute a CLI command and return the result.

**Headers:**
- `Content-Type: application/json`
- `X-API-Key: <your-api-key>` or `Authorization: Bearer <your-api-key>`

**Request Body:**
```json
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

Check service health status.

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2025-01-15T10:30:00Z",
  "uptime": "2h30m15s",
  "version": "1.0.0",
  "config": {
    "allowed_commands": 5,
    "log_level": "info",
    "port": 8080
  }
}
```

## Configuration Reference

| Field | Type | Description |
|-------|------|-------------|
| `server.host` | string | Server bind address (default: "0.0.0.0") |
| `server.port` | int | Server port (default: 8080) |
| `security.api_key` | string | API key for authentication (required) |
| `commands.allowed` | []string | List of allowed CLI commands |
| `commands.default_timeout` | duration | Default command timeout (default: "30s") |
| `logging.level` | string | Log level: info, warn, error (default: "info") |
| `logging.format` | string | Log format: json, text (default: "json") |
| `health.enabled` | bool | Enable health endpoint (default: true) |
| `health.endpoint` | string | Health endpoint path (default: "/health") |

## Security

- **Command Allow-listing**: Only commands in `commands.allowed` can be executed
- **API Key Authentication**: Requests must include valid API key in header
- **Input Validation**: Request parameters are validated before execution
- **Timeout Protection**: Commands are terminated if they exceed timeout limits

## Deployment with Ploy

CHTTP is designed to be deployed via Ploy's platform, which provides:

- **Infrastructure Management**: Scaling, load balancing, service discovery
- **Advanced Security**: TLS termination, network policies, authentication
- **Monitoring & Alerting**: Metrics collection, distributed tracing, alerting
- **Deployment Automation**: Blue-green, canary, rolling deployments

Example Ploy deployment:

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

## Development

### Project Structure

```
chttp/
├── cmd/chttp/              # Main application entry point
├── internal/
│   ├── config/             # Configuration management
│   ├── executor/           # CLI command execution
│   ├── handler/            # HTTP request handlers
│   ├── health/             # Health checking
│   ├── logging/            # Structured logging
│   └── server/             # HTTP server setup
├── configs/                # Example configurations
├── tests/                  # Basic integration tests
└── README.md
```

### Building

```bash
# Build the server
go build -o chttp ./cmd/chttp

# Run tests
go test ./...

# Build for production
CGO_ENABLED=0 GOOS=linux go build -o chttp-linux ./cmd/chttp
```

### Testing

```bash
# Run unit tests
go test -v ./internal/...

# Run integration tests
go test -v ./tests/...

# Test with actual server
./chttp -config configs/config.yaml &
curl -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret-api-key" \
  -d '{"command": "echo", "args": ["test"]}'
```

## Architecture Principles

CHTTP follows these design principles:

1. **Simplicity**: Focus solely on CLI-to-HTTP translation
2. **Security**: Command allow-listing and API key authentication
3. **Reliability**: Basic error handling and timeout protection
4. **Observability**: Structured logging for operations tracking
5. **Ploy Integration**: Designed for Ploy platform deployment

## Limitations

CHTTP is intentionally simple and does **not** include:

- Complex pipeline orchestration (use external orchestration tools)
- Advanced observability features (handled by Ploy platform)
- Load balancing or service discovery (handled by Ploy/Traefik)
- File upload/streaming (commands work with local filesystem only)
- Process sandboxing (relies on container/deployment security)

For enterprise features like advanced monitoring, deployment automation, and infrastructure management, deploy CHTTP services via the Ploy platform.

## License

MIT License - See LICENSE file for details.