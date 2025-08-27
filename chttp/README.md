# CHTTP - CLI-over-HTTP Microservices

CHTTP provides a secure, distributed architecture for running static analysis tools as containerized microservices. This replaces the previous in-process analysis system with sandboxed, scalable services.

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Controller    │───▶│  CHTTP Client   │───▶│  Pylint CHTTP   │
│   (API Server)  │    │   (HTTP Client) │    │   (Service)     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                │                        │
                                ▼                        ▼
                       ┌─────────────────┐    ┌─────────────────┐
                       │ Public Key Auth │    │ Sandboxed Exec  │
                       │   (Security)    │    │   (Isolation)   │
                       └─────────────────┘    └─────────────────┘
```

## Features

- **Security**: Sandboxed execution with process isolation
- **Scalability**: Independent service scaling and load balancing
- **Container-Native**: 25-35MB Docker images with distroless base
- **Authentication**: Public key cryptography for service access
- **Resource Limits**: CPU/memory constraints and filesystem restrictions
- **Health Monitoring**: Built-in health checks and metrics

## Services

### Pylint CHTTP Service

Python static analysis service using Pylint with JSON output formatting.

**Configuration**: `configs/pylint-chttp-config.yaml`
```yaml
service:
  name: "pylint-chttp"
  port: 8080

executable:
  path: "pylint"
  args: ["--output-format=json", "--reports=no"]
  timeout: "5m"

security:
  auth_method: "public_key"
  run_as_user: "pylint"
  max_memory: "512MB"
  max_cpu: "1.0"
```

## Development

### Prerequisites

- Docker and Docker Compose
- Go 1.24+
- Python 3.11+ (for Pylint)

### Building

```bash
# Build all components locally
go build -o build/pylint-chttp ./cmd/pylint-chttp

# Build Docker image
./scripts/build-docker.sh

# Build with specific version
./scripts/build-docker.sh v1.0.0
```

### Development Environment

```bash
# Start development stack
docker-compose up -d

# Generate test data
docker-compose --profile testing up test-generator

# Run integration tests
docker-compose --profile testing run chttp-client

# View logs
docker-compose logs -f pylint-chttp
```

### Testing

```bash
# Unit tests
go test ./...

# Integration tests with Docker
docker-compose --profile testing up --abort-on-container-exit

# Manual testing
curl http://localhost:8080/health

# Test analysis (development mode)
echo 'import os\nprint("hello")' | tar -czf test.tar.gz -T-
curl -X POST -H "Content-Type: application/gzip" \
  --data-binary @test.tar.gz \
  http://localhost:8080/analyze
```

## Production Deployment

### Container Registry

```bash
# Tag for registry
docker tag ploy/pylint-chttp:latest registry.example.com/ploy/pylint-chttp:v1.0.0

# Push to registry
docker push registry.example.com/ploy/pylint-chttp:v1.0.0
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pylint-chttp
spec:
  replicas: 3
  selector:
    matchLabels:
      app: pylint-chttp
  template:
    spec:
      containers:
      - name: pylint-chttp
        image: registry.example.com/ploy/pylint-chttp:v1.0.0
        ports:
        - containerPort: 8080
        resources:
          requests:
            memory: "256Mi"
            cpu: "200m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
```

### Docker Swarm/Nomad

```bash
# Deploy with Docker Swarm
docker service create \
  --name pylint-chttp \
  --replicas 3 \
  --publish 8080:8080 \
  --constraint 'node.role==worker' \
  ploy/pylint-chttp:latest

# Deploy with Nomad (see platform/nomad/pylint-chttp.hcl)
nomad job run platform/nomad/pylint-chttp.hcl
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CHTTP_CONFIG_PATH` | Path to YAML config file | `/etc/chttp/config.yaml` |
| `CHTTP_SERVICE_NAME` | Service identifier | `pylint-chttp` |
| `CHTTP_LOG_LEVEL` | Logging level | `info` |
| `CHTTP_AUTH_DISABLED` | Disable auth (dev only) | `false` |

### Security Configuration

```yaml
security:
  auth_method: "public_key"
  public_key_path: "/etc/chttp/public.pem"
  run_as_user: "pylint"
  max_memory: "512MB"
  max_cpu: "1.0"
  sandbox_enabled: true
  temp_dir: "/tmp"
```

## API Reference

### Health Check

```
GET /health
```

Response:
```json
{
  "status": "ok",
  "timestamp": "2025-08-26T10:30:00Z",
  "service": "pylint-chttp"
}
```

### Analysis

```
POST /analyze
Content-Type: application/gzip
Authorization: Bearer <signed-token>

[tar.gz archive of source code]
```

Response:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "success",
  "timestamp": "2025-08-26T10:30:00Z",
  "result": {
    "issues": [
      {
        "file": "main.py",
        "line": 10,
        "column": 1,
        "severity": "warning",
        "rule": "unused-import",
        "message": "Unused import 'os'"
      }
    ]
  }
}
```

## Monitoring

### Metrics

CHTTP services expose Prometheus metrics on `/metrics`:

- `chttp_requests_total{method,status}` - Request count by method/status
- `chttp_request_duration_seconds` - Request duration histogram
- `chttp_analysis_duration_seconds` - Analysis execution time
- `chttp_active_analyses` - Currently running analyses

### Logging

Structured JSON logging with configurable levels:

```json
{
  "timestamp": "2025-08-26T10:30:00Z",
  "level": "info",
  "service": "pylint-chttp",
  "message": "Analysis completed",
  "analysis_id": "550e8400-e29b-41d4-a716-446655440000",
  "duration_ms": 1250,
  "files_processed": 15,
  "issues_found": 3
}
```

## Security

### Process Isolation

- Non-root user execution (UID 1000)
- Read-only container filesystem
- Temporary filesystem for analysis (`tmpfs`)
- Dropped capabilities except essential ones
- No new privileges allowed

### Network Security

- TLS encryption for all communications
- Public key authentication
- IP-based access controls
- Request rate limiting

### Data Protection

- No persistent storage of analyzed code
- Automatic cleanup after analysis
- Memory limits to prevent resource exhaustion
- Timeout protection against long-running analyses

## Contributing

1. Fork the repository
2. Create a feature branch
3. Write tests for new functionality
4. Ensure all tests pass: `go test ./...`
5. Build and test Docker image: `./scripts/build-docker.sh`
6. Submit a pull request

## License

See [LICENSE](../LICENSE) file for details.