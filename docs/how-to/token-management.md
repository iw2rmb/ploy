# Token Management Guide

This guide covers bearer token authentication in Ploy, including how to create, list, and revoke tokens for CLI access.

## Overview

Ploy uses JWT-based bearer tokens for authentication:

- **API Tokens**: Long-lived tokens for CLI authentication (default: 365 days)

## Token Types

### API Tokens

API tokens are used by the CLI to authenticate with the control plane. Each token has:

- **Role**: Determines access level (`cli-admin`, `control-plane`, or `worker`)
- **Description**: Human-readable label for the token
- **Expiration**: When the token becomes invalid
- **Token ID**: Unique identifier for revocation

API tokens are stored in the `api_tokens` table in PostgreSQL.

## Roles

Ploy supports three roles:

- **`cli-admin`**: Full administrative access, including token management
- **`control-plane`**: Standard CLI access for running Migs and viewing cluster state
- **`worker`**: Node agent role

## Managing API Tokens

### Create a Token

Create a new API token with the `ploy cluster token create` command:

```bash
# Create a token for CI/CD with control-plane access
ploy cluster token create --role control-plane --expires 90 --description "CI/CD pipeline"

# Create an admin token
ploy cluster token create --role cli-admin --expires 365 --description "Admin workstation"

# Create a short-lived token for testing
ploy cluster token create --role control-plane --expires 1 --description "Temporary test token"
```

**Output:**
```
Token created successfully.

WARNING: Save this token securely. It will not be shown again.

Token:     eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjbHVzdGVyX2lkIjoiYWxwaGEtY2x1c3RlciIsInJvbGUiOiJjb250cm9sLXBsYW5lIiwidG9rZW5fdHlwZSI6ImFwaSIsImV4cCI6MTc0NTI1NjAwMCwiaWF0IjoxNzM3NDgwMDAwLCJqdGkiOiJhYmMxMjMifQ.signature
Token ID:  abc123
Role:      control-plane
Expires:   2026-01-19T12:00:00Z

Store this token in a secure location (e.g., password manager, CI/CD secrets).
```

**Important**: The full token is only shown once. Store it securely before closing the terminal.

### List Tokens

View all API tokens in the cluster:

```bash
ploy cluster token list
```

**Output:**
```
TOKEN ID     ROLE             DESCRIPTION           ISSUED AT                 EXPIRES AT                LAST USED
abc123       control-plane    CI/CD pipeline        2025-01-19T12:00:00Z     2026-04-19T12:00:00Z     2025-11-19T10:30:00Z
def456       cli-admin        Admin workstation     2025-10-01T08:00:00Z     2026-10-01T08:00:00Z     2025-11-18T15:45:00Z
ghi789       control-plane    Dev team access       2025-11-01T10:00:00Z     2026-02-01T10:00:00Z     (never)
```

**Note**: The full token is not displayed for security reasons. Only the token ID is shown.

### Revoke a Token

Revoke a token to immediately invalidate it:

```bash
ploy cluster token revoke abc123
```

**Output:**
```
Token abc123 revoked successfully.

Any requests using this token will now fail with a 401 Unauthorized error.
```

You can also add a confirmation prompt:

```bash
ploy cluster token revoke abc123 --confirm
```

**Revocation is immediate**: All in-flight requests using the revoked token will fail after the current request completes.

## Using Tokens

### CLI Configuration

Store your token in the cluster descriptor under `PLOY_CONFIG_HOME` (or home default).
The local Docker cluster uses `PLOY_CONFIG_HOME="$HOME/.config/ploy/local"` and
`address: "http://127.0.0.1:${PLOY_SERVER_PORT:-8080}"`.

```json
{
  "cluster_id": "local",
  "address": "http://127.0.0.1:8080",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

If you set `PLOY_SERVER_PORT` to a non-default value, use that port in `address`.

The CLI automatically uses this token for all requests.

### CI/CD Integration

For CI/CD pipelines, store the token as a secret and pass it via environment variable or config file:

**GitLab CI:**
```yaml
deploy:
  script:
    - mkdir -p ~/.config/ploy/<cluster-id>
    - echo "$PLOY_DESCRIPTOR" > ~/.config/ploy/<cluster-id>/auth.json
    - ploy run --repo $CI_PROJECT_URL --base-ref main --target-ref ploy/$CI_PIPELINE_ID --spec mig.yaml --follow
  variables:
    PLOY_DESCRIPTOR: $PLOY_CLUSTER_DESCRIPTOR  # Set in GitLab CI/CD variables
```

**GitHub Actions:**
```yaml
- name: Configure Ploy
  run: |
    mkdir -p ~/.config/ploy
    echo "$PLOY_DESCRIPTOR" > ~/.config/ploy/<cluster-id>/auth.json
  env:
    PLOY_DESCRIPTOR: ${{ secrets.PLOY_CLUSTER_DESCRIPTOR }}

- name: Run Migs
  run: ploy run --repo ${{ github.repositoryUrl }} --base-ref main --target-ref ploy/${{ github.run_id }} --spec mig.yaml --follow
```

## Worker Node Authentication (Local Docker)

In the local Docker cluster, the worker node uses a long-lived bearer token stored at
`/etc/ploy/bearer-token` in the node container. `ploy cluster deploy` provisions this token.

## Token Security

### Best Practices

1. **Never commit tokens to version control**: Use environment variables or secret management systems.
2. **Use short-lived tokens for testing**: Set `--expires 1d` or `--expires 1h` for temporary access.
3. **Rotate tokens regularly**: Create new tokens and revoke old ones periodically.
4. **Use separate tokens for different environments**: Don't reuse production tokens in staging/dev.
5. **Monitor token usage**: Check `last_used_at` timestamps to identify unused tokens.
6. **Revoke compromised tokens immediately**: If a token is leaked, revoke it and create a new one.

### Token Storage

- **Local development**: Store in `~/.config/ploy/<cluster-id>/auth.json` with file permissions `0600`.
- **CI/CD**: Use CI/CD secret management (GitLab CI/CD variables, GitHub Secrets, etc.).
- **Production servers**: Use a secrets management service (HashiCorp Vault, AWS Secrets Manager, etc.).

### Auditing

Monitor token usage in the database:

```sql
-- Find tokens that haven't been used recently
SELECT token_id, role, description, issued_at, last_used_at
FROM api_tokens
WHERE last_used_at < NOW() - INTERVAL '30 days'
  AND revoked_at IS NULL;
```

## Troubleshooting

### "Token expired"

Your token has exceeded its `expires_at` timestamp. Create a new token:

```bash
ploy cluster token create --role cli-admin --expires 365 --description "Replacement token"
```

Update your cluster descriptor with the new token.

### "Token revoked"

The token has been manually revoked. Create a new token and update your configuration.

### "Invalid token"

Possible causes:
- Token is malformed (copy/paste error)
- Token was signed with a different `PLOY_AUTH_SECRET`
- Token signature doesn't match

Verify:
1. Token is complete and unmodified
2. Server's `PLOY_AUTH_SECRET` hasn't changed
3. Token is valid JSON Web Token (JWT) format

### "Insufficient permissions"

Your token's role doesn't allow the requested operation. For example:

- `control-plane` tokens cannot create or revoke tokens (requires `cli-admin`)
- `control-plane` tokens cannot sign certificates (requires `cli-admin`)

Create a new token with the appropriate role or contact an admin.

## API Reference

### POST /v1/tokens

Create a new API token.

**Authorization**: `cli-admin` role required

**Request:**
```json
{
  "role": "control-plane",
  "description": "CI/CD pipeline token",
  "expires_in_days": 365
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_id": "abc123",
  "role": "control-plane",
  "expires_at": "2026-01-19T12:00:00Z",
  "warning": "Save this token securely. It will not be shown again."
}
```

### GET /v1/tokens

List all API tokens.

**Authorization**: `cli-admin` role required

**Response:**
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

Revoke an API token.

**Authorization**: `cli-admin` role required

**Response:**
```json
{
  "message": "Token revoked successfully"
}
```

## Related Documentation

- [Deploy](deploy.md) - Local Docker development setup
- [Environment Variables](../envs/README.md) - Configuration reference
- [Troubleshooting Guide](bearer-token-troubleshooting.md) - Common authentication issues
