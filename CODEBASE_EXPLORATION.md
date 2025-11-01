# Ploy Codebase Exploration Summary

## Overview
Ploy is a workstation-first orchestration stack for code-mod (Mods) workflows. It consists of:
- `ploy` — CLI for submitting Mods, following logs, managing artifacts, and administering nodes
- `ployd` — daemon running both HTTPS control-plane APIs and worker execution loop

## Directory Structure

### Core Layout
```
/Users/vk/@iw2rmb/ploy/
├── cmd/                    # Entry points
│   ├── ploy/              # CLI application
│   └── ployd/             # Daemon server
├── internal/              # Private packages
│   ├── api/               # HTTP and daemon components
│   │   ├── config/        # Configuration loading and types
│   │   ├── daemon/        # Daemon initialization (default.go is main entry)
│   │   ├── httpserver/    # Fiber HTTP server and endpoints
│   │   ├── pki/           # PKI/certificate management
│   │   ├── controlplane/  # Control-plane handlers
│   │   ├── admin/         # Admin service (node registration)
│   │   ├── executor/      # Step execution logic
│   │   ├── metrics/       # Prometheus metrics
│   │   ├── scheduler/     # Task scheduling
│   │   ├── runtime/       # Runtime plugins
│   │   └── status/        # Node status provider
│   ├── bootstrap/         # Bootstrap logic
│   ├── controlplane/      # Control-plane orchestration
│   ├── deploy/            # Deployment utilities (PKI bootstrap, CA rotation)
│   ├── node/              # Node-local components (jobs, lifecycle, workers)
│   ├── store/             # PostgreSQL store (pgx + sqlc generated)
│   ├── workflow/          # Workflow execution and artifacts
│   ├── metrics/           # Metrics collection
│   └── ... (other internal packages)
├── pkg/                   # Public packages (no SSH tunnels; CLI uses HTTPS)
├── tests/                 # Integration and unit tests
└── docs/                  # Documentation
```

## Main Entry Point: cmd/ployd/main.go

### Commands
The ployd binary supports multiple subcommands:
1. **`bootstrap-ca`** — Bootstrap PKI and issue initial certificates
2. **`slot-guard`** — Guard process for transfer slots
3. **`status-snapshot`** — One-shot lifecycle snapshot collection
4. **default (daemon mode)** — Start the ployd server

### Server Startup Flow
1. Load configuration from `/etc/ploy/ployd.yaml` (or via `-config` flag)
2. Call `daemon.NewDefault(cfg)` to initialize all components
3. Set up signal handlers (SIGINT, SIGTERM, SIGHUP for reload)
4. Call `svc.Run(ctx)` to start the daemon

## Daemon Initialization: internal/api/daemon/default.go

This is the **primary integration point** where all major components wire together:

### Key Components Initialized

1. **Postgres Store** (lines 78-87)
   - Resolution order: `PLOY_SERVER_PG_DSN` → `PLOY_POSTGRES_DSN` → config file `postgres.dsn`
   - Uses `store.NewStore()` which creates a pgx connection pool
   - Implements `Store` interface with sqlc-generated `Queries`

2. **Log Streams Hub** (line 61)
   - Manages real-time job log streaming
   - Used by job log endpoints

3. **Workflow Runtime Registry** (lines 62-68)
   - Loads runtime plugins and feature flags
   - Default factories registered for step execution

4. **Status Provider** (lines 70-75)
   - Provides node status snapshots for `/v1/node/status` endpoint
   - Uses lifecycle cache for real-time data

5. **Control-Plane Integration** (lines 89-95, 347-524)
   - **HTTP Client** (lines 305-345): Mutual TLS to control-plane
     - Requires: `ControlPlaneConfig.Endpoint` (HTTPS only), `CAPath`, `Certificate`, `Key`
     - Min TLS version: TLS 1.2
   - **Control-Plane Handler** (lines 347-524): Builds HTTP handler tree
     - Integrates etcd client for cluster coordination
     - Initializes scheduler, mods service, artifact store, transfer manager
     - Wires in authorization and role-based access control

6. **PKI Manager** (lines 102-109)
   - Manages certificate renewal with periodic rotation
   - Uses `fileRotator` to ensure certificate files exist
   - Default renewal interval: `cfg.PKI.RenewBefore` (default 1 hour)

7. **HTTP Server** (lines 212-223)
   - Fiber-based server with:
     - Node-local endpoints (`/v1/node/*`)
     - Admin registration endpoints (`/v1/admin/*`)
     - Control-plane routes (`/v1/*`, `/v2/*`, `/metrics`)

8. **Lifecycle Collector & Publisher** (lines 153-209)
   - Collects node health data (Docker, IPFS, Java build gate readiness)
   - Publishes status to etcd (if available)
   - Falls back to local cache if etcd unavailable

9. **Control-Plane Client** (lines 128-136)
   - Heartbeat mechanism to control-plane
   - Assignment polling and job execution

10. **Task Scheduler** (line 138)
    - Background task execution (lifecycle publishing, etc.)

### Environment Variables Resolved
- `PLOY_SERVER_PG_DSN` / `PLOY_POSTGRES_DSN` — PostgreSQL connection
- `PLOY_IPFS_CLUSTER_API` / `PLOY_IPFS_CLUSTER_TOKEN` / etc. — IPFS artifact publishing
- `PLOY_GITLAB_SIGNER_AES_KEY` — GitLab signer initialization
- `PLOY_CLUSTER_ID` — Cluster identity
- `PLOY_LIFECYCLE_NET_IGNORE` — Network interfaces to ignore in health checks

## Server Implementation: internal/api/httpserver/server.go

### HTTP Server Structure
- Uses **Fiber v2** framework for request handling
- TLS support: Configurable via `cfg.HTTP.TLS.Enabled`, `CertPath`, `KeyPath`
- Graceful shutdown with timeout (5 seconds)
- Configuration hot-reload via `Reload()` method

### Route Mounting (lines 224-243)

**Node-Local Endpoints** (no TLS/auth required):
- `GET /v1/node/status` — Node health snapshot
- `GET /v1/node/health` — Alias for status
- `GET /v1/node/jobs` — List active jobs
- `GET /v1/node/jobs/{jobID}` — Job details
- `GET /v1/node/jobs/{jobID}/logs/stream` — Server-sent events job log stream
- `GET /v1/node/jobs/{jobID}/logs/snapshot` — Full job log snapshot
- `POST /v1/node/jobs/{jobID}/logs/entries` — Log entry submission
- `POST /v1/node/jobs/{jobID}/cancel` — Cancel running job
- `POST /v1/admin/nodes` — Node registration (via `AdminService.RegisterNode`)

**Control-Plane Routes** (mutual TLS + bearer token + scope-based):
- All routes under `/v1/*` and `/v2/*` are delegated to `controlPlaneHandler`
- Also includes `/metrics` (Prometheus)

## Control-Plane API Handler: internal/api/httpserver/controlplane_server.go

### Security Architecture
1. **Mutual TLS** (requires client certificate)
   - Enforced by `security.Manager.Middleware()`
   - Checks `r.TLS.PeerCertificates` or `r.TLS.VerifiedChains`

2. **Bearer Token Authentication**
   - Extracted from `Authorization: Bearer <token>` header
   - Verified via `TokenVerifier.Verify()` interface
   - Returns `Principal` with scopes and expiry

3. **Scope-Based Authorization**
   - Scopes: `admin`, `mods`, `jobs`, `artifact.read`, `artifact.write`, `registry.pull`, `registry.push`
   - Enforced per route via `registerRoute(..., scopes...)`

4. **Role-Based Access Control** (optional)
   - Roles: `RoleControlPlane`, `RoleCLIAdmin`, `RoleWorker`
   - Applied via `Authorizer.Middleware(roles...)`

### Registered Routes

**Job Management**:
- `GET/POST /v1/jobs` — List/submit jobs
- `POST /v1/jobs/claim` — Worker claims next job
- Dynamic subpaths under `/v1/jobs/{id}/*`

**Node Management**:
- `GET/PATCH /v1/nodes` — List/update node status
- Dynamic subpaths under `/v1/nodes/{id}/*`

**Configuration**:
- `GET/PUT /v1/config` — Cluster configuration (admin only)
- `GET/PUT /v1/config/gitlab` — GitLab signer config (admin only)

**Security & PKI**:
- `GET /v1/security/ca` — Return CA bundle (admin only)
- `POST /v1/security/certificates/control-plane` — Issue control-plane certificate (admin only)

**Mods Orchestration** (if mods service enabled):
- `POST /v1/mods` — Submit mod
- `GET /v1/mods/{id}` — Get mod details
- `POST /v1/mods/tickets` — Create mod ticket
- Dynamic subpaths under `/v1/mods/tickets/{id}/*`

**Artifacts**:
- `POST /v1/artifacts/upload` — Upload artifact (write scope)
- `GET /v1/artifacts` — List artifacts (read scope)
- Dynamic subpaths under `/v1/artifacts/{id}/*`
- **v2 aliases** — Same handlers for migration

**File Transfers**:
- `POST /v1/transfers/upload` — Initiate upload
- `POST /v1/transfers/download` — Initiate download
- `POST /v1/transfers/{slotID}` — Slot actions (write scope)

**Misc**:
- `GET /v1/health` — Health check (no auth)
- `GET /v1/version` — Version info (no auth)
- `GET /v1/status` — Cluster status summary (control-plane/admin/worker roles)
- `PUT /v1/gitlab/signer/secrets` — Manage signer secrets (admin only)
- `POST /v1/gitlab/signer/tokens` — Create signer tokens (admin only)
- `GET /v1/gitlab/signer/rotations` — Get rotation events (admin only)
- `GET /metrics` — Prometheus metrics (no auth)

## PKI & TLS Configuration: internal/api/pki/manager.go

### PKI Manager
- Wraps `config.PKIConfig` with periodic renewal monitoring
- Calls `Rotator.Renew()` every `cfg.PKI.RenewBefore` interval
- Default: 1 hour renewal check

### File Rotator (in daemon/default.go)
- Ensures certificate/key directories exist
- Creates empty files if they don't exist (mode 0o600 for keys, 0o755 for dirs)

### HTTP TLS Configuration: internal/api/config/types.go

```go
type TLSConfig struct {
    Enabled           bool   // Enable TLS on HTTP server
    CertPath          string // Path to server certificate
    KeyPath           string // Path to server private key
    ClientCAPath      string // Path to client CA for mutual TLS
    RequireClientCert bool   // Enforce mutual TLS
}
```

## Store/Database Integration: internal/store/store.go

### Store Interface
```go
type Store interface {
    Querier  // sqlc-generated query methods
    Close()
}
```

### Implementation: PgStore
- Wraps `pgxpool.Pool` from `jackc/pgx/v5`
- Created by `NewStore(ctx, dsn)` with connection validation
- Resources released by `Close()`

### Generated Code (via sqlc)
- **Files**: `cluster.sql.go`, `mods.sql.go`, `nodes.sql.go`, `repos.sql.go`, `runs.sql.go`
- **Location**: `internal/store/` with schema in `queries/` subdirectory
- **Models**: `models.go` with sqlc-generated struct definitions

### Configuration
- DSN passed via environment (`PLOY_SERVER_PG_DSN` preferred)
- Fallback to config file `postgres.dsn`

## Bootstrap & PKI Initialization: cmd/ployd/bootstrap_ca.go

### Bootstrap CA Command
```bash
ployd bootstrap-ca \
  --cluster-id <id> \
  --node-id <id> \
  --address <hostname> \
  --ca <path> \
  --cert <path> \
  --key <path>
```

### Flow
1. Connects to etcd (via `etcdutil.ConfigFromEnv()`)
2. Ensures cluster PKI exists via `deploy.EnsureClusterPKI()`
3. Creates CA rotation manager: `deploy.NewCARotationManager()`
4. Issues control-plane certificate: `manager.IssueControlPlaneCertificate()`
5. Retrieves current CA state: `manager.State()`
6. Writes PEM files to disk (CA bundle, node cert, node key)

### Default Paths
- CA: `/etc/ploy/pki/control-plane-ca.pem`
- Cert: `/etc/ploy/pki/node.pem`
- Key: `/etc/ploy/pki/node-key.pem`

## Configuration: internal/api/config/types.go

### Primary Config Sections

**HTTPConfig**
- `listen` — Bind address (default `:8443`)
- `tls.*` — TLS settings (enabled, cert, key, client_ca, require_client_cert)
- `read_timeout`, `write_timeout`, `idle_timeout`

**PKIConfig**
- `bundle_dir` — Directory for cert bundle (default `/etc/ploy/pki`)
- `certificate` — Path to node certificate
- `key` — Path to node private key
- `renew_before` — Renewal check interval (default 1 hour)
- `ca_endpoint` — CA service endpoint

**ControlPlaneConfig**
- `endpoint` — Control-plane URL (HTTPS required)
- `node_id` — Unique node identifier
- `ca` — Path to control-plane CA bundle
- `certificate` — Path to client certificate
- `key` — Path to client private key
- `heartbeat_interval`, `assignment_poll_interval`, `status_publish_interval`
- Various endpoint overrides for health, config, assignments, etc.

**PostgresConfig**
- `dsn` — PostgreSQL connection string

**Other Sections**
- `MetricsConfig` — Prometheus listen address
- `AdminConfig` — Admin socket/listen
- `BootstrapConfig` — Bootstrap mode settings
- `WorkerConfig` — Worker execution settings
- `RuntimeConfig` — Runtime plugins
- `TransfersConfig` — Transfer guard settings
- `LoggingConfig` — Logging configuration

## Security Patterns

### Mutual TLS (mTLS)
1. Node → Control-Plane: Client cert in `ControlPlaneConfig`
2. Control-Plane ← Node: Server cert in `HTTPConfig.TLS`
3. Both require CA bundle for verification

### Bearer Token Authentication
- Managed by `httpserver/security/security.go`
- Token verified via `TokenVerifier` interface
- Principal stored in context via `WithPrincipal()`

### Scope-Based Access Control
- Defined in `httpserver/security/security.go`: `ScopeAdmin`, `ScopeMods`, `ScopeJobs`, etc.
- Enforced per route in `controlplane_server.go::registerRoute()`

### Role-Based Access Control
- Defined in `controlplane/auth/` (not shown in detail)
- Roles: `RoleControlPlane`, `RoleCLIAdmin`, `RoleWorker`
- Optional layer on top of bearer auth

## Key Integration Points

1. **Daemon Startup** → `cmd/ployd/main.go` → `daemon.NewDefault(cfg)`
2. **Config Loading** → `internal/api/config/loader.go`
3. **HTTP Server** → `internal/api/httpserver/server.go` (Fiber)
4. **Control-Plane Handler** → `internal/api/httpserver/controlplane_server.go` (http.ServeMux)
5. **PKI Management** → `internal/api/pki/manager.go` + `cmd/ployd/bootstrap_ca.go`
6. **Database** → `internal/store/store.go` (pgx + sqlc)
7. **Security** → `internal/api/httpserver/security/security.go`
8. **Authorization** → `internal/controlplane/auth/` (roles)
9. **Node Coordination** → etcd via `internal/api/daemon/default.go::buildControlPlaneHTTP()`

## Files Summary

| File | Purpose |
|------|---------|
| `cmd/ployd/main.go` | Entry point, command routing |
| `cmd/ployd/bootstrap_ca.go` | PKI bootstrap utility |
| `internal/api/daemon/default.go` | Component wiring and initialization |
| `internal/api/daemon/daemon.go` | Daemon interface and lifecycle |
| `internal/api/httpserver/server.go` | Fiber HTTP server |
| `internal/api/httpserver/controlplane_server.go` | Control-plane route registration |
| `internal/api/httpserver/security/security.go` | Bearer token + mutual TLS middleware |
| `internal/api/pki/manager.go` | Certificate renewal orchestration |
| `internal/api/config/types.go` | Configuration struct definitions |
| `internal/api/config/loader.go` | YAML config loading |
| `internal/api/config/defaults.go` | Default configuration values |
| `internal/store/store.go` | PostgreSQL store interface & implementation |
| `internal/store/models.go` | sqlc-generated database models |

## Key Architectural Patterns

1. **Dependency Injection**: `NewDefault()` constructs and wires all components
2. **Interface-Based Design**: Store, StatusProvider, TokenVerifier, Rotator all use interfaces
3. **Context-Based Configuration**: Uses `context.Context` for graceful shutdown
4. **Hot Reload**: `Reload()` methods allow config reloading without server restart
5. **Middleware Pattern**: Security enforcement via HTTP middleware wrapping
6. **Event-Driven Logging**: Log streaming via SSE (`text/event-stream`)
7. **Prometheus Metrics**: Built-in observability with standard prometheus client
