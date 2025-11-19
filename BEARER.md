# Bearer Token Authentication Implementation Plan

## Overview

This document outlines the migration from mTLS-only authentication to bearer token authentication with bootstrap tokens for node provisioning.

### Current Architecture
- All authentication via mutual TLS (mTLS)
- Client certificates required for CLI and node agents
- Role extraction from certificate OU/CN fields
- Certificate management via SSH during node provisioning

### Target Architecture
- Bearer token authentication for CLI and nodes
- HTTPS termination at load balancer (ployd accepts plain HTTP)
- Bootstrap token flow for node provisioning:
  1. CLI requests short-lived bootstrap token from ployd
  2. CLI SSHs to VPS, writes bootstrap token securely
  3. ployd-node starts, generates private key and CSR
  4. ployd-node exchanges bootstrap token for signed certificate
  5. ployd-node uses certificate for subsequent requests (or long-lived token)

---

## Component Changes

### 1. Server (ployd)

#### 1.1 Dependencies
**File**: `go.mod`

Add JWT library:
```go
require (
    github.com/golang-jwt/jwt/v5 v5.2.0
)
```

#### 1.2 Token Generation & Validation
**New file**: `internal/server/auth/token.go`

```go
package auth

import (
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "time"
    "github.com/golang-jwt/jwt/v5"
)

// Token types
const (
    TokenTypeAPI       = "api"       // Long-lived API tokens for CLI
    TokenTypeBootstrap = "bootstrap" // Short-lived tokens for node bootstrapping
)

// JWT Claims structure
type TokenClaims struct {
    ClusterID string `json:"cluster_id"`
    Role      string `json:"role"`        // "cli-admin", "control-plane", "worker"
    TokenType string `json:"token_type"`  // "api" or "bootstrap"
    NodeID    string `json:"node_id,omitempty"` // Only for bootstrap tokens
    jwt.RegisteredClaims
}

// GenerateAPIToken creates a long-lived bearer token for CLI usage
func GenerateAPIToken(secret, clusterID, role string, expiresAt time.Time) (string, error) {
    claims := &TokenClaims{
        ClusterID: clusterID,
        Role:      role,
        TokenType: TokenTypeAPI,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(expiresAt),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            ID:        generateTokenID(),
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(secret))
}

// GenerateBootstrapToken creates a short-lived token for node bootstrapping
func GenerateBootstrapToken(secret, clusterID, nodeID string, expiresAt time.Time) (string, error) {
    claims := &TokenClaims{
        ClusterID: clusterID,
        Role:      RoleWorker,
        TokenType: TokenTypeBootstrap,
        NodeID:    nodeID,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(expiresAt),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            ID:        generateTokenID(),
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(secret))
}

// ValidateToken verifies and parses a JWT token
func ValidateToken(tokenString, secret string) (*TokenClaims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return []byte(secret), nil
    })

    if err != nil {
        return nil, fmt.Errorf("parse token: %w", err)
    }

    claims, ok := token.Claims.(*TokenClaims)
    if !ok || !token.Valid {
        return nil, fmt.Errorf("invalid token")
    }

    return claims, nil
}

func generateTokenID() string {
    b := make([]byte, 16)
    rand.Read(b)
    return base64.RawURLEncoding.EncodeToString(b)
}
```

#### 1.3 Database Schema
**New migration file**: `internal/store/migrations/NNNN_bearer_tokens.sql`

```sql
-- API tokens (long-lived, for CLI usage)
CREATE TABLE api_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash VARCHAR(64) NOT NULL UNIQUE,  -- SHA-256 of full token
    token_id VARCHAR(32) NOT NULL,           -- JWT "jti" claim for lookup
    cluster_id VARCHAR(100) NOT NULL,
    role VARCHAR(50) NOT NULL,
    description TEXT,                         -- User-provided description
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_by VARCHAR(255),                  -- Which user created it
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_api_tokens_token_id ON api_tokens(token_id);
CREATE INDEX idx_api_tokens_cluster_id ON api_tokens(cluster_id);

-- Bootstrap tokens (short-lived, for node provisioning)
CREATE TABLE bootstrap_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    token_id VARCHAR(32) NOT NULL,
    node_id UUID NOT NULL,
    cluster_id VARCHAR(100) NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,                      -- NULL until used
    cert_issued_at TIMESTAMPTZ,               -- NULL if cert issuance failed
    revoked_at TIMESTAMPTZ,
    issued_by VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_bootstrap_tokens_token_id ON bootstrap_tokens(token_id);
CREATE INDEX idx_bootstrap_tokens_node_id ON bootstrap_tokens(node_id);
```

#### 1.4 Authorization Middleware Update
**File**: `internal/server/auth/authorizer.go`

Modify `identityFromRequest()` (currently lines 119-140):

```go
func (a *Authorizer) identityFromRequest(r *http.Request) (Identity, error) {
    // Try bearer token authentication first
    if authHeader := r.Header.Get("Authorization"); authHeader != "" {
        if strings.HasPrefix(authHeader, "Bearer ") {
            token := strings.TrimPrefix(authHeader, "Bearer ")
            return a.identityFromBearerToken(token)
        }
    }

    // No valid authentication
    return Identity{}, errors.New("authentication required: provide Bearer token")
}

// New method
func (a *Authorizer) identityFromBearerToken(tokenString string) (Identity, error) {
    claims, err := ValidateToken(tokenString, a.tokenSecret)
    if err != nil {
        return Identity{}, fmt.Errorf("invalid token: %w", err)
    }

    // Verify token is not expired
    if time.Now().After(claims.ExpiresAt.Time) {
        return Identity{}, errors.New("token expired")
    }

    // Check if token is revoked (query database)
    revoked, err := a.isTokenRevoked(claims.ID)
    if err != nil {
        return Identity{}, fmt.Errorf("check token revocation: %w", err)
    }
    if revoked {
        return Identity{}, errors.New("token revoked")
    }

    // Update last_used_at timestamp (async, don't block request)
    go a.updateTokenLastUsed(claims.ID)

    return Identity{
        Role:       claims.Role,
        CommonName: claims.ID,  // Use token ID as identifier
        ClusterID:  claims.ClusterID,
    }, nil
}
```

Add new fields to `Authorizer`:
```go
type Authorizer struct {
    allowInsecure bool
    defaultRole   string
    tokenSecret   string  // NEW: JWT signing secret
    db            *sql.DB // NEW: Database for token validation
}
```

#### 1.5 Token Management API
**New file**: `internal/server/handlers/handlers_tokens.go`

```go
package handlers

// POST /v1/tokens - Create new API token
func (s *Server) HandleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
    // Requires: cli-admin role
    // Request body: { "role": "control-plane", "description": "...", "expires_in_days": 365 }
    // Response: { "token": "eyJ...", "token_id": "...", "expires_at": "..." }
}

// GET /v1/tokens - List API tokens
func (s *Server) HandleListAPITokens(w http.ResponseWriter, r *http.Request) {
    // Requires: cli-admin role
    // Response: [{ "token_id": "...", "role": "...", "description": "...", "expires_at": "...", "last_used_at": "..." }]
}

// DELETE /v1/tokens/{id} - Revoke API token
func (s *Server) HandleRevokeAPIToken(w http.ResponseWriter, r *http.Request) {
    // Requires: cli-admin role
    // Sets revoked_at = NOW()
}
```

#### 1.6 Bootstrap Token API
**New file**: `internal/server/handlers/handlers_bootstrap.go`

```go
package handlers

// POST /v1/bootstrap/tokens - Create bootstrap token for node
func (s *Server) HandleCreateBootstrapToken(w http.ResponseWriter, r *http.Request) {
    // Requires: control-plane or cli-admin role
    // Request: { "node_id": "uuid", "expires_in_minutes": 15 }
    // Response: { "token": "eyJ...", "expires_at": "...", "node_id": "..." }

    // Generate bootstrap token with node_id claim
    // Store hash in bootstrap_tokens table
    // Return token to CLI
}

// POST /v1/pki/bootstrap - Exchange bootstrap token for certificate
func (s *Server) HandleBootstrapCertificate(w http.ResponseWriter, r *http.Request) {
    // Requires: bootstrap token in Authorization header
    // Request: { "csr": "-----BEGIN CERTIFICATE REQUEST-----..." }
    // Response: { "certificate": "...", "ca_bundle": "..." }

    // 1. Validate bootstrap token
    // 2. Extract node_id from token claims
    // 3. Verify CSR CN matches "node:<node_id>"
    // 4. Sign CSR with cluster CA
    // 5. Mark bootstrap token as used (set used_at, cert_issued_at)
    // 6. Return signed certificate + CA bundle
}
```

#### 1.7 Route Registration
**File**: `internal/server/handlers/register.go`

Update route registration (currently lines 10-71):

```go
func RegisterRoutes(mux *http.ServeMux, h *Server, auth *serverAuth.Authorizer) {
    // Public endpoints (no auth)
    mux.HandleFunc("GET /health", h.HandleHealth)

    // Token management (cli-admin only)
    mux.Handle("POST /v1/tokens", auth.Middleware(serverAuth.RoleCLIAdmin)(http.HandlerFunc(h.HandleCreateAPIToken)))
    mux.Handle("GET /v1/tokens", auth.Middleware(serverAuth.RoleCLIAdmin)(http.HandlerFunc(h.HandleListAPITokens)))
    mux.Handle("DELETE /v1/tokens/{id}", auth.Middleware(serverAuth.RoleCLIAdmin)(http.HandlerFunc(h.HandleRevokeAPIToken)))

    // Bootstrap tokens (control-plane or cli-admin)
    mux.Handle("POST /v1/bootstrap/tokens", auth.Middleware(serverAuth.RoleControlPlane)(http.HandlerFunc(h.HandleCreateBootstrapToken)))
    mux.Handle("POST /v1/pki/bootstrap", auth.Middleware(serverAuth.RoleWorker)(http.HandlerFunc(h.HandleBootstrapCertificate)))

    // Existing endpoints (control-plane or cli-admin)
    mux.Handle("GET /v1/config/gitlab", auth.Middleware(serverAuth.RoleControlPlane)(http.HandlerFunc(h.HandleGetGitLabConfig)))
    mux.Handle("POST /v1/config/gitlab", auth.Middleware(serverAuth.RoleCLIAdmin)(http.HandlerFunc(h.HandleSetGitLabConfig)))
    // ... rest of routes
}
```

#### 1.8 Configuration
**File**: `internal/server/config/types.go`

Update `Config` struct:

```go
type Config struct {
    HTTP     HTTPConfig     `yaml:"http"`
    Metrics  MetricsConfig  `yaml:"metrics"`
    Auth     AuthConfig     `yaml:"auth"`      // NEW
    Postgres PostgresConfig `yaml:"postgres"`
}

type HTTPConfig struct {
    Listen string    `yaml:"listen"`  // Change to plain HTTP, e.g., "127.0.0.1:8080"
    // Remove TLS config entirely
}

// NEW
type AuthConfig struct {
    BearerTokens BearerTokenConfig `yaml:"bearer_tokens"`
}

type BearerTokenConfig struct {
    Enabled bool   `yaml:"enabled"`
    Secret  string `yaml:"secret"`  // JWT signing secret (or load from env)
}
```

Example `ployd.yaml`:
```yaml
http:
  listen: "127.0.0.1:8080"  # Plain HTTP, LB handles HTTPS

auth:
  bearer_tokens:
    enabled: true
    secret: ""  # Load from PLOY_AUTH_SECRET env var

postgres:
  dsn: "postgres://..."
```

#### 1.9 Server Startup
**File**: `cmd/ployd/main.go`

Update server initialization (currently lines 80-82):

```go
// Load auth secret from env or config
authSecret := os.Getenv("PLOY_AUTH_SECRET")
if authSecret == "" && cfg.Auth.BearerTokens.Secret != "" {
    authSecret = cfg.Auth.BearerTokens.Secret
}
if authSecret == "" {
    log.Fatal("PLOY_AUTH_SECRET environment variable or auth.bearer_tokens.secret config required")
}

// Create authorizer with bearer token support
authorizer := auth.NewAuthorizer(auth.Options{
    TokenSecret: authSecret,
    DB:          db,
})
```

**File**: `internal/server/http/server.go`

Update `listen()` to remove TLS configuration (lines 169-215):

```go
func (s *Server) listen(ctx context.Context, address string) (net.Listener, error) {
    // Plain HTTP only (TLS handled by load balancer)
    lc := net.ListenConfig{}
    return lc.Listen(ctx, "tcp", address)
}
```

---

### 2. CLI (ploy)

#### 2.1 Token Management Commands
**New file**: `cmd/ploy/token_commands.go`

```go
package main

// ploy token create --role control-plane --expires 365d --description "CI/CD pipeline"
func runTokenCreate(cfg *config.Descriptor, args []string) error {
    // Parse flags: role, expires, description
    // POST /v1/tokens
    // Display token (only shown once)
    // Print warning: "Save this token securely, it won't be shown again"
}

// ploy token list
func runTokenList(cfg *config.Descriptor, args []string) error {
    // GET /v1/tokens
    // Display table: ID, Role, Description, Expires, Last Used
}

// ploy token revoke <token-id>
func runTokenRevoke(cfg *config.Descriptor, args []string) error {
    // DELETE /v1/tokens/{id}
    // Confirm before revoking
}
```

#### 2.2 Node Add Command Update
**File**: `cmd/ploy/node_command.go`

Replace certificate generation flow (lines 147-180) with bootstrap token flow:

```go
func runNodeAdd(cfg *config.Descriptor, args []string) error {
    // Parse flags
    // ...

    // 1. Generate node ID
    nodeID := uuid.New().String()
    fmt.Fprintf(stderr, "Generated node ID: %s\n", nodeID)

    // 2. Request bootstrap token from server
    fmt.Fprintln(stderr, "Requesting bootstrap token...")
    bootstrapToken, err := requestBootstrapToken(cfg, nodeID)
    if err != nil {
        return fmt.Errorf("request bootstrap token: %w", err)
    }
    fmt.Fprintln(stderr, "Bootstrap token received")

    // 3. Prepare bootstrap script environment
    scriptEnv := map[string]string{
        "CLUSTER_ID":           cfg.ClusterID,
        "NODE_ID":              nodeID,
        "NODE_ADDRESS":         cfg.Address,
        "PLOY_BOOTSTRAP_TOKEN": bootstrapToken,  // NEW
        "PLOY_SERVER_URL":      serverURL,
        "PLOY_CA_CERT_PEM":     caCert,          // Still needed for cert verification
    }

    // 4. Provision node via SSH
    fmt.Fprintln(stderr, "Provisioning node...")
    err = deploy.ProvisionNode(ctx, cfg.Address, cfg.SSHIdentityPath, scriptEnv)
    if err != nil {
        return fmt.Errorf("provision node: %w", err)
    }

    // 5. Wait for node heartbeat (with timeout)
    fmt.Fprintln(stderr, "Waiting for node to come online...")
    err = waitForNodeHeartbeat(cfg, nodeID, 2*time.Minute)
    if err != nil {
        return fmt.Errorf("node did not come online: %w", err)
    }

    fmt.Fprintf(stderr, "Node %s successfully added\n", nodeID)
    return nil
}

func requestBootstrapToken(cfg *config.Descriptor, nodeID string) (string, error) {
    client := resolveControlPlaneHTTP(cfg)

    reqBody := map[string]interface{}{
        "node_id":             nodeID,
        "expires_in_minutes":  15,  // Configurable
    }

    resp, err := client.Post(cfg.Address+"/v1/bootstrap/tokens", "application/json", jsonBody(reqBody))
    // Parse response, extract token
    return token, nil
}
```

#### 2.3 Cluster Descriptor
**File**: `internal/cli/config/config.go`

Update `Descriptor` struct (currently lines 22-32):

```go
type Descriptor struct {
    ClusterID       ClusterID `json:"cluster_id"`
    Address         string    `json:"address"`          // https://lb.example.com
    Scheme          string    `json:"scheme,omitempty"` // Optional
    SSHIdentityPath string    `json:"ssh_identity_path,omitempty"`

    // Bearer token authentication
    Token           string    `json:"token,omitempty"`  // NEW: API bearer token

    // Legacy mTLS fields (remove these)
    // CAPath          string    `json:"ca_path,omitempty"`
    // CertPath        string    `json:"cert_path,omitempty"`
    // KeyPath         string    `json:"key_path,omitempty"`

    Default         bool      `json:"default,omitempty"`
}
```

Example cluster descriptor:
```json
{
  "cluster_id": "prod-cluster",
  "address": "https://ploy.example.com",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "ssh_identity_path": "~/.ssh/id_ed25519"
}
```

#### 2.4 HTTP Client
**File**: `cmd/ploy/common_http.go`

Update `resolveControlPlaneHTTP()` (currently lines 20-66):

```go
func resolveControlPlaneHTTP(cfg *config.Descriptor) *http.Client {
    // Simple HTTP client (TLS handled by load balancer)
    transport := &http.Transport{
        TLSClientConfig: &tls.Config{
            MinVersion: tls.VersionTLS13,
        },
    }

    return &http.Client{
        Transport: transport,
        Timeout:   10 * time.Second,
    }
}

// Add authorization header to all requests
func makeAuthenticatedRequest(client *http.Client, cfg *config.Descriptor, method, url string, body io.Reader) (*http.Response, error) {
    req, err := http.NewRequest(method, url, body)
    if err != nil {
        return nil, err
    }

    // Add bearer token
    if cfg.Token != "" {
        req.Header.Set("Authorization", "Bearer "+cfg.Token)
    }

    return client.Do(req)
}
```

#### 2.5 Server Deploy Command
**File**: `cmd/ploy/server_deploy_run.go`

Update to generate initial admin token instead of admin certificate:

```go
func runServerDeploy(cfg *config.Descriptor, args []string) error {
    // ... existing provisioning logic ...

    // After server is deployed and running:
    // Generate initial admin API token via direct database insert or special bootstrap endpoint
    // Save token to cluster descriptor

    descriptor := config.Descriptor{
        ClusterID: clusterID,
        Address:   fmt.Sprintf("https://%s", cfg.Address),
        Token:     initialAdminToken,  // Save token, not cert paths
    }

    // Save descriptor to ~/.config/ploy/clusters/{cluster-id}.json
    // ...
}
```

---

### 3. Node Agent (ployd-node)

#### 3.1 Bootstrap Process
**File**: `internal/nodeagent/agent.go`

Add bootstrap logic to agent startup:

```go
func (a *Agent) Start(ctx context.Context) error {
    // Check if certificate already exists
    certPath := "/etc/ploy/pki/node.crt"
    keyPath := "/etc/ploy/pki/node.key"

    certExists := fileExists(certPath) && fileExists(keyPath)

    if !certExists {
        // Bootstrap: exchange token for certificate
        if err := a.bootstrap(ctx); err != nil {
            return fmt.Errorf("bootstrap: %w", err)
        }
    }

    // Load certificate and start normal operations
    // ...
}

func (a *Agent) bootstrap(ctx context.Context) error {
    log.Println("Starting bootstrap process...")

    // 1. Read bootstrap token from secure location
    tokenPath := "/run/ploy/bootstrap-token"
    tokenBytes, err := os.ReadFile(tokenPath)
    if err != nil {
        return fmt.Errorf("read bootstrap token: %w", err)
    }
    bootstrapToken := strings.TrimSpace(string(tokenBytes))

    // 2. Generate private key and CSR
    log.Println("Generating private key and CSR...")
    nodeID := a.cfg.NodeID  // From config file
    clusterID := a.cfg.ClusterID
    keyBundle, csrPEM, err := pki.GenerateNodeCSR(nodeID, clusterID, "")
    if err != nil {
        return fmt.Errorf("generate CSR: %w", err)
    }

    // 3. Exchange bootstrap token for certificate
    log.Println("Requesting certificate from server...")
    cert, caCert, err := a.requestCertificate(ctx, bootstrapToken, csrPEM)
    if err != nil {
        return fmt.Errorf("request certificate: %w", err)
    }

    // 4. Write certificate and key to disk
    if err := os.WriteFile("/etc/ploy/pki/ca.crt", []byte(caCert), 0644); err != nil {
        return fmt.Errorf("write CA cert: %w", err)
    }
    if err := os.WriteFile("/etc/ploy/pki/node.crt", []byte(cert), 0644); err != nil {
        return fmt.Errorf("write node cert: %w", err)
    }
    if err := os.WriteFile("/etc/ploy/pki/node.key", []byte(keyBundle.KeyPEM), 0600); err != nil {
        return fmt.Errorf("write node key: %w", err)
    }

    // 5. Delete bootstrap token
    os.Remove(tokenPath)

    log.Println("Bootstrap complete")
    return nil
}

func (a *Agent) requestCertificate(ctx context.Context, token string, csrPEM []byte) (cert, caCert string, err error) {
    client := &http.Client{Timeout: 10 * time.Second}

    reqBody := map[string]string{
        "csr": string(csrPEM),
    }

    req, err := http.NewRequest("POST", a.cfg.ServerURL+"/v1/pki/bootstrap", jsonBody(reqBody))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")

    // Retry with exponential backoff (max 5 attempts over ~30 seconds)
    for attempt := 0; attempt < 5; attempt++ {
        resp, err := client.Do(req)
        if err == nil && resp.StatusCode == 200 {
            // Parse response
            var result struct {
                Certificate string `json:"certificate"`
                CABundle    string `json:"ca_bundle"`
            }
            json.NewDecoder(resp.Body).Decode(&result)
            return result.Certificate, result.CABundle, nil
        }

        // Exponential backoff: 1s, 2s, 4s, 8s, 16s
        time.Sleep(time.Duration(1<<attempt) * time.Second)
    }

    return "", "", fmt.Errorf("failed to obtain certificate after retries")
}
```

#### 3.2 Node Configuration
**File**: `internal/nodeagent/config.go`

No changes needed - same config structure, but certificates are obtained via bootstrap instead of SSH.

---

### 4. Bootstrap Script

#### 4.1 Bootstrap Script Update
**File**: `internal/deploy/bootstrap/bootstrap.go`

Update script to write bootstrap token instead of certificates (lines 180-194):

```go
// Write bootstrap token to secure location
b.WriteString("  if [ -n \"${PLOY_BOOTSTRAP_TOKEN:-}\" ]; then\n")
b.WriteString("    mkdir -p /run/ploy\n")
b.WriteString("    echo \"$PLOY_BOOTSTRAP_TOKEN\" > /run/ploy/bootstrap-token\n")
b.WriteString("    chmod 600 /run/ploy/bootstrap-token\n")
b.WriteString("  fi\n")

// Write CA cert for server verification
b.WriteString("  if [ -n \"${PLOY_CA_CERT_PEM:-}\" ]; then\n")
b.WriteString("    mkdir -p /etc/ploy/pki\n")
b.WriteString("    echo \"$PLOY_CA_CERT_PEM\" > /etc/ploy/pki/ca.crt\n")
b.WriteString("    chmod 644 /etc/ploy/pki/ca.crt\n")
b.WriteString("  fi\n")

// Remove sections that write node.crt and node.key
```

---

## Security Considerations

### 1. Token Binding
- Bootstrap tokens MUST encode `node_id` in JWT claims
- Server MUST validate CSR CN matches token's `node_id`
- Prevents token reuse for different nodes

### 2. Single-Use Enforcement
- Mark `used_at` timestamp after first successful cert issuance
- Reject tokens where `used_at IS NOT NULL`
- Allow idempotent retries if cert issuance failed (check `cert_issued_at`)

### 3. Private Key Security
- Node generates private key locally (never sent to server)
- Server signs CSR only (never receives private key)
- Private key stored with `chmod 600` permissions

### 4. Token Storage
- Bootstrap token written to `/run/ploy/bootstrap-token` (tmpfs on most systems)
- File permissions: `chmod 600` (owner read/write only)
- Deleted immediately after successful cert fetch
- Not logged in systemd journal or SSH history

### 5. Network Security
- ployd listens on `127.0.0.1:8080` or internal network IP only
- Never bind to `0.0.0.0` to prevent direct internet access
- Load balancer handles HTTPS termination and rate limiting
- Consider mutual TLS between LB and ployd even for internal network

### 6. Token Expiry
- Bootstrap tokens: 15 minutes default (configurable)
- API tokens: 365 days default (configurable)
- Server MUST check `exp` claim in JWT
- Server MUST check `expires_at` in database (allows early revocation)

### 7. Token Revocation
- Store SHA-256 hash of tokens in database
- Check `revoked_at IS NULL` on every request
- Provide `ploy token revoke` command
- Consider token rotation strategy

### 8. Audit Trail
- Log all token creation events
- Log all token usage (update `last_used_at`)
- Log bootstrap token → certificate exchange
- Include `issued_by` field to track which admin created tokens

---

## Implementation Checklist

### Phase 1: Foundation (Server-Side)
- [x] Add `github.com/golang-jwt/jwt/v5` to `go.mod`
- [x] Create `internal/server/auth/token.go` with JWT generation/validation
- [x] Create database migration `NNNN_bearer_tokens.sql`
- [x] Update `internal/server/auth/authorizer.go`:
  - [x] Add `identityFromBearerToken()` method
  - [x] Update `identityFromRequest()` to try bearer tokens
  - [x] Add token revocation checking
- [x] Update `internal/server/config/types.go`:
  - [x] Remove `TLSConfig` struct
  - [x] Add `AuthConfig` struct with bearer token settings
- [x] Update `cmd/ployd/main.go`:
  - [x] Load `PLOY_AUTH_SECRET` environment variable
  - [x] Initialize authorizer with token support
- [x] Update `internal/server/http/server.go`:
  - [x] Remove TLS listener logic
  - [x] Use plain HTTP listener only

### Phase 2: API Endpoints (Server-Side)
- [x] Create `internal/server/handlers/handlers_tokens.go`:
  - [x] `HandleCreateAPIToken` - POST /v1/tokens
  - [x] `HandleListAPITokens` - GET /v1/tokens
  - [x] `HandleRevokeAPIToken` - DELETE /v1/tokens/{id}
- [x] Create `internal/server/handlers/handlers_bootstrap.go`:
  - [x] `HandleCreateBootstrapToken` - POST /v1/bootstrap/tokens
  - [x] `HandleBootstrapCertificate` - POST /v1/pki/bootstrap
- [x] Update `internal/server/handlers/register.go`:
  - [x] Register token management routes
  - [x] Register bootstrap routes
  - [x] Remove mTLS-specific routes

### Phase 3: CLI Commands
- [x] Update `internal/cli/config/config.go`:
  - [x] Add `Token` field to `Descriptor`
  - [x] Remove `CAPath`, `CertPath`, `KeyPath` fields
- [x] Update `cmd/ploy/common_http.go`:
  - [x] Simplify HTTP client (no mTLS)
  - [x] Add `makeAuthenticatedRequest()` helper
- [x] Create `cmd/ploy/token_commands.go`:
  - [x] `handleTokenCreate` - ploy token create
  - [x] `handleTokenList` - ploy token list
  - [x] `handleTokenRevoke` - ploy token revoke
- [x] Update `cmd/ploy/node_command.go`:
  - [x] Replace cert generation with bootstrap token request
  - [x] Update `runNodeAdd()` to use new flow
  - [ ] Add `waitForNodeHeartbeat()` helper (not required for basic functionality)
- [x] Update `cmd/ploy/server_deploy_run.go`:
  - [x] Provide instructions for generating initial admin token
  - [ ] Auto-generate and save token (deferred - requires additional infrastructure)

### Phase 4: Node Agent
- [x] Update `internal/nodeagent/agent.go`:
  - [x] Add `bootstrap()` method
  - [x] Add `requestCertificate()` method with retry logic
  - [x] Check for existing cert on startup
  - [x] Call bootstrap if cert missing
- [x] Update `internal/deploy/bootstrap/bootstrap.go`:
  - [x] Write bootstrap token to `/run/ploy/bootstrap-token`
  - [x] Remove cert/key writing logic
  - [x] Keep CA cert writing for server verification

### Phase 5: Testing & Documentation
- [x] Update integration tests:
  - [x] Remove mTLS-specific tests
  - [x] Add bearer token auth tests
  - [x] Add bootstrap flow tests
- [x] Update documentation:
  - [x] Update `docs/how-to/deploy-locally.md`
  - [x] Update `docs/how-to/deploy-a-cluster.md`
  - [x] Add token management guide
  - [x] Add troubleshooting guide
- [ ] Update local development:
  - [ ] Remove `local/gen-certs.sh` (or keep for CA only)
  - [ ] Update `local/docker-compose.yml` config
  - [ ] Update `local/server/ployd.yaml`
  - [ ] Update `local/node/ployd-node.yaml`

### Phase 6: Cleanup
- [ ] Remove unused PKI code:
  - [ ] Remove `internal/server/handlers/handlers_pki.go` (old cert signing)
  - [ ] Remove mTLS client cert generation from `internal/pki/ca.go`
  - [ ] Keep CSR signing functions (still needed for bootstrap)
- [ ] Remove environment variables:
  - [ ] `PLOY_SERVER_CA_CERT`, `PLOY_SERVER_CA_KEY`
  - [ ] `PLOY_SERVER_CA_CERT_PATH`, `PLOY_SERVER_CA_KEY_PATH`
- [ ] Update `docs/envs/README.md`:
  - [ ] Document new `PLOY_AUTH_SECRET` variable
  - [ ] Remove mTLS-related variables

---

## API Specifications

### POST /v1/tokens
**Create new API token**

**Authorization**: `cli-admin` role required

**Request**:
```json
{
  "role": "control-plane",
  "description": "CI/CD pipeline token",
  "expires_in_days": 365
}
```

**Response** (200 OK):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_id": "abc123...",
  "role": "control-plane",
  "expires_at": "2026-01-19T12:00:00Z",
  "warning": "Save this token securely. It will not be shown again."
}
```

### GET /v1/tokens
**List all API tokens**

**Authorization**: `cli-admin` role required

**Response** (200 OK):
```json
{
  "tokens": [
    {
      "token_id": "abc123",
      "role": "control-plane",
      "description": "CI/CD pipeline token",
      "issued_at": "2025-01-19T12:00:00Z",
      "expires_at": "2026-01-19T12:00:00Z",
      "last_used_at": "2025-11-19T10:30:00Z",
      "revoked_at": null
    }
  ]
}
```

### DELETE /v1/tokens/{id}
**Revoke API token**

**Authorization**: `cli-admin` role required

**Response** (200 OK):
```json
{
  "message": "Token revoked successfully"
}
```

### POST /v1/bootstrap/tokens
**Create bootstrap token for node provisioning**

**Authorization**: `control-plane` or `cli-admin` role required

**Request**:
```json
{
  "node_id": "123e4567-e89b-12d3-a456-426614174000",
  "expires_in_minutes": 15
}
```

**Response** (200 OK):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "node_id": "123e4567-e89b-12d3-a456-426614174000",
  "expires_at": "2025-11-19T12:15:00Z"
}
```

### POST /v1/pki/bootstrap
**Exchange bootstrap token for certificate**

**Authorization**: Bootstrap token in `Authorization: Bearer` header

**Request**:
```json
{
  "csr": "-----BEGIN CERTIFICATE REQUEST-----\nMIIBATCBqAIBADBb..."
}
```

**Response** (200 OK):
```json
{
  "certificate": "-----BEGIN CERTIFICATE-----\nMIICEjCCAXugAwIBAgIRAL...",
  "ca_bundle": "-----BEGIN CERTIFICATE-----\nMIIBkTCCATigAwIBAgIQaW...",
  "serial": "12345",
  "fingerprint": "SHA256:abc123...",
  "not_before": "2025-11-19T12:00:00Z",
  "not_after": "2026-11-19T12:00:00Z"
}
```

**Errors**:
- `400 Bad Request` - Invalid CSR or CN mismatch
- `401 Unauthorized` - Invalid/expired/used bootstrap token
- `500 Internal Server Error` - CA signing failed

---

## Configuration Examples

### Server Configuration (`/etc/ploy/ployd.yaml`)

```yaml
http:
  listen: "127.0.0.1:8080"  # Plain HTTP, bind to localhost only

metrics:
  listen: ":9100"

auth:
  bearer_tokens:
    enabled: true
    secret: ""  # Load from PLOY_AUTH_SECRET env var (never commit to config)

postgres:
  dsn: "postgres://ploy:password@localhost:5432/ploy?sslmode=require"
```

### Node Configuration (`/etc/ploy/ployd-node.yaml`)

```yaml
server_url: "https://ploy.example.com"  # Load balancer URL
node_id: "123e4567-e89b-12d3-a456-426614174000"
cluster_id: "prod-cluster"

http:
  listen: ":8444"
  # TLS optional for node's own endpoint

heartbeat:
  interval: 30s
  timeout: 10s
```

### CLI Cluster Descriptor (`~/.config/ploy/clusters/prod.json`)

```json
{
  "cluster_id": "prod-cluster",
  "address": "https://ploy.example.com",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjbHVzdGVyX2lkIjoicHJvZC1jbHVzdGVyIiwicm9sZSI6ImNsaS1hZG1pbiIsInRva2VuX3R5cGUiOiJhcGkiLCJleHAiOjE3Mzc0NTYwMDAsImlhdCI6MTcwNTgzNjAwMCwianRpIjoiYWJjMTIzIn0.signature",
  "ssh_identity_path": "~/.ssh/id_ed25519"
}
```

---

## Migration Steps

### For Existing Deployments

1. **Update server** (rolling deployment):
   - Deploy new ployd binary with bearer token support
   - Set `PLOY_AUTH_SECRET` environment variable
   - Update `ployd.yaml` to remove TLS config, add auth config
   - Restart ployd service
   - Configure load balancer for HTTPS termination

2. **Generate initial admin token**:
   - SSH to server
   - Use direct database insert or special bootstrap endpoint to create first admin token
   - Save token securely (e.g., password manager)

3. **Update CLI configuration**:
   - Replace cluster descriptor with new format (URL + token)
   - Remove old cert files
   - Test with `ploy nodes list`

4. **Re-provision nodes** (gradual):
   - Use `ploy node add` with new bootstrap flow
   - Old nodes will continue working until cert expiry
   - Gradually replace old nodes with new bootstrap-based nodes

### For New Deployments

1. **Deploy server**:
   - `ploy server deploy --address <host>`
   - Server generates auth secret automatically
   - Returns admin token in descriptor

2. **Add nodes**:
   - `ploy node add --cluster-id <id> --address <host>`
   - CLI requests bootstrap token
   - Node fetches certificate on first boot

3. **Create additional tokens**:
   - `ploy token create --role control-plane --expires 90d --description "Dev team"`
   - Share tokens with team members
   - Revoke when no longer needed

---

## Testing Checklist

- [ ] Generate API token with `ploy token create`
- [ ] List tokens with `ploy token list`
- [ ] Revoke token with `ploy token revoke`
- [ ] Verify revoked token returns 401
- [ ] Add node with `ploy node add`
- [ ] Verify bootstrap token is single-use
- [ ] Verify bootstrap token expires after timeout
- [ ] Verify CSR CN must match bootstrap token node_id
- [ ] Verify node starts successfully with certificate
- [ ] Verify node sends heartbeats after bootstrap
- [ ] Verify expired API token returns 401
- [ ] Verify invalid token signature returns 401
- [ ] Verify server only accepts HTTP on internal IP
- [ ] Load test token validation performance
- [ ] Test bootstrap retry logic (network failures)
- [ ] Test bootstrap timeout scenarios

---

## Performance Considerations

### Token Validation
- **In-memory cache**: Cache token validation results for 60 seconds to reduce DB queries
- **Database indexes**: Ensure `token_id` and `token_hash` columns are indexed
- **Async updates**: Update `last_used_at` asynchronously (don't block request)

### Bootstrap Flow
- **Connection pooling**: Use HTTP keep-alive for bootstrap requests
- **Retry backoff**: Exponential backoff prevents server overload during outages
- **Timeout tuning**: Bootstrap token lifetime should account for slow SSH connections

### Database
- **Token cleanup**: Periodically delete expired tokens (`expires_at < NOW() - INTERVAL '30 days'`)
- **Audit log archival**: Move old bootstrap token records to archive table

---

## Rollback Plan

If critical issues are discovered:

1. **Quick rollback**:
   - Revert to previous ployd binary
   - Restore `ployd.yaml` with TLS config
   - Re-enable mTLS in load balancer
   - Old CLI with cert-based auth still works

2. **Data preservation**:
   - Token tables remain in database (no data loss)
   - Can resume bearer token deployment after fix

3. **Gradual rollback**:
   - Keep bearer tokens enabled
   - Re-introduce mTLS as fallback option
   - Migrate back to certs over time

---

## Future Enhancements

- **OIDC/OAuth integration**: Allow CLI to use Google/GitHub OAuth tokens
- **Fine-grained permissions**: Role-based permissions beyond 3 roles
- **Token rotation**: Automatic token rotation before expiry
- **Hardware security modules**: Store JWT signing key in HSM
- **Rate limiting**: Per-token rate limits to prevent abuse
- **Session management**: Track active sessions, allow remote logout
