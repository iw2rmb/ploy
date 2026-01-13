# Bearer Token Authentication Troubleshooting Guide

This guide helps diagnose and resolve common issues with bearer token authentication in Ploy.

## Quick Diagnostics

### Check Authentication Method

Verify your request is using bearer token authentication:

```bash
# Correct format
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
     http://localhost:8080/health

# Incorrect formats (will fail)
curl -H "Authorization: eyJ..."  # Missing "Bearer " prefix
curl -H "Token: eyJ..."          # Wrong header name
```

### Verify Token Format

A valid JWT token has three parts separated by dots:

```
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjbHVzdGVyX2lkIjoiYWxwaGEtY2x1c3RlciIsInJvbGUiOiJjb250cm9sLXBsYW5lIiwidG9rZW5fdHlwZSI6ImFwaSIsImV4cCI6MTc0NTI1NjAwMCwiaWF0IjoxNzM3NDgwMDAwLCJqdGkiOiJhYmMxMjMifQ.signature
└─────────────── header ───────────────┘ └──────────────────────────────── payload ───────────────────────────────┘ └─ signature ─┘
```

If your token doesn't have three parts, it's malformed.

### Decode Token (Client-Side)

Decode the token payload to inspect claims (without verifying signature):

```bash
# Extract payload (second part)
echo "eyJjbHVzdGVyX2lkIjoiYWxwaGEtY2x1c3RlciIsInJvbGUiOiJjb250cm9sLXBsYW5lIiwidG9rZW5fdHlwZSI6ImFwaSIsImV4cCI6MTc0NTI1NjAwMCwiaWF0IjoxNzM3NDgwMDAwLCJqdGkiOiJhYmMxMjMifQ" | base64 -d | jq

# Output:
{
  "cluster_id": "alpha-cluster",
  "role": "control-plane",
  "token_type": "api",
  "exp": 1745256000,  # Expiration timestamp
  "iat": 1737480000,  # Issued at timestamp
  "jti": "abc123"     # Token ID
}
```

Check:
- `exp` (expiration) is in the future: `date -r 1745256000` (Unix timestamp)
- `role` matches your expected permissions
- `token_type` is `"api"`

## Common Errors

### 401 Unauthorized: "authentication required: provide Bearer token"

**Cause**: No `Authorization` header provided.

**Solution**:
1. Ensure the CLI descriptor contains a valid token:
   ```bash
   cat "${PLOY_CONFIG_HOME:-$HOME/.config/ploy}/clusters/default"
   # Should show the cluster ID

   cat "${PLOY_CONFIG_HOME:-$HOME/.config/ploy}/clusters/<cluster-id>.json"
   # Should contain "token": "eyJ..."
   ```

2. If the token is missing, create a new one:
  ```bash
  # If you have existing admin access
   ploy cluster token create --role cli-admin --expires 365

   # Otherwise, contact your cluster admin or create a new token
   ```

3. Update the descriptor:
   ```bash
   # Edit ~/.config/ploy/clusters/<cluster-id>.json
   {
     "cluster_id": "alpha-cluster",
     "address": "https://ploy.example.com",
     "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
   }
   ```

### 401 Unauthorized: "invalid token"

**Cause**: Token signature doesn't match or token is malformed.

**Possible reasons**:
1. **Token was signed with a different secret**: The server's `PLOY_AUTH_SECRET` changed.
2. **Token is corrupted**: Copy/paste error or truncated token.
3. **Token format is incorrect**: Missing parts or invalid base64 encoding.

**Solution**:

1. Verify the token is complete (three parts separated by dots):
   ```bash
   TOKEN=$(jq -r .token ~/.config/ploy/clusters/<cluster-id>.json)
   echo "$TOKEN" | tr '.' '\n' | wc -l
   # Should output: 3
   ```

2. Check server logs for signature validation errors:
   ```bash
   docker compose -f local/docker-compose.yml logs --tail=200 server | rg -i "token|auth|401|403" || true
   ```

3. If the server's `PLOY_AUTH_SECRET` changed, all existing tokens are invalid. Create a new token using the updated secret or contact your cluster admin.

4. Generate a fresh token:
  ```bash
   ploy cluster token create --role cli-admin --expires 365 --description "Replacement token"
   ```

### 401 Unauthorized: "token expired"

**Cause**: Token's `exp` (expiration) claim is in the past.

**Solution**:

1. Check token expiration:
   ```bash
   TOKEN=$(jq -r .token ~/.config/ploy/clusters/<cluster-id>.json)
   PAYLOAD=$(echo "$TOKEN" | cut -d. -f2)
   echo "$PAYLOAD" | base64 -d | jq -r .exp | xargs -I {} date -r {}
   ```

2. Create a new token:
   ```bash
   ploy cluster token create --role cli-admin --expires 365 --description "Renewed token"
   ```

3. Update the descriptor with the new token.

### 401 Unauthorized: "token revoked"

**Cause**: Token was manually revoked via `ploy cluster token revoke`.

**Solution**:

1. Verify the token is revoked:
  ```bash
   ploy cluster token list
   # Check if your token ID appears with a revoked_at timestamp
   ```

2. Create a new token:
  ```bash
   ploy cluster token create --role cli-admin --expires 365 --description "Replacement token"
   ```

3. Update the descriptor with the new token.

### 403 Forbidden: "insufficient permissions"

**Cause**: Token's role doesn't allow the requested operation.

**Examples**:
- `control-plane` token trying to create tokens (requires `cli-admin`)
- `control-plane` token trying to sign certificates (requires `cli-admin`)
- `worker` token trying to access admin endpoints

**Solution**:

1. Check your token's role:
  ```bash
   ploy cluster token list
   # Find your token ID and check the Role column
   ```

2. If you need admin access, create a `cli-admin` token:
  ```bash
   # Requires existing cli-admin access
   ploy cluster token create --role cli-admin --expires 365 --description "Admin token"
   ```

3. If you don't have admin access, contact your cluster administrator.

## Server-Side Debugging (Local Docker)

### Check Server Logs

Monitor the server logs for authentication errors:

```bash
docker compose -f local/docker-compose.yml logs -f server
```

### Verify Server Configuration

Check the server's authentication configuration:

```bash
cat local/server/ployd.yaml | rg -n "auth|bearer" || true
```

Expected output:
```yaml
auth:
  bearer_tokens:
    enabled: true
    # secret loaded from PLOY_AUTH_SECRET environment variable
```

Verify the environment variable is set:

```bash
docker compose -f local/docker-compose.yml exec -T server env | rg PLOY_AUTH_SECRET || true
```

### Check Database State

Query the token tables directly:

```sql
-- List all API tokens
SELECT token_id, role, description, issued_at, expires_at, revoked_at, last_used_at
FROM api_tokens
ORDER BY issued_at DESC
LIMIT 10;

-- Find expired tokens
SELECT token_id, role, description, expires_at
FROM api_tokens
WHERE expires_at < NOW()
  AND revoked_at IS NULL;
```

### Validate JWT Secret

Ensure `PLOY_AUTH_SECRET` is consistent:

```bash
# Check current secret (be careful not to log this!)
docker compose -f local/docker-compose.yml exec -T server env | rg PLOY_AUTH_SECRET || true

# If the secret changed, all tokens are invalid
# Generate a new admin token manually via database or API
```

## Performance Issues

### Slow Token Validation

**Symptom**: Requests are slow despite low server load.

**Cause**: Database queries for token validation are not optimized.

**Solution**:

1. Verify indexes exist:
   ```sql
   \d api_tokens
   -- Should show indexes on token_id and token_hash
   ```

2. Add missing indexes:
   ```sql
   CREATE INDEX IF NOT EXISTS idx_api_tokens_token_id ON api_tokens(token_id);
   CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash);
   ```

### High Database Load

**Symptom**: PostgreSQL CPU/disk usage is high during peak traffic.

**Cause**: `last_used_at` updates on every request.

**Solution**:

1. The server updates `last_used_at` asynchronously to avoid blocking requests. Verify this is working:
   ```bash
   docker compose -f local/docker-compose.yml logs --tail=500 server | rg "last_used_at" || true
   ```

2. If synchronous updates are blocking, check for database connection pool exhaustion:
   ```sql
   SELECT count(*) FROM pg_stat_activity WHERE datname = 'ploy';
   ```

3. Increase the connection pool size in `ployd.yaml`:
   ```yaml
   postgres:
     dsn: "postgres://ploy:password@localhost:5432/ploy?pool_max_conns=50"
   ```

## Security Concerns

### Token Leaked

**Action**:
1. Revoke the compromised token immediately:
  ```bash
   ploy cluster token revoke <token-id>
   ```

2. Check `last_used_at` to see if it was used:
  ```bash
   ploy cluster token list | grep <token-id>
   ```

3. Review server logs for suspicious activity:
   ```bash
   docker compose -f local/docker-compose.yml logs --tail=500 server | rg <token-id> || true
   ```

4. Create a new token and update all systems using the old token.

### Suspicious Activity

Monitor for:
- Multiple failed authentication attempts from the same IP
- Tokens with unusual usage patterns (e.g., used from many different IPs)
- Expired tokens still attempting to authenticate

**Query for suspicious patterns**:
```sql
-- Tokens with recent activity
SELECT token_id, role, last_used_at,
       AGE(NOW(), last_used_at) as time_since_use
FROM api_tokens
WHERE last_used_at > NOW() - INTERVAL '1 hour'
ORDER BY last_used_at DESC;

-- Tokens that expired recently but may still be in use
SELECT token_id, role, expires_at
FROM api_tokens
WHERE expires_at BETWEEN NOW() - INTERVAL '7 days' AND NOW()
  AND revoked_at IS NULL;
```

## Migration from mTLS

If you're migrating from mTLS-only authentication:

### Update Cluster Descriptors

Old format (mTLS):
```json
{
  "cluster_id": "alpha-cluster",
  "address": "https://ploy.example.com:8443",
  "ca_path": "/path/to/ca.crt",
  "cert_path": "/path/to/admin.crt",
  "key_path": "/path/to/admin.key"
}
```

New format (bearer token):
```json
{
  "cluster_id": "alpha-cluster",
  "address": "https://ploy.example.com",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### Update HTTP Clients

Old code (mTLS):
```go
cert, _ := tls.LoadX509KeyPair("admin.crt", "admin.key")
caCert, _ := os.ReadFile("ca.crt")
caCertPool := x509.NewCertPool()
caCertPool.AppendCertsFromPEM(caCert)

client := &http.Client{
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{
            Certificates: []tls.Certificate{cert},
            RootCAs:      caCertPool,
        },
    },
}
```

New code (bearer token):
```go
client := &http.Client{}
req, _ := http.NewRequest("GET", "https://ploy.example.com/v1/nodes", nil)
req.Header.Set("Authorization", "Bearer "+token)
resp, _ := client.Do(req)
```

### Update Node Provisioning

In the local Docker cluster, the node authenticates to the control plane using
the bearer token at `/etc/ploy/bearer-token`. `scripts/deploy-locally.sh` writes
this file into the node container and restarts it.

## Related Documentation

- [Token Management Guide](token-management.md) - Creating, listing, and revoking tokens
- [Deploy Locally](deploy-locally.md) - Local development setup
- [Environment Variables](../envs/README.md) - Configuration reference
