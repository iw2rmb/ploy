# Deploy a Ploy Cluster (Server/Node Architecture)

This guide describes how to deploy a Ploy cluster using the server/node architecture
with bearer token authentication and bootstrap token provisioning, as outlined in `README.md`
and implemented as of November 2025.
The deployment separates control-plane (`ployd` server) from worker execution (`ployd-node`) and
assumes a 1x server + 2x node layout.

**Authentication Model**:
- **Bearer tokens** for CLI authentication (JWT-based)
- **Bootstrap tokens** for node provisioning (short-lived, single-use)
- **Plain HTTP** for ployd (HTTPS termination at load balancer)
- **Certificate-based authentication** for nodes (after bootstrap)

**Note**: This replaces the legacy mTLS-only authentication. See `README.md` for architecture details.

## Prerequisites

- SSH access to all hosts with sudo privileges (default user `root`, port `22`).
- Go 1.25+ installed locally for building binaries.
- Docker Engine 28.0+ on worker nodes for job execution (installed automatically during `ploy node add`).
- PostgreSQL 14+ (installed automatically on the server host when `--postgresql-dsn` is omitted).
- Build the CLI and binaries locally: `make build` (CLI placed at `dist/ploy`).

Related env vars are documented in `docs/envs/README.md` (PostgreSQL DSN, PKI, optional DockerHub/OpenAI keys).

## Deployment Steps

### 1. Deploy the Control-Plane Server

Use `ploy server deploy` to install the control-plane on a VPS:

```bash
dist/ploy server deploy --address <host-or-ip>
```

This command:
- Copies the `ployd` server binary over SSH to `/tmp/ployd-{random}`, then installs it to `/usr/local/bin/ployd` (mode 0755).
- Generates a cluster Certificate Authority (CA) locally (still required for node certificate issuance).
- Generates a `cluster_id` (used for PKI and local descriptors). No row is written to PostgreSQL during bootstrap.
- Generates a secure random JWT signing secret for bearer token authentication.
- Creates `/etc/ploy/` and `/etc/ploy/pki/` directories on the remote host.
- Writes CA certificate to `/etc/ploy/pki/ca.crt` (mode 644) for signing node certificates.
- Writes CA private key to `/etc/ploy/pki/ca.key` (mode 600).
- If `--postgresql-dsn` is **not** provided, installs PostgreSQL on the VPS, creates database `ploy` and user `ploy` with a randomly generated 32-character hex password, and exports `PLOY_POSTGRES_DSN` in the format `postgres://ploy:{PASSWORD}@localhost:5432/ploy?sslmode=disable`. The bootstrap writes this DSN as a literal value into `/etc/ploy/ployd.yaml`.
- Writes server configuration to `/etc/ploy/ployd.yaml` with the following structure:
  - `http.listen: 127.0.0.1:8080` (plain HTTP, bind to localhost only)
  - `metrics.listen: :9100`
  - `auth.bearer_tokens.enabled: true`
  - `postgres.dsn: ${PLOY_POSTGRES_DSN:-}` (expanded at bootstrap time to a literal DSN in the file)
  - Note: HTTPS termination is expected at a load balancer; ployd accepts plain HTTP.
- Installs systemd unit `/etc/systemd/system/ployd.service` with:
  - `ExecStart=/usr/local/bin/ployd`
  - `Restart=always`, `RestartSec=5`
  - `Environment=PLOYD_CONFIG_PATH=/etc/ploy/ployd.yaml`
  - `Environment=PLOY_AUTH_SECRET=<generated-secret>` (JWT signing secret)
  - `Environment=PLOY_SERVER_CA_CERT=<ca-cert-pem>` (for node CSR signing)
  - `Environment=PLOY_SERVER_CA_KEY=<ca-key-pem>` (for node CSR signing)
  - `After=network.target postgresql.service`
- Runs `systemctl daemon-reload` and `systemctl enable --now ployd.service`.
- Creates an initial admin token and saves it to the local cluster descriptor.

At the end of bootstrap, a summary is printed showing the config path, PKI directory, detected certificate files, the systemd service name (with active/enabled status), and helpful commands for viewing logs and checking status, for example:

```
========================================
Bootstrap completed successfully.
========================================

Configuration:
  Config file: /etc/ploy/ployd.yaml
  PKI directory: /etc/ploy/pki
    - CA cert: /etc/ploy/pki/ca.crt
    - Server cert: /etc/ploy/pki/server.crt
    - Server key: /etc/ploy/pki/server.key

Service:
  Service name: ployd.service
  Status: active
  Enabled: enabled

To view logs:
  journalctl -u ployd.service -f

To check status:
  systemctl status ployd.service
```

**Optional flags:**
- `--postgresql-dsn <dsn>` — Use an external PostgreSQL instance instead of installing locally.
- `--user <name>` / `--ssh-port <port>` / `--identity <path>` — Override SSH connection parameters.
- `--ployd-binary <path>` — Explicit path to the `ployd` server binary to upload (defaults to alongside the CLI).
 - `--reuse[=true|false]` — When true (default), attempts to detect an existing cluster on the host and reuse its CA and server certificate. When false, skips detection.
 - `--force-new-ca` — Force generation of a new cluster CA and server certificate, even if a cluster is detected (overrides `--reuse`).
 - `--refresh-admin-cert` — Refresh the local CLI admin mTLS bundle for the default cluster by generating a CSR and calling the server's `/v1/pki/sign/admin` endpoint. Writes `~/.config/ploy/certs/<cluster>-{ca,admin}.{crt,key}` and updates the default descriptor's `ca_path/cert_path/key_path`. Intended for reuse flows where the server already has a CA, and your workstation needs a fresh admin certificate. Note: the server must permit the request (mTLS in production; tests may run with an insecure authorizer).
 - `--dry-run` — Print detected cluster state and the exact set of planned actions without making any changes.

#### Dry‑run preview

To verify connectivity, detection, and the bootstrap plan without changing the host, run:

```bash
dist/ploy server deploy --address <host-or-ip> --dry-run
```

Output includes a clear `DRY RUN` header, whether an existing cluster was detected and reused, certificate subjects for a new deployment, PostgreSQL handling, and the full list of planned steps. The command exits with status 0 and does not modify remote state.

Example:

```bash
dist/ploy server deploy --address 203.0.113.42
```

### 2. Add Worker Nodes

Use `ploy node add` to register worker nodes with the cluster:

```bash
dist/ploy node add --cluster-id <cluster-id> --address <host-or-ip> --server-url https://<load-balancer-host>
```

This command implements the bootstrap token flow:

**CLI-side actions:**
- Generates a unique `node_id` (UUID).
- Requests a short-lived bootstrap token from the server (`POST /v1/bootstrap/tokens`).
- Copies the `ployd-node` binary over SSH to `/tmp/ployd-{random}`, then installs it to `/usr/local/bin/ployd-node` (mode 0755).
- Writes the bootstrap token securely to `/run/ploy/bootstrap-token` (mode 600) on the remote host.
- Writes CA certificate to `/etc/ploy/pki/ca.crt` (mode 644) for server verification.
- Creates `/etc/ploy/` and `/etc/ploy/pki/` directories on the remote host.

**Node-side bootstrap (on first start):**
- Checks for existing certificate at `/etc/ploy/pki/node.{crt,key}`.
- If certificates don't exist:
  1. Reads bootstrap token from `/run/ploy/bootstrap-token`.
  2. Generates private key and CSR locally.
  3. Exchanges bootstrap token for signed certificate (`POST /v1/pki/bootstrap`).
  4. Writes certificate to `/etc/ploy/pki/node.crt` and key to `/etc/ploy/pki/node.key` (mode 600).
  5. Deletes the bootstrap token file.
  6. Proceeds with normal operation.

**Installed configuration:**
- Writes node configuration to `/etc/ploy/ployd-node.yaml` with the following structure:
  - `server_url: <load-balancer-url>` (HTTPS)
  - `node_id: <generated-uuid>`
  - `cluster_id: <cluster-id>`
  - `http.listen: :8444`
  - `heartbeat.interval: 30s`, `heartbeat.timeout: 10s`
- Installs systemd unit `/etc/systemd/system/ployd-node.service` with:
  - `ExecStart=/usr/local/bin/ployd-node`
  - `Restart=always`, `RestartSec=5`
  - `After=network.target`
- Runs `systemctl daemon-reload` and `systemctl enable --now ployd-node.service`.

Example:

```bash
dist/ploy node add --cluster-id alpha-cluster --address 203.0.113.43 --server-url https://ploy.example.com
dist/ploy node add --cluster-id alpha-cluster --address 203.0.113.44 --server-url https://ploy.example.com
```

**Security notes:**
- Bootstrap tokens expire after 15 minutes (configurable).
- Bootstrap tokens are single-use (marked as used after successful cert issuance).
- The token is written to `/run/ploy/` (tmpfs on most systems) and deleted immediately after use.
- The server validates that the CSR CN matches the `node_id` in the bootstrap token.

This step also installs Docker on each node (via apt/yum or get.docker.com), writes `/etc/ploy/ployd-node.yaml` with the
literal `server_url` and `node_id`, installs and starts `ployd-node.service`, and enables the Docker daemon.

### 3. Submit a Run

Once the server and at least one node are deployed, submit a Mods run:

```bash
dist/ploy mod run --repo-url https://github.com/example/repo.git \
  --repo-base-ref main --repo-target-ref feature-branch \
  --follow
```

The server schedules the run, and a node claims it, clones the repository shallow, executes the build gate,
and uploads logs/diffs/artifacts to PostgreSQL.

## Reuse Existing Cluster

When redeploying a Ploy server to a host that already contains a cluster, the deploy command automatically detects and reuses the existing cluster CA and server identity. This enables **idempotent** deployments: running `ploy server deploy` multiple times against the same host will not clobber PKI material or cluster identity.

### How Detection Works

The deploy command probes the target host for:
- `/etc/ploy/pki/ca.crt` — Existing cluster CA certificate
- `/etc/ploy/ployd.yaml` — Existing server configuration

When both are found, the command:
1. Parses the server certificate subject (CN: `ployd-<clusterID>`) to extract the cluster ID.
2. Skips CA generation and server certificate issuance.
3. Skips writing PKI files to `/etc/ploy/pki/` (bootstrap script detects `/etc/ploy/pki/ca.key` and omits CA/server writes).
4. Uses the existing cluster ID for local descriptor updates.
5. Restarts the `ployd.service` with the existing configuration.

### Flags

- **`--reuse`** (default: `true`) — Enable detection and reuse of an existing cluster. When detection succeeds, the CA and server certificate are preserved.
- **`--force-new-ca`** — Force generation of a new cluster CA and server certificate, even if an existing cluster is detected. This overrides `--reuse` and is useful for cluster reinitialization. **Warning**: This invalidates all existing node certificates; nodes must be re-added.
- **`--refresh-admin-cert`** — Generate a new admin mTLS certificate bundle for the CLI. This flag is intended for reuse scenarios where the server already has a CA, but your local workstation needs a fresh admin certificate. The command generates a CSR locally and submits it to the server's `/v1/pki/sign/admin` endpoint. The resulting certificate and CA are written to `~/.config/ploy/certs/<cluster>-{ca,admin}.{crt,key}`, and the default descriptor's `ca_path`, `cert_path`, and `key_path` are updated.

### Expected Outputs

#### Reuse (Default Behavior)

When deploying to a host with an existing cluster (with `--reuse=true` or omitted):

```
Detecting existing cluster on 203.0.113.42...
Found existing cluster: alpha-cluster
Reusing CA and server certificate.

Updating server binary...
Restarting ployd.service...

========================================
Bootstrap completed successfully.
========================================

Configuration:
  Config file: /etc/ploy/ployd.yaml
  PKI directory: /etc/ploy/pki
    - CA cert: /etc/ploy/pki/ca.crt (reused)
    - Server cert: /etc/ploy/pki/server.crt (reused)
    - Server key: /etc/ploy/pki/server.key (reused)

Service:
  Service name: ployd.service
  Status: active
  Enabled: enabled

To view logs:
  journalctl -u ployd.service -f

To check status:
  systemctl status ployd.service
```

#### Force New CA

When deploying with `--force-new-ca`:

```
Forcing new cluster CA and server certificate.
Generating new cluster CA...
Issuing new server certificate...

Installing server binary...
Writing PKI files to /etc/ploy/pki...
Creating systemd service...

========================================
Bootstrap completed successfully.
========================================

Configuration:
  Config file: /etc/ploy/ployd.yaml
  PKI directory: /etc/ploy/pki
    - CA cert: /etc/ploy/pki/ca.crt
    - Server cert: /etc/ploy/pki/server.crt
    - Server key: /etc/ploy/pki/server.key

Service:
  Service name: ployd.service
  Status: active
  Enabled: enabled

Warning: All existing node certificates are now invalid and must be re-issued.

To view logs:
  journalctl -u ployd.service -f
```

#### Refresh Admin Certificate

When deploying with `--refresh-admin-cert`:

```
Detecting existing cluster on 203.0.113.42...
Found existing cluster: alpha-cluster
Reusing CA and server certificate.

Refreshing admin certificate...
Generating admin CSR...
Submitting CSR to /v1/pki/sign/admin...
Writing admin certificate to ~/.config/ploy/certs/alpha-cluster-admin.crt
Writing admin key to ~/.config/ploy/certs/alpha-cluster-admin.key
Writing CA to ~/.config/ploy/certs/alpha-cluster-ca.crt
Updating descriptor at ~/.config/ploy/clusters/alpha-cluster.json

Admin certificate refreshed successfully.

========================================
Bootstrap completed successfully.
========================================
```

### Use Cases

- **Idempotent deployment**: Redeploy a server (e.g., after a binary upgrade) without changing cluster identity or invalidating node certificates.
- **Workstation refresh**: Use `--refresh-admin-cert` to obtain a new admin certificate when moving to a new machine or after your local certificate expires.
- **Cluster reinitialization**: Use `--force-new-ca` to start fresh (requires re-adding all nodes).

## VPS Lab Walkthrough (1× Server, 2× Nodes)

Use the shared VPS lab nodes from `AGENTS.md`:
- A (server): `45.9.42.212`
- B (node):   `193.242.109.13`
- C (node):   `45.130.213.91`

### Automated Verification

An automated verification script is available to validate the complete walkthrough:

```bash
# Validate prerequisites and SSH connectivity (no deployment)
make vps-lab-walkthrough-dry-run

# Run full deployment walkthrough
make vps-lab-walkthrough
```

The script (`scripts/vps-lab-walkthrough.sh`) performs all steps below automatically and verifies:
- Local binaries are built
- SSH connectivity to all hosts
- Server deployment and service status
- Node provisioning and service status
- PKI and configuration files are in place
- API endpoints are listening

### Manual Steps

If running manually instead of using the automated script:

- Build CLI/binaries locally: `make build` (creates `dist/ploy`, `dist/ployd`, `dist/ployd-linux`, `dist/ployd-node`, `dist/ployd-node-linux`).
- Deploy server on A (installs PostgreSQL if DSN omitted):
  - `dist/ploy server deploy --address 45.9.42.212`
  - The command prints the generated `cluster_id` and persists a local descriptor under `~/.config/ploy/clusters/`.
    - Current default cluster: `cat ~/.config/ploy/clusters/default` → `<cluster-id>`
    - Full descriptor: `~/.config/ploy/clusters/<cluster-id>.json`
- Add nodes on B and C (replace `<cluster-id>` with the value from the previous step):
  - `dist/ploy node add --cluster-id <cluster-id> --address 193.242.109.13 --server-url https://45.9.42.212:8443`
  - `dist/ploy node add --cluster-id <cluster-id> --address 45.130.213.91 --server-url https://45.9.42.212:8443`
- Smoke test a run (control plane at A on `:8443`):
  - `dist/ploy mod run --repo-url https://github.com/example/repo.git --repo-base-ref main --repo-target-ref feature --follow`

Firewall notes:
- Ensure TCP `8443` open from your workstation to A (server API, mTLS).
- Nodes must be able to reach A on `8443` (client mTLS to server) and fetch public Git repositories.

## Architecture Overview

- **ployd (server)**: Runs the control-plane API, scheduler, and PostgreSQL-backed storage. Exposes
  endpoints like `/v1/repos`, `/v1/mods`, and `/v1/pki/sign`.
- **ployd-node**: Lightweight worker that polls for runs, executes jobs in ephemeral workspaces,
  and streams results back to the server. Nodes use mTLS to communicate with the server.
- **Certificates**: The cluster CA issues all certificates. Nodes submit CSRs to `/v1/pki/sign` to
  obtain signed certificates with both `serverAuth` and `clientAuth` EKUs for bidirectional mTLS.

See also:
- `README.md` — Pivot architecture and current API surface.
- See `CHANGELOG.md` for status and acceptance summary.

## Operations

### Monitoring

- **Metrics**: The server exposes Prometheus metrics on port `:9100` (scrape `/metrics`). Node metrics endpoints are not emitted by `ployd-node` yet.
- **Logs**: Structured logs (slog) on stdout; capture with journalctl or systemd.
- **Database**: Monitor PostgreSQL disk usage, connection pool, and query performance.

### Follow Ticket Events

```bash
dist/ploy mods logs <ticket-id>
```

Logs stream via SSE from `/v1/mods/{id}/events`. Final logs are persisted in PostgreSQL.

### TTL and Cleanup

- The server runs a TTL worker to purge old `logs`, `diffs`, `events`, and `artifact_bundles` (default: 30 days).
- Prefer time-based partitioning and drop whole partitions daily for performance.
- See `README.md` for partition management examples.

### Certificate Rotation

To rotate node certificates:
1. Generate a new CSR on the node.
2. Submit to the server's `/v1/pki/sign` endpoint.
3. Install the new certificate and restart `ployd-node`.

The cluster CA itself should be rotated infrequently and requires reissuing all node certificates.

Server certificate auto‑renewal:
- The control plane server includes a lightweight PKI rotator. When the active
  certificate pointed to by `pki.certificate` is within the `pki.renew_before`
  window, the rotator attempts to re‑issue a new certificate with the same Subject
  and SANs, reusing the existing private key from `pki.key`.
- The rotator requires the cluster CA material via environment variables on the server:
  - `PLOY_SERVER_CA_CERT` — PEM CA certificate
  - `PLOY_SERVER_CA_KEY` — PEM CA private key
- Example `ployd.yaml` excerpt:

  pki:
    bundle_dir: /etc/ploy/pki
    certificate: /etc/ploy/pki/server.crt
    key: /etc/ploy/pki/server.key
    renew_before: 720h   # renew when <30d remain

If the CA variables are not present, rotation is skipped and a warning is logged so you can
renew via your external process.

Legacy endpoint notice:
- All `/v1/jobs*` endpoints and `/v1/mods/{ticket}/logs/stream` have been removed. Use `/v1/mods/*` and `/v1/nodes/*` equivalents:
  - Logs: `GET /v1/mods/{id}/events`
  - Heartbeat/complete: `POST /v1/nodes/{id}/heartbeat` and `POST /v1/nodes/{id}/complete`

## Connectivity and Authentication

- **Bearer Token Authentication**: CLI authenticates using JWT bearer tokens in the `Authorization: Bearer <token>` header.
- **Bootstrap Token Flow**: Nodes obtain certificates during initial provisioning using short-lived bootstrap tokens.
- **Node Certificates**: After bootstrap, nodes use mTLS with certificates issued via `/v1/pki/bootstrap` to communicate with the server.
- **CLI & descriptors**: The server bootstrap saves a local cluster descriptor at `~/.config/ploy/clusters/<cluster-id>.json` and marks it as default. Descriptors include:
  - `address` — Server URL (e.g., `https://ploy.example.com`)
  - `token` — Bearer token for authentication
  - `cluster_id` — Cluster identifier
  - `ssh_identity_path` — Path to SSH key for node provisioning (optional)
- **HTTPS Termination**: In production, a load balancer terminates HTTPS and forwards plain HTTP to ployd on `127.0.0.1:8080`.
- **Token Management**: Use `ploy token create`, `ploy token list`, and `ploy token revoke` commands to manage API tokens. See `docs/how-to/token-management.md` for details.

## Appendix: Environment Variables

Operator‑facing variables are listed in `docs/envs/README.md` (control plane URL override, PostgreSQL DSN, metrics ports,
optional DockerHub creds and OpenAI keys). During server bootstrap, `PLOY_POSTGRES_DSN` is set automatically when
PostgreSQL is installed on the host.
