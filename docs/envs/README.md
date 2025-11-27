# Environment Variables

**Note: Postgres/Bearer Token Pivot (November 2025)**

As of the server/node pivot described in `README.md`, the following legacy systems have been removed:
- **etcd**: All `PLOY_ETCD_*` variables are no longer consumed by the codebase.
- **mTLS client authentication**: Replaced with bearer token authentication for CLI and nodes.
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
- (removed) `PLOY_CONTROL_PLANE_URL` — The CLI no longer supports overriding the control‑plane URL. It always uses the
  default descriptor at `~/.config/ploy/clusters/default` (or `PLOY_CONFIG_HOME`/XDG path) and negotiates mTLS when the
  descriptor specifies HTTPS.
- `PLOY_BUILDGATE_IMAGE` — Optional unified override for the Docker image used by the
  Build Gate executor for any stack. When set, it takes precedence over language‑specific
  defaults. Commands still auto‑select by workspace (Maven vs Gradle).
- `PLOY_BUILDGATE_PROFILE` — Optional gate profile selector. Allowed explicit values:
  `java`, `java-maven`, `java-gradle`. When unset/unknown, auto-detects: pom.xml → maven,
  build.gradle(.kts) → gradle, else plain `java`.
- `PLOY_BUILDGATE_JAVA_IMAGE` — Deprecated legacy override for Maven projects. Defaults to
  `maven:3-eclipse-temurin-17` when neither `PLOY_BUILDGATE_IMAGE` nor this value is set.
- `PLOY_BUILDGATE_GRADLE_IMAGE` — Deprecated legacy override for Gradle projects. Defaults to
  `gradle:8.8-jdk17` when neither `PLOY_BUILDGATE_IMAGE` nor this value is set.
- `PLOY_CONFIG_HOME` — Optional override for the base directory where cluster descriptors
  are stored. When unset, the CLI falls back to `XDG_CONFIG_HOME/ploy` or `~/.config/ploy`.
  Priority: `PLOY_CONFIG_HOME` → `XDG_CONFIG_HOME/ploy` → `~/.config/ploy`.
- `XDG_CONFIG_HOME` — Standard XDG Base Directory specification variable. When set
  (and `PLOY_CONFIG_HOME` is not), the CLI uses `$XDG_CONFIG_HOME/ploy` for cluster
  descriptor storage. Falls back to `~/.config/ploy` when both are unset.

Local cluster descriptors (written under `~/.config/ploy/clusters/`) now use bearer token authentication:
- `token` — Bearer token for authenticating with the control plane. Generate using `ploy token create`.

Role model (bearer token claims):

- `cli-admin` — administrative CLI role. Allowed to perform admin operations (e.g., token management, node bootstrap) and all
  standard client operations. For authorization, `cli-admin` is treated as a superset of `control-plane`.
- `control-plane` — standard CLI role. Allowed to run Mods workflows and use control-plane APIs that do not require
  administrative privileges. Not allowed to hit admin-only endpoints like token creation.
- `worker` — node agent role. Used by nodes after bootstrap to authenticate with the control plane.
- `bootstrap` — short-lived token type used during node provisioning to exchange for a node certificate.
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
- Cross-phase input directory: `/in` is mounted read-only for healing mods (e.g., `mods-codex`).
  - `/in/build-gate.log` — First Build Gate failure log (node persists to temp host file and mounts)
  - `/in/prompt.txt` — Default prompt location when provided in spec (node mounts it R/O)
- `--spec` — Path to a YAML/JSON spec file for `ploy mod run` defining mod parameters,
  Build Gate settings, and healing configuration. The spec supports:
  - `mod.env` — Inline environment variables for the main mod container
  - `mod.env_from_file` — File-based secrets (CLI reads and inlines content before submit)
  - `build_gate_healing` — Automated repair sequence executed when Build Gate fails
  - GitLab MR settings (`mr_on_success`, `mr_on_fail`, `gitlab_domain`, `gitlab_pat`)
  - See `docs/schemas/mod.example.yaml` for the full schema
- `build_gate_healing` — Spec block defining the healing loop when Build Gate fails:
  - `retries` — Maximum number of healing attempts (default: 1)
  - `mods[]` — Array of healing steps (each is a container with image/command/env/retain)
  - After each healing attempt, the Build Gate is re-run; on pass, the main mod proceeds
  - If healing exhausts retries and gate still fails, run terminates with `reason="build-gate"`
  - Cross-phase inputs (`/in/build-gate.log`, `/in/prompt.txt`) are available to healing mods
  - Gate status visibility: Use `ploy mod inspect <ticket-id>` to view gate results (format: `Gate: passed duration=1234ms` or `Gate: failed pre-gate duration=567ms`). Also available via `GET /v1/mods/{id}` API in `Metadata["gate_summary"]`.

## Healing Container Environment

The node agent injects the following environment variables into healing containers to support
Build Gate verification via the repo+diff HTTP API model. These vars enable healing mods to
derive the same Git baseline used by the Mods run and call the Build Gate API directly.

Repo metadata (injected from StartRunRequest):
- `PLOY_REPO_URL` — Git repository URL for cloning/verification (same as the Mods run)
- `PLOY_BASE_REF` — Base Git reference (branch or tag) for the run
- `PLOY_TARGET_REF` — Target Git reference for the run
- `PLOY_COMMIT_SHA` — Pinned commit SHA when available (may be empty)

Server connection details (for Build Gate HTTP API access):
- `PLOY_SERVER_URL` — Control plane base URL (e.g., `https://<server>:8443`)
- `PLOY_HOST_WORKSPACE` — Host filesystem path to workspace for in-container tooling
- `PLOY_CA_CERT_PATH` — Path to CA certificate inside healing container (`/etc/ploy/certs/ca.crt`)
- `PLOY_CLIENT_CERT_PATH` — Path to client certificate (`/etc/ploy/certs/client.crt`)
- `PLOY_CLIENT_KEY_PATH` — Path to client key (`/etc/ploy/certs/client.key`)
- `PLOY_API_TOKEN` — Bearer token for API authentication (when configured on node). On TLS-disabled
  HTTP stacks (e.g., `docs/how-to/deploy-locally.md`), the node agent also falls back to reading the
  worker bearer token from `/etc/ploy/bearer-token` and exposing it as `PLOY_API_TOKEN` inside healing
  containers so tools can call `POST /v1/buildgate/validate` without extra wiring.

See `docs/build-gate/README.md` for the complete HTTP Build Gate API contract.
- `PLOYD_CONFIG_PATH` — When set, provides the default ployd configuration file
  location (default `/etc/ploy/ployd.yaml`). The ployd flag `--config` overrides this
  environment variable when explicitly provided.
- `PLOYD_NODE_ID` — Node identifier for the ployd daemon. Set during bootstrap to a sanitized
  version of the node name. Note: currently exported by bootstrap but not consumed at runtime;
  node identity is specified in the node YAML (`node_id`).
- `PLOYD_HOME_DIR` — Home directory for the ployd daemon. Exported by bootstrap as `/root` for
  systemd context; not currently read by the codebase.
- `PLOYD_CACHE_HOME` — Cache directory for working data. Defaults to `/var/cache/ploy` when set
  by bootstrap. Used at runtime by the node agent for ephemeral workspaces and git clone caching.
  When set, the node agent caches base git clones under `$PLOYD_CACHE_HOME/git-clones/` to avoid
  repeated network fetches for the same repo/ref/commit combination. The cache key is derived from
  the normalized repo URL, base_ref, and commit_sha. Subsequent hydrations for the same run or
  different runs with identical repo parameters reuse the cached clone, significantly reducing
  clone time and network bandwidth usage.
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
  The nodeagent heartbeat manager reads this environment variable at startup and passes the parsed patterns to the lifecycle collector via `lifecycle.Options.IgnoreInterfaces`.
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

## GitLab Merge Request Integration

Ploy can automatically create GitLab merge requests when Mods runs complete.

**Recommended approach:** Use `ploy config gitlab set` to store credentials on the control plane
(see [docs/how-to/create-mr.md](../how-to/create-mr.md) for usage examples).

Control plane configuration (set via CLI or YAML):
- `gitlab.domain` (config YAML) — GitLab base URL or host (e.g., `https://gitlab.com` or `gitlab.com`). Optional; Ploy normalizes either form.
- `gitlab.token` (config YAML) — Inline GitLab Personal Access Token. Optional; stored only in
  memory at runtime, not persisted back to disk.
- `gitlab.token_file` (config YAML) — Path to a file containing the PAT. Optional. See details below.

Per-run overrides (CLI flags on `ploy mod run`):
- `--gitlab-pat` — Override the control plane PAT for this run only
- `--gitlab-domain` — Override the control plane domain for this run only
- `--mr-success` — Create an MR when the run succeeds
- `--mr-fail` — Create an MR when the run fails

Branch naming semantics:
- The MR source branch is always `ploy-<ticket-id>` to guarantee uniqueness across retries and repeated runs.
- The base branch is whatever you pass via `--repo-base-ref` (commonly `main`).
- The `--repo-target-ref` value is accepted for workspace context but is not used as the MR source branch.

Quick test (PAT via config or flags):
For local testing or CI environments, set the PAT via control plane config or per‑run flags.
The recommended production approach is to use the control plane config with `gitlab.token_file`.

Example control plane config snippet (`/etc/ploy/ployd.yaml`):
```yaml
gitlab:
  domain: https://gitlab.com
  token_file: /etc/ploy/secrets/gitlab-pat.txt
```

Example usage:
```bash
# Configure once on control plane
ploy config gitlab set --file gitlab-config.json

# Run with MR on success (server assigns ticket)
ploy mod run --mr-success \
  --repo-url https://gitlab.com/org/repo.git \
  --repo-base-ref main \
  --repo-target-ref workflow/upgrade

# Per-run override
ploy mod run --mr-success \
  --gitlab-pat glpat-xxxxxxxxxxxxxxxxxxxx \
  --gitlab-domain https://gitlab.example.com \
  --repo-url https://gitlab.example.com/org/repo.git
```



## gapi

- No environment variables are active for gapi within this codebase; record
  future additions here once the API slices land.

## Control Plane

- (removed) `PLOY_CONTROL_PLANE_URL` — Legacy override removed. Components derive the endpoint and mTLS materials from
  the cached cluster descriptor created during `ploy server deploy`.

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

TLS/mTLS (config YAML):
- `http.tls.enabled` — Enable TLS. When true, the server enforces mutual TLS (mTLS) for all HTTPS connections.
- `http.tls.cert` / `http.tls.key` — Server certificate/key paths.
- `http.tls.client_ca` — CA certificate used to verify client certificates.
- TLS version is pinned to 1.3.
- Schema change (Nov 2025): `http.tls.require_client_cert` was removed. mTLS is always required when `http.tls.enabled` is true; there is no opt-out flag.

GitLab integration for automatic MR creation:
- `gitlab.domain` (config YAML) — GitLab base URL or host (e.g., `https://gitlab.com` or `gitlab.com`). Optional; Ploy normalizes either form.
- `gitlab.token` (config YAML) — Inline GitLab Personal Access Token. Optional; stored only in
  memory at runtime, not persisted back to disk.
- `gitlab.token_file` (config YAML) — Path to a file containing the PAT. Optional. When set and
  `gitlab.token` is not provided, the server reads the token from this file at startup.
  Requirements:
  - File permissions must not grant group/other access (≤ `0600`).
  - Empty/whitespace-only files are rejected.
  - Relative paths resolve relative to the config file location (e.g., `/etc/ploy/ployd.yaml`).
  - Absolute paths are accepted as-is. Symlink handling is platform-default (`os.Stat`).
  Precedence: `gitlab.token` (inline) wins over `gitlab.token_file` when both are set.
See the **GitLab Merge Request Integration** section above for usage examples and recommended configuration approach.

### Authentication

- `PLOY_AUTH_SECRET` — JWT signing secret used by the server to generate and validate bearer tokens.
  Required when bearer token authentication is enabled. This should be a strong random string
  (e.g., generated via `openssl rand -hex 32`). The same secret must be used consistently across
  server restarts to maintain token validity. Never commit this secret to version control.

### PKI

- `PLOY_SERVER_CA_CERT` — PEM-encoded cluster CA certificate used to sign node certificates during
  bootstrap. Required for the `/v1/pki/bootstrap` endpoint to issue certificates.
- `PLOY_SERVER_CA_KEY` — PEM-encoded cluster CA private key used to sign node CSRs during bootstrap.
  Required alongside `PLOY_SERVER_CA_CERT`. When either value is missing (empty or whitespace-only),
  the bootstrap endpoint responds with `503 PKI not configured`.
  If values are set but invalid (malformed PEM), the server returns `500 Internal Server Error`
  and logs details; fix the stored CA materials. Can also be configured via `pki.ca_cert_path` and
  `pki.ca_key_path` in the server config file.


## PostgreSQL

The control plane can use PostgreSQL via `pgx/v5` and `pgxpool`.

Precedence at server startup:
- `PLOY_POSTGRES_DSN` (preferred)
- `postgres.dsn` in the config file

- `PLOY_POSTGRES_DSN` — DSN the server reads at startup to open a PostgreSQL pool.
  Example: `postgres://user:pass@localhost:5432/ploy?sslmode=disable`.
  When `ploy server deploy` runs without `--postgresql-dsn`, the bootstrap installs
  PostgreSQL on the VPS and derives a password‑based TCP DSN suitable for the
  root‑run `ployd` service, e.g.: `host=127.0.0.1 port=5432 user=ploy password=ploy dbname=ploy sslmode=disable`.
  The server no longer recognizes `PLOY_SERVER_PG_DSN`.
- `PLOY_TEST_PG_DSN` — Optional Postgres DSN used by integration tests (e.g., `tests/integration/*` and
  packages that hit a real database such as `internal/store`). When unset, such tests skip automatically.

`ployd` reads `PLOY_POSTGRES_DSN` at startup; when unset, it falls back to `postgres.dsn` in the config file. Placeholders like `${PLOY_POSTGRES_DSN}` in
the config file are treated as unset unless the environment variable is actually present.

## Bootstrap Script

These environment variables are used internally by the bootstrap script generated during
`ploy server deploy` and `ploy node add` flows. They are not required for day‑to‑day CLI
usage but are documented here for completeness.

- `PLOY_BOOTSTRAP_VERSION` — Version string exported at the top of generated bootstrap scripts
  (default: `dev` in source, overridden at build time).
- `PLOY_INSTALL_POSTGRESQL` — When `true`, the bootstrap script installs PostgreSQL on the
  target host and derives `PLOY_POSTGRES_DSN`; when `false`, the provided DSN is used as-is.
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
- `PLOY_DOCKER_NETWORK` — Optional Docker network name to attach runtime containers (Build Gate
  and healing mods) to. When set on the node, the node agent's Docker runtime uses this network
  so containers (e.g., `mods-codex`) can reach the control-plane service by its Docker network
  hostname (e.g., `server:8080` in the local Docker stack). When unset, the default Docker
  network is used (no behavior change for non-Docker/VPS deployments).
- `PLOY_SERVER_URL` — Control-plane base URL used by `ploy node add` bootstrap to populate
  `server_url` in `/etc/ploy/ployd-node.yaml` (e.g., `https://<server-host>:8443`).
  Additionally, healing containers may consume this variable to call
  the Build Gate HTTP API directly during the fail→heal→re‑gate workflow. The CLI
  separately exposes a `--server-url` flag and `PLOY_CONTROL_PLANE_URL` override
  for client operations.

Primary reuse behavior:
- On control‑plane (primary) hosts, when `/etc/ploy/pki/ca.key` already exists, the bootstrap
  script treats the host as an existing cluster and skips all PKI writes (CA cert/key and
  server cert/key). It logs a reuse message and proceeds to (re)write configs and systemd units
  only. This prevents accidental clobbering of an existing cluster PKI.

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

- [README.md](../../README.md) — Server/node pivot architecture
- See `CHANGELOG.md` for migration status and recent slices
- [docs/how-to/deploy-a-cluster.md](../how-to/deploy-a-cluster.md) — Deployment guide

## Build Gate Limits

The Build Gate executor supports optional resource limits via environment variables on worker nodes:

- `PLOY_BUILDGATE_LIMIT_MEMORY_BYTES` — Memory limit for the gate container. Supports human suffixes
  such as `768MiB`, `1G`, or plain bytes. Parsed with Docker's units parser.
- `PLOY_BUILDGATE_LIMIT_DISK_SPACE` — Disk/quota limit for the gate container's writable layer. Supports
  human suffixes (e.g., `2G`). Passed to Docker as the storage option `size` (driver dependent; requires
  overlay2 with xfs project quotas or equivalent). When unsupported by the driver, container creation may fail.
- `PLOY_BUILDGATE_LIMIT_CPU_MILLIS` — CPU limit in millicores (e.g., `500` = 0.5 CPU, `1500` = 1.5 CPU).

Notes:
- When both `PLOY_BUILDGATE_IMAGE` and a language default apply, `PLOY_BUILDGATE_IMAGE` wins.
- Memory and disk limits accept human‑friendly suffixes; CPU uses numeric millicores only.
