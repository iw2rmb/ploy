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
- **`control-plane`**: Standard CLI access for running Migs and viewing control-plane state
- **`worker`**: Node agent role

## Managing API Tokens

### Create a Token

Create a new API token with the `ploy cluster token create` command:

```bash
# Create a token for CI/CD with control-plane access
ploy cluster token create --role control-plane --username ci-bot --expires 90 --description "CI/CD pipeline"

# Create an admin token
ploy cluster token create --role cli-admin --expires 365 --description "Admin workstation"

# Create a short-lived token for testing
ploy cluster token create --role control-plane --username test-user --expires 1 --description "Temporary test token"
```

**Output:**
```
Token created successfully.

WARNING: Save this token securely. It will not be shown again.

Token:     eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJyb2xlIjoiY29udHJvbC1wbGFuZSIsInRva2VuX3R5cGUiOiJhcGkiLCJleHAiOjE3NDUyNTYwMDAsImlhdCI6MTczNzQ4MDAwMCwianRpIjoiYWJjMTIzIn0.signature
Token ID:  abc123
Role:      control-plane
Username:  ci-bot
Expires:   2026-01-19T12:00:00Z

Store this token in a secure location (e.g., password manager, CI/CD secrets).
```

**Important**: The full token is only shown once. Store it securely before closing the terminal.

### List Tokens

View all API tokens:

```bash
ploy cluster token list
```

**Output:**
```
TOKEN ID     ROLE             USERNAME  DESCRIPTION        EXPIRES     LAST USED   STATUS
abc123       control-plane    ci-bot    CI/CD pipeline     2026-04-19  2025-11-19  active
def456       cli-admin        -         Admin workstation  2026-10-01  2025-11-18  active
ghi789       control-plane    dev-team  Dev team access    2026-02-01  never       active
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

Configure CLI authentication with environment variables:

```bash
export PLOY_SERVER_URL="http://127.0.0.1:${PLOY_SERVER_PORT:-8080}"
export PLOY_AUTH_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

If `PLOY_AUTH_TOKEN` is non-empty, the CLI sends it as
`Authorization: Bearer <token>` on HTTP requests.

### CI/CD Integration

For CI/CD pipelines, store the server URL and token as secrets and pass them as
environment variables:

**GitLab CI:**
```yaml
deploy:
  script:
    - ploy run mig.yaml "$CI_PROJECT_PATH:main" --pull
  variables:
    PLOY_SERVER_URL: $PLOY_SERVER_URL
    PLOY_AUTH_TOKEN: $PLOY_AUTH_TOKEN
```

**GitHub Actions:**
```yaml
- name: Configure Ploy
  run: ploy version
  env:
    PLOY_SERVER_URL: ${{ secrets.PLOY_SERVER_URL }}
    PLOY_AUTH_TOKEN: ${{ secrets.PLOY_AUTH_TOKEN }}

- name: Run Migs
  run: ploy run mig.yaml "${{ github.repository }}:main" --pull
  env:
    PLOY_SERVER_URL: ${{ secrets.PLOY_SERVER_URL }}
    PLOY_AUTH_TOKEN: ${{ secrets.PLOY_AUTH_TOKEN }}
```

## Worker Node Authentication

The worker node uses a long-lived bearer token stored at
`/etc/ploy/bearer-token` in the node container. The compose stack mounts the host
file selected by `WORKER_TOKEN_PATH`.

## Token Security

### Best Practices

1. **Never commit tokens to version control**: Use environment variables or secret management systems.
2. **Use short-lived tokens for testing**: Set `--expires 1d` or `--expires 1h` for temporary access.
3. **Rotate tokens regularly**: Create new tokens and revoke old ones periodically.
4. **Use separate tokens for different environments**: Don't reuse production tokens in staging/dev.
5. **Monitor token usage**: Check `last_used_at` timestamps to identify unused tokens.
6. **Revoke compromised tokens immediately**: If a token is leaked, revoke it and create a new one.

### Token Storage

- **Local development**: Export `PLOY_SERVER_URL` and `PLOY_AUTH_TOKEN` in your shell or shell profile.
- **CI/CD**: Use CI/CD secret management (GitLab CI/CD variables, GitHub Secrets, etc.).
- **Production servers**: Use a secrets management service (HashiCorp Vault, AWS Secrets Manager, etc.).

### Auditing

Monitor token usage in the database:

```sql
-- Find tokens that haven't been used recently
SELECT token_id, role, username, description, issued_at, last_used_at
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

Update `PLOY_AUTH_TOKEN` with the new token.

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
  "username": "ci-bot",
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
  "username": "ci-bot",
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
      "username": "ci-bot",
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

- [Environment Variables](../envs/README.md) - Configuration reference
- [Troubleshooting Guide](bearer-token-troubleshooting.md) - Common authentication issues
