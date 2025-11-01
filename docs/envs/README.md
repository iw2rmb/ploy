# Environment Variables

**Note: Postgres/mTLS Pivot (November 2025)**

As of the server/node pivot described in `SIMPLE.md`, the following legacy systems have been removed:
- **IPFS Cluster**: All `PLOY_IPFS_*` variables are no longer consumed by the codebase.
- **etcd**: All `PLOY_ETCD_*` variables are no longer consumed by the codebase.
- **Token-based auth**: Bearer tokens replaced with mTLS-only authentication.
- **Node labels**: Removed in favor of resource-snapshot scheduling.

This document tracks the environment variables that the server, node, and CLI
use after the pivot. Update this file whenever a new variable is introduced,
defaults change, or components adopt additional configuration.

## Dependencies

- [cmd/ploy/dependencies.go](../../cmd/ploy/dependencies.go) — runtime factories
  resolving control-plane endpoints via mTLS.

## CLI

- `PLOY_RUNTIME_ADAPTER` — Optional runtime adapter selector. Defaults to
  `local-step`. Other adapters (e.g., `k8s`, `nomad`) can plug in here; the CLI
  fails fast when an unknown adapter name is provided.
- `PLOY_ASTER_ENABLE` — Opt-in switch for the experimental Aster bundle
  integration. Current default: `unset` (Aster toggles stay disabled).
- `PLOY_CONTROL_PLANE_URL` — Optional override for the control-plane base URL when cached descriptors do not yet
  embed the endpoint (new workstation) or you need to target a secondary cluster explicitly. Descriptors discovered via
  `ploy server deploy` or `ploy node add` remain the default for CLI calls.
- `PLOY_BUILDGATE_JAVA_IMAGE` — Optional override for the Docker image used by the
  Java build gate executor when Gradle/Maven wrappers are not present in the workspace.
  Defaults to `maven:3-eclipse-temurin-17`.
- `DOCKERHUB_USERNAME` — Docker Hub namespace used by runner templates. Images resolve to
  `docker.io/$DOCKERHUB_USERNAME/<name>:latest`.
- `DOCKERHUB_PAT` — Docker Hub Personal Access Token used for non‑interactive `docker login`
  on worker nodes during bootstrap. If set on the node, bootstrap performs
  `echo "$DOCKERHUB_PAT" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin`.
- `MODS_IMAGE_PREFIX` — Optional absolute image prefix (e.g., `docker.io/org` or `ghcr.io/org`).
  Takes effect only when `DOCKERHUB_USERNAME` is unset.
- `PLOY_OPENAI_API_KEY` — Optional OpenAI API key propagated to Mods LLM lanes. When set on the control
  plane, the runner injects it into the `mods-llm` container as `OPENAI_API_KEY`. You can also set it on
  worker nodes via a systemd drop-in to make it available cluster-wide.
- `PLOYD_CONFIG_PATH` — When set during bootstrap, overrides the generated ployd configuration file
  location (default `/etc/ploy/ployd.yaml`).
- `PLOYD_HTTP_LISTEN` — Optional address override for the ployd HTTP API listener when bootstrap
  generates the initial configuration (default `0.0.0.0:8443`).
- `PLOYD_METRICS_LISTEN` — Optional override for the ployd Prometheus metrics listener (defaults to
  `:9100`).
  

## Worker Nodes

- `PLOY_CA_CERT_PEM` — Cluster CA presented to the node for mTLS trust (PEM-encoded).
  Required for node→server and server→node mTLS connections.
- `PLOY_SERVER_CERT_PEM` / `PLOY_SERVER_KEY_PEM` — The node's TLS certificate and key
  (CSR-signed by the control plane). Despite the name, bootstrap uses these variables
  for both server and node flows and writes to `/etc/ploy/pki/node.pem` and
  `/etc/ploy/pki/node-key.pem`.
- `PLOY_NODE_CONCURRENCY` — Maximum concurrent runs the node will execute (default: `1`).
- `PLOY_LIFECYCLE_NET_IGNORE` — Optional comma-separated list of network interface patterns (supports `*` globs) that the node lifecycle collector skips when computing throughput metrics. Example: `lo,cni*,docker*`.
  - Pin via systemd drop-in or in `ployd.yaml` under `environment:` e.g.:

    environment:
      PLOY_LIFECYCLE_NET_IGNORE: "docker*,veth*,br-*"

## E2E Harness

- `ploy mod run` executes Mods against the Ploy control plane; no tenant variable is required.
- `PLOY_E2E_TICKET_PREFIX` — Optional ticket ID prefix for Mods E2E runs
  (default `e2e`).
- `PLOY_E2E_REPO_OVERRIDE` — Optional Git repository override used by the Mods
  E2E scenarios in place of the default Java sample repo.
- `PLOY_E2E_GITLAB_TOKEN` — Optional GitLab PAT so the E2E harness can clean up
  branches after creating merge requests.
- `PLOY_E2E_LIVE_SCENARIOS` — Optional comma-separated scenario IDs that the
  live Mods smoke test should execute (defaults to `simple-openrewrite`).

 

## gapi

- No environment variables are active for gapi within this codebase; record
  future additions here once the API slices land.

## Control Plane

- `PLOY_CONTROL_PLANE_URL` — Optional control-plane base URL override used by
  the CLI and node bootstrap commands. When unset, the CLI derives the endpoint
  and CA bundle from the cached cluster descriptor created during
  `ploy server deploy`.

### Server (Control Plane)

- `PLOY_SERVER_HTTP_LISTEN` — Address the server listens on for HTTPS API/SSE (default: `:8443`).
- `PLOY_SERVER_METRICS_LISTEN` — Address for Prometheus metrics endpoint (default: `:9100`).
- `PLOY_SERVER_CLUSTER_ID` — Unique identifier for the cluster (set during `ploy server deploy`).
- `PLOY_SERVER_TLS_CERT` / `PLOY_SERVER_TLS_KEY` — PEM-encoded server TLS certificate and key
  for the HTTPS API. Issued by the cluster CA during `ploy server deploy`.

### PKI

- `PLOY_SERVER_CA_CERT` — PEM-encoded cluster CA certificate presented to nodes. Required for
  the `/v1/pki/sign` endpoint to return signed certificates.
- `PLOY_SERVER_CA_KEY` — PEM-encoded cluster CA private key used to sign node CSRs. Required
  alongside `PLOY_SERVER_CA_CERT` for `/v1/pki/sign`. When either value is missing, the server
  responds with `503 PKI not configured`.

## PostgreSQL

The control plane can use PostgreSQL via `pgx/v5` and `pgxpool`.

- `PLOY_SERVER_PG_DSN` — Primary DSN the server reads at startup to open a PostgreSQL pool.
  Example: `postgres://user:pass@localhost:5432/ploy?sslmode=disable`.
  When `ploy server deploy` runs without `--postgresql-dsn`, the bootstrap installs
  PostgreSQL on the VPS and derives a password‑based TCP DSN suitable for the
  root‑run `ployd` service, e.g.: `host=127.0.0.1 port=5432 user=ploy password=ploy dbname=ploy sslmode=disable`.
- `PLOY_POSTGRES_DSN` — Backward‑compatible alias recognized by `ployd` during the transition. Prefer
  `PLOY_SERVER_PG_DSN` going forward.
- `PLOY_TEST_PG_DSN` — Optional Postgres DSN used by `internal/store` integration tests. When unset, tests
  that require a live database are skipped.

Alternatively, you can specify the DSN in the config file under `postgres.dsn`. Environment variables take
precedence over the config file when both are present.

## Legacy (Removed November 2025)

The following variables are **no longer consumed** by the codebase after the Postgres/mTLS pivot:

### GitLab Signer (Removed)
- `PLOY_GITLAB_SIGNER_AES_KEY` — Removed (GitLab signer deleted).
- `PLOY_GITLAB_SIGNER_DEFAULT_TTL` — Removed.
- `PLOY_GITLAB_SIGNER_MAX_TTL` — Removed.
- `PLOY_GITLAB_API_BASE_URL` — Removed.
- `PLOY_GITLAB_ADMIN_TOKEN` — Removed.

### IPFS Cluster (Removed)
- `PLOY_IPFS_CLUSTER_API` — Replaced with PostgreSQL storage for diffs/logs/artifact bundles.
- `PLOY_IPFS_CLUSTER_TOKEN` — Token auth removed; mTLS only.
- `PLOY_IPFS_CLUSTER_USERNAME` / `PLOY_IPFS_CLUSTER_PASSWORD` — Removed.
- `PLOY_IPFS_CLUSTER_REPL_MIN` / `PLOY_IPFS_CLUSTER_REPL_MAX` — No IPFS replication.
- `PLOY_IPFS_CLUSTER_LOCAL` — Removed.
- `PLOY_IPFS_GATEWAY` — Removed.
- `PLOY_HYDRATION_PUBLISH_SNAPSHOT` — Removed (repos cloned shallow on-demand).
- `PLOY_ARTIFACT_PUBLISH` — Removed.

### etcd (Removed)
- `PLOY_ETCD_USERNAME` / `PLOY_ETCD_PASSWORD` — Replaced with PostgreSQL.
- `PLOY_ETCD_TLS_CA` / `PLOY_ETCD_TLS_CERT` / `PLOY_ETCD_TLS_KEY` — Removed.
- `PLOY_ETCD_TLS_SKIP_VERIFY` — Removed.

### SSH Tunnels (Removed)
- `PLOY_SSH_USER` — CLI uses direct HTTPS/mTLS.
- `PLOY_SSH_IDENTITY` — Removed.
- `PLOY_SSH_SOCKET_DIR` — Removed.
- `PLOY_CACHE_HOME` — Removed.
- `PLOY_TRANSFERS_BASE_DIR` — Removed (SSH-based artifact staging deleted).

### Other
- `PLOY_ARTIFACT_ROOT` — Local artifact caching removed; nodes use ephemeral workspaces.

## Related Docs

- [SIMPLE.md](../../SIMPLE.md) — Server/node pivot architecture
- [ROADMAP.md](../../ROADMAP.md) — Migration checklist
- [docs/how-to/deploy-a-cluster.md](../how-to/deploy-a-cluster.md) — Deployment guide
