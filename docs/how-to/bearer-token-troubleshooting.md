# Bearer Token Troubleshooting

This guide covers the current bearer-token authentication contract for the CLI,
control plane, and worker nodes.

## Quick Checks

CLI commands that call the control plane require:

```bash
export PLOY_SERVER_URL="https://ploy.example.com"
export PLOY_AUTH_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

`PLOY_SERVER_URL` is required. `PLOY_AUTH_TOKEN` is optional for unauthenticated
local paths, but authenticated API calls need it.

Raw HTTP requests must use the bearer header:

```bash
curl -H "Authorization: Bearer $PLOY_AUTH_TOKEN" "$PLOY_SERVER_URL/v1/nodes"
```

Common malformed forms:

```bash
curl -H "Authorization: $PLOY_AUTH_TOKEN" "$PLOY_SERVER_URL/v1/nodes"
curl -H "Token: $PLOY_AUTH_TOKEN" "$PLOY_SERVER_URL/v1/nodes"
```

## Decode Claims

A valid JWT has three dot-separated parts. Decode the payload without verifying
the signature:

```bash
PAYLOAD=$(printf '%s' "$PLOY_AUTH_TOKEN" | cut -d. -f2)
printf '%s' "$PAYLOAD" | base64 -d | jq
```

Expected API-token claims:

```json
{
  "role": "control-plane",
  "token_type": "api",
  "exp": 1745256000,
  "iat": 1737480000,
  "jti": "abc123"
}
```

Check:

- `exp` is in the future.
- `role` is appropriate for the endpoint.
- `token_type` is `"api"` for normal CLI calls.

## Common Errors

### `PLOY_SERVER_URL is required`

The CLI could not resolve the control-plane URL.

```bash
export PLOY_SERVER_URL="http://127.0.0.1:${PLOY_SERVER_PORT:-8080}"
```

### 401 Unauthorized: Missing Bearer Token

Set `PLOY_AUTH_TOKEN` or pass the header explicitly:

```bash
export PLOY_AUTH_TOKEN="<token>"
ploy cluster token list
```

If no token exists, create one from an existing `cli-admin` session:

```bash
ploy cluster token create --role cli-admin --expires 365
```

### 401 Unauthorized: Invalid Token

Possible causes:

- Token is truncated or malformed.
- Token was signed with a different `PLOY_AUTH_SECRET`.
- Server restarted with a different `PLOY_AUTH_SECRET`.

Verify the token shape:

```bash
printf '%s' "$PLOY_AUTH_TOKEN" | tr '.' '\n' | wc -l
```

Check server logs:

```bash
docker compose -f /Users/v.v.kovalev/@scale/ploy-lib/images/docker-compose.yml logs --tail=200 server | rg -i "token|auth|401|403" || true
```

### 401 Unauthorized: Token Expired

Check expiration:

```bash
printf '%s' "$PLOY_AUTH_TOKEN" | cut -d. -f2 | base64 -d | jq -r .exp | xargs -I {} date -r {}
```

Create a replacement token:

```bash
ploy cluster token create --role cli-admin --expires 365 --description "Replacement token"
export PLOY_AUTH_TOKEN="<new-token>"
```

### 401 Unauthorized: Token Revoked

List tokens and look for revoked status:

```bash
ploy cluster token list
```

Create a replacement token and update `PLOY_AUTH_TOKEN`.

### 403 Forbidden

The token role is valid but insufficient.

- `control-plane` cannot create or revoke tokens.
- `control-plane` cannot sign certificates.
- `worker` cannot access admin endpoints.

Use a `cli-admin` token for admin endpoints.

## Server Checks

Verify auth config:

```bash
docker compose -f /Users/v.v.kovalev/@scale/ploy-lib/images/docker-compose.yml exec -T server env | rg -n "PLOY_AUTH_SECRET|PLOYD_AUTH_BEARER_TOKENS_ENABLED" || true
```

Query token state:

```sql
SELECT token_id, role, description, issued_at, expires_at, revoked_at, last_used_at
FROM api_tokens
ORDER BY issued_at DESC
LIMIT 10;
```

Find expired active tokens:

```sql
SELECT token_id, role, description, expires_at
FROM api_tokens
WHERE expires_at < NOW()
  AND revoked_at IS NULL;
```

## Security Response

If a token leaks:

1. Revoke it immediately:
   ```bash
   ploy cluster token revoke <token-id>
   ```
2. Check recent usage:
   ```bash
   ploy cluster token list | grep <token-id>
   ```
3. Review server logs:
   ```bash
   docker compose -f /Users/v.v.kovalev/@scale/ploy-lib/images/docker-compose.yml logs --tail=500 server | rg <token-id> || true
   ```
4. Create a new token and update systems that used the old token.

## Worker Nodes

Worker nodes authenticate to the control plane with a bearer token mounted at
`/etc/ploy/bearer-token`. In compose deployments, `WORKER_TOKEN_PATH` selects the
host file to mount. Update that file and restart the node when rotating worker
credentials.

## Related Documentation

- [Token Management Guide](token-management.md)
- [Environment Variables](../envs/README.md)
