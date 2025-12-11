# Update a Ploy Cluster (Server/Node Architecture)

This guide aligns with the architecture pivot in README.md: a single control‑plane server binary (`ployd`) and
one or more worker nodes (`ployd-node`). The server hosts the API/scheduler/PostgreSQL; nodes
execute jobs and communicate with the server over mTLS. The VPS lab layout we use:

- Server (A): 45.9.42.212
- Node (B):   193.242.109.13
- Node (C):   45.130.213.91

Update order: update the control‑plane server first, then roll the worker nodes.

## 1) Build Binaries

Use the Makefile to build the CLI and daemons:

```bash
make build
# Produces at least:
#   dist/ploy                  # CLI (runs on your workstation)
#   dist/ployd                 # server (if cmd/ployd present)
#   dist/ployd-linux           # server (linux)
#   dist/ployd-node            # worker agent
#   dist/ployd-node-linux      # worker agent (linux)
```

Optional: verify versions locally

```bash
./dist/ployd-linux --version || true
./dist/ployd-node-linux --version || true
```

## 2) Update the Control‑Plane Server (A)

Use the first‑class rollout command. Do not re‑run `ploy cluster deploy` for an
update (that regenerates PKI).

```bash
dist/ploy cluster rollout server \
  --address 45.9.42.212 \
  --binary dist/ployd-linux \
  --user root \
  --timeout 60
```

Flags:

- `--address` — target server IP or hostname
- `--binary` — path to the new `ployd` binary (Linux build)
- `--user` — SSH username (default `root`)
- `--identity` — SSH private key path (defaults to `~/.ssh/id_rsa`)
- `--ssh-port` — SSH port (default `22`)
- `--timeout` — rollout timeout in seconds (default `60`)

The rollout command will:

1. Copy the binary to the target server via SCP
2. Atomically replace the running binary
3. Restart the `ployd` service
4. Poll for health and verify the service is active
5. Verify the API port (8443) is listening

Dry‑run the server rollout to preview actions without changes:

```bash
dist/ploy cluster rollout server --address 45.9.42.212 --binary dist/ployd-linux --dry-run
```
Output lists the upload, install, restart, health checks, and port verification steps and ends with a "Dry run complete" notice.

Sanity checks:

```bash
ssh root@45.9.42.212 'systemctl status --no-pager ployd'
ssh root@45.9.42.212 'journalctl -u ployd -n 50 --no-pager'
curl -sk https://45.9.42.212:8443/v1/version | jq .
```

## 3) Rolling Update of Nodes

The `ploy cluster rollout nodes` command performs a safe, batched update of worker
nodes with automatic draining and health checks. Each node goes through the
following lifecycle:

1. **Drain** — mark the node as unavailable for new job claims
2. **Wait idle** — wait for active runs to complete
3. **Update binary** — upload and install the new `ployd-node` binary via SCP/SSH
4. **Restart service** — restart the `ployd-node` systemd unit
5. **Health check** — poll for service active and wait for heartbeat
6. **Undrain** — restore the node to available status

The command persists rollout state to `~/.config/ploy/rollout/state.json`, allowing
resumption if interrupted.

### Basic Examples

**Roll all nodes sequentially (safest, one at a time):**

```bash
dist/ploy cluster rollout nodes \
  --all \
  --binary dist/ployd-node-linux \
  --user root \
  --timeout 90
```

**Roll only nodes matching a pattern:**

```bash
dist/ploy cluster rollout nodes \
  --selector 'worker-*' \
  --binary dist/ployd-node-linux \
  --user root \
  --timeout 90
```

**Roll nodes in batches of 2 (faster, requires spare capacity):**

```bash
dist/ploy cluster rollout nodes \
  --all \
  --concurrency 2 \
  --binary dist/ployd-node-linux \
  --user root \
  --timeout 90
```

### Flags

- `--all` or `--selector '<pattern>'` — select nodes to roll (required,
  mutually exclusive)
- `--concurrency N` — number of nodes to update per batch (default: 1)
- `--binary` — path to the `ployd-node` binary (Linux build)
- `--user` — SSH username for node connection (default: `root`)
- `--identity` — SSH private key path (default: `~/.ssh/id_rsa`)
- `--ssh-port` — SSH port for node connection (default: `22`)
- `--timeout` — timeout in seconds per node rollout (default: `90`)
- `--max-attempts N` — maximum rollout attempts per node across resumes
  (default: `3`). Attempts increment on each failed node rollout and are
  persisted in `~/.config/ploy/rollout/state.json`.

### Concurrency Guidance

**Concurrency = 1 (default):**

- Safest option: only one node is drained at a time.
- Ensures maximum capacity remains available for active workloads.
- Recommended for clusters with N ≤ 3 nodes or when running near capacity.

**Concurrency = 2 or higher:**

- Faster rollout: multiple nodes are updated in parallel batches.
- Requires spare capacity to absorb workload from drained nodes.
- Recommended for clusters with N ≥ 4 nodes and <50% utilization.
- Example: with 6 nodes and concurrency=2, rollout completes in 3 batches
  instead of 6.

**Choosing concurrency:**

- Ensure `concurrency < total_nodes` to maintain cluster availability.
- Monitor active runs during rollout: `ploy run list --status running`.
- If runs queue or stall, reduce concurrency on next rollout.

### Resume on Failure

If the rollout fails mid-way (network issues, timeout, node offline), the command
saves state to `~/.config/ploy/rollout/state.json`. Re-run the same command to
resume from the last completed node.

Example:

```bash
# First attempt: fails on node 3 of 5
dist/ploy cluster rollout nodes --all --binary dist/ployd-node-linux

# Output: Rollout summary: 2 succeeded, 1 failed
# Resume state saved to: ~/.config/ploy/rollout/state.json

# Fix the issue (e.g., bring node back online), then resume:
dist/ploy cluster rollout nodes --all --binary dist/ployd-node-linux

# Output: [node-1] Already completed, skipping
#         [node-2] Already completed, skipping
#         [node-3] Starting rollout...
```

The state file is automatically removed on full success.

### Retries & Backoff

Rollout operations use exponential backoff for polling steps and a
persistent attempt counter for each node:

- Polling backoff: starts at 2s and doubles each retry up to 30s
  (cap). Server and node service health checks use this policy.
- Heartbeat wait: polls with the same backoff policy for up to 15
  attempts after restarting `ployd-node`.
- Per-node attempts: `--max-attempts` caps how many rollout attempts are
  made for a node across repeated runs. The resume state tracks the
  attempts and prevents further tries once the cap is reached.

### Sanity Checks

After rollout, verify each node is healthy:

```bash
ssh root@193.242.109.13 'systemctl status --no-pager ployd-node'
ssh root@193.242.109.13 'journalctl -u ployd-node -n 50 --no-pager'

ssh root@45.130.213.91 'systemctl status --no-pager ployd-node'
ssh root@45.130.213.91 'journalctl -u ployd-node -n 50 --no-pager'
```

Check that all nodes are undrained and reporting heartbeats via the API (future):

```bash
# Future: GET /v1/nodes will show drained=false and recent last_heartbeat
curl -sk https://45.9.42.212:8443/v1/nodes | jq '.[] | {name, drained, last_heartbeat}'
```

## 4) Cluster Verification

- Descriptor: confirm your CLI points at the updated server

```bash
cat ~/.config/ploy/clusters/default          # shows current cluster-id
cat ~/.config/ploy/clusters/<cluster-id>.json
```

- Submit a quick Mods run and follow logs:

```bash
./dist/ploy mod run \
  --repo-url https://github.com/example/repo.git \
  --repo-base-ref main \
  --repo-target-ref feature/update-rollout \
  --follow
```

- Check runs/events if needed:

```bash
./dist/ploy run events <run-id>
```

- **Batch run verification**: Test batch workflows to confirm multi-repo scheduling works:

```bash
# Create a batch (no repos yet).
./dist/ploy mod run --spec mod.yaml --name post-update-test

# Add a repo and restart it with a different branch to test the restart flow.
./dist/ploy mod run repo add \
  --repo-url https://github.com/example/repo.git \
  --base-ref main \
  --target-ref feature/verify-batch \
  post-update-test

# Repo IDs are NanoID(8) strings (e.g., "a1b2c3d4").
./dist/ploy mod run repo restart \
  --repo-id <repo-id> \
  --target-ref hotfix \
  post-update-test

# Follow batch logs.
./dist/ploy run events post-update-test
```

See `cmd/ploy/README.md` § "Batched Mod Runs" for full batch command reference.

## Rollback Tips

- Keep a backup of the previous binaries on each host (e.g., `/usr/local/bin/ployd.prev`, `/usr/local/bin/ployd-node.prev`).
- To rollback, swap the symlink or `install` the previous file and restart the corresponding service.

## Notes

- mTLS only: no SSH tunnels or bearer tokens are used at runtime. The CLI reads endpoint and CA bundle
  from the cluster descriptor under `~/.config/ploy/clusters/` (see `docs/envs/README.md`).
- PostgreSQL remains untouched during a binary update. If your server uses a local Postgres installed by
  the bootstrap, ensure the service is healthy before restarting `ployd`.
- Prefer rolling nodes one at a time to keep capacity available during updates.
- **Docker Engine v29.0+** is required on worker nodes. Nodes running older Docker versions may fail
  API negotiation (minimum API v1.44). See the "Docker Engine Upgrade" section below for upgrade steps.
- **GitLab MR integration**: Node agents use `gitlab.com/gitlab-org/api/client-go` for GitLab API interactions with automatic retry on transient failures (rate limits, 5xx errors, network issues). The client integrates with the shared backoff policy (`GitLabMRPolicy`: 4 max attempts, 1s/2s/4s backoff schedule with jitter) and automatically redacts Personal Access Tokens from all logs and error messages. See `cmd/ploy/README.md#gitlab-mr-integration` for retry behavior details and `docs/how-to/create-mr.md` for configuration examples.

---

## Docker Engine Upgrade

Worker nodes require **Docker Engine v29.0 or later** (API v1.44+). This section describes how to
upgrade nodes running older Docker versions.

### Prerequisites

Before upgrading Docker:

1. **Drain the node** to prevent new job claims during the upgrade:
   ```bash
   # Drain via rollout (node stops claiming new jobs).
   dist/ploy cluster rollout nodes --selector '<node-pattern>' --drain-only
   ```
   Alternatively, wait for active runs to complete naturally if the cluster has low activity.

2. **Verify current Docker version** on the target node:
   ```bash
   ssh root@<node-ip> 'docker version --format "Engine: {{.Server.Version}}, API: {{.Server.APIVersion}}"'
   ```
   If the output shows Engine v29.0+ and API v1.44+, no upgrade is needed.

### Upgrade Steps (Debian/Ubuntu)

Run these commands on each worker node via SSH:

```bash
# 1. Stop the node agent to prevent container operations during upgrade.
ssh root@<node-ip> 'systemctl stop ployd-node'

# 2. Remove old Docker packages (keeps configuration and images).
ssh root@<node-ip> 'apt-get remove -y docker docker-engine docker.io containerd runc || true'

# 3. Install Docker Engine v29 using the official convenience script.
#    The script auto-detects the OS and installs the latest stable release.
ssh root@<node-ip> 'curl -fsSL https://get.docker.com | sh'

# 4. Verify the new Docker version (should show 29.x or higher).
ssh root@<node-ip> 'docker version'

# 5. Restart the Docker daemon (usually automatic after install).
ssh root@<node-ip> 'systemctl enable docker && systemctl start docker'

# 6. Start the node agent.
ssh root@<node-ip> 'systemctl start ployd-node'
```

### Upgrade Steps (RHEL/CentOS/Rocky)

```bash
# 1. Stop the node agent.
ssh root@<node-ip> 'systemctl stop ployd-node'

# 2. Remove old Docker packages.
ssh root@<node-ip> 'yum remove -y docker docker-common docker-engine || true'

# 3. Install Docker Engine v29 using the convenience script.
ssh root@<node-ip> 'curl -fsSL https://get.docker.com | sh'

# 4. Verify the new version.
ssh root@<node-ip> 'docker version'

# 5. Start Docker and the node agent.
ssh root@<node-ip> 'systemctl enable docker && systemctl start docker'
ssh root@<node-ip> 'systemctl start ployd-node'
```

### Post-Upgrade Verification

After upgrading Docker on each node:

```bash
# 1. Verify Docker Engine version is 29.0 or higher.
ssh root@<node-ip> 'docker version --format "{{.Server.Version}}"'
# Expected: 29.0.0 or higher

# 2. Verify Docker API version is 1.44 or higher.
ssh root@<node-ip> 'docker version --format "{{.Server.APIVersion}}"'
# Expected: 1.44 or higher

# 3. Verify ployd-node is running and healthy.
ssh root@<node-ip> 'systemctl status ployd-node --no-pager'

# 4. Check node agent logs for errors.
ssh root@<node-ip> 'journalctl -u ployd-node -n 20 --no-pager'

# 5. Verify the node is sending heartbeats (future: via API).
#    For now, check logs for "heartbeat sent" or similar messages.
```

### Rollback (Emergency)

If Docker v29 causes issues, you can rollback to a specific version:

```bash
# Debian/Ubuntu: Install a specific older version (NOT recommended long-term).
ssh root@<node-ip> 'apt-cache madison docker-ce | head -5'  # List available versions
ssh root@<node-ip> 'apt-get install -y docker-ce=<version> docker-ce-cli=<version>'

# Note: Ploy requires v29.0+. Downgrading will cause API negotiation failures.
# Only use rollback as a temporary measure while investigating the root cause.
```

### Upgrade Checklist

| Step | Action | Verification |
|------|--------|--------------|
| 1 | Drain node | Node stops claiming new jobs |
| 2 | Wait for active runs | `ploy run list --node <id>` shows no running jobs |
| 3 | Stop ployd-node | `systemctl status ployd-node` shows inactive |
| 4 | Upgrade Docker | `docker version` shows v29.0+ |
| 5 | Start ployd-node | `systemctl status ployd-node` shows active |
| 6 | Verify heartbeat | Logs show successful heartbeat to control plane |
| 7 | Undrain node | Node resumes claiming jobs |

Cross-references:
- `docs/how-to/deploy-a-cluster.md` § "Docker Engine v29 Requirements" for new deployments.
- `GOLANG.md` § "Docker Engine Requirements" for SDK module and API version details.

---

## Appendix: Backdoor (Manual Commands)

If you need to bypass the CLI for troubleshooting or in very old environments, the
following manual commands replicate what `ploy cluster rollout server` does:

### Server Update (Manual)

```bash
scp -q dist/ployd-linux root@45.9.42.212:/usr/local/bin/ployd.new
ssh -q root@45.9.42.212 'install -m 0755 /usr/local/bin/ployd.new /usr/local/bin/ployd && rm -f /usr/local/bin/ployd.new && systemctl restart ployd && systemctl is-active --quiet ployd'
```

**Warning:** The manual approach lacks the health checks and retries that the rollout
command provides. Use the CLI command when possible.
Tip: wrapper for lab

If you prefer not to type the lab URL each time, use the wrapper:

```bash
scripts/ploy-lab.sh cluster rollout server --address 45.9.42.212 --binary dist/ployd-linux --user root
scripts/ploy-lab.sh cluster rollout nodes --all --binary dist/ployd-node-linux --user root
scripts/ploy-lab.sh mod run --repo-url https://github.com/example/repo.git --repo-base-ref main --repo-target-ref feature/x --follow
```

The wrapper sets `PLOY_CONTROL_PLANE_URL` to the lab endpoint and delegates to `dist/ploy`.
TLS credentials are still read from the active descriptor under `~/.config/ploy/clusters/`.
