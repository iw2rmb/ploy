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
  `local-step`. Other adapters (e.g., `k8s`) can plug in here; the CLI
  fails fast when an unknown adapter name is provided.
- `PLOY_DB_DSN` — Required by `deploy/local/run.sh`.
  Used both for host-side setup SQL (DB create/drop, token insert, node seed)
  and injected into the server container as `PLOY_POSTGRES_DSN`.
  Host-side DSN may use `localhost`; local deploy rewrites loopback hosts
  (`localhost`, `127.0.0.1`, `::1`) to `host.docker.internal` for container use.
  Non-loopback hosts must be reachable from inside containers.
  Example:
  `postgres://ploy:ploy@localhost:5432/ploy?sslmode=disable`.
- `PLOY_CA_CERTS` — Optional path to a PEM CA bundle used by
  `deploy/local/run.sh` to configure Docker daemon trust for Docker Hub
  endpoints (`docker.io`, `registry-1.docker.io`, `auth.docker.io`,
  `index.docker.io`).
  The script also installs the bundle into system CA trust before restarting
  Docker, so Docker Hub auth/token TLS uses the same root CAs.
  The same bundle is provided to local `server`/`node` image builds through
  a BuildKit secret (`ploy_ca_bundle`) so early `apk add` steps can trust
  your corporate/private CAs without printing certificate content in build logs.
  Current automation targets:
  - Docker context `colima` (installs CA inside the Colima VM and restarts Docker)
  - Linux hosts (installs CA under `/etc/docker/certs.d/...` and restarts Docker)
- `PLOY_SERVER_PORT` — Optional host port mapped to the server container's internal
  port `8080` in `deploy/local/docker-compose.yml`. Default: `8080`. Use this when host port `8080`
  is already occupied (example: `PLOY_SERVER_PORT=18080`).
- `WORKER_TOKEN_PATH` — Optional host path used by local scripts to persist the worker bearer
  token and mounted into the node container at `/etc/ploy/bearer-token`.
  Default: `deploy/local/node/bearer-token` (file path). If this path is a directory, scripts
  replace it with a file automatically.
- `PLOY_CONTAINER_SOCKET_PATH` — Optional host socket path mounted into the local
  `node` container at `/var/run/docker.sock`.
  Docker script default: `/var/run/docker.sock`.
- (removed) `PLOY_CONTROL_PLANE_URL` — The CLI no longer supports overriding the control‑plane URL. It always uses the
  default descriptor at `~/.config/ploy/clusters/default` (or `PLOY_CONFIG_HOME`/XDG path) and negotiates mTLS when the
  descriptor specifies HTTPS.
- `PLOY_BUILDGATE_IMAGE` — Optional unified override for the Docker image used by the
  Build Gate executor for any stack. When set, it takes precedence over language‑specific
  defaults. Commands still auto‑select by workspace (Maven vs Gradle).
- `PLOY_BUILDGATE_TIMEOUT` — Optional maximum duration for Build Gate HTTP polling (e.g., `5m`). When the request
  context has no deadline, HTTP-based gate executors use this value as the polling timeout; defaults to `10m` when unset or invalid.
- `PLOY_CONFIG_HOME` — Optional override for the base directory where cluster descriptors
  are stored. When unset, the CLI falls back to `XDG_CONFIG_HOME/ploy` or `~/.config/ploy`.
  Priority: `PLOY_CONFIG_HOME` → `XDG_CONFIG_HOME/ploy` → `~/.config/ploy`.
- `XDG_CONFIG_HOME` — Standard XDG Base Directory specification variable. When set
  (and `PLOY_CONFIG_HOME` is not), the CLI uses `$XDG_CONFIG_HOME/ploy` for cluster
  descriptor storage. Falls back to `~/.config/ploy` when both are unset.

Local cluster descriptors (written under `~/.config/ploy/clusters/`) now use bearer token authentication:
- `token` — Bearer token for authenticating with the control plane. Generate using `ploy cluster token create`.

Role model (bearer token claims):

- `cli-admin` — administrative CLI role. Allowed to perform admin operations (e.g., token management, node bootstrap) and all
  standard client operations. For authorization, `cli-admin` is treated as a superset of `control-plane`.
- `control-plane` — standard CLI role. Allowed to run Mods workflows and use control-plane APIs that do not require
  administrative privileges. Not allowed to hit admin-only endpoints like token creation.
- `worker` — node agent role. Used by nodes after bootstrap to authenticate with the control plane.
- `bootstrap` — short-lived token type used during node provisioning to exchange for a node certificate.
- `USER` — Standard Unix environment variable indicating the current user. The CLI
  reads this to populate the `Submitter` field when creating mig runs via `ploy mig run`.
- `PLOY_CONTAINER_REGISTRY` — Registry/repository prefix used by runner templates.
  Images resolve to `$PLOY_CONTAINER_REGISTRY/<name>:latest` (example: `ghcr.io/iw2rmb`).
- `DOCKERHUB_PAT` — Docker Hub Personal Access Token used for non‑interactive `docker login`
  on worker nodes during bootstrap. If set on the node, bootstrap performs
  `echo "$DOCKERHUB_PAT" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin`.
- `MODS_IMAGE_PREFIX` — Optional absolute image prefix (e.g., `docker.io/org` or `ghcr.io/org`).
  Takes effect only when `DOCKERHUB_USERNAME` is unset.
- `PLOY_OPENAI_API_KEY` — Optional OpenAI API key propagated to Mods LLM lanes. When set on the control
  plane, the runner injects it into the `migs-llm` container as `OPENAI_API_KEY`. You can also set it on
  worker nodes via a systemd drop-in to make it available cluster-wide.
- Cross-phase input directory: `/in` is mounted read-only for healing migs (e.g., `migs-codex`).
  - `/in/build-gate.log` — First Build Gate failure log (node persists to temp host file and mounts)
  - `/in/prompt.txt` — Default prompt location when provided in spec (node mounts it R/O)
- `--spec` — Path to a YAML/JSON spec file for `ploy mig run` defining mig parameters,
  Build Gate settings, and healing configuration. The spec supports:
  - `env` — Inline environment variables for single-step runs (and base env for multi-step runs)
  - `env_from_file` — File-based secrets (CLI reads and inlines content before submit)
  - `migs[]` — Multi-step spec steps (each with its own image/command/env/retain_container)
  - `build_gate_healing` — Automated repair sequence executed when Build Gate fails
  - GitLab MR settings (`mr_on_success`, `mr_on_fail`, `gitlab_domain`, `gitlab_pat`)
  - See `docs/schemas/mig.example.yaml` for the full schema
- `--name` — Creates a **batch run** with the given name (no repository attached yet).
  Used with `mig run repo add` to attach multiple repositories under a shared spec.
  Example: `ploy mig run --spec mig.yaml --name my-batch` followed by
  `ploy mig run repo add --repo-url https://... --base-ref main --target-ref feature my-batch`.
  See `cmd/ploy/README.md` § "Batched Mod Runs" for full usage.
- `build_gate_healing` — Spec block defining the healing loop when Build Gate fails:
  - `retries` — Maximum number of healing attempts (default: 1)
  - `mig` — Single healing mig (container with image/command/env/retain)
  - After each healing attempt, the Build Gate is re-run; on pass, the main mig proceeds
  - If healing exhausts retries and gate still fails, run terminates with `reason="build-gate"`
  - Cross-phase inputs (`/in/build-gate.log`, `/in/prompt.txt`) are available to healing migs
- Gate status visibility: Use `GET /v1/runs/{id}/status` to view gate results (format: `Gate: passed duration=1234ms` or `Gate: failed pre-gate duration=567ms`) via `Metadata["gate_summary"]`.

## Healing Container Environment

The node agent injects the following environment variables into healing containers to support
Build Gate verification. These vars enable healing migs to derive the same Git baseline used
by the Mods run.

Repo metadata (injected from StartRunRequest):
- `PLOY_REPO_URL` — Git repository URL for cloning/verification (same as the Mods run)
- `PLOY_BASE_REF` — Base Git reference (branch or tag) for the run
- `PLOY_TARGET_REF` — Target Git reference for the run
- `PLOY_COMMIT_SHA` — Pinned commit SHA when available (may be empty)

Server connection details:
- `PLOY_SERVER_URL` — Control plane base URL (e.g., `https://<server>:8443`)
- `PLOY_HOST_WORKSPACE` — Host filesystem path to workspace for in-container tooling
- `PLOY_CA_CERT_PATH` — Path to CA certificate inside healing container (`/etc/ploy/certs/ca.crt`)
- `PLOY_CLIENT_CERT_PATH` — Path to client certificate (`/etc/ploy/certs/client.crt`)
- `PLOY_CLIENT_KEY_PATH` — Path to client key (`/etc/ploy/certs/client.key`)
- `PLOY_API_TOKEN` — Bearer token for API authentication (when configured on node).

See `docs/build-gate/README.md` for Build Gate configuration and execution details.
- `PLOYD_CONFIG_PATH` — When set, provides the default ployd configuration file
  location (default `/etc/ploy/ployd.yaml`). The ployd flag `--config` overrides this
  environment variable when explicitly provided.
  Relevant `ployd.yaml` scheduler keys for stale-job recovery:
  - `scheduler.stale_job_recovery_interval` (default `30s`; set `0` to disable recovery)
  - `scheduler.node_stale_after` (default `1m`; stale cutoff for node heartbeats)
  Recovery observability and troubleshooting:
  - Recovery emits structured logs (`stale-job-recovery: cycle completed`) with
    `stale_nodes`, `stale_attempts`, `repos_updated`, `jobs_cancelled`, and
    `runs_finalized` counters.
  - If a stale attempt is recovered to terminal, check `GET /v1/runs/{id}/status`
    and `GET /v1/runs/{id}/logs` to confirm final repo/run state and terminal SSE.
- `PLOYD_NODE_ID` — Node identifier for the ployd daemon. Set during bootstrap as a NanoID(6)
  string (6 characters from URL-safe alphabet A-Za-z0-9_-). This compact format balances
  brevity with sufficient uniqueness for typical cluster sizes. Note: currently exported by
  bootstrap but not consumed at runtime; node identity is specified in the node YAML (`node_id`).
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

### Docker Engine v29 Environment Variables

The node agent uses the moby Engine v29 SDK (`github.com/moby/moby/client`) for all
container operations. The SDK's `client.FromEnv` function reads the following standard
Docker environment variables when constructing the client. These rarely need explicit
setting on typical deployments where Docker runs on the default Unix socket.

| Variable             | Default                          | Description                                                  |
|----------------------|----------------------------------|--------------------------------------------------------------|
| `DOCKER_HOST`        | `unix:///var/run/docker.sock`    | Docker daemon address (Unix socket or TCP endpoint)          |
| `DOCKER_TLS_VERIFY`  | (unset)                          | Set to `"1"` to enable TLS verification for TCP connections  |
| `DOCKER_CERT_PATH`   | (unset)                          | Directory containing `ca.pem`, `cert.pem`, `key.pem` for TLS |
| `DOCKER_API_VERSION` | (auto-negotiated)                | Override API version; normally unnecessary with v29+         |

**Implementation**: `internal/workflow/step/container_docker.go:59-66` constructs the
Docker client with `client.FromEnv` and `client.WithAPIVersionNegotiation`.

- `DOCKER_AUTH_CONFIG` — Optional Docker auth config JSON used by node image pulls.
  When set, the node extracts credentials for the target image registry and passes
  them to Docker Engine via `ImagePullOptions.RegistryAuth`.
  Example:
  `{"auths":{"ghcr.io":{"auth":"<base64(username:token)>"}}}`.
- `PLOY_DOCKER_AUTH_CONFIG` — Optional override for `DOCKER_AUTH_CONFIG`.
  When both are set, `PLOY_DOCKER_AUTH_CONFIG` wins.

**When to set these variables:**
- **Remote Docker daemon**: Set `DOCKER_HOST=tcp://<host>:2376` and TLS variables when the
  daemon runs on a different host or requires TLS authentication.
- **Custom socket path**: Set `DOCKER_HOST=unix:///custom/path/docker.sock` if the daemon
  uses a non-standard socket location.
- **API version pinning**: Set `DOCKER_API_VERSION=1.44` only if auto-negotiation causes
  issues (rare; Engine v29+ handles this automatically).

**Cross-references:**
- Engine version requirements: Docker Engine v29+ (moby Engine v29 SDK)
- Migration status: complete (`github.com/docker/docker` removed)

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
  parity with the server's `PLOYD_CONFIG_PATH`.
- (removed) `PLOY_BUILDGATE_WORKER_ENABLED` — Previously enabled Build Gate worker mode
  via the HTTP Build Gate API. Removed in favor of the unified jobs pipeline. All nodes
  now claim work (including gate jobs) from the same `jobs` queue. This variable is no
  longer consumed by the codebase.
- (removed) `PLOY_BUILDGATE_MODE` — Previously controlled gate execution mode (`remote-http`
  vs local Docker). Removed in favor of local Docker-only execution. Gate jobs run as
  part of the unified jobs pipeline on the claiming node. This variable is no longer
  consumed by the codebase.

## E2E Harness

- `ploy mig run` executes Mods against the Ploy control plane; no tenant variable is required.
- `PLOY_E2E_RUN_PREFIX` — Optional run ID prefix for Mods E2E runs
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

Per-run overrides (CLI flags on `ploy mig run`):
- `--gitlab-pat` — Override the control plane PAT for this run only
- `--gitlab-domain` — Override the control plane domain for this run only
- `--mr-success` — Create an MR when the run succeeds
- `--mr-fail` — Create an MR when the run fails

Branch naming semantics:
- The MR source branch is always the effective target ref for the run. When `--repo-target-ref` is provided, that value is used. When it is omitted, the node derives a default of `ploy/{run_name|run_id}` using the run name when set (e.g., batch name) or the run ID (KSUID string) otherwise.
- The base branch is whatever you pass via `--repo-base-ref` (commonly `main`).

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

# Run with MR on success (server assigns run ID)
ploy mig run --mr-success \
  --repo-url https://gitlab.com/org/repo.git \
  --repo-base-ref main \
  --repo-target-ref workflow/upgrade

# Per-run override
ploy mig run --mr-success \
  --gitlab-pat glpat-xxxxxxxxxxxxxxxxxxxx \
  --gitlab-domain https://gitlab.example.com \
  --repo-url https://gitlab.example.com/org/repo.git
```



## gapi

- No environment variables are active for gapi within this codebase.

## Control Plane

- (removed) `PLOY_CONTROL_PLANE_URL` — Legacy override removed. Components derive the endpoint and token from
  the default cluster descriptor under `PLOY_CONFIG_HOME` (or XDG/home default).

### Server (Control Plane)

- `http.listen` (config YAML) — Address the server listens on for HTTPS API/SSE. Default `:8443`.
  There is no environment variable; set this in `ployd.yaml` under `http.listen`.
- `metrics.listen` (config YAML) — Address for Prometheus metrics endpoint. Default `:9100`.
  There is no environment variable; set this in `ployd.yaml` under `metrics.listen`.
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
  The server no longer recognizes `PLOY_SERVER_PG_DSN`.
- `PLOY_TEST_PG_DSN` — Optional Postgres DSN used by integration tests (e.g., `tests/integration/*` and
  packages that hit a real database such as `internal/store`). When unset, such tests skip automatically.

`ployd` reads `PLOY_POSTGRES_DSN` at startup; when unset, it falls back to `postgres.dsn` in the config file. Placeholders like `${PLOY_POSTGRES_DSN}` in
the config file are treated as unset unless the environment variable is actually present.

- `PLOY_DOCKER_NETWORK` — Optional Docker network name to attach runtime containers (Build Gate
  and healing migs) to. When set on the node, the node agent's Docker runtime uses this network
  so containers (e.g., `migs-codex`) can reach the control-plane service by its Docker network
  hostname (e.g., `server:8080` in the local Docker stack). When unset, the default Docker
  network is used.

Alternatively, you can specify the DSN in the config file under `postgres.dsn`. Environment variables take
precedence over the config file when both are present.

## Object Store (Garage/S3)

The control plane uses an S3-compatible object store (e.g., Garage) for blob storage of logs, diffs,
and artifacts. Database tables store metadata with deterministic object keys; the blobs themselves
are stored in the object store.

| Variable | Description | Default |
|----------|-------------|---------|
| `PLOY_OBJECTSTORE_ENDPOINT` | S3-compatible endpoint URL (e.g., `http://garage:3900`) | - |
| `PLOY_OBJECTSTORE_BUCKET` | Bucket name for blob storage | - |
| `PLOY_OBJECTSTORE_ACCESS_KEY` | Access key ID | - |
| `PLOY_OBJECTSTORE_SECRET_KEY` | Secret access key | - |
| `PLOY_OBJECTSTORE_SECURE` | Use TLS (true/false) | `false` |
| `PLOY_OBJECTSTORE_REGION` | AWS region (optional; for local Garage use `garage`) | - |

For local development, these are configured in `deploy/local/docker-compose.yml`. The local stack includes
a Garage service with automatic bucket/access-key bootstrap via `garage-init`.

Alternatively, you can specify these in the server config file under `object_store.*`. Environment
variables take precedence over the config file when both are present.

## Global Env Configuration

The control plane supports centralized global environment variables that are automatically injected
into job containers based on scope rules. This enables cluster-wide configuration of credentials,
CA bundles, and API keys without embedding them in every spec file.

### Configuration via CLI

Use the `ploy config env` subcommands to manage global environment variables:

```bash
# Set a CA certificate bundle (injected into all job types)
ploy config env set --key CA_CERTS_PEM_BUNDLE --file ca-bundle.pem --scope all

# Set Codex auth credentials (injected only into mig and post_gate jobs)
ploy config env set --key CODEX_AUTH_JSON --file ~/.codex/auth.json --scope migs

# Set OpenAI API key (injected into all jobs)
ploy config env set --key OPENAI_API_KEY --value sk-... --scope all

# List configured variables (secret values redacted)
ploy config env list

# Show a specific variable (use --raw to reveal secret values)
ploy config env show --key OPENAI_API_KEY --raw

# Delete a variable
ploy config env unset --key OLD_VAR
```

### Scope Semantics

The `scope` parameter controls which job types receive each variable:

| Scope | Job Types | Use Case |
|-------|-----------|----------|
| `all` | Every job type (mig, heal, pre_gate, re_gate, post_gate) | Credentials needed everywhere (CA certs, API keys) |
| `migs` | `mig`, `post_gate` | Credentials for code modification phases |
| `heal` | `heal`, `re_gate` | Credentials specific to healing/retry phases |
| `gate` | `pre_gate`, `re_gate`, `post_gate` | Credentials for gate execution phases |

### Injection Flow

1. **Storage**: Variables are persisted in the `config_env` table and cached in the
   control-plane's `ConfigHolder` at startup.
2. **Claim-time merge**: When a node claims a job via `/v1/nodes/{id}/claim`, the server
   calls `mergeGlobalEnvIntoSpec()` to inject matching global env vars into the job's spec.
   The job spec must be a JSON object; invalid/non-object specs are rejected at submission
   time (400). If a persisted spec in the DB is invalid or non-object, claim fails with a 500.
3. **Precedence**: Per-run env vars (in spec or CLI flags) take precedence—existing keys
   in the spec are never overwritten by global env.
4. **Container injection**: The node agent propagates the merged `env` map to the
   container runtime, which sets them in the running container. For Build Gate jobs,
   the node agent mirrors job env into the gate spec env so gate build images
   receive the same injected variables.

### Common Variables Consumed by Official Images

| Variable | Consumer | Description |
|----------|----------|-------------|
| `CA_CERTS_PEM_BUNDLE` | ORW migs, build-gate, custom migs | PEM-encoded CA certificates installed into the container's trust store |
| `CODEX_AUTH_JSON` | `mig-codex` | JSON credentials written to `/root/.codex/auth.json` at container startup |
| `OPENAI_API_KEY` | Future OpenAI-integrated migs | API key for LLM operations |
| `PLOY_GRADLE_BUILD_CACHE_URL` | Build Gate (Gradle), `orw-gradle` | HTTP URL of the remote Gradle Build Cache endpoint (e.g. `http://gradle-build-cache:5071/cache/`). When unset, remote cache is disabled. |
| `PLOY_GRADLE_BUILD_CACHE_PUSH` | Build Gate (Gradle), `orw-gradle` | Whether to push results to the remote cache. Defaults to `true` when `PLOY_GRADLE_BUILD_CACHE_URL` is set. |

### How Official Images Consume These Variables

**Codex images (`mig-codex`)**: The entrypoint script checks for `CODEX_AUTH_JSON` and, when
present, writes it to `/root/.codex/auth.json` before invoking the Codex CLI.

**Build Gate images (Maven/Gradle)**: The gate executor prepends a CA-install preamble that:
1. Writes `CA_CERTS_PEM_BUNDLE` to a temp file
2. Splits the bundle into individual `.crt` files
3. Copies them to `/usr/local/share/ca-certificates/ploy/`
4. Runs `update-ca-certificates` (on Debian/Ubuntu images)
5. Optionally imports into Java cacerts via `keytool` when available

**Build Gate Gradle images (`ploy-gate-gradle:*`)**: Ship a Gradle init script under `~/.gradle/init.d/` that enables a remote Gradle Build Cache when `PLOY_GRADLE_BUILD_CACHE_URL` is set (push behavior controlled by `PLOY_GRADLE_BUILD_CACHE_PUSH`).

**ORW images (`orw-maven`, `orw-gradle`)**: Similar CA bundle handling as build-gate, ensuring
OpenRewrite can fetch dependencies from internal artifact repositories.

`orw-gradle` additionally injects a Gradle init script at runtime when `PLOY_GRADLE_BUILD_CACHE_URL` is set and runs Gradle with `--build-cache`.

### Security Considerations

- **Secrets flag**: Variables marked with `--secret=true` (the default) are redacted in
  `ploy config env list` output to prevent accidental exposure.
- **mTLS protection**: The `/v1/config/env` endpoints require mTLS with `cli-admin` role.
- **In-memory caching**: The control plane caches global env in memory; values are loaded
  from the database at startup and updated on each `set`/`unset` operation.

### API Reference

See `docs/api/OpenAPI.yaml` paths:
- `GET /v1/config/env` — List all global env entries (secrets redacted)
- `GET /v1/config/env/{key}` — Get single entry (full value for admins)
- `PUT /v1/config/env/{key}` — Create or update an entry
- `DELETE /v1/config/env/{key}` — Delete an entry

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
- [docs/testing-workflow.md](../testing-workflow.md) — Go testing workflow and local validation commands
- See `CHANGELOG.md` for migration status and recent slices
- [docs/how-to/deploy-locally.md](../how-to/deploy-locally.md) — Local Docker cluster

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
