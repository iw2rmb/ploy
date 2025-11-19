# Token Management Guide

This guide covers bearer token authentication in Ploy, including how to create, list, and revoke tokens for CLI access and node provisioning.

## Overview

Ploy uses JWT-based bearer tokens for authentication:

- **API Tokens**: Long-lived tokens for CLI authentication (default: 365 days)
- **Bootstrap Tokens**: Short-lived, single-use tokens for node provisioning (default: 15 minutes)

## Token Types

### API Tokens

API tokens are used by the CLI to authenticate with the control plane. Each token has:

- **Role**: Determines access level (`cli-admin`, `control-plane`, or `worker`)
- **Description**: Human-readable label for the token
- **Expiration**: When the token becomes invalid
- **Token ID**: Unique identifier for revocation

API tokens are stored in the `api_tokens` table in PostgreSQL.

### Bootstrap Tokens

Bootstrap tokens are used during node provisioning. They:

- Are tied to a specific `node_id`
- Expire after 15 minutes (configurable)
- Can only be used once
- Are automatically marked as used after certificate issuance

Bootstrap tokens are stored in the `bootstrap_tokens` table in PostgreSQL.

## Roles

Ploy supports three roles:

- **`cli-admin`**: Full administrative access, including token management and node provisioning
- **control-plane`**: Standard CLI access for running Mods and viewing cluster state
- **`worker`**: Node agent role (automatically assigned to bootstrap tokens)

## Managing API Tokens

### Create a Token

Create a new API token with the `ploy token create` command:

```bash
# Create a token for CI/CD with control-plane access
ploy token create --role control-plane --expires 90d --description "CI/CD pipeline"

# Create an admin token
ploy token create --role cli-admin --expires 365d --description "Admin workstation"

# Create a short-lived token for testing
ploy token create --role control-plane --expires 1d --description "Temporary test token"
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
ploy token list
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
ploy token revoke abc123
```

**Output:**
```
Token abc123 revoked successfully.

Any requests using this token will now fail with a 401 Unauthorized error.
```

You can also add a confirmation prompt:

```bash
ploy token revoke abc123 --confirm
```

**Revocation is immediate**: All in-flight requests using the revoked token will fail after the current request completes.

## Using Tokens

### CLI Configuration

Store your token in the cluster descriptor at `~/.config/ploy/clusters/<cluster-id>.json`:

```json
{
  "cluster_id": "alpha-cluster",
  "address": "https://ploy.example.com",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "ssh_identity_path": "~/.ssh/id_ed25519"
}
```

The CLI automatically uses this token for all requests.

### CI/CD Integration

For CI/CD pipelines, store the token as a secret and pass it via environment variable or config file:

**GitLab CI:**
```yaml
deploy:
  script:
    - mkdir -p ~/.config/ploy/clusters
    - echo "$PLOY_DESCRIPTOR" > ~/.config/ploy/clusters/prod.json
    - ploy mod run --repo-url $CI_PROJECT_URL --follow
  variables:
    PLOY_DESCRIPTOR: $PLOY_CLUSTER_DESCRIPTOR  # Set in GitLab CI/CD variables
```

**GitHub Actions:**
```yaml
- name: Configure Ploy
  run: |
    mkdir -p ~/.config/ploy/clusters
    echo "$PLOY_DESCRIPTOR" > ~/.config/ploy/clusters/prod.json
  env:
    PLOY_DESCRIPTOR: ${{ secrets.PLOY_CLUSTER_DESCRIPTOR }}

- name: Run Mods
  run: ploy mod run --repo-url ${{ github.repositoryUrl }} --follow
```

## Bootstrap Tokens

Bootstrap tokens are managed automatically by the CLI during node provisioning. You typically don't need to create them manually.

### How Bootstrap Tokens Work

When you run `ploy node add`:

1. CLI generates a unique `node_id`
2. CLI requests a bootstrap token from `POST /v1/bootstrap/tokens`
3. CLI writes the token to `/run/ploy/bootstrap-token` on the target host
4. Node agent reads the token, generates a CSR, and exchanges it for a certificate
5. Token is marked as used and deleted from the node

### Creating a Bootstrap Token Manually

In rare cases, you may need to create a bootstrap token manually:

```bash
# This endpoint is not exposed via CLI; use direct API call
curl -X POST https://ploy.example.com/v1/bootstrap/tokens \
  -H "Authorization: Bearer $PLOY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "node_id": "123e4567-e89b-12d3-a456-426614174000",
    "expires_in_minutes": 15
  }'
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "node_id": "123e4567-e89b-12d3-a456-426614174000",
  "expires_at": "2025-11-19T12:15:00Z"
}
```

## Token Security

### Best Practices

1. **Never commit tokens to version control**: Use environment variables or secret management systems.
2. **Use short-lived tokens for testing**: Set `--expires 1d` or `--expires 1h` for temporary access.
3. **Rotate tokens regularly**: Create new tokens and revoke old ones periodically.
4. **Use separate tokens for different environments**: Don't reuse production tokens in staging/dev.
5. **Monitor token usage**: Check `last_used_at` timestamps to identify unused tokens.
6. **Revoke compromised tokens immediately**: If a token is leaked, revoke it and create a new one.

### Token Storage

- **Local development**: Store in `~/.config/ploy/clusters/<cluster-id>.json` with file permissions `0600`.
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

-- View recent bootstrap token activity
SELECT node_id, issued_at, used_at, cert_issued_at
FROM bootstrap_tokens
WHERE issued_at > NOW() - INTERVAL '7 days'
ORDER BY issued_at DESC;
```

## Troubleshooting

### "Token expired"

Your token has exceeded its `expires_at` timestamp. Create a new token:

```bash
ploy token create --role cli-admin --expires 365d --description "Replacement token"
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

### Bootstrap token already used

Bootstrap tokens are single-use. If node provisioning fails after the token is used, you need to:

1. Request a new bootstrap token
2. Retry the provisioning

The `ploy node add` command handles this automatically by requesting a fresh token for each attempt.

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

### POST /v1/bootstrap/tokens

Create a bootstrap token for node provisioning.

**Authorization**: `control-plane` or `cli-admin` role required

**Request:**
```json
{
  "node_id": "123e4567-e89b-12d3-a456-426614174000",
  "expires_in_minutes": 15
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "node_id": "123e4567-e89b-12d3-a456-426614174000",
  "expires_at": "2025-11-19T12:15:00Z"
}
```

### POST /v1/pki/bootstrap

Exchange a bootstrap token for a node certificate.

**Authorization**: Bootstrap token in `Authorization: Bearer` header

**Request:**
```json
{
  "csr": "-----BEGIN CERTIFICATE REQUEST-----\nMIIBATCBqAIBADBb..."
}
```

**Response:**
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

## Related Documentation

- [Deploy a Cluster](deploy-a-cluster.md) - Full deployment guide with bootstrap token flow
- [Deploy Locally](deploy-locally.md) - Local Docker development setup
- [Environment Variables](../envs/README.md) - Configuration reference
- [Troubleshooting Guide](bearer-token-troubleshooting.md) - Common authentication issues
