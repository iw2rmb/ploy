# Bearer Token Authentication Troubleshooting Guide

This guide helps diagnose and resolve common issues with bearer token authentication in Ploy.

## Quick Diagnostics

### Check Authentication Method

Verify your request is using bearer token authentication:

```bash
# Correct format
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
     https://ploy.example.com/health

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
- `token_type` is `"api"` for CLI tokens or `"bootstrap"` for node tokens

## Common Errors

### 401 Unauthorized: "authentication required: provide Bearer token"

**Cause**: No `Authorization` header provided.

**Solution**:
1. Ensure the CLI descriptor contains a valid token:
   ```bash
   cat ~/.config/ploy/clusters/default
   # Should show the cluster ID

   cat ~/.config/ploy/clusters/<cluster-id>.json
   # Should contain "token": "eyJ..."
   ```

2. If the token is missing, create a new one:
   ```bash
   # If you have existing admin access
   ploy token create --role cli-admin --expires 365d

   # Otherwise, contact your cluster admin or bootstrap a new token
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
   journalctl -u ployd.service -n 50 | grep -i "token\|auth"
   ```

3. If the server's `PLOY_AUTH_SECRET` changed, all existing tokens are invalid. Create a new token using the updated secret or contact your cluster admin.

4. Generate a fresh token:
   ```bash
   ploy token create --role cli-admin --expires 365d --description "Replacement token"
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
   ploy token create --role cli-admin --expires 365d --description "Renewed token"
   ```

3. Update the descriptor with the new token.

### 401 Unauthorized: "token revoked"

**Cause**: Token was manually revoked via `ploy token revoke`.

**Solution**:

1. Verify the token is revoked:
   ```bash
   ploy token list
   # Check if your token ID appears with a revoked_at timestamp
   ```

2. Create a new token:
   ```bash
   ploy token create --role cli-admin --expires 365d --description "Replacement token"
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
   ploy token list
   # Find your token ID and check the Role column
   ```

2. If you need admin access, create a `cli-admin` token:
   ```bash
   # Requires existing cli-admin access
   ploy token create --role cli-admin --expires 365d --description "Admin token"
   ```

3. If you don't have admin access, contact your cluster administrator.

### Bootstrap Token Errors

#### "bootstrap token expired"

**Cause**: Bootstrap tokens expire after 15 minutes by default.

**Solution**:

The `ploy cluster node add` command automatically requests a fresh token, so this should be rare. If provisioning takes longer than 15 minutes:

1. Increase the bootstrap token lifetime (requires server config change):
   ```yaml
   # /etc/ploy/ployd.yaml
   auth:
     bearer_tokens:
       bootstrap_token_ttl: 30m  # Extend to 30 minutes
   ```

2. Restart the server:
   ```bash
   systemctl restart ployd.service
   ```

3. Retry `ploy cluster node add`.

#### "bootstrap token already used"

**Cause**: Bootstrap tokens are single-use and marked as used after successful certificate issuance.

**Solution**:

1. If node provisioning failed after the token was used, the CLI will automatically request a new token on retry.

2. If manually provisioning, request a new bootstrap token:
   ```bash
   # Direct API call (not exposed via CLI)
   curl -X POST https://ploy.example.com/v1/bootstrap/tokens \
     -H "Authorization: Bearer $PLOY_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "node_id": "aB3xY9",
       "expires_in_minutes": 15
     }'
   ```

#### "CSR CN does not match bootstrap token node_id"

**Cause**: The Certificate Signing Request (CSR) Common Name doesn't match the `node_id` in the bootstrap token.

**Solution**:

Ensure the CSR CN is formatted as `node:<node_id>`:

```bash
# Correct format (NanoID(6) - 6 characters from URL-safe alphabet A-Za-z0-9_-)
CN=node:aB3xY9

# Incorrect formats
CN=aB3xY9           # Missing "node:" prefix
CN=node-aB3xY9      # Wrong separator (hyphen instead of colon)
```

The `ploy cluster node add` command handles this automatically. If generating CSRs manually, use:

```bash
# NODE_ID is a NanoID(6) string (e.g., "aB3xY9")
openssl req -new -key node.key -out node.csr -subj "/CN=node:$NODE_ID"
```

## Server-Side Debugging

### Check Server Logs

Monitor the server logs for authentication errors:

```bash
# Real-time logs
journalctl -u ployd.service -f

# Recent authentication errors
journalctl -u ployd.service -n 100 | grep -E "auth|token|401|403"

# Successful authentications
journalctl -u ployd.service -n 100 | grep "authenticated"
```

### Verify Server Configuration

Check the server's authentication configuration:

```bash
cat /etc/ploy/ployd.yaml | grep -A 5 auth
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
systemctl show ployd.service | grep PLOY_AUTH_SECRET
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

-- Recent bootstrap token activity
SELECT node_id, issued_at, used_at, expires_at, cert_issued_at
FROM bootstrap_tokens
WHERE issued_at > NOW() - INTERVAL '1 day'
ORDER BY issued_at DESC;

-- Find unused bootstrap tokens
SELECT node_id, issued_at, expires_at
FROM bootstrap_tokens
WHERE used_at IS NULL
  AND expires_at > NOW();
```

### Validate JWT Secret

Ensure `PLOY_AUTH_SECRET` is consistent:

```bash
# Check current secret (be careful not to log this!)
sudo systemctl show ployd.service -p Environment | grep PLOY_AUTH_SECRET

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

   \d bootstrap_tokens
   -- Should show indexes on token_id and node_id
   ```

2. Add missing indexes:
   ```sql
   CREATE INDEX IF NOT EXISTS idx_api_tokens_token_id ON api_tokens(token_id);
   CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash);
   CREATE INDEX IF NOT EXISTS idx_bootstrap_tokens_token_id ON bootstrap_tokens(token_id);
   ```

3. Enable token caching (future enhancement):
   - Cache token validation results for 60 seconds in memory
   - Reduces database queries for repeated requests

### High Database Load

**Symptom**: PostgreSQL CPU/disk usage is high during peak traffic.

**Cause**: `last_used_at` updates on every request.

**Solution**:

1. The server updates `last_used_at` asynchronously to avoid blocking requests. Verify this is working:
   ```bash
   journalctl -u ployd.service | grep "last_used_at"
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
   ploy token revoke <token-id>
   ```

2. Check `last_used_at` to see if it was used:
   ```bash
   ploy token list | grep <token-id>
   ```

3. Review server logs for suspicious activity:
   ```bash
   journalctl -u ployd.service | grep <token-id>
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
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "ssh_identity_path": "~/.ssh/id_ed25519"
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

Nodes still use certificates after bootstrap, but the provisioning flow changes:

**Old flow (mTLS)**:
1. CLI generates node certificate locally
2. CLI copies certificate to node via SSH
3. Node uses certificate immediately

**New flow (bootstrap token)**:
1. CLI requests bootstrap token from server
2. CLI writes token to node via SSH
3. Node generates key+CSR locally
4. Node exchanges token for certificate
5. Node uses certificate for subsequent requests

No code changes needed if using `ploy cluster node add`.

## Related Documentation

- [Token Management Guide](token-management.md) - Creating, listing, and revoking tokens
- [Deploy a Cluster](deploy-a-cluster.md) - Full deployment guide
- [Deploy Locally](deploy-locally.md) - Local development setup
- [Environment Variables](../envs/README.md) - Configuration reference
