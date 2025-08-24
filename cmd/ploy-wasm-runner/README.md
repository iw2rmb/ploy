# ploy-wasm-runner

## Purpose
This is the WebAssembly runtime engine for Lane G deployments. It runs INSIDE deployed containers to execute WASM modules compiled from user applications.

**IMPORTANT**: This is NOT a CLI tool for end users. This is a deployment runtime component.

## Architecture Role

- **NOT a CLI tool** - Users never interact with it directly
- **NOT part of controller** - Controller packages it but doesn't run it  
- **YES a deployment artifact** - Runs as the main process in Lane G containers

## How It Works

1. **Build Phase**: Controller compiles user code to WASM (`app.wasm`)
2. **Package Phase**: Controller builds `ploy-wasm-runner` binary for target architecture
3. **Deploy Phase**: Both artifacts get packaged into deployment image
4. **Runtime Phase**: Nomad deploys container with `ploy-wasm-runner` as entrypoint
5. **Execution Phase**: `ploy-wasm-runner` serves HTTP by executing the WASM module

## Deployment Context

When deployed in Lane G, the Nomad job configuration looks like:

```hcl
task "wasm-runner" {
  driver = "exec"
  
  config {
    command = "/usr/local/bin/ploy-wasm-runner"
    args = [
      "--module", "local/app.wasm",
      "--port", "${NOMAD_PORT_http}",
      "--max-memory", "32MB",
      "--timeout", "30s"
    ]
  }
}
```

## Runtime Analogies

`ploy-wasm-runner` serves the same role for WASM apps as:
- **Node.js runtime** for JavaScript applications
- **Python interpreter** for Python applications
- **JVM** for Java applications
- **CLR** for .NET applications

## Features

- **HTTP Server**: Exposes WASM module functionality via HTTP endpoints
- **WASI Support**: Provides WASI (WebAssembly System Interface) for system calls
- **Resource Limits**: Enforces memory and execution time constraints
- **Health Monitoring**: Provides `/health` and `/wasm-health` endpoints
- **Metrics Collection**: Exposes Prometheus-style metrics at `/metrics`
- **Graceful Shutdown**: Handles SIGTERM for clean container termination

## Building

The controller builds this binary during Lane G deployment preparation:

```bash
# Built by controller, not by users
go build -o ploy-wasm-runner ./cmd/ploy-wasm-runner
```

## Configuration

Runtime configuration via command-line flags:
- `--module`: Path to WASM module file (required)
- `--port`: HTTP server port (default: 8080)
- `--max-memory`: Maximum memory for WASM module (default: 32MB)
- `--timeout`: Execution timeout per request (default: 30s)
- `--env`: Environment variables for WASM module
- `--wasi-root`: WASI filesystem root (default: /tmp/wasm-sandbox)
- `--log-level`: Logging level (debug, info, warn, error)

## HTTP Endpoints

When running, the service exposes:
- `/` - Main application endpoint (executes WASM module)
- `/health` - Basic health check
- `/wasm-health` - WASM runtime health validation
- `/metrics` - Prometheus metrics

## Development

For local testing during Lane G development:

```bash
# Build the runner
go build -o build/ploy-wasm-runner ./cmd/ploy-wasm-runner

# Test with a WASM module
./build/ploy-wasm-runner \
  --module test.wasm \
  --port 8080 \
  --max-memory 64MB
```

## Integration with Ploy

This component integrates with the Ploy deployment pipeline:
1. User pushes code with `ploy push`
2. Controller detects Lane G (WASM target)
3. Controller compiles source to WASM
4. Controller packages `ploy-wasm-runner` + `app.wasm`
5. Nomad deploys the package
6. `ploy-wasm-runner` serves the application

## Security Considerations

- Runs with restricted permissions in containers
- Memory and CPU limits enforced at WASM runtime level
- WASI filesystem access restricted to sandbox directory
- No network access from WASM unless explicitly configured