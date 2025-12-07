# Create GitLab Merge Requests with Ploy

Overview
- Ploy can automatically create GitLab merge requests (MRs) when Mods runs complete
- MRs can be triggered on success, failure, or both
- GitLab credentials can be configured globally on the control plane or overridden per run

Prerequisites
- A GitLab instance (self-hosted or gitlab.com)
- A Personal Access Token (PAT) with `api` scope
- A Ploy cluster deployed and accessible via `ploy` CLI

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
- Ploy uses the effective target ref as the MR source branch. When you pass `--repo-target-ref`, that value is used directly. When you omit it, the node derives a default of `/mod/<run-id>` using the database run UUID. The base branch remains the one provided via `--repo-base-ref` (commonly `main`).

Create an MR on success (capture server-assigned run via JSON):
```bash
TICKET=$(ploy mod run --json \
  --repo-url https://gitlab.com/yourorg/yourproject.git \
  --repo-base-ref main \
  --repo-target-ref workflow/upgrade-java-17 \
  --mr-success \
  --follow | jq -r '.run_id')
```

Create an MR on failure (useful for debugging):
```bash
TICKET=$(ploy mod run --json \
  --repo-url https://gitlab.com/yourorg/yourproject.git \
  --repo-base-ref main \
  --repo-target-ref workflow/debug-build-failure \
  --mr-fail \
  --follow | jq -r '.run_id')
```

Create an MR in both success and failure cases and capture MR URL too:
```bash
read TICKET MR_URL < <(ploy mod run --json \
  --repo-url https://gitlab.com/yourorg/yourproject.git \
  --repo-base-ref main \
  --repo-target-ref workflow/experiment \
  --mr-success \
  --mr-fail \
  --follow | jq -r '[.run_id, .mr_url] | @tsv')
echo "Run: $TICKET"
echo "MR:     ${MR_URL:-<none>}"
```

### Step 4: View the MR URL

If you only captured the run, you can inspect to retrieve MR URL later:
```bash
ploy mod inspect "$TICKET"
# Output includes:
# MR: https://gitlab.com/yourorg/yourproject/-/merge_requests/123
```

## Method 2: Per-Run GitLab Credentials

Override the global GitLab configuration for a single run using flags, capture run+MR via JSON:

```bash
read TICKET MR_URL < <(ploy mod run --json \
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
read TICKET MR_URL < <(ploy mod run --json \
  --repo-url https://gitlab.com/yourorg/spring-petclinic.git \
  --repo-base-ref main \
  --repo-target-ref workflow/java-17-upgrade \
  --mr-success \
  --follow | jq -r '[.run_id, .mr_url] | @tsv')

# 3. View the MR (source branch will be ploy-$TICKET)
echo "Run: $TICKET"; echo "MR: ${MR_URL:-<none>}"
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
ploy mod run --spec mod.yaml --name java17-fleet --mr-success

# 3. Add repositories — each will create an MR on success.
ploy mod run repo add \
  --repo-url https://gitlab.com/org/service-a.git \
  --base-ref main \
  --target-ref workflow/java-17-upgrade \
  java17-fleet

ploy mod run repo add \
  --repo-url https://gitlab.com/org/service-b.git \
  --base-ref main \
  --target-ref workflow/java-17-upgrade \
  java17-fleet

# 4. Follow batch progress.
ploy runs follow java17-fleet
```

### Restart a Repo with a Hotfix Branch

If one repository fails due to a missing fix, restart it with a different branch:

```bash
ploy mod run repo restart \
  --repo-id <repo-uuid> \
  --target-ref hotfix \
  java17-fleet
```

When the restarted job succeeds, an MR is created for the `hotfix` branch merge.

### Inspect MRs from a Batch

Use `ploy mod inspect` to view MR URLs for each repo in the batch:

```bash
ploy mod inspect java17-fleet
# Output lists each run_repo with its MR URL (if created).
```

See `cmd/ploy/README.md` § "Batched Mod Runs" for the full batch command reference.

## Related Documentation

- [docs/envs/README.md](../envs/README.md) — Environment variables and control plane config options
- [docs/how-to/publish-mods.md](publish-mods.md) — Publishing custom Mods images
- ROADMAP.md Phase G — Implementation details for GitLab MR support
