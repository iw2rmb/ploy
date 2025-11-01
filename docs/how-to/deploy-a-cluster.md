# Deploy a Ploy Cluster (Server/Node Architecture)

This guide describes how to deploy a Ploy cluster using the new server/node architecture
(Postgres + mTLS) as outlined in `SIMPLE.md` and implemented per the slices in `ROADMAP.md`.
The deployment separates control-plane (`ployd` server) from worker execution (`ployd-node`) and
assumes a 1x server + 2x node layout.

**Note**: This replaces the legacy etcd + IPFS Cluster stack. See `SIMPLE.md` for architecture details.

## Prerequisites

- SSH access to all hosts with sudo privileges (default user `root`, port `22`).
- Go 1.25+ installed locally for building binaries.
- Docker Engine 28.0+ on worker nodes for job execution.
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
- Installs the `ployd` server binary over SSH.
- Generates a cluster Certificate Authority (CA).
- Issues a server TLS certificate with appropriate SANs.
- Creates a `cluster_id` and records it in PostgreSQL and `/etc/ploy/cluster-id`.
- If `--postgresql-dsn` is **not** provided, installs PostgreSQL on the VPS and creates database `ploy`.
- Bootstraps the `ployd` systemd unit with `PLOY_SERVER_PG_DSN`.

**Optional flags:**
- `--postgresql-dsn <dsn>` — Use an external PostgreSQL instance instead of installing locally.
- `--cluster-id <id>` — Override the generated cluster ID (useful for deterministic deployments).
- `--user <name>` / `--ssh-port <port>` / `--identity <path>` — Override SSH connection parameters.
- `--ployd-binary <path>` — Explicit path to the `ployd` server binary to upload (defaults to alongside the CLI).

Example:

```bash
dist/ploy server deploy --address 203.0.113.42
```

### 2. Add Worker Nodes

Use `ploy node add` to register worker nodes with the cluster:

```bash
dist/ploy node add --cluster-id <cluster-id> --address <host-or-ip> --server-url https://<server-host>:8443
```

This command:
- Installs `ployd-node` binary over SSH.
- Generates a node private key and CSR locally.
- Submits the CSR to the server's `/v1/pki/sign` endpoint for signing.
- Installs the issued node certificate, key, and CA bundle on the node under `/etc/ploy/pki`.
- Records the node IP in the `nodes` table.
- Bootstraps the `ployd-node` systemd unit; node communicates with server via mTLS.

Example:

```bash
dist/ploy node add --cluster-id alpha-cluster --address 203.0.113.43 --server-url https://203.0.113.42:8443
dist/ploy node add --cluster-id alpha-cluster --address 203.0.113.44 --server-url https://203.0.113.42:8443
```

### 3. Submit a Run

Once the server and at least one node are deployed, submit a Mods run:

```bash
dist/ploy mod run --repo-url https://github.com/example/repo.git \
  --repo-base-ref main --repo-target-ref feature-branch \
  --follow
```

The server schedules the run, and a node claims it, clones the repository shallow, executes the build gate,
and uploads logs/diffs/artifacts to PostgreSQL.

## VPS Lab Walkthrough (1× Server, 2× Nodes)

Use the shared VPS lab nodes from `AGENTS.md`:
- A (server): `45.9.42.212`
- B (node):   `46.173.16.177`
- C (node):   `81.200.119.187`

Steps:
- Build CLI/binaries locally: `make build` (creates `dist/ploy`, `dist/ployd`, `dist/ployd-linux`, `dist/ployd-node`, `dist/ployd-node-linux`).
- Deploy server on A (installs PostgreSQL if DSN omitted):
  - `dist/ploy server deploy --address 45.9.42.212`
  - The command prints the generated `cluster_id` and persists a local descriptor under `~/.config/ploy/clusters/`.
    - Current default cluster: `cat ~/.config/ploy/clusters/default` → `<cluster-id>`
    - Full descriptor: `~/.config/ploy/clusters/<cluster-id>.json`
- Add nodes on B and C (replace `<cluster-id>` with the value from the previous step):
  - `dist/ploy node add --cluster-id <cluster-id> --address 46.173.16.177`
  - `dist/ploy node add --cluster-id <cluster-id> --address 81.200.119.187`
- Smoke test a run (control plane at A on `:8443`):
  - `dist/ploy mod run --repo-url https://github.com/example/repo.git --repo-base-ref main --repo-target-ref feature --follow`

Firewall notes:
- Ensure TCP `8443` open from your workstation to A (server API, mTLS).
- Nodes must be able to reach A on `8443` (client mTLS to server) and fetch public Git repositories.

## Architecture Overview

- **ployd (server)**: Runs the control-plane API, scheduler, and PostgreSQL-backed storage. Exposes
  endpoints like `/v1/repos`, `/v1/mods/crud`, `/v1/jobs`, and `/v1/pki/sign`.
- **ployd-node**: Lightweight worker that polls for runs, executes jobs in ephemeral workspaces,
  and streams results back to the server. Nodes use mTLS to communicate with the server.
- **Certificates**: The cluster CA issues all certificates. Nodes submit CSRs to `/v1/pki/sign` to
  obtain signed certificates with both `serverAuth` and `clientAuth` EKUs for bidirectional mTLS.

See also:
- `SIMPLE.md` — Pivot architecture and API surface.
- `ROADMAP.md` — Implementation checklist and acceptance criteria.

## Operations

### Monitoring

- **Metrics**: Both server and nodes expose Prometheus metrics on port `:9100` (scrape `/metrics`).
- **Logs**: Structured logs (slog) on stdout; capture with journalctl or systemd.
- **Database**: Monitor PostgreSQL disk usage, connection pool, and query performance.

### Follow Run Logs

```bash
dist/ploy jobs follow <job-id>
```

Logs stream via SSE from `/v1/jobs/{id}/logs/stream`. Final logs are persisted in PostgreSQL.

### TTL and Cleanup

- The server runs a TTL worker to purge old `logs`, `diffs`, `events`, and `artifact_bundles` (default: 30 days).
- Prefer time-based partitioning and drop whole partitions daily for performance.
- See `SIMPLE.md` for partition management examples.

### Certificate Rotation

To rotate node certificates:
1. Generate a new CSR on the node.
2. Submit to the server's `/v1/pki/sign` endpoint.
3. Install the new certificate and restart `ployd-node`.

The cluster CA itself should be rotated infrequently and requires reissuing all node certificates.

## Connectivity and Authentication

- **mTLS Only**: All communication uses mutual TLS. Bearer tokens have been removed.
- **CLI**: Uses mTLS with operator client certificate (minted during `ploy server deploy`).
- **Nodes**: Use certificates issued via `/v1/pki/sign` to communicate with the server.
- **Descriptors**: The CLI reads endpoint + CA bundle from cluster descriptors saved during deployment.

## Appendix: Environment Variables

Operator‑facing variables are listed in `docs/envs/README.md` (control plane URL override, PostgreSQL DSN, metrics ports,
optional DockerHub creds and OpenAI keys). During server bootstrap, `PLOY_SERVER_PG_DSN` is set automatically when
PostgreSQL is installed on the host.
