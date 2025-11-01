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
- Security patterns (mTLS, roles)
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
| **HTTP routes** | See control-plane packages under `internal/api/*` and `internal/controlplane/*` |
| **API endpoints** | `docs/api/OpenAPI.yaml` (authoritative routes) |
| **Security** | mTLS-only; see `internal/controlplane/auth` and `internal/api/pki` |
| **PKI management** | `internal/api/pki/manager.go` + `cmd/ployd/bootstrap_ca.go` |
| **Database** | `internal/store/store.go` (pgx + sqlc) |
| **Configuration** | `internal/api/config/types.go` + `loader.go` + `defaults.go` |

### Understanding Flows

| Scenario | Key Files |
|----------|-----------|
| **Server startup** | `cmd/ployd/main.go` → `daemon.NewDefault()` in `internal/api/daemon/default.go` |
| **HTTP request handling** | Control-plane handlers under `internal/controlplane/*` |
| **PKI bootstrap** | `cmd/ployd/bootstrap_ca.go` → `internal/deploy/*` → file I/O + PostgreSQL |
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

### 3. HTTP Security
- **Transport layer**: Mutual TLS with X509 certificates (mTLS-only)
  - Ensures encryption and client identity via certificates
  - Roles (RoleControlPlane, RoleCLIAdmin, RoleWorker) applied inside handlers

### 4. HTTP Using net/http
- **http.ServeMux**: Thin server wrapper for route mounting
- Future slices will add control-plane and metrics endpoints

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
- [x] **Role-Based Access Control**: Coarse-grained role checks within handlers
- [x] **Principal Context**: Authenticated caller available in handlers for audit logging
- [x] **Certificate Renewal**: PKI manager handles certificate lifetimes
- [x] **Bootstrap PKI**: One-time CA setup utility

## Configuration Resolution Order

1. Load YAML configuration file (default: `/etc/ploy/ployd.yaml`)
2. Apply built-in defaults from `internal/api/config/defaults.go`
3. Override with environment variables (see `docs/envs/README.md`):
   - `PLOY_SERVER_PG_DSN` (preferred) / `PLOY_POSTGRES_DSN`
   - `PLOY_CLUSTER_ID`
   - `PLOY_LIFECYCLE_NET_IGNORE`

## Common Questions

**Q: Where do I add a new API endpoint?**
A: Implement a handler under `internal/controlplane/` and register it per the package’s pattern. Keep `docs/api/OpenAPI.yaml` in sync.

**Q: How does authentication work?**
A: Client establishes a mutual TLS connection. The server authenticates the caller from the client certificate; no bearer tokens are used.

**Q: How is configuration reloaded?**
A: Send SIGHUP to the daemon. `main.go` intercepts it, reloads the config file, and calls `svc.Reload()`.

**Q: Where are the database models?**
A: Generated by sqlc in `internal/store/models.go`. SQL definitions in `internal/store/queries/`.

**Q: How does PKI work?**
A: Bootstrap command (`ployd bootstrap-ca`) generates the cluster CA and server certificate. Nodes submit CSRs to the control plane (`/v1/pki/sign`) to receive signed certificates. The runtime PKI manager monitors certificate lifetimes.

**Q: Can I run without PostgreSQL?**
A: No — the control plane requires PostgreSQL for state. For tests, use `PLOY_TEST_PG_DSN` or skip DB-backed tests when unset.

**Q: Can I run without TLS on the HTTP server?**
A: No — all control-plane traffic uses mutual TLS. Development flows rely on a generated cluster CA.

## Files to Review First

For a quick understanding, read in this order:
1. **cmd/ployd/main.go** (3-5 min) — Understand the entry point
2. **internal/api/daemon/default.go** (10 min) — See how all components wire together
3. **internal/controlplane/** (10 min) — Review handler organization and authorization
4. **docs/api/OpenAPI.yaml** (5 min) — Confirm routes and schemas
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
