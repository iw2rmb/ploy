# Ploy Codebase Exploration Index

## Overview
This project contains a comprehensive exploration of the Ploy codebase structure, architecture, and implementation patterns. All exploration was conducted on November 1, 2025.

## Documents Generated

### EXPLORATION_README.md (Start Here)
- **Size**: 8.0 KB
- **Purpose**: Navigation guide and overview
- **Key Sections**:
  - Document descriptions
  - Quick navigation tables
  - Key architectural insights
  - Configuration resolution order
  - Common questions (FAQ)
  - Recommended reading order

**Read this first** to understand what's available and how to use these documents.

### CODEBASE_EXPLORATION.md (Reference)
- **Size**: 16 KB
- **Purpose**: Comprehensive technical reference
- **Key Sections**:
  - Complete directory structure
  - Main entry point (cmd/ployd/main.go)
  - Daemon initialization (internal/api/daemon/default.go)
  - HTTP server implementation
  - Control-plane API handler
  - PKI & TLS configuration
  - Store/database integration
  - Bootstrap procedures
  - Configuration structures
  - Security patterns
  - Integration points
  - File summary table

**Use this for** detailed understanding of code locations and functionality.

### ARCHITECTURE_DIAGRAM.md (Visual)
- **Size**: 36 KB
- **Purpose**: Visual representations and data flows
- **Key Diagrams**:
  - High-level component diagram
  - Security architecture (mTLS + bearer token)
  - Configuration flow
  - Database integration (PostgreSQL + sqlc)
  - PKI management lifecycle
  - Request flow example with all security layers

**Use this for** understanding data flow and security enforcement.

## Quick Facts About Ploy

### What Is It?
Ploy is a workstation-first orchestration stack for code-mod (Mods) workflows. It consists of:
- `ploy` — CLI for submitting Mods, managing artifacts, administering nodes
- `ployd` — daemon running HTTPS control-plane APIs and worker execution

### Core Technology Stack
- **Language**: Go 1.25+
- **HTTP Server**: Fiber v2 (for node endpoints) + http.ServeMux (for control-plane)
- **Database**: PostgreSQL with pgx/v5 + sqlc
- **Security**: Mutual TLS + Bearer token authentication
- **Message Queue**: etcd for cluster coordination
- **Metrics**: Prometheus

### Key Components
1. **HTTP Server** (Fiber + standard library)
2. **PKI Manager** (certificate renewal)
3. **PostgreSQL Store** (pgx + sqlc)
4. **Control-Plane Client** (mTLS to control-plane)
5. **Lifecycle Collector** (node health)
6. **Task Scheduler** (background jobs)
7. **Workflow Runtime** (execution engine)

### Entry Points
- **cmd/ployd/main.go** — Main daemon executable
  - `bootstrap-ca` — Initial PKI setup
  - `slot-guard` — Transfer slot management
  - `status-snapshot` — Health snapshot
  - default (daemon mode) — Full server startup

### Configuration
- **Config File**: `/etc/ploy/ployd.yaml`
- **Format**: YAML
- **Overrides**: Environment variables (PLOY_*_*)
- **Reload**: SIGHUP signal

### API Endpoints

**Node-Local** (no auth):
- `/v1/node/status` — Health status
- `/v1/node/jobs/*` — Job management
- `/v1/admin/nodes` — Node registration

**Control-Plane** (mTLS + bearer token + scopes):
- `/v1/jobs/*` — Job submission, claiming
- `/v1/nodes/*` — Node management
- `/v1/config/*` — Configuration management
- `/v1/security/*` — PKI/certificates
- `/v1/mods/*` — Mods orchestration
- `/v1/artifacts/*` — Artifact management
- `/v1/transfers/*` — File transfers
- `/metrics` — Prometheus metrics

### Security Features
- Mutual TLS (X509 client certificates)
- Bearer token authentication
- Scope-based access control (admin, mods, jobs, artifact.read/write, registry.pull/push)
- Role-based access control (ControlPlane, CLIAdmin, Worker)
- Principal context for audit logging
- Automatic certificate renewal (default: 1 hour checks)
- TLS 1.2+ enforcement

## File Structure

```
/Users/vk/@iw2rmb/ploy/
├── EXPLORATION_INDEX.md (THIS FILE)
├── EXPLORATION_README.md (Navigation Guide)
├── CODEBASE_EXPLORATION.md (Technical Reference)
├── ARCHITECTURE_DIAGRAM.md (Visual Diagrams)
│
├── cmd/
│   ├── ployd/
│   │   ├── main.go ⭐ ENTRY POINT
│   │   ├── bootstrap_ca.go (PKI Setup)
│   │   └── ... (other ployd commands)
│   └── ploy/ (CLI)
│
├── internal/
│   ├── api/
│   │   ├── daemon/
│   │   │   ├── default.go ⭐ COMPONENT WIRING
│   │   │   └── daemon.go
│   │   ├── httpserver/
│   │   │   ├── server.go ⭐ HTTP SERVER
│   │   │   ├── controlplane_server.go ⭐ ROUTES & SECURITY
│   │   │   └── security/
│   │   │       ├── security.go ⭐ AUTH MIDDLEWARE
│   │   │       └── ...
│   │   ├── config/
│   │   │   ├── types.go ⭐ CONFIG STRUCTS
│   │   │   ├── loader.go
│   │   │   └── defaults.go
│   │   ├── pki/
│   │   │   └── manager.go ⭐ PKI RENEWAL
│   │   ├── admin/
│   │   ├── controlplane/
│   │   ├── metrics/
│   │   ├── scheduler/
│   │   └── ...
│   ├── store/
│   │   ├── store.go ⭐ DATABASE INTERFACE
│   │   ├── models.go (sqlc-generated)
│   │   ├── *.sql.go (sqlc-generated)
│   │   └── queries/ (SQL definitions)
│   ├── deploy/ (PKI bootstrap utilities)
│   ├── controlplane/ (Orchestration)
│   ├── node/ (Worker components)
│   ├── workflow/ (Execution)
│   └── ...
│
├── pkg/
│   └── (empty — SSH tunnel utilities removed; CLI uses HTTPS)
│
└── docs/
    ├── next/ (Architectural docs)
    ├── api/ (OpenAPI specification)
    └── ...
```

Legend: ⭐ = Key files to understand first

## Recommended Learning Path

### Phase 1: Understand the Entry Point (5 minutes)
1. Read **EXPLORATION_README.md** — Get oriented
2. Read **cmd/ployd/main.go** — See command routing and startup

### Phase 2: Component Wiring (15 minutes)
3. Read **internal/api/daemon/default.go** — See all components initialize
4. Skim **CODEBASE_EXPLORATION.md** "Daemon Initialization" section

### Phase 3: HTTP & Security (15 minutes)
5. Read **internal/api/httpserver/server.go** — Understand Fiber setup
6. Skim **internal/api/httpserver/controlplane_server.go** (first 250 lines)
7. Read **internal/api/httpserver/security/security.go** — See auth middleware

### Phase 4: Visual Understanding (10 minutes)
8. Review **ARCHITECTURE_DIAGRAM.md** diagrams
9. Focus on "Security Architecture" and "Request Flow Example"

### Phase 5: Deep Dives (as needed)
- **Database**: Read `internal/store/store.go` + `CODEBASE_EXPLORATION.md` Store section
- **PKI**: Read `cmd/ployd/bootstrap_ca.go` + `internal/api/pki/manager.go`
- **Configuration**: Read `internal/api/config/types.go`

Total time for Phases 1-4: **45 minutes**

## Key Insights

### Architectural Patterns
1. **Dependency Injection**: All components wired in `daemon.NewDefault()`
2. **Interface-Based**: Store, StatusProvider, TokenVerifier, Rotator use interfaces
3. **Middleware Pattern**: Security via HTTP middleware composition
4. **Context-Based**: Graceful shutdown via context cancellation
5. **Hot Reload**: Configuration reload without full restart
6. **Event-Driven Logging**: Server-sent events for real-time logs

### Security Model
- **Layer 1 (Transport)**: Mutual TLS with X509 certificates
- **Layer 2 (Authentication)**: Bearer token verification
- **Layer 3 (Authorization)**: Scope-based per-endpoint authorization
- **Layer 4 (Access Control)**: Role-based coarse-grained checks

### Data Flow
1. Client establishes TLS connection with client certificate
2. Client sends HTTP request with `Authorization: Bearer <token>` header
3. Server validates certificate, extracts and verifies token
4. Server checks scopes and roles
5. Handler receives authenticated context
6. Handler can access `Principal` for audit logging

### Configuration Resolution
1. YAML file loads defaults
2. Built-in defaults fill gaps
3. Environment variables override both
4. Reload-safe separation of concerns

## File Statistics

| Document | Size | Lines | Focus |
|----------|------|-------|-------|
| EXPLORATION_README.md | 8.0 KB | ~250 | Navigation |
| CODEBASE_EXPLORATION.md | 16 KB | ~600 | Reference |
| ARCHITECTURE_DIAGRAM.md | 36 KB | ~1000 | Visuals |
| **Total** | **60 KB** | **~1850** | Complete |

## Maintenance Notes

These documents are living references. They should be updated when:
- New major components are added
- Initialization order changes
- Security mechanisms are modified
- Configuration schema evolves
- API endpoints are added/removed
- Integration patterns change

## Questions?

Check **EXPLORATION_README.md** FAQ section for answers to common questions like:
- Where do I add a new API endpoint?
- How does authentication work?
- Can I run without PostgreSQL?
- How is PKI managed?

---

**Generation Date**: November 1, 2025
**Target Audience**: Developers implementing PKI/TLS features, API enhancements, or architectural changes
**Status**: Complete and ready for use

Start with **EXPLORATION_README.md** →
