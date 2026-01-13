# ployd-node - Node Agent

The `ployd-node` binary is the node agent that runs on worker nodes in the Ploy cluster. It polls the control-plane server for jobs and executes them.

## Features

- **HTTPS server with mTLS**: Secure communication with the control-plane using mutual TLS authentication
- **Claim loop (canonical execution path)**: Polls the control-plane for jobs via `POST /v1/nodes/{id}/claim`
- **Run management endpoints** (primarily for local/manual use):
  - `POST /v1/run/start`: Start a run/job via the node HTTP API
  - `POST /v1/run/stop`: Stop/cancel a running job via the node HTTP API
  - `GET /health`: Health check endpoint
- **Heartbeat mechanism**: Periodically sends resource snapshots to the control-plane server
- **Resource monitoring**: Tracks CPU, memory, and disk usage using the existing lifecycle collector

## Configuration

The node agent is configured via a YAML file (default: `/etc/ploy/ployd-node.yaml`):

```yaml
# Server URL for the control-plane
server_url: https://ployd.example.com:8443

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
    cert_path: /etc/ploy/pki/node.crt
    key_path: /etc/ploy/pki/node.key
    ca_path: /etc/ploy/pki/ca.crt
    # Optional CA bundle to verify the control-plane server during bootstrap
    # (before node certificates are obtained). If omitted, ca_path is used
    # when present; otherwise system roots are used.
    bootstrap_ca_path: /etc/ploy/pki/ca.crt

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

1. Node agent polls `POST /v1/nodes/{id}/claim` for work
2. Server returns `204 No Content` when no work is available, or `200 OK` with a claimed job payload (including `spec`)
3. Node agent parses the spec into typed execution inputs and executes the claimed job
4. Node agent reports job status and uploads artifacts/diffs back to the control-plane

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

See `docs/how-to/deploy-locally.md`.
