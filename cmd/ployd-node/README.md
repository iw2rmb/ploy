# ployd-node - Node Agent

The `ployd-node` binary is the node agent that runs on worker nodes in the Ploy cluster. It receives run requests from the control-plane server and executes them.

## Features

- **HTTPS server with mTLS**: Secure communication with the control-plane using mutual TLS authentication
- **Run management endpoints**:
  - `POST /v1/run/start`: Start a new run
  - `POST /v1/run/stop`: Stop/cancel a running job
  - `GET /health`: Health check endpoint
- **Heartbeat mechanism**: Periodically sends resource snapshots to the control-plane server
- **Resource monitoring**: Tracks CPU, memory, and disk usage using the existing lifecycle collector

## Configuration

The node agent is configured via a YAML file (default: `/etc/ploy/ployd-node.yaml`):

```yaml
# Server URL for the control-plane
server_url: https://ployd-server.example.com:8443

# Node identifier
node_id: node-001

# Maximum concurrent runs
concurrency: 4

# HTTP server configuration
http:
  listen: ":8444"
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s
  tls:
    enabled: true
    cert_path: /etc/ploy/certs/node.crt
    key_path: /etc/ploy/certs/node.key
    ca_path: /etc/ploy/certs/ca.crt

# Heartbeat configuration
heartbeat:
  interval: 30s
  timeout: 10s
```

## Usage

```bash
# Start the node agent with default config
ployd-node

# Start with custom config path
ployd-node -config /path/to/config.yaml
```

## Implementation Details

### Package Structure

- `cmd/ployd-node/main.go`: Entry point
- `internal/nodeagent/`:
  - `config.go`: Configuration loading and validation
  - `agent.go`: Main agent orchestration
  - `server.go`: HTTPS server with mTLS
  - `handlers.go`: HTTP request handlers
  - `heartbeat.go`: Heartbeat manager and resource reporting

### Run Lifecycle

1. Server sends `POST /v1/run/start` with run details
2. Node agent accepts the run and returns HTTP 202 Accepted
3. Run is tracked in memory (execution logic to be implemented in next phase)
4. Server can cancel via `POST /v1/run/stop`

### Security & Logging

- TLS 1.3 minimum version
- Mutual TLS (mTLS) required for all endpoints
- Client certificates verified against cluster CA
- Structured logging via Go's `log/slog` to stderr

### Testing

Run tests with:

```bash
go test -v ./internal/nodeagent/...
go test -race -cover ./internal/nodeagent/...
```

## Next Steps

The skeleton implementation provides the foundation for:
- Node execution contract (ephemeral workspaces, git cloning, build execution)
- Log streaming to server
- Diff and artifact upload
- Build gate execution
