# Ploy Codebase Exploration

This directory contains comprehensive documentation of the Ploy codebase structure, architecture, and integration patterns.

## Documents in This Directory

### 1. CODEBASE_EXPLORATION.md
**Comprehensive reference guide** covering:
- Directory structure and component organization
- Main entry points and startup flow
- Daemon initialization and component wiring
- HTTP server implementation and routing
- Control-plane API handler with security layers
- PKI and TLS configuration
- Store/database integration (PostgreSQL + sqlc)
- Bootstrap and PKI initialization
- Configuration types and resolution
- Security patterns (mTLS, bearer tokens, scopes, roles)
- Key integration points
- File summary table

**Use this document to:**
- Understand the overall codebase organization
- Find where specific features are implemented
- Learn how components interact
- Reference configuration options
- Study security patterns

### 2. ARCHITECTURE_DIAGRAM.md
**Visual representations** including:
- High-level component diagram showing initialization flow
- Security architecture with TLS and token enforcement
- Configuration flow from YAML to runtime
- Database integration with PostgreSQL + sqlc
- PKI management and certificate lifecycle
- Request flow example with security layers

**Use this document to:**
- Visualize how components connect
- Understand data flow through the system
- Learn security enforcement mechanisms
- See the PKI bootstrap and renewal process
- Follow an example request through all layers

## Quick Navigation

### Finding Code

| What | Where |
|------|-------|
| **Server entry point** | `cmd/ployd/main.go` |
| **Component wiring** | `internal/api/daemon/default.go` |
| **HTTP routes** | `internal/api/httpserver/server.go` (Fiber) + `internal/api/httpserver/controlplane_server.go` (control-plane) |
| **API endpoints** | `internal/api/httpserver/controlplane_server.go` (lines 174-206 show route registration) |
| **Security** | `internal/api/httpserver/security/security.go` (mutual TLS + bearer token) |
| **PKI management** | `internal/api/pki/manager.go` + `cmd/ployd/bootstrap_ca.go` |
| **Database** | `internal/store/store.go` (pgx + sqlc) |
| **Configuration** | `internal/api/config/types.go` + `loader.go` + `defaults.go` |

### Understanding Flows

| Scenario | Key Files |
|----------|-----------|
| **Server startup** | `cmd/ployd/main.go` → `daemon.NewDefault()` in `internal/api/daemon/default.go` |
| **HTTP request handling** | `httpserver/server.go` (Fiber) → `controlplane_server.go` (routes + security middleware) |
| **PKI bootstrap** | `cmd/ployd/bootstrap_ca.go` → `internal/deploy/*` → etcd + file I/O |
| **Certificate renewal** | `internal/api/pki/manager.go` (periodic loop) → `fileRotator.Renew()` |
| **Database queries** | `internal/store/store.go` (Store interface) → sqlc-generated code in `cluster.sql.go`, etc. |

## Key Architectural Insights

### 1. Single Entry Point with Multiple Modes
- **Default mode**: Full daemon with HTTP server, PKI manager, control-plane client
- **bootstrap-ca**: One-time setup to generate initial certificates
- **slot-guard**: Transfer slot management
- **status-snapshot**: Health snapshot collection

### 2. Dependency Injection Pattern
All components are wired in `daemon.NewDefault()`:
1. Configuration is loaded from YAML
2. Environment variables override config values
3. Components are initialized in order
4. Later components depend on earlier ones
5. Shutdown functions are collected for graceful termination

### 3. Two-Level HTTP Security
- **Transport layer**: Mutual TLS with X509 certificates
  - Ensures node-to-control-plane encryption
  - Client cert identifies the node
- **Application layer**: Bearer token + scopes + roles
  - TokenVerifier validates and extracts principal
  - Scopes provide fine-grained authorization
  - Roles (RoleControlPlane, RoleCLIAdmin, RoleWorker) provide coarse-grained roles

### 4. Fiber + Standard Library
- **Fiber app**: Handles node-local endpoints, admin endpoints
- **http.ServeMux**: Handles control-plane endpoints (for better middleware composition)
- Both mounted in the same server via Fiber's adaptor

### 5. PostgreSQL with sqlc
- Eliminates ORM boilerplate
- Type-safe SQL generation
- Migrations managed separately
- Store interface allows testing and swapping implementations

### 6. Configuration Hot-Reload
- SIGHUP triggers reload
- Configuration is applied without restarting (unless TLS settings change)
- Separate `Reload()` methods on HTTP server, PKI manager

## Security Features Checklist

- [x] **Mutual TLS**: Client must present valid certificate
- [x] **Encrypted Transport**: HTTPS (TLS 1.2+) for all control-plane traffic
- [x] **Bearer Token Auth**: Application-level authentication via Authorization header
- [x] **Token Verification**: TokenVerifier interface allows custom verification logic
- [x] **Scope-Based Access Control**: Fine-grained per-endpoint authorization
- [x] **Role-Based Access Control**: Coarse-grained role checks
- [x] **Principal Context**: Authenticated caller available in handlers for audit logging
- [x] **Certificate Renewal**: PKI manager automatically renews certificates
- [x] **Bootstrap PKI**: One-time CA setup utility

## Configuration Resolution Order

1. Load YAML configuration file (default: `/etc/ploy/ployd.yaml`)
2. Apply built-in defaults from `internal/api/config/defaults.go`
3. Override with environment variables:
   - `PLOY_SERVER_PG_DSN` (preferred) / `PLOY_POSTGRES_DSN`
   - `PLOY_IPFS_CLUSTER_API`, etc.
   - `PLOY_GITLAB_SIGNER_AES_KEY`
   - `PLOY_CLUSTER_ID`
   - `PLOY_LIFECYCLE_NET_IGNORE`

## Common Questions

**Q: Where do I add a new API endpoint?**
A: In `internal/api/httpserver/controlplane_server.go`, add a call to `h.registerRoute()` (after line 206) and implement the handler function.

**Q: How does authentication work?**
A: Client establishes mutual TLS connection, then includes `Authorization: Bearer <token>` header. Server's `security.Manager.Middleware()` validates both.

**Q: How is configuration reloaded?**
A: Send SIGHUP to the daemon. `main.go` intercepts it, reloads the config file, and calls `svc.Reload()`.

**Q: Where are the database models?**
A: Generated by sqlc in `internal/store/models.go`. SQL definitions in `internal/store/queries/`.

**Q: How does PKI work?**
A: Bootstrap command (`ployd bootstrap-ca`) generates initial CA + node certs via etcd-backed CA. Runtime PKI manager periodically calls `fileRotator.Renew()` to ensure files exist.

**Q: Can I run without PostgreSQL?**
A: Yes — the store is optional. If `pgDSN` is empty, `pgStore` stays nil, and control-plane features requiring DB are disabled.

**Q: Can I run without TLS on the HTTP server?**
A: Yes — set `http.tls.enabled: false` in config. But control-plane client still uses mTLS to call the control-plane.

## Files to Review First

For a quick understanding, read in this order:
1. **cmd/ployd/main.go** (3-5 min) — Understand the entry point
2. **internal/api/daemon/default.go** (10 min) — See how all components wire together
3. **internal/api/httpserver/server.go** (5 min) — Understand HTTP server setup
4. **internal/api/httpserver/controlplane_server.go** (first 200 lines, 10 min) — See route registration and security
5. **internal/api/config/types.go** (5 min) — Understand configuration structure

Total time: ~35 minutes for a solid understanding of the core system.

## Additional Resources

- **README.md** — Project overview, build instructions, quick start
- **docs/next/** — Architectural docs, API reference, operational guides
- **docs/api/OpenAPI.yaml** — API specification in OpenAPI format
- **SIMPLE.md** — Proposed simplified architecture notes

## Document Maintenance

These exploration documents are meant to be living references. Update them when:
- Major components are added or refactored
- Entry points or initialization order changes
- New security mechanisms are added
- Configuration schema changes
- Integration patterns evolve

Last updated: November 1, 2025
