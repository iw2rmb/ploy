# Ploy Architecture Diagram

## High-Level Component Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                       cmd/ployd/main.go                         │
│                   Entry Point & Signal Handling                  │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ daemon.NewDefault(cfg)                                  │   │
│  │ [Initializes ALL components - see below]                │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                   │
│  Signal Handlers: SIGINT, SIGTERM, SIGHUP (reload)             │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│           internal/api/daemon/default.go - Component Wiring     │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 1. Postgres Store (pgx + sqlc)                           │  │
│  │    Env: PLOY_SERVER_PG_DSN / PLOY_POSTGRES_DSN          │  │
│  │    → store.NewStore(ctx, dsn)                            │  │
│  │    → PgStore wraps pgxpool.Pool                          │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 2. Log Streams Hub                                       │  │
│  │    → Real-time job log streaming                         │  │
│  │    → Used by /v1/node/jobs/{id}/logs/stream              │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 3. Workflow Runtime Registry                             │  │
│  │    → Load runtime plugins                                │  │
│  │    → Default factories for step execution                │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 4. Status Provider + Lifecycle Collector                 │  │
│  │    → Node health: Docker, IPFS, Java build gate          │  │
│  │    → Publishes to etcd (if available)                    │  │
│  │    → Falls back to local cache                           │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 5. Control-Plane HTTP Client (Mutual TLS)                │  │
│  │    → Endpoint: ControlPlaneConfig.Endpoint (HTTPS)       │  │
│  │    → Client Cert/Key: ControlPlaneConfig.*               │  │
│  │    → CA Bundle: ControlPlaneConfig.CAPath                │  │
│  │    → Min TLS: TLS 1.2                                    │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 6. Control-Plane Handler (HTTP Handler Tree)             │  │
│  │    → Etcd client for cluster coordination                │  │
│  │    → Scheduler, Mods Service, Artifact Store             │  │
│  │    → Authorization + Role-Based Access Control           │  │
│  │    → Routes: /v1/*, /v2/*, /metrics                      │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 7. PKI Manager (Certificate Renewal)                     │  │
│  │    → Periodic renewal check (default: 1 hour)            │  │
│  │    → fileRotator ensures cert directories exist          │  │
│  │    → Interval: cfg.PKI.RenewBefore                       │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 8. HTTP Server (Fiber v2)                                │  │
│  │    → Node-local endpoints (/v1/node/*)                   │  │
│  │    → Admin endpoints (/v1/admin/*)                       │  │
│  │    → Control-plane routes (delegated handler)            │  │
│  │    → TLS: cfg.HTTP.TLS.Enabled, CertPath, KeyPath        │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 9. Control-Plane Client                                  │  │
│  │    → Heartbeat to control-plane                          │  │
│  │    → Assignment polling                                  │  │
│  │    → Job execution                                       │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 10. Task Scheduler                                       │  │
│  │     → Background task execution                          │  │
│  │     → Lifecycle publishing                               │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                   internal/api/httpserver/                       │
│                    HTTP Server & Routes                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  Fiber Application (fiber.App)                                  │
│  ├─ TLS Listener (crypto/tls or plain TCP)                     │
│  ├─ Graceful Shutdown (5 second timeout)                       │
│  └─ Hot Reload Support (Reload method)                         │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Node-Local Routes (No Auth Required)                    │   │
│  ├─────────────────────────────────────────────────────────┤   │
│  │ GET  /v1/node/status                                    │   │
│  │ GET  /v1/node/health                                    │   │
│  │ GET  /v1/node/jobs                                      │   │
│  │ GET  /v1/node/jobs/{id}                                 │   │
│  │ GET  /v1/node/jobs/{id}/logs/stream (SSE)              │   │
│  │ GET  /v1/node/jobs/{id}/logs/snapshot                  │   │
│  │ POST /v1/node/jobs/{id}/logs/entries                   │   │
│  │ POST /v1/node/jobs/{id}/cancel                         │   │
│  │ POST /v1/admin/nodes (RegisterNode)                    │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Control-Plane Routes (Mutual TLS + Bearer Token)        │   │
│  │ [Delegated to controlPlaneHandler]                      │   │
│  ├─────────────────────────────────────────────────────────┤   │
│  │ All /v1/* and /v2/* routes                              │   │
│  │ /metrics (Prometheus)                                   │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│              internal/api/httpserver/controlplane_server.go      │
│                  Control-Plane HTTP Handler                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  http.ServeMux with Security Middleware Stack                   │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ Layer 1: Mutual TLS Enforcement                        │    │
│  │ (security.Manager checks r.TLS.PeerCertificates)       │    │
│  └────────────────────────────────────────────────────────┘    │
│                      ↓                                           │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ Layer 2: Bearer Token Authentication                  │    │
│  │ (TokenVerifier.Verify() → Principal with scopes)       │    │
│  └────────────────────────────────────────────────────────┘    │
│                      ↓                                           │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ Layer 3: Scope-Based Authorization                    │    │
│  │ Scopes: admin, mods, jobs, artifact.read/write, etc.   │    │
│  └────────────────────────────────────────────────────────┘    │
│                      ↓                                           │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ Layer 4: Role-Based Access Control (Optional)         │    │
│  │ Roles: RoleControlPlane, RoleCLIAdmin, RoleWorker      │    │
│  └────────────────────────────────────────────────────────┘    │
│                      ↓                                           │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ Registered Routes (registerRoute Method)               │    │
│  ├────────────────────────────────────────────────────────┤    │
│  │ Jobs:        /v1/jobs, /v1/jobs/claim, /v1/jobs/{id}*  │    │
│  │ Nodes:       /v1/nodes, /v1/nodes/{id}*                │    │
│  │ Config:      /v1/config, /v1/config/gitlab             │    │
│  │ Security:    /v1/security/ca                           │    │
│  │              /v1/security/certificates/control-plane   │    │
│  │ Mods:        /v1/mods, /v1/mods/{id}*, /v1/mods/...*  │    │
│  │ Artifacts:   /v1/artifacts, /v1/artifacts/*, /v2/...*  │    │
│  │ Transfers:   /v1/transfers/upload, /download, /{id}*   │    │
│  │ Misc:        /v1/health, /v1/version, /v1/status       │    │
│  │              /v1/gitlab/signer/*, /metrics             │    │
│  └────────────────────────────────────────────────────────┘    │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

## Security Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│               Mutual TLS + Bearer Token Authentication            │
├──────────────────────────────────────────────────────────────────┤
│                                                                    │
│  Node → Control-Plane Request                                    │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 1. Establishes TCP connection                              │ │
│  │ 2. Performs TLS handshake with client certificate          │ │
│  │    - Client Cert: cfg.ControlPlane.Certificate             │ │
│  │    - Client Key:  cfg.ControlPlane.Key                     │ │
│  │    - Server CA:   cfg.ControlPlane.CAPath                  │ │
│  │ 3. Adds bearer token header:                               │ │
│  │    Authorization: Bearer <token>                           │ │
│  │ 4. Send HTTP request                                       │ │
│  └────────────────────────────────────────────────────────────┘ │
│                          ↓                                        │
│  Control-Plane Receives Request                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ security.Manager.Middleware enforces:                      │ │
│  │                                                             │ │
│  │ 1. Check Mutual TLS: r.TLS.PeerCertificates must exist     │ │
│  │    → Deny with HTTP 400 Bad Request if missing             │ │
│  │                                                             │ │
│  │ 2. Extract Bearer Token: Authorization header              │ │
│  │    → Deny with HTTP 401 Unauthorized if missing            │ │
│  │                                                             │ │
│  │ 3. Verify Token: TokenVerifier.Verify()                    │ │
│  │    → Returns Principal{scopes, expiry, ...}                │ │
│  │    → Deny with HTTP 401 if token invalid                   │ │
│  │                                                             │ │
│  │ 4. Check Scopes: principal.HasScope(required)              │ │
│  │    → Deny with HTTP 403 Forbidden if scope missing         │ │
│  │                                                             │ │
│  │ 5. Apply Role-Based Access (if Authorizer present)         │ │
│  │    → Additional role checks per route                      │ │
│  │                                                             │ │
│  │ 6. Store Principal in Context: WithPrincipal()             │ │
│  │    → Available in handler via PrincipalFromContext()       │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                    │
└──────────────────────────────────────────────────────────────────┘
```

## Configuration Flow

```
┌──────────────────────────────────────────────────────────────────┐
│                   Configuration Resolution                        │
├──────────────────────────────────────────────────────────────────┤
│                                                                    │
│  cmd/ployd/main.go                                               │
│  ├─ configPath flag: -config /etc/ploy/ployd.yaml                │
│  └─ Load: config.Load(configPath)                                │
│       └─ YAML parsing into config.Config struct                  │
│           └─ Contains: HTTP, PKI, ControlPlane, Postgres, etc.   │
│                                                                    │
│  internal/api/config/loader.go                                   │
│  └─ Load function: reads YAML → config.Config                    │
│                                                                    │
│  internal/api/config/types.go                                    │
│  └─ Defines all configuration structures:                        │
│     ├─ HTTPConfig (listen, TLS settings, timeouts)               │
│     ├─ TLSConfig (enabled, cert, key, client_ca, ...)            │
│     ├─ PKIConfig (bundle_dir, certificate, key, renew_before)    │
│     ├─ ControlPlaneConfig (endpoint, node_id, ca, cert, key)     │
│     ├─ PostgresConfig (dsn)                                      │
│     ├─ RuntimeConfig (plugins, feature flags)                    │
│     └─ ... (other configs)                                       │
│                                                                    │
│  internal/api/config/defaults.go                                 │
│  └─ ApplyDefaults: fills in missing values                       │
│     ├─ HTTP.Listen: ":8443"                                      │
│     ├─ PKI.BundleDir: "/etc/ploy/pki"                            │
│     ├─ PKI.RenewBefore: 1 hour                                   │
│     └─ ... (other defaults)                                      │
│                                                                    │
│  Environment Variable Overrides:                                 │
│  ├─ PLOY_SERVER_PG_DSN (preferred) / PLOY_POSTGRES_DSN          │
│  ├─ PLOY_IPFS_CLUSTER_API, _TOKEN, _USERNAME, _PASSWORD         │
│  ├─ PLOY_GITLAB_SIGNER_AES_KEY                                   │
│  ├─ PLOY_CLUSTER_ID                                              │
│  └─ PLOY_LIFECYCLE_NET_IGNORE                                    │
│                                                                    │
└──────────────────────────────────────────────────────────────────┘
```

## Database Integration (PostgreSQL + sqlc)

```
┌──────────────────────────────────────────────────────────────────┐
│              PostgreSQL Store (pgx + sqlc)                        │
├──────────────────────────────────────────────────────────────────┤
│                                                                    │
│  Configuration Resolution:                                       │
│  1. Check env: PLOY_SERVER_PG_DSN (preferred)                   │
│  2. Fallback:  PLOY_POSTGRES_DSN                                │
│  3. Config:    cfg.Postgres.DSN from YAML                        │
│                                                                    │
│  Connection Pool (pgxpool):                                      │
│  internal/store/store.go                                         │
│  └─ NewStore(ctx, dsn) → PgStore                                │
│     ├─ pgxpool.ParseConfig(dsn)                                  │
│     ├─ pgxpool.NewWithConfig(ctx, config)                        │
│     ├─ pool.Ping(ctx) — verify connectivity                      │
│     └─ Return PgStore wrapper                                    │
│                                                                    │
│  Store Interface:                                                │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Store interface {                                           │ │
│  │   Querier  // sqlc-generated methods                       │ │
│  │   Close()                                                  │ │
│  │ }                                                           │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                    │
│  sqlc-Generated Queries:                                         │
│  internal/store/                                                 │
│  ├─ cluster.sql.go   (cluster operations)                        │
│  ├─ mods.sql.go      (mods/module operations)                    │
│  ├─ nodes.sql.go     (node operations)                           │
│  ├─ repos.sql.go     (repository operations)                     │
│  ├─ runs.sql.go      (run operations)                            │
│  ├─ models.go        (database model structs)                    │
│  └─ querier.go       (Querier interface definition)              │
│                                                                    │
│  Schema Definitions:                                             │
│  internal/store/queries/                                         │
│  └─ SQL files with schema and query definitions                 │
│                                                                    │
│  Migrations:                                                     │
│  internal/store/migrations/                                      │
│  └─ Version-controlled schema changes                            │
│                                                                    │
└──────────────────────────────────────────────────────────────────┘
```

## PKI Management Flow

```
┌──────────────────────────────────────────────────────────────────┐
│           Certificate Lifecycle & Bootstrap Flow                  │
├──────────────────────────────────────────────────────────────────┤
│                                                                    │
│  Bootstrap (One-Time Setup):                                     │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ cmd/ployd/bootstrap_ca.go                                  │ │
│  │                                                             │ │
│  │ $ ployd bootstrap-ca \                                     │ │
│  │   --cluster-id myid \                                      │ │
│  │   --node-id control \                                      │ │
│  │   --address hostname                                       │ │
│  │                                                             │ │
│  │ 1. Connect to etcd (ConfigFromEnv)                        │ │
│  │ 2. deploy.EnsureClusterPKI(ctx, client, cluster_id)      │ │
│  │    → Initialize root CA if not exists                      │ │
│  │ 3. NewCARotationManager(client, cluster_id)              │ │
│  │    → Create rotation manager                              │ │
│  │ 4. IssueControlPlaneCertificate(ctx, node_id, address)   │ │
│  │    → Generate node cert + key                             │ │
│  │ 5. State(ctx) → Get current CA bundle                     │ │
│  │ 6. Write PEM files:                                        │ │
│  │    - /etc/ploy/pki/control-plane-ca.pem                   │ │
│  │    - /etc/ploy/pki/node.pem                               │ │
│  │    - /etc/ploy/pki/node-key.pem                           │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                    │
│  Runtime (Ongoing Renewal):                                      │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ internal/api/pki/manager.go                                │ │
│  │                                                             │ │
│  │ Manager.Start(ctx)                                         │ │
│  │ ├─ Creates background renewal loop                         │ │
│  │ └─ Runs m.loop() in goroutine                              │ │
│  │                                                             │ │
│  │ m.loop():                                                   │ │
│  │ ├─ Get current config: cfg.PKI.RenewBefore                 │ │
│  │ ├─ Call rotator.Renew(ctx, cfg)                            │ │
│  │ │   └─ fileRotator ensures cert dirs exist                │ │
│  │ ├─ Wait for next interval (default: 1 hour)               │ │
│  │ └─ Repeat                                                  │ │
│  │                                                             │ │
│  │ Manager.Stop(ctx)                                          │ │
│  │ └─ Signal cancellation, wait for loop to finish            │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                    │
│  TLS Server Configuration:                                       │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ internal/api/httpserver/server.go::listenLocked()          │ │
│  │                                                             │ │
│  │ if cfg.HTTP.TLS.Enabled:                                   │ │
│  │   ├─ Load cert: tls.LoadX509KeyPair(certPath, keyPath)    │ │
│  │   ├─ Create TLS listener: tls.Listen("tcp", address, ...) │ │
│  │   └─ cfg.HTTP.TLS.CertPath and KeyPath                    │ │
│  │ else:                                                       │ │
│  │   └─ Plain TCP listener                                    │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                    │
└──────────────────────────────────────────────────────────────────┘
```

## Request Flow Example: Job Submission

```
Client → Control-Plane: Submit Job
│
├─ TLS Handshake (Client Cert required)
│  └─ Client: cfg.ControlPlane.Certificate, Key
│  └─ Client verifies server with: cfg.ControlPlane.CAPath
│
├─ HTTP Request
│  POST /v1/jobs
│  Authorization: Bearer <token>
│
└─ Server Processing (controlplane_server.go::registerRoute)
   │
   ├─ Layer 1: Mutual TLS Check
   │  └─ security.Manager checks r.TLS.PeerCertificates
   │  └─ Fail → HTTP 400 Bad Request
   │
   ├─ Layer 2: Bearer Token Verification
   │  └─ Extract: Authorization header
   │  └─ Verify: TokenVerifier.Verify(ctx, token)
   │  └─ Fail → HTTP 401 Unauthorized
   │
   ├─ Layer 3: Scope Check
   │  └─ Route registered with scopes (e.g., "jobs")
   │  └─ Check: principal.HasScope("jobs")
   │  └─ Fail → HTTP 403 Forbidden
   │
   ├─ Layer 4: Role Check (optional)
   │  └─ Route routes: RoleControlPlane, RoleCLIAdmin
   │  └─ Fail → HTTP 403 Forbidden
   │
   ├─ Store Principal in Context
   │  └─ ctx = WithPrincipal(ctx, principal)
   │
   └─ Invoke Handler
      └─ h.handleJobs(w, r) with authenticated context
```

This architecture ensures:
1. **Confidentiality**: Mutual TLS encrypts all traffic
2. **Authentication**: Client cert identifies node, bearer token identifies caller
3. **Authorization**: Scopes + roles ensure fine-grained access control
4. **Auditability**: Principal available in all handlers for logging
