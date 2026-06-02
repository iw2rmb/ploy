# Environment Variables

**Note: Postgres/Bearer Token Pivot (November 2025)**

As of the server/node pivot, the following legacy systems have been removed:
- **etcd**: All `PLOY_ETCD_*` variables are no longer consumed by the codebase.
- **mTLS client authentication**: Replaced with bearer token authentication for CLI and nodes.
- **Node labels**: Removed in favor of resource-snapshot scheduling.

This document tracks the environment variables that the server, node, and CLI
use after the pivot. Update this file whenever a new variable is introduced,
defaults change, or components adopt additional configuration.

## Dependencies

- Runtime factory wiring and control-plane endpoint resolution are covered in
  [Migs lifecycle](../migs-lifecycle.md).

## CLI

- `PLOY_RUNTIME_ADAPTER` — Optional runtime adapter selector. Defaults to
  `local-step`. Other adapters (e.g., `k8s`) can plug in here; the CLI
  fails fast when an unknown adapter name is provided.
- `PLOY_DB_DSN` — Required by the server container.
  Non-loopback hosts must be reachable from inside containers.
  Example:
  `postgres://ploy:ploy@localhost:5432/ploy?sslmode=disable`.
- `PLOY_SERVER_PORT` — Optional host port mapped to the server container's internal
  port `8080` in the local compose stack. Default: `8080`. Use this when the
  host port `8080` is already occupied (example: `PLOY_SERVER_PORT=18080`).
- `PLOY_IMAGE_TAG` — Runtime compose tag for the `server` and `node` images in
  the external compose assets under `ploy-lib/images`. Defaults to `latest`.
- `WORKER_TOKEN_PATH` — Required host path mounted into the node container at
  `/etc/ploy/bearer-token`.
- `PLOY_CONTAINER_SOCKET_PATH` — Optional host socket path mounted into the local
  `node` container at `/var/run/docker.sock`.
  Docker script default: `/var/run/docker.sock`.
- (removed) `PLOY_CONTROL_PLANE_URL` — The CLI no longer supports overriding the control‑plane URL. It always uses the
  default descriptor at `~/.config/ploy/default` (or `PLOY_CONFIG_HOME` path) and negotiates mTLS when the
  descriptor specifies HTTPS.
- `PLOY_BUILDGATE_TIMEOUT` — Optional maximum duration for Build Gate HTTP polling (e.g., `5m`). When the request
  context has no deadline, HTTP-based gate executors use this value as the polling timeout; defaults to `10m` when unset or invalid.
- `PLOY_CONFIG_HOME` — Optional override for the base directory where cluster descriptors
  are stored. When unset, the CLI falls back to `~/.config/ploy`.
  Priority: `PLOY_CONFIG_HOME` → `~/.config/ploy`.

Local cluster descriptors (written under `~/.config/ploy/{cluster}/`) now use bearer token authentication:
- `token` — Bearer token for authenticating with the control plane. Generate using `ploy cluster token create`.

Role model (bearer token claims):

- `cli-admin` — administrative CLI role. Allowed to perform admin operations (e.g., token management, node bootstrap) and all
  standard client operations. For authorization, `cli-admin` is treated as a superset of `control-plane`.
- `control-plane` — standard CLI role. Allowed to run Migs workflows and use control-plane APIs that do not require
  administrative privileges. Not allowed to hit admin-only endpoints like token creation.
- `worker` — node agent role. Used by nodes after bootstrap to authenticate with the control plane.
- `bootstrap` — short-lived token type used during node provisioning to exchange for a node certificate.
- `USER` — Standard Unix environment variable indicating the current user. The CLI
  reads this to populate creator metadata when submitting runs.
- `PLOY_CONTAINER_REGISTRY` — Registry/repository prefix used by runner templates.
  Images resolve to `$PLOY_CONTAINER_REGISTRY/<name>:latest`. Runtime compose
  assets default to `docker-hosted.artifactory.tcsbank.ru/at-scale/ploy`.
- `PLOY_OBJECTSTORE_ENDPOINT` — S3-compatible endpoint URL provided by environment.
- `PLOY_OBJECTSTORE_ACCESS_KEY` — S3 access key ID provided by environment.
- `PLOY_OBJECTSTORE_SECRET_KEY` — S3 secret access key provided by environment.

- `CLUSTER_ID` — Optional cluster ID passed to the server container by the
  external compose assets. Default: `local`.
### Run Spec Files

`ploy run <spec-path>` accepts a YAML/JSON spec file, or a directory containing
`mig.yaml`, defining mig parameters, Build Gate settings, and file inputs. The
spec supports:
  - `envs` — Environment variables (key-value map, merged by key across precedence layers)
  - `in` — Read-only input files (`src:/in/dst`; CLI compiles local paths to `shortHash:/in/dst`)
  - `out` — Read-write output files (`src:/out/dst`; CLI compiles local paths to `shortHash:/out/dst`)
  - `home` — Home-relative files (`src:dst{:ro}`; CLI compiles to `shortHash:dst{:ro}`)
  - `steps[]` — Multi-step spec steps (each with its own `image`/`command`/`envs`/`in`/`out`/`home`)
  - `build_gate.pre.stack` / `build_gate.post.stack` — Stack-detection policy for gate phases
  - `build_gate.images` — Build Gate image overrides selected by stack rules
  - See [mig.example.yaml](../schemas/mig.example.yaml) for the full schema.

### Hydra file-record compilation

The CLI compiles local file paths in `in`, `out`, and `home` fields into
canonical `shortHash:dst` records before spec submission:

1. Resolves each source path relative to the spec file directory.
2. Computes a content hash, uploads the archive when missing, and rewrites
   the entry to `shortHash:dst` form.
3. The node agent downloads bundles by hash and mounts them at the declared
   destination (read-only for `in`, read-write for `out`, configurable
   for `home`).

**Example spec fragment (before CLI compile):**
```yaml
steps:
  - image: docker.io/your-dh-user/migs-openrewrite:latest
    in:
      - ./recipe.yaml:/in/recipe.yaml

build_gate:
  post:
    stack:
      mode: fallback
      language: java
      tool: maven
      release: "17"
```

**After CLI compile (canonical form submitted to server):**
```yaml
steps:
  - image: docker.io/your-dh-user/migs-openrewrite:latest
    in:
      - "a1b2c3d4e5f6g7:/in/recipe.yaml"

build_gate:
  post:
    stack:
      mode: fallback
      language: java
      tool: maven
      release: "17"
```

- `--name` — Creates a mig project with `ploy mig add --name <name> [--spec <path|->]`.
  Use `ploy mig repo add` to attach repositories under a shared spec, then run them via
  `ploy mig run <mig-id|name> [namespace/repo:main] [--follow]`.
  Example: `ploy mig add --name my-wave --spec mig.yaml` followed by
  `ploy mig repo add my-wave namespace/repo:main`.
  See [Migs lifecycle](../migs-lifecycle.md) for full usage.
- Container cleanup model:
  - Containers are retained after step/gate completion.
  - Host `ploy-node-cleanup` systemd timers prune completed containers and cache state.
  - Disk telemetry selects the lowest-free configured storage path from `/`,
    `DOCKER_ROOT_DIR`, `PLOYD_CACHE_HOME`, `PLOY_BUILDGATE_CACHE_ROOT`, and `TMPDIR`.
- Gate status visibility: Use `GET /v1/runs/{id}/status` to view gate results (format: `Gate: passed duration=1234ms` or `Gate: failed pre-gate duration=567ms`) via `Metadata["gate_summary"]`.
- SBOM persistence contract:
  - Gate post-tasks persist package rows from `/share/sbom.spdx.json` for both `pre_gate` and `post_gate`.
  - SBOM row persistence is independent from artifact bundle upload.
  - No dedicated SBOM environment variables exist in this slice; stack identity comes from gate metadata (`lang`, `release`, `tool`) and claim context.

## Healing Container Environment

The node agent injects the following environment variables into healing containers to support
Build Gate verification. These vars enable healing migs to derive the same Git baseline used
by the Migs run.

Repo metadata (injected from StartRunRequest):
- `PLOY_REPO_URL` — Git repository URL for cloning/verification (same as the Migs run)
- `PLOY_BASE_REF` — Base Git reference (branch or tag) for the run
- `PLOY_COMMIT_SHA` — Pinned commit SHA when available (may be empty)

Server connection details:
- `PLOY_SERVER_URL` — Control plane base URL (e.g., `https://<server>:8443`)
- `PLOY_HOST_WORKSPACE` — Host filesystem path to workspace for in-container tooling
- `PLOY_CLIENT_CERT_PATH` — Path to client certificate (`/etc/ploy/certs/client.crt`)
- `PLOY_CLIENT_KEY_PATH` — Path to client key (`/etc/ploy/certs/client.key`)
- `PLOY_API_TOKEN` — Bearer token for API authentication (when configured on node).

Healing runtime context:
- `PLOY_GATE_PHASE` — phase that failed (`pre_gate|post_gate`)
- `PLOY_LOOP_KIND` — loop context (`healing`)

See [Build Gate docs](../build-gate/README.md) for Build Gate configuration and execution details.
- `PLOYD_HTTP_LISTEN` — Server HTTP listen address (default `:8080`).
- `PLOYD_METRICS_LISTEN` — Server metrics listen address (default `:9100`).
- `PLOYD_SCHEDULER_STALE_JOB_RECOVERY_INTERVAL` — Stale-job recovery interval
  (default `30s`; set `0` to disable recovery).
- `PLOYD_SCHEDULER_NODE_STALE_AFTER` — Node heartbeat stale cutoff (default `1m`).
  Recovery observability and troubleshooting:
  - Recovery emits structured logs (`stale-job-recovery: cycle completed`) with
    `stale_nodes`, `stale_attempts`, `repos_updated`, `jobs_cancelled`, and
    `runs_finalized` counters.
  - If a stale attempt is recovered to terminal, check `GET /v1/runs/{id}/status`
    to confirm final repo/run state.
  Node startup crash reconciliation policy (fixed, no knob):
  - Startup executes one reconciliation pass before the normal claim loop.
  - Terminal replay uses `finished_at >= now-120s` (terminal timestamp only;
    container create time is not used).
  - Completion replay uses canonical `POST /v1/jobs/{job_id}/complete`; startup
    replay treats `409 Conflict` as idempotent success.
  - There is currently no environment variable or scheduler key to tune the 120s window.
- `PLOYD_NODE_ID` — Node identifier for the ployd daemon. Set during bootstrap as a NanoID(6)
  string (6 characters from URL-safe alphabet A-Za-z0-9_-). This compact format balances
  brevity with sufficient uniqueness for typical cluster sizes. Note: currently exported by
  bootstrap but not consumed at runtime; node identity is specified in the node YAML (`node_id`).
- `PLOYD_HOME_DIR` — Home directory for the ployd daemon. Exported by bootstrap as `/root` for
  systemd context; not currently read by the codebase.
- `PLOYD_CACHE_HOME` — Cache directory for working data. Defaults to `/var/cache/ploy` when set
  by bootstrap. Used at runtime by the node agent for ephemeral workspaces and git clone caching.
  When set, the node agent caches base git clones under `$PLOYD_CACHE_HOME/git-clones/` to avoid
  repeated network fetches for the same repo snapshot. Cache entries are pure shallow Git clones
  stored as `git-clones/<domain>/<namespace>/<repo>/<full_commit_sha>/`. The path is derived from
  the normalized repo URL and resolved full commit SHA; `base_ref` is only used to resolve or fetch
  the snapshot. Subsequent hydrations for the same repo commit reuse the cached clone, significantly
  reducing clone time and network bandwidth usage.
- `PLOYD_METRICS_LISTEN` — Read by `ployd` at runtime as the metrics listen address
  (default `:9100`).


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

Runtime behavior: the node's Docker client is created from standard Docker env vars with API version negotiation enabled.

- `PLOY_DOCKER_AUTH_CONFIG_FILE` — Optional path to a Docker auth config JSON
  file. Production nodes use `/etc/ploy/docker-auth-config/config.json`. When
  set, this file is read for each job image pull so host auth refreshes take
  effect without recreating the node container. If set and unreadable, image
  pulls fail explicitly instead of silently pulling without credentials.
  Example:
  `{"auths":{"ghcr.io":{"auth":"<base64(username:token)>"}}}`.
- `PLOY_DOCKER_AUTH_REFRESH_SOCKET` — Optional Unix socket used only after
  Docker returns an auth error for a job image pull. The node asks the host to
  refresh registry auth, then retries the same pull once. The host owns the DP
  key and Docker auth file writes.

The node does not consume inline registry-auth environment variables for job
image pulls. Use `PLOY_DOCKER_AUTH_CONFIG_FILE` for private registries.

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
  - Pin via systemd drop-in `environment:` e.g.:

    environment:
      PLOY_LIFECYCLE_NET_IGNORE: "docker*,veth*,br-*"

- ployd-node config path — The node agent reads its YAML config from
  `/etc/ploy/ployd-node.yaml` by default and accepts an override via the
  CLI flag `--config`. There is currently no environment variable override
  for this path.
- `PLOYD_LOG_LEVEL` — Optional server daemon minimum log level
  (`debug`, `info`, `warn`, or `error`). Defaults to `info`. Daemon process
  logs are always newline-delimited JSON on stdout/stderr; this variable only
  changes filtering.
- `PLOY_LOG_ENV` — Optional daemon log envelope environment value. Defaults to
  `prod`.
- `PLOY_LOG_SYSTEM` — Optional daemon log envelope system value. Defaults to
  `ploy-server`.
- `PLOY_LOG_INST` — Optional daemon log envelope instance value. Defaults to
  `ploy.t-tech.team`.
- (removed) `PLOYD_LOG_JSON`, `PLOYD_LOG_FILE`, `PLOYD_LOG_STATIC_FIELDS`,
  `PLOYD_LOG_MAX_SIZE_MB`, `PLOYD_LOG_MAX_BACKUPS`, and
  `PLOYD_LOG_MAX_AGE_DAYS` — Daemon process logs no longer support alternate
  formats, file output, or process-local rotation settings.
- (removed) `PLOY_BUILDGATE_MODE` — Previously controlled gate execution mode (`remote-http`
  vs local Docker). Removed in favor of local Docker-only execution. Gate jobs run as
  part of the unified jobs pipeline on the claiming node. This variable is no longer
  consumed by the codebase.

## E2E Harness

- `ploy run` executes Migs against the Ploy control plane; no tenant variable is required.
- `PLOY_E2E_RUN_PREFIX` — Optional run ID prefix for Migs E2E runs
  (default `e2e`).
- `PLOY_E2E_REPO_OVERRIDE` — Optional Git repository override used by the Migs
  E2E scenarios in place of the default Java sample repo.
- `PLOY_E2E_LIVE_SCENARIOS` — Optional comma-separated scenario IDs that the
  live Migs smoke test should execute (defaults to `simple-openrewrite`).

## GitLab Source Hydration

GitLab credentials are server-owned `ployd` environment only. They are used by
the control plane to resolve source SHAs and materialize repo snapshots that
workers download through the snapshot endpoint.

- `PLOY_GITLAB_DOMAIN` — GitLab base URL or host (for example `https://gitlab.com` or `gitlab.com`). Optional; when set, token auth is scoped to that host.
- `PLOY_GITLAB_TOKEN` — GitLab Personal Access Token used by `ployd` for source resolution and snapshot materialization. It is not accepted in specs, CLI flags, or node manifests.

## gapi

- No environment variables are active for gapi within this codebase.

## Control Plane

- (removed) `PLOY_CONTROL_PLANE_URL` — Legacy override removed. Components derive the endpoint and token from
  the default cluster descriptor under `PLOY_CONFIG_HOME` (or home default).

### Server (Control Plane)

- `PLOYD_HTTP_LISTEN` — Address the server listens on for API/SSE. Default `:8080`.
- `PLOYD_METRICS_LISTEN` — Address for Prometheus metrics endpoint. Default `:9100`.
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

GitLab source hydration:
- `PLOY_GITLAB_DOMAIN` — GitLab base URL or host. Optional; when set, token auth is scoped to that host.
- `PLOY_GITLAB_TOKEN` — GitLab Personal Access Token used by `ployd` for source resolution and snapshot materialization.

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
- `PLOY_DB_DSN` (preferred)
- `postgres.dsn` in the config file

- `PLOY_DB_DSN` — DSN the server reads at startup to open a PostgreSQL pool.
  Example: `postgres://user:pass@localhost:5432/ploy?sslmode=disable`.
- `PLOY_TEST_DB_DSN` — Optional Postgres DSN used by integration and database-backed tests. When unset, such tests skip automatically.

`ployd` reads `PLOY_DB_DSN` at startup; when unset, it falls back to `postgres.dsn` in the config file. Placeholders like `$PLOY_DB_DSN` in
the config file are treated as unset unless the environment variable is actually present.

- `PLOY_DOCKER_NETWORK` — Optional Docker network name to attach runtime containers (Build Gate
  and healing migs) to. When set on the node, the node agent's Docker runtime uses this network
  so containers (e.g., `codex`) can reach the control-plane service by its Docker network
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
| `PLOY_OBJECTSTORE_BUCKET` | Bucket name for blob storage | `${CLUSTER_ID:-local}` in runtime compose |
| `PLOY_OBJECTSTORE_ACCESS_KEY` | Access key ID | - |
| `PLOY_OBJECTSTORE_SECRET_KEY` | Secret access key | - |
| `PLOY_OBJECTSTORE_SECURE` | Use TLS (true/false) | `false` |
| `PLOY_OBJECTSTORE_REGION` | AWS region (optional; for local Garage use `garage`) | - |


## Global Env Configuration

The control plane supports centralized global environment variables that are automatically injected
into cluster components based on target rules. This enables cluster-wide configuration of credentials,
and API keys without embedding them in every spec file.

### Configuration via CLI

Use the `ploy config env` subcommands to manage global environment variables:

```bash
# Set OpenAI API key (injected into gate and step jobs — default --on jobs)
ploy config env set --key OPENAI_API_KEY --value sk-...

# List configured variables (secret values redacted)
ploy config env list

# Show a specific variable (use --raw to reveal secret values)
ploy config env show --key OPENAI_API_KEY --raw

# Delete a variable (use --from when key exists for multiple targets)
ploy config env unset --key OLD_VAR
```

### Target Semantics

**Targets** control which components receive each variable:

| Target | Components | Use Case |
|--------|------------|----------|
| `server` | Server process | Server-side credentials and configuration |
| `nodes` | Node agent processes | Node-level configuration |
| `gates` | Gate jobs (`pre_gate`, `post_gate`) | Build gate credentials |
| `steps` | Step jobs (`mig`, `heal`) | Mig execution credentials |

The `set` command uses **`--on` selectors** for convenience:

| Selector | Expands To | Notes |
|----------|------------|-------|
| `all` | server, nodes, gates, steps | All targets |
| `jobs` | gates, steps | Default when `--on` is omitted |
| `server` | server | Single target |
| `nodes` | nodes | Single target |
| `gates` | gates | Single target |
| `steps` | steps | Single target |

The `show` and `unset` commands use **`--from`** to specify the target:
- When omitted and the key exists for only one target, the target is inferred automatically.
- When the key exists for multiple targets, `--from` is required or the command returns an
  ambiguity error listing available targets.

### Injection Flow

1. **Storage**: Variables are persisted in the `config_env` table as one row per key-target
   pair and cached in the control-plane's `ConfigHolder` at startup.
2. **Claim-time merge**: When a node claims a job via `/v1/nodes/{id}/claim`, the server
   merges matching global env vars into the job's spec based on target-to-job-type mapping
   (gates → pre_gate/post_gate; steps → mig/heal).
   The job spec must be a JSON object; invalid/non-object specs are rejected at submission
   time (400). If a persisted spec in the DB is invalid or non-object, claim fails with a 500.
3. **Precedence**: Per-run env vars (in spec or CLI flags) take precedence—existing keys
   in the spec are never overwritten by global env. Job-target env overrides nodes-target
   env on key collisions.
4. **Container injection**: The node agent propagates the merged `env` map to the
   container runtime, which sets them in the running container. For Build Gate jobs,
   the node agent mirrors job env into the gate spec env so gate build images
   receive the same injected variables.

### Common Variables Consumed by Official Images

| Variable | Consumer | Description |
|----------|----------|-------------|
| `home` (spec field) | `codex` | Per-run file mounts relative to `$HOME` |
| `in` (typed) | `codex`, healing | Read-only input file mounts |
| `OPENAI_API_KEY` | Future OpenAI-integrated migs | API key for LLM operations |
| `PLOY_GRADLE_BUILD_CACHE_URL` | Build Gate (Gradle) | HTTP URL of the remote Gradle Build Cache endpoint (e.g. `http://gradle-build-cache:5071/cache/`). When unset, remote cache is disabled. |
| `PLOY_GRADLE_BUILD_CACHE_PUSH` | Build Gate (Gradle) | Whether to push results to the remote cache. Defaults to `true` when `PLOY_GRADLE_BUILD_CACHE_URL` is set. |

### ORW CLI Contract (Typed)

The shared ORW runtime contract is consumed by runtime and node parsing code to keep ORW behavior deterministic.

Recipe coordinates are required for class-only/custom recipe-artifact mode:

| Variable | Description |
|----------|-------------|
| `RECIPE_GROUP` | Recipe artifact group ID |
| `RECIPE_ARTIFACT` | Recipe artifact ID |
| `RECIPE_CLASSNAME` | Fully qualified recipe class name |

YAML mode defaults (`/out/rewrite.yml` present):

| Variable | Default |
|----------|---------|
| `RECIPE_GROUP` | `org.openrewrite` |
| `RECIPE_ARTIFACT` | `rewrite-java` |
| `RECIPE_CLASSNAME` | `org.openrewrite.java.ChangeMethodName` |

Optional repository and execution controls:

| Variable | Description |
|----------|-------------|
| `ORW_REPOS` | Comma-separated Maven repository URLs |
| `RECIPE_VERSION` | Optional recipe artifact version (when unset, ORW resolves the latest available version from configured repositories) |
| `ORW_REPO_USERNAME` | Repository username (must be paired with `ORW_REPO_PASSWORD`) |
| `ORW_REPO_PASSWORD` | Repository password (must be paired with `ORW_REPO_USERNAME`) |
| `ORW_CONFIG_PATH` | Optional path to rewrite YAML config; when unset ORW uses `/out/rewrite.yml` |
| `ORW_ACTIVE_RECIPES` | Comma-separated override list of active recipes |
| `ORW_FAIL_ON_UNSUPPORTED` | Boolean flag, default `true` |
| `ORW_EXCLUDE_PATHS` | Comma-separated glob patterns excluded from ORW parsing (for example `**/*.proto`); `orw-cli` may append proto3/edition `.proto` paths during preflight |
| `ORW_CLI_BIN` | OpenRewrite CLI executable name/path (default: `rewrite`) |

Required typed file input:

| Path | Description |
|------|-------------|
| `/share/java.classpath` | Newline-delimited absolute classpath entries produced by SBOM/build-gate and mounted into ORW jobs. Gradle cache entries must use `/root/.gradle/...` (not `/home/gradle/.gradle/...`). |

Healing execution (custom recipe via Amata lane):

- Canonical command:
  - `heal-orw --apply --dir /workspace --out /out/orw-task`
- ORW wrapper behavior:
  - `orw-cli` always passes `--classpath-file /share/java.classpath` to the OpenRewrite runner.
  - Missing/invalid `/share/java.classpath` is treated as deterministic `input` failure.
- `rewrite.yml` support:
  - Config resolution order is: `ORW_CONFIG_PATH` -> `/out/rewrite.yml`.
  - When a config file is found, ORW activates top-level `name:` by default.
  - Use `ORW_ACTIVE_RECIPES` to override active recipe names.
  - In YAML mode, runtime fills recipe coordinates with defaults when missing.
  - For custom recipe artifacts, set `RECIPE_GROUP`/`RECIPE_ARTIFACT`/`RECIPE_CLASSNAME` explicitly.
- `heal-orw` requires canonical stack tuple env:
  - `PLOY_STACK_LANGUAGE`
  - `PLOY_STACK_TOOL`
  - `PLOY_STACK_RELEASE`
- `heal-orw` supports only `java+maven` and `java+gradle` tuples.
- On missing or unsupported stack tuple, `heal-orw` writes deterministic
  failure metadata to `/out/orw-task/report.json`.
- Before the first ORW run, `orw-cli` pre-scans workspace `.proto` files and appends proto3/edition paths to `ORW_EXCLUDE_PATHS`.
- Failure artifacts are written to `/out/orw-task/report.json` and
  `/out/orw-task/transform.log`.

`report.json` contract (`/out/report.json`):

```json
{
  "success": false,
  "error_kind": "unsupported",
  "reason": "type-attribution-unavailable",
  "message": "Type attribution is unavailable for this repository"
}
```

Failure taxonomy (`error_kind`):
- `input` — Invalid or missing runtime input.
- `resolution` — Dependency or repository resolution failure.
- `execution` — OpenRewrite CLI execution failure.
- `unsupported` — Deterministic unsupported mode.
- `internal` — Unexpected runtime internal failure.

Unsupported reason contract:
- `error_kind=unsupported` requires `reason=type-attribution-unavailable`.

Run/API metadata propagation:
- When `report.json` contains `success=false`, node uploads:
  - `metadata.orw_error_kind = report.json.error_kind`
  - `metadata.orw_reason = report.json.reason` (when present)

### How Official Images Consume These Variables

**Amata image (`amata`)** ships OpenRewrite helpers:
- `heal-orw` — canonical wrapper that resolves build-system and invokes ORW runtime.
- `orw-cli` — ORW contract wrapper producing deterministic `/out/report.json`.
- `rewrite` — bundled OpenRewrite CLI runner executable used by `orw-cli`.

Config files are delivered via Hydra `home` mounts to their
expected paths under `$HOME`. No env-based materialization is performed:
- `auth.json` → `$HOME/.codex/auth.json` (via `home` mount)
- `config.toml` → `$HOME/.codex/config.toml` (via `home` mount)
- `config.json` → `$HOME/.claude-code-router/config.json` (via `home` mount)
- `crush.json` → `$HOME/.config/crush/crush.json` (via `home` mount)

`amata` sets `CODEX_HOME=$HOME/.codex` by default. Configure delivery via
the run spec `home` field.

If `/root/.claude-code-router/config.json` exists at startup, `amata` runs:
- `ccr start`
- `eval "$(ccr activate)"`

**Build Gate Gradle images (`gate-gradle:*`)**: Ship a Gradle init script under `~/.gradle/init.d/` that enables a remote Gradle Build Cache when `PLOY_GRADLE_BUILD_CACHE_URL` is set (push behavior controlled by `PLOY_GRADLE_BUILD_CACHE_PUSH`).

Build Gate jobs also use node-local persistent tool caches under
`$PLOY_BUILDGATE_CACHE_ROOT/<language>/<tool>/<release>`.
When `PLOY_BUILDGATE_CACHE_ROOT` is unset, default root is `/var/cache/ploy/gates`
and, when not writable, the node falls back to `${TMPDIR:-/tmp}/ploy/gates`.
- Gradle gates mount that path to `/root/.gradle`.
- Maven gates mount that path to `/root/.m2`.

**Java non-gate jobs (`mig`, `heal`)**: Use the same centralized
cache-root policy as Build Gate when stack tuple env is set to Java:
- `PLOY_STACK_LANGUAGE=java`
- `PLOY_STACK_TOOL=gradle|maven`
- `PLOY_STACK_RELEASE=<release>`

Runtime mounts:
- Gradle tuple -> `$PLOY_BUILDGATE_CACHE_ROOT/java/gradle/<release>` to `/root/.gradle`
- Maven tuple -> `$PLOY_BUILDGATE_CACHE_ROOT/java/maven/<release>` to `/root/.m2`
- `java.classpath` portability requirement for Gradle entries: `/root/.gradle/...` only.

When `PLOY_STACK_RELEASE` is empty, runtime uses `unknown-release` as the lane key.
Image-name marker fallback is not used.

**ORW images (`orw-cli-java-17-maven`, `orw-cli-java-17-gradle`)**: Run with the same stack-env-driven cache behavior as other Java lanes while staying isolated from Maven/Gradle project task execution.

Both images ship a bundled `rewrite` executable (`/usr/local/bin/rewrite`) backed
by an embedded standalone runner JAR. `ORW_CLI_BIN` defaults to this bundled
binary and should only be overridden for controlled debugging. Recipes are
resolved dynamically from `RECIPE_GROUP/RECIPE_ARTIFACT` and optional `RECIPE_VERSION`; no
per-recipe image rebuild is required.

ORW jobs use the same stack-env-driven Java cache mounting behavior as other
non-gate jobs, with the same cache-root defaults/fallbacks as Build Gate.

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


### Other
- `PLOY_ARTIFACT_ROOT` — Local artifact caching removed; nodes use ephemeral workspaces.

## Related Docs

- [Migs lifecycle](../migs-lifecycle.md) — Server/node execution and orchestration flow

## Build Gate Limits

The Build Gate executor supports optional resource limits via environment variables on worker nodes:

- `PLOY_BUILDGATE_LIMIT_MEMORY_BYTES` — Memory limit for the gate container. Supports human suffixes
  such as `768MiB`, `1G`, or plain bytes. Parsed with Docker's units parser.
- `PLOY_BUILDGATE_LIMIT_DISK_SPACE` — Disk/quota limit for the gate container's writable layer. Supports
  human suffixes (e.g., `2G`). Passed to Docker as the storage option `size` (driver dependent; requires
  overlay2 with xfs project quotas or equivalent). When unsupported by the driver, container creation may fail.
- `PLOY_BUILDGATE_LIMIT_CPU_MILLIS` — CPU limit in millicores (e.g., `500` = 0.5 CPU, `1500` = 1.5 CPU).
- `PLOY_BUILDGATE_CACHE_ROOT` — Host path root for persistent Build Gate tool caches.
  Default is `/var/cache/ploy/gates`; when unset and not writable, fallback is `${TMPDIR:-/tmp}/ploy/gates`.

Notes:
- Memory and disk limits accept human‑friendly suffixes; CPU uses numeric millicores only.
