# Create GitLab Merge Requests with Ploy

Overview
- Ploy can automatically create GitLab merge requests (MRs) when Mods runs complete
- MRs can be triggered on success, failure, or both
- GitLab credentials can be configured globally on the control plane or overridden per run

Prerequisites
- A GitLab instance (self-hosted or gitlab.com)
- A Personal Access Token (PAT) with `api` scope
- A local Ploy cluster running (see `deploy/local/run.sh`) and accessible via `ploy` CLI (`PLOY_CONFIG_HOME=$PWD/deploy/local/cli`)

## Method 1: Configure Global GitLab Credentials (Recommended)

Configure GitLab credentials once on the control plane, and all Mods runs can use them automatically.

### Step 1: Create a GitLab configuration file

Create a JSON file with your GitLab domain and PAT:

```bash
cat > gitlab-config.json <<'EOF'
{
  "domain": "https://gitlab.com",
  "token": "glpat-xxxxxxxxxxxxxxxxxxxx"
}
EOF
chmod 600 gitlab-config.json
```

For self-hosted GitLab:
```json
{
  "domain": "https://gitlab.example.com",
  "token": "glpat-xxxxxxxxxxxxxxxxxxxx"
}
```

### Step 2: Apply the configuration to the control plane

```bash
ploy config gitlab set --file gitlab-config.json
```

Verify the configuration (token will be redacted):
```bash
ploy config gitlab show
# Output:
# Domain: https://gitlab.com
# Token:  glpat-xx...
```

### Step 3: Run a Mod with MR creation

Source branch naming
- Ploy uses the effective target ref as the MR source branch. When you pass `--repo-target-ref`, that value is used directly. When you omit it, the node derives a default of `ploy/{run_name|run_id}` using the run name when set (e.g., batch name) or the run ID (KSUID string) otherwise. The base branch remains the one provided via `--repo-base-ref` (commonly `main`).

Create an MR on success (capture server-assigned run ID via JSON):
```bash
RUN_ID=$(ploy mig run --json \
  --repo-url https://gitlab.com/yourorg/yourproject.git \
  --repo-base-ref main \
  --repo-target-ref workflow/upgrade-java-17 \
  --mr-success \
  --follow | jq -r '.run_id')
```

Create an MR on failure (useful for debugging):
```bash
RUN_ID=$(ploy mig run --json \
  --repo-url https://gitlab.com/yourorg/yourproject.git \
  --repo-base-ref main \
  --repo-target-ref workflow/debug-build-failure \
  --mr-fail \
  --follow | jq -r '.run_id')
```

Create an MR in both success and failure cases and capture MR URL too:
```bash
read RUN_ID MR_URL < <(ploy mig run --json \
  --repo-url https://gitlab.com/yourorg/yourproject.git \
  --repo-base-ref main \
  --repo-target-ref workflow/experiment \
  --mr-success \
  --mr-fail \
  --follow | jq -r '[.run_id, .mr_url] | @tsv')
echo "Run: $RUN_ID"
echo "MR:     ${MR_URL:-<none>}"
```

### Step 4: View the MR URL

If you only captured the run, you can inspect to retrieve MR URL later via the HTTP API:
```bash
curl -sk "$PLOY_CONTROL_PLANE_URL/v1/runs/$RUN_ID/status" | jq '.metadata.mr_url'
```

## Method 2: Per-Run GitLab Credentials

Override the global GitLab configuration for a single run using flags, capture run+MR via JSON:

```bash
read RUN_ID MR_URL < <(ploy mig run --json \
  --repo-url https://gitlab.com/yourorg/yourproject.git \
  --repo-base-ref main \
  --repo-target-ref workflow/upgrade \
  --gitlab-pat glpat-xxxxxxxxxxxxxxxxxxxx \
  --gitlab-domain https://gitlab.com \
  --mr-success \
  --follow | jq -r '[.run_id, .mr_url] | @tsv')
```

This is useful for:
- Testing with a different GitLab instance
- Using a project-specific PAT with restricted permissions
- Overriding temporarily without changing the global config

Precedence: per-run flags always override the control plane global config.

Note: `--gitlab-domain` accepts either a bare host (e.g., `gitlab.com`) or a full URL (e.g., `https://gitlab.com`). Ploy normalizes either form for API calls.

Repo URL schemes
- For pushing the MR branch, Ploy uses PAT-based HTTPS. If your `repo_url` is `https://...`, it is used as-is. If it is `ssh://git@host/...`, Ploy synthesizes an HTTPS remote using the configured GitLab domain and project path. `file://` repositories are not supported for MR creation.

## Security Notes

- PATs are never logged or written to disk on worker nodes
- The CLI redacts tokens in all output
- Tokens are transmitted securely via mTLS from control plane to nodes
- Store your `gitlab-config.json` file securely with `chmod 600`
- Consider using `gitlab.token_file` in the control plane config for additional security (see docs/envs/README.md)

## Validate Configuration Without Saving

Test your JSON configuration file before applying it:

```bash
ploy config gitlab validate --file gitlab-config.json
# Output:
# GitLab configuration is valid
```

## Troubleshooting

**MR not created:**
- Verify GitLab credentials with `ploy config gitlab show`
- Check that the PAT has `api` scope
- Ensure the repository URL is accessible and the PAT has write access
- Inspect the run logs for GitLab API errors

**Authentication errors:**
- Verify the domain URL includes the scheme (https:// or http://)
- For self-hosted GitLab, ensure the domain is reachable from worker nodes
- Check that the PAT is not expired

**Push errors:**
- Ensure the target branch does not already exist or is pushable
- Verify the repository URL is correct and accessible
- Check that the PAT has `write_repository` permissions

## Example Workflow: OpenRewrite Java Upgrade

```bash
# 1. Configure GitLab once
cat > gitlab-config.json <<'EOF'
{
  "domain": "https://gitlab.com",
  "token": "glpat-your-token-here"
}
EOF
ploy config gitlab set --file gitlab-config.json

# 2. Run OpenRewrite to upgrade Java 17
read RUN_ID MR_URL < <(ploy mig run --json \
  --repo-url https://gitlab.com/yourorg/spring-petclinic.git \
  --repo-base-ref main \
  --repo-target-ref workflow/java-17-upgrade \
  --mr-success \
  --follow | jq -r '[.run_id, .mr_url] | @tsv')

# 3. View the MR (source branch will be ploy-$RUN_ID)
echo "Run: $RUN_ID"; echo "MR: ${MR_URL:-<none>}"
# Copy the MR URL and review in GitLab

# 4. If tests pass, merge the MR
# Otherwise, iterate with additional Mods runs
```

## Batch Workflows with MR Creation

Batch runs enable applying the same mod spec to multiple repositories, with MR creation
per repo. Each `run_repo` can produce its own merge request.

### Create Batch with MR-on-Success

```bash
# 1. Configure GitLab credentials once.
ploy config gitlab set --file gitlab-config.json

# 2. Create a named batch.
ploy mig run --spec mod.yaml --name java17-fleet --mr-success

# 3. Add repositories — each will create an MR on success.
ploy mig run repo add \
  --repo-url https://gitlab.com/org/service-a.git \
  --base-ref main \
  --target-ref workflow/java-17-upgrade \
  java17-fleet

ploy mig run repo add \
  --repo-url https://gitlab.com/org/service-b.git \
  --base-ref main \
  --target-ref workflow/java-17-upgrade \
  java17-fleet

# 4. Follow batch progress.
ploy run logs java17-fleet
```

### Restart a Repo with a Hotfix Branch

If one repository fails due to a missing fix, restart it with a different branch:

```bash
# Repo IDs are NanoID(8) strings (e.g., "a1b2c3d4").
ploy mig run repo restart \
  --repo-id <repo-id> \
  --target-ref hotfix \
  java17-fleet
```

When the restarted job succeeds, an MR is created for the `hotfix` branch merge.

### Inspect Batch Status

Use `ploy run status` and `ploy mig run repo status` to inspect batch state:

```bash
# Batch-level status and repo counts:
ploy run status java17-fleet

# Per-repo status within the batch:
ploy mig run repo status java17-fleet
```

See `cmd/ploy/README.md` § "Batched Mod Runs" for the full batch command reference.

### Pull Changes Locally (Alternative to MR)

Instead of reviewing changes via GitLab MRs, you can pull Mods-generated diffs directly
into your local repository. This is useful for manual inspection, local testing, or
when MR creation is not needed.

```bash
# After a batch run completes, pull changes locally:
cd service-a
ploy mig pull java17-fleet

# Preview what would be pulled:
ploy mig pull --dry-run java17-fleet
```

The `mod pull` command:
1. Resolves the run using your local git remote URL.
2. Creates a new branch at the run's pinned commit.
3. Applies all stored Mods diffs to reconstruct the changes.

This approach complements MR-based workflows—you can use MRs for production changes
while using `mod pull` for local development and testing.

See `cmd/ploy/README.md` § "Pull Mods Changes Locally" for detailed usage.

## Related Documentation

- [docs/envs/README.md](../envs/README.md) — Environment variables and control plane config options
- [docs/how-to/publish-migs.md](publish-migs.md) — Publishing custom Mods images
