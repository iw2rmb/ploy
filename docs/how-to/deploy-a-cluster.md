# Deploy a Ploy Cluster (Server/Node Architecture)

This guide describes how to deploy a Ploy cluster using the new server/node architecture
(Postgres + mTLS) as outlined in `SIMPLE.md`. The deployment separates control-plane
(ployd-server) from worker execution (ployd-node).

**Note**: This replaces the legacy etcd + IPFS Cluster stack. See `SIMPLE.md` for architecture details.

## Prerequisites

- SSH access to all hosts with sudo privileges.
- Go 1.25+ installed for building binaries.
- Docker Engine 28.0+ for node execution.
- PostgreSQL 14+ (installed automatically if `--postgresql-dsn` is not provided).

## Deployment Steps

### 1. Deploy the Control-Plane Server

Use `ploy server deploy` to install the control-plane on a VPS:

```bash
dist/ploy server deploy --address <host-or-ip>
```

This command:
- Installs `ployd-server` binary over SSH.
- Generates a cluster Certificate Authority (CA).
- Issues a server TLS certificate with appropriate SANs.
- Creates a `cluster_id` and records it in PostgreSQL and `/etc/ploy/cluster-id`.
- If `--postgresql-dsn` is **not** provided, installs PostgreSQL on the VPS and creates database `ploy`.
- Bootstraps the `ployd-server` systemd unit with `PLOY_SERVER_PG_DSN`.

**Optional flags:**
- `--postgresql-dsn <dsn>` — Use an external PostgreSQL instance instead of installing locally.
- `--cluster-id <id>` — Override the generated cluster ID (useful for deterministic deployments).

Example:

```bash
dist/ploy server deploy --address 203.0.113.42
```

### 2. Add Worker Nodes

Use `ploy node add` to register worker nodes with the cluster:

```bash
dist/ploy node add --cluster-id <cluster-id> --address <host-or-ip>
```

This command:
- Installs `ployd-node` binary over SSH.
- Generates a private key and CSR on the node.
- Submits the CSR to the server's `/v1/pki/sign` endpoint for signing.
- Installs the issued node certificate and CA bundle to `/etc/ploy/pki`.
- Records the node IP in the `nodes` table.
- Bootstraps the `ployd-node` systemd unit; node communicates with server via mTLS.

Example:

```bash
dist/ploy node add --cluster-id alpha-cluster --address 203.0.113.43
dist/ploy node add --cluster-id alpha-cluster --address 203.0.113.44
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

## Architecture Overview

- **ployd-server**: Runs the control-plane API, scheduler, and PostgreSQL-backed storage. Exposes
  endpoints like `/v1/repos`, `/v1/mods/crud`, `/v1/runs`, and `/v1/pki/sign`.
- **ployd-node**: Lightweight worker that polls for runs, executes jobs in ephemeral workspaces,
  and streams results back to the server. Nodes use mTLS to communicate with the server.
- **Certificates**: The cluster CA issues all certificates. Nodes submit CSRs to `/v1/pki/sign` to
  obtain signed certificates with both `serverAuth` and `clientAuth` EKUs for bidirectional mTLS.

## Operations

### Monitoring

- **Metrics**: Both server and nodes expose Prometheus metrics on port `:9100`.
- **Logs**: Structured logs (slog) on stdout; capture with journalctl or systemd.
- **Database**: Monitor PostgreSQL disk usage, connection pool, and query performance.

### Follow Run Logs

```bash
dist/ploy jobs follow <job-id>
```

Logs stream via SSE from `/v1/runs/{id}/events`. Final logs are persisted in PostgreSQL.

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
