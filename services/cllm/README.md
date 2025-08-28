# CLLM Service

CLLM (Code LLM) is a standalone microservice for secure, sandboxed LLM-based code transformation and analysis, designed to enable ARF's self-healing capabilities.

## Quick Start

### Local Development

```bash
# Setup development environment
make dev-setup

# Run tests
make test-unit

# Start TDD watch mode
make tdd

# Build and run locally
make build
make run
```

### Docker Development

```bash
# Start full development stack
make docker-compose-up

# Check service health
curl http://localhost:8082/health

# Stop stack
make docker-compose-down
```

## API Endpoints

### Health Endpoints
- `GET /health` - Service health check
- `GET /ready` - Service readiness check
- `GET /version` - Service version information

### Core API (v1)
- `POST /v1/analyze` - Code analysis endpoint (planned)
- `POST /v1/transform` - Code transformation endpoint (planned)

## Configuration

Configuration is loaded from:
1. Default values (embedded)
2. Environment variables (prefix: `CLLM_`)
3. Configuration file (YAML)

### Environment Variables

```bash
# Server configuration
CLLM_SERVER_HOST=0.0.0.0
CLLM_SERVER_PORT=8082
CLLM_SERVER_READ_TIMEOUT=30s
CLLM_SERVER_WRITE_TIMEOUT=30s

# Sandbox configuration
CLLM_SANDBOX_WORK_DIR=/tmp/cllm-sandbox
CLLM_SANDBOX_MAX_MEMORY=1GB
CLLM_SANDBOX_MAX_CPU_TIME=300s

# LLM provider configuration
CLLM_OLLAMA_URL=http://localhost:11434
CLLM_OLLAMA_MODEL=codellama:7b
```

### Configuration Files

- `configs/cllm-config.yaml` - Default configuration
- `configs/development.yaml` - Development environment
- `configs/production.yaml` - Production environment

## Development

### Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Make

### Testing

```bash
# Run unit tests
make test-unit

# Run tests with coverage
make test-coverage

# Check coverage threshold (60% minimum)
make test-coverage-threshold

# TDD watch mode (requires entr)
make tdd
```

### Building

```bash
# Build binary
make build

# Build Docker image
make docker-build

# Build all artifacts
make build-all
```

## Architecture

### Service Components

```
cllm/
├── cmd/server/           # HTTP server entry point
├── internal/
│   ├── api/             # HTTP handlers and routing
│   ├── config/          # Configuration management
│   ├── sandbox/         # Sandboxed execution engine (planned)
│   ├── providers/       # LLM provider implementations (planned)
│   ├── analysis/        # Code analysis and context (planned)
│   └── diff/            # Git diff generation (planned)
├── configs/             # Configuration templates
└── tests/              # Service-specific tests
```

### Current Status

This service is in **Phase 1: Foundation** implementation:

✅ **Completed**:
- HTTP service framework with Fiber
- Configuration management with YAML support
- Health and readiness endpoints
- Basic API structure and middleware
- Docker containerization
- TDD development workflow
- Unit test coverage >90%

🚧 **In Progress**:
- Project scaffolding completion
- Documentation updates
- Infrastructure integration

📋 **Planned**:
- Sandbox execution engine (Phase 1)
- LLM provider integrations (Phase 1)
- Code analysis and diff generation (Phase 1)
- Model management system (Phase 2)
- Self-healing integration (Phase 3)
- Production features (Phase 4)

## Deployment

### Docker

```bash
# Build and run with Docker
docker build -t cllm:latest .
docker run -p 8082:8082 cllm:latest
```

### Docker Compose

```bash
# Start development stack with Ollama
docker-compose up -d

# Check logs
docker-compose logs -f cllm
```

### Production

Production deployment uses Nomad orchestration with:
- Service discovery via Consul
- Load balancing via Traefik
- Model storage via SeaweedFS
- Monitoring via Prometheus

## Contributing

1. Follow TDD development cycle (Red-Green-Refactor)
2. Maintain >60% test coverage (critical components >90%)
3. Use `make tdd` for watch mode development
4. Update documentation for any API changes
5. Test locally before pushing

## License

Part of the Ploy project - see main repository for license details.