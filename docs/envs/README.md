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
- `PLOY_CONFIG_HOME` — Optional override for the base directory where cluster descriptors
  are stored. When unset, the CLI falls back to `XDG_CONFIG_HOME/ploy` or `~/.config/ploy`.
  Priority: `PLOY_CONFIG_HOME` → `XDG_CONFIG_HOME/ploy` → `~/.config/ploy`.
- `XDG_CONFIG_HOME` — Standard XDG Base Directory specification variable. When set
  (and `PLOY_CONFIG_HOME` is not), the CLI uses `$XDG_CONFIG_HOME/ploy` for cluster
  descriptor storage. Falls back to `~/.config/ploy` when both are unset.

Local cluster descriptors (written under `~/.config/ploy/clusters/`) now embed TLS material used by the CLI for mTLS:
- `ca_path` — CA certificate used as root trust for the control-plane.
- `cert_path` — Client certificate presented by the CLI.
- `key_path` — Private key for the client certificate.
When these fields are present for the default cluster, the CLI enforces TLS 1.3 and uses mTLS for all control‑plane calls.
- `USER` — Standard Unix environment variable indicating the current user. The CLI
  reads this to populate the `Submitter` field when creating mod runs via `ploy mod run`.
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
- `PLOYD_CONFIG_PATH` — When set, provides the default ployd configuration file
  location (default `/etc/ploy/ployd.yaml`). The ployd flag `--config` overrides this
  environment variable when explicitly provided.
- `PLOYD_NODE_ID` — Node identifier for the ployd daemon. Set during bootstrap to a sanitized
  version of the node name. Note: currently exported by bootstrap but not consumed at runtime;
  node identity is specified in the node YAML (`node_id`).
- `PLOYD_HOME_DIR` — Home directory for the ployd daemon. Exported by bootstrap as `/root` for
  systemd context; not currently read by the codebase.
- `PLOYD_CACHE_HOME` — Cache directory for working data. Defaults to `/var/cache/ploy` when set
  by bootstrap. Used at runtime by the node agent for ephemeral workspaces.
- `PLOYD_METRICS_LISTEN` — Exported by bootstrap as `127.0.0.1:9101` for early scripts; not
  read by `ployd` at runtime. Use the YAML key `metrics.listen` (default `:9100`).


## Worker Nodes

- `PLOY_CA_CERT_PEM` — Cluster CA certificate presented to the node for mTLS trust (PEM-encoded).
  Required for node→server and server→node mTLS connections.
- `PLOY_CA_KEY_PEM` — Cluster CA private key (PEM-encoded). Set during bootstrap on the
  control-plane node to enable the `/v1/pki/sign` endpoint for signing node CSRs. Should
  only be present on the control-plane server; worker nodes do not require this variable.
- `PLOY_SERVER_CERT_PEM` / `PLOY_SERVER_KEY_PEM` — The node's TLS certificate and key
  (CSR-signed by the control plane). Despite the name, bootstrap uses these variables
  for both server and node flows and writes to `/etc/ploy/pki/node.crt` and
  `/etc/ploy/pki/node.key` on worker nodes.
- `concurrency` (config YAML) — Maximum concurrent runs the node will execute. Set in the
  node YAML under `concurrency`; defaults to `1` if not set.
- `PLOY_LIFECYCLE_NET_IGNORE` — Optional comma-separated list of network interface patterns (supports `*` globs) that the node lifecycle collector skips when computing throughput metrics. Example: `lo,cni*,docker*`.
  TODO: lifecycle collector to read this in an upcoming slice.
  - Pin via systemd drop-in or in `ployd.yaml` under `environment:` e.g.:

    environment:
      PLOY_LIFECYCLE_NET_IGNORE: "docker*,veth*,br-*"

- ployd-node config path — The node agent reads its YAML config from
  `/etc/ploy/ployd-node.yaml` by default and accepts an override via the
  CLI flag `--config`. There is currently no environment variable override
  for this path. TODO: consider introducing `PLOYD_NODE_CONFIG_PATH` for
  parity with the server’s `PLOYD_CONFIG_PATH`.

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

- `PLOY_GITLAB_PAT` — Optional GitLab Personal Access Token used by the Mods E2E
  walkthroughs and MR creation notes in `STATE.md`. TODO: server-side GitLab wiring pending.

 

## gapi

- No environment variables are active for gapi within this codebase; record
  future additions here once the API slices land.

## Control Plane

- `PLOY_CONTROL_PLANE_URL` — Optional control-plane base URL override used by
  the CLI and node bootstrap commands. When unset, the CLI derives the endpoint
  and CA bundle from the cached cluster descriptor created during
  `ploy server deploy`.

### Server (Control Plane)

- `http.listen` (config YAML) — Address the server listens on for HTTPS API/SSE. Default `:8443`.
  There is no environment variable; set this in `ployd.yaml` under `http.listen`.
- `metrics.listen` (config YAML) — Address for Prometheus metrics endpoint. Default `:9100`.
  There is no environment variable; set this in `ployd.yaml` under `metrics.listen`.
- `PLOY_SERVER_CLUSTER_ID` — Unique identifier for the cluster (set during `ploy server deploy`).
  Currently set by bootstrap but not yet persisted or loaded by the server runtime.
- `PLOY_SERVER_CERT_PEM` / `PLOY_SERVER_KEY_PEM` — PEM-encoded server TLS certificate and key
  used by the bootstrap script to write files at `/etc/ploy/pki/server.crt` and
  `/etc/ploy/pki/server.key` for the HTTPS API. At runtime the server reads file paths from
  config: `http.tls.cert`, `http.tls.key`, and `http.tls.client_ca`.

### PKI

- `PLOY_SERVER_CA_CERT` — PEM-encoded cluster CA certificate presented to nodes. Required for
  the `/v1/pki/sign` endpoint to return signed certificates.
- `PLOY_SERVER_CA_KEY` — PEM-encoded cluster CA private key used to sign node CSRs. Required
  alongside `PLOY_SERVER_CA_CERT` for `/v1/pki/sign`. When either value is missing (empty or
  whitespace-only), the server responds with `503 PKI not configured`.
  If values are set but invalid (malformed PEM), the server returns `500 Internal Server Error`
  and logs details; fix the stored CA materials.


## PostgreSQL

The control plane can use PostgreSQL via `pgx/v5` and `pgxpool`.

Precedence at server startup:
- `PLOY_SERVER_PG_DSN` (preferred)
- `PLOY_POSTGRES_DSN` (alias)
- `postgres.dsn` in the config file

- `PLOY_SERVER_PG_DSN` — Primary DSN the server reads at startup to open a PostgreSQL pool.
  Example: `postgres://user:pass@localhost:5432/ploy?sslmode=disable`.
  When `ploy server deploy` runs without `--postgresql-dsn`, the bootstrap installs
  PostgreSQL on the VPS and derives a password‑based TCP DSN suitable for the
  root‑run `ployd` service, e.g.: `host=127.0.0.1 port=5432 user=ploy password=ploy dbname=ploy sslmode=disable`.
- `PLOY_POSTGRES_DSN` — Backward‑compatible alias recognized by `ployd` during the transition. Prefer
  `PLOY_SERVER_PG_DSN` going forward.
- `PLOY_TEST_PG_DSN` — Optional Postgres DSN used by integration tests (e.g., `tests/integration/*` and
  packages that hit a real database such as `internal/store`). When unset, such tests skip automatically.

`ployd` reads `PLOY_SERVER_PG_DSN` (or `PLOY_POSTGRES_DSN`) at startup; when unset,
it falls back to `postgres.dsn` in the config file. Placeholders like `${PLOY_SERVER_PG_DSN}` in
the config file are treated as unset unless the environment variable is actually present.

## Bootstrap Script

These environment variables are used internally by the bootstrap script generated during
`ploy server deploy` and `ploy node add` flows. They are not required for day‑to‑day CLI
usage but are documented here for completeness.

- `PLOY_BOOTSTRAP_VERSION` — Version string exported at the top of generated bootstrap scripts
  (default: `dev` in source, overridden at build time).
- `PLOY_INSTALL_POSTGRESQL` — When `true`, the bootstrap script installs PostgreSQL on the
  target host and derives `PLOY_SERVER_PG_DSN`; when `false`, the provided DSN is used as-is.
  Not exported as an environment variable; checked inline within the script body.
- `PLOY_DB_PASSWORD` — Ephemeral password generated during PostgreSQL install flows and used
  to create the `ploy` database role and DSN. Set only within the bootstrap script scope.
- `BOOTSTRAP_PRIMARY` — When `true`, the bootstrap script performs control‑plane specific actions
  (e.g., writing server certs instead of node certs). Passed as `--primary` flag and checked
  inline within the script.
- `NODE_ID` — Node identifier used in the node agent config. Passed as `--node-id` script
  argument and referenced in `/etc/ploy/ployd-node.yaml` generation.
- `CLUSTER_ID` — Cluster identifier passed as `--cluster-id` script argument. Currently used
  for labeling during provisioning; not yet persisted or consumed by server runtime.
- `NODE_ADDRESS` — IP/hostname of the node being provisioned, passed as `--node-address` script
  argument.
- `PLOY_SERVER_URL` — Control-plane base URL used by `ploy node add` bootstrap to populate
  `server_url` in `/etc/ploy/ployd-node.yaml` (e.g., `https://<server-host>:8443`).
  This variable is consumed only by the bootstrap script; the CLI separately exposes a
  `--server-url` flag and `PLOY_CONTROL_PLANE_URL` override for client operations.

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
