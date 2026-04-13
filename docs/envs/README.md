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
- `PLOY_DB_DSN` — Required by local and offline-VPS deploy workflows.
  Used both for host-side setup SQL (DB create/drop, token insert, node seed)
  and injected into the server container as `PLOY_DB_DSN`.
  Host-side DSN may use `localhost`; local deploy rewrites loopback hosts
  (`localhost`, `127.0.0.1`, `::1`) to `host.docker.internal` for container use.
  For VPS deploy, the DSN must already be reachable from both the remote host
  and the remote `server` container; the offline-VPS deploy flow does not rewrite it.
  Non-loopback hosts must be reachable from inside containers.
  Example:
  `postgres://ploy:ploy@localhost:5432/ploy?sslmode=disable`.
- `PLOY_DEPLOY_CA_BUNDLE` — Optional path to a PEM CA bundle used by
  local, runtime-local, and offline-VPS deploy workflows to configure Docker daemon trust for container registries
  (`docker.io`, `registry-1.docker.io`, `auth.docker.io`, `index.docker.io`, `ghcr.io`).
  The script also installs the bundle into system CA trust before restarting
  Docker, so Docker Hub auth/token TLS uses the same root CAs.
  The same bundle is mounted into runtime containers (`server`/`node`) as a local file
  and exported through runtime TLS env vars; it is not baked into images.
  Deploy scripts seed the typed `ca` config field via `ploy config ca set` so
  mig/build-gate containers receive the same CA bundle at runtime through
  the Hydra `ca` materialization path (not as a raw env var).
  Current automation targets:
  - Docker context `colima` (installs CA inside the Colima VM and restarts Docker)
  - Linux hosts (installs CA under `/etc/docker/certs.d/...` and restarts Docker)
  The offline-VPS deploy flow also accepts `PLOY_CA_CERT` as an alias for this value.
- `PLOY_SERVER_PORT` — Optional host port mapped to the server container's internal
  port `8080` in the local compose stack. Default: `8080`. Both local
  and runtime-local/VPS deploy scripts pass it through to the compose stack. Use this when the
  host port `8080` is already occupied (example: `PLOY_SERVER_PORT=18080`).
- `PLOY_RUNTIME_PULL_IMAGES` — Runtime-local deploy toggle for pull-before-start behavior.
  Defaults to `1` (enabled). Set `0`/`false` to skip `docker compose pull`.
- `PLOY_VERSION` — Runtime image semver tag used by `ploy cluster deploy` when explicit image
  overrides are not set. Defaults to `./VERSION` in the repo root.
- `PLOY_RUNTIME_SERVER_IMAGE` — Optional runtime-local server image override.
  Default: `ghcr.io/iw2rmb/ploy/server:${PLOY_VERSION}`.
- `PLOY_RUNTIME_NODE_IMAGE` — Optional runtime-local node image override.
  Default: `ghcr.io/iw2rmb/ploy/node:${PLOY_VERSION}`.
- `WORKER_TOKEN_PATH` — Optional host path used by local deploy scripts to persist the worker bearer
  token and mounted into the node container at `/etc/ploy/bearer-token`.
  Default: `<PLOY_CONFIG_HOME>/<cluster>/bearer-token` (for example `~/.config/ploy/local/bearer-token`).
  If this path is a directory, scripts replace it with a file automatically.
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
  reads this to populate the `Submitter` field when creating mig runs via `ploy mig run`.
- `PLOY_CONTAINER_REGISTRY` — Registry/repository prefix used by runner templates.
  Images resolve to `$PLOY_CONTAINER_REGISTRY/<name>:latest`. Deploy scripts expect this to be provided.
- `PLOY_OBJECTSTORE_ENDPOINT` — S3-compatible endpoint URL provided by environment.
- `PLOY_OBJECTSTORE_ACCESS_KEY` — S3 access key ID provided by environment.
- `PLOY_OBJECTSTORE_SECRET_KEY` — S3 secret access key provided by environment.

- `AUTH_SECRET_PATH` — Optional path to the auth-secret file used by
  local and offline-VPS deploy workflows for JWT signing secret reuse.
  Defaults:
  - local deploy: `auth-secret.txt` under local deploy workspace
  - VPS deploy: `auth-secret.txt` under offline-VPS deploy workspace
- `CLUSTER_ID` — Optional cluster ID used by local and offline-VPS deploy workflows
  when generating bearer tokens. Default: `local`.
- `NODE_ID` — Optional node ID used by local and offline-VPS deploy workflows
  when seeding the default worker node row and worker token
  description. Default: `local1`.
- `DOCKERHUB_PAT` — Optional Docker Hub Personal Access Token for authenticated pulls when you use Docker Hub
  as `PLOY_CONTAINER_REGISTRY`.
- `PLOY_OPENAI_API_KEY` — Optional OpenAI API key propagated to Migs LLM lanes. When set on the control
  plane, the runner injects it into the `migs-llm` container as `OPENAI_API_KEY`. You can also set it on
  worker nodes via a systemd drop-in to make it available cluster-wide.
- Cross-phase input directory: `/in` is mounted read-only for healing migs (e.g., `codex`).
  - `/in/build-gate.log` — First Build Gate failure log (primarily from claim `recovery_context`; node-local cache fallback)
  - `/in/errors.yaml` — Structured gate errors payload hydrated from claim `recovery_context.errors` when present (optional)
  - `/in/gate_profile.json` — Gate profile used by the failed gate when available (provided for `infra` healing context)
  - `/in/gate_profile.schema.json` — Gate profile schema for `infra` healing context (`title: Ploy Build Gate Profile`, includes `$comment` guidance for key fields)
  - `/in/amata.yaml` — Amata workflow spec materialized from `amata.spec`
- `--spec` — Path to a YAML/JSON spec file for `ploy run` defining mig parameters,
  Build Gate settings, and healing configuration. The spec supports:
  - `envs` — Environment variables (key-value map, merged by key across precedence layers)
  - `ca` — CA certificate entries (local paths or canonical `shortHash` values)
  - `in` — Read-only input files (`src:/in/dst`; CLI compiles local paths to `shortHash:/in/dst`)
  - `out` — Read-write output files (`src:/out/dst`; CLI compiles local paths to `shortHash:/out/dst`)
  - `home` — Home-relative files (`src:dst{:ro}`; CLI compiles to `shortHash:dst{:ro}`)
  - `steps[]` — Multi-step spec steps (each with its own `image`/`command`/`envs`/`ca`/`in`/`out`/`home`)
  - `build_gate.heal` — Automated healing action for Build Gate failures (`retries`, `image`, `command`, `envs`, `ca`, `in`, `out`, `home`, optional `amata`, optional `expectations`)
  - GitLab MR settings (`mr_on_success`, `mr_on_fail`, `gitlab_domain`, `gitlab_pat`)
  - See [mig.example.yaml](../schemas/mig.example.yaml) for the full schema.

### Hydra file-record compilation

The CLI compiles local file paths in `ca`, `in`, `out`, and `home` fields into
canonical `shortHash:dst` records before spec submission:

1. Resolves each source path relative to the spec file directory.
2. Computes a content hash, uploads the archive when missing, and rewrites
   the entry to `shortHash:dst` form.
3. The node agent downloads bundles by hash and mounts them at the declared
   destination (read-only for `ca`/`in`, read-write for `out`, configurable
   for `home`).

**Example spec fragment (before CLI compile):**
```yaml
steps:
  - image: docker.io/your-dh-user/migs-openrewrite:latest
    in:
      - ./recipe.yaml:/in/recipe.yaml
    ca:
      - ./ca-bundle.pem

build_gate:
  heal:
    image: docker.io/your-dh-user/amata:latest
    in:
      - ./prompt-extra.txt:/in/prompt-extra.txt
```

**After CLI compile (canonical form submitted to server):**
```yaml
steps:
  - image: docker.io/your-dh-user/migs-openrewrite:latest
    in:
      - "a1b2c3d4e5f6g7:/in/recipe.yaml"
    ca:
      - "f8e9d0c1b2a3"

build_gate:
  heal:
    image: docker.io/your-dh-user/amata:latest
    in:
      - "g7h8i9j0k1l2:/in/prompt-extra.txt"
```

- `--name` — Creates a mig project with `ploy mig add --name <name> [--spec <path|->]`.
  Use `ploy mig run repo add` to attach multiple repositories under a shared spec, then run it via
  `ploy mig run <mig-id|name> [--follow]`.
  Example: `ploy mig add --name my-batch --spec mig.yaml` followed by
  `ploy mig run repo add --repo-url https://... --base-ref main --target-ref feature my-batch`.
  See [Migs lifecycle](../migs-lifecycle.md) § "1.4 Batched Migs Runs (`runs` + `run_repos`)"
  for full usage.
  - `build_gate.heal` — Spec block defining the single healing action:
  - Action fields support include-composition (`retries`, `image`, `command`, `envs`, `ca`, `in`, `out`, `home`, optional `amata`, optional `expectations`)
  - After each healing attempt, the Build Gate is re-run; on pass, the main mig proceeds
  - If healing exhausts retries and gate still fails, run terminates with `reason="build-gate"`
  - Cross-phase inputs (`/in/build-gate.log`, optional `/in/errors.yaml`, `/in/gate_profile.json`, `/in/amata.yaml`) are available to healing migs
  - Task-oriented healing routers may consume `/in/errors.yaml` and emit `tasks[]` with `error_kind` in `code|deps|infra` and `items[]` indexes into `errors` entries
  - For `expectations.artifacts` schema `gate_profile_v1`, healing is expected to write `/out/gate-profile-candidate.json` with explicit `targets.active` (`all_tests|unit|build|unsupported`); candidate promotion to repo `gate_profile` occurs only on successful follow-up `re_gate`
  - Terminal unsupported candidate contract: `targets.active=unsupported`, `targets.build.status=failed`, `targets.build.failure_code=infra_support`
- Container cleanup model:
  - Containers are retained after step/gate completion.
  - Cleanup trigger: before claim; threshold: 1 GiB free on Docker data-root filesystem (`DockerRootDir`).
- Gate status visibility: Use `GET /v1/runs/{id}/status` to view gate results (format: `Gate: passed duration=1234ms` or `Gate: failed pre-gate duration=567ms`) via `Metadata["gate_summary"]`.
- SBOM compatibility contract for `deps` healing:
  - Successful gate outputs under `/out/*` are the evidence source for SBOM persistence and compatibility lookup.
  - Healing claims may provide stack-prefilled compatibility endpoint input (`/v1/sboms/compat?...`) to `deps` strategies.
  - No dedicated SBOM environment variables exist in this slice; stack identity comes from gate metadata (`lang`, `release`, `tool`) and claim context.
  - TODO: if future slices add query limits/timeouts as env/config knobs, document them here.

## Healing Container Environment

The node agent injects the following environment variables into healing containers to support
Build Gate verification. These vars enable healing migs to derive the same Git baseline used
by the Migs run.

Repo metadata (injected from StartRunRequest):
- `PLOY_REPO_URL` — Git repository URL for cloning/verification (same as the Migs run)
- `PLOY_BASE_REF` — Base Git reference (branch or tag) for the run
- `PLOY_TARGET_REF` — Target Git reference for the run
- `PLOY_COMMIT_SHA` — Pinned commit SHA when available (may be empty)

Server connection details:
- `PLOY_SERVER_URL` — Control plane base URL (e.g., `https://<server>:8443`)
- `PLOY_HOST_WORKSPACE` — Host filesystem path to workspace for in-container tooling
- CA certificates are delivered via Hydra `ca` mount entries (mounted under `/etc/ploy/ca/`), not as an environment variable.
- `PLOY_CLIENT_CERT_PATH` — Path to client certificate (`/etc/ploy/certs/client.crt`)
- `PLOY_CLIENT_KEY_PATH` — Path to client key (`/etc/ploy/certs/client.key`)
- `PLOY_API_TOKEN` — Bearer token for API authentication (when configured on node).

Healing runtime context:
- `PLOY_GATE_PHASE` — phase that failed (`pre_gate|post_gate|re_gate`)
- `PLOY_LOOP_KIND` — loop context (`healing`)

See [Build Gate docs](../build-gate/README.md) for Build Gate configuration and execution details.
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

Runtime behavior: the node's Docker client is created from standard Docker env vars with API version negotiation enabled.

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

- `ploy run` executes Migs against the Ploy control plane; no tenant variable is required.
- `PLOY_E2E_RUN_PREFIX` — Optional run ID prefix for Migs E2E runs
  (default `e2e`).
- `PLOY_E2E_REPO_OVERRIDE` — Optional Git repository override used by the Migs
  E2E scenarios in place of the default Java sample repo.
- `PLOY_E2E_GITLAB_TOKEN` — Optional GitLab PAT so the E2E harness can clean up
  branches after creating merge requests.
- `PLOY_E2E_LIVE_SCENARIOS` — Optional comma-separated scenario IDs that the
  live Migs smoke test should execute (defaults to `simple-openrewrite`).

## GitLab Merge Request Integration

Ploy can automatically create GitLab merge requests when Migs runs complete.

**Recommended approach:** Use `ploy config gitlab set` to store credentials on the control plane
(see [docs/how-to/create-mr.md](../how-to/create-mr.md) for usage examples).

Control plane configuration (set via CLI or YAML):
- `gitlab.domain` (config YAML) — GitLab base URL or host (e.g., `https://gitlab.com` or `gitlab.com`). Optional; Ploy normalizes either form.
- `gitlab.token` (config YAML) — Inline GitLab Personal Access Token. Optional; stored only in
  memory at runtime, not persisted back to disk.
- `gitlab.token_file` (config YAML) — Path to a file containing the PAT. Optional. See details below.

Per-run overrides (CLI flags on `ploy run`):
- `--gitlab-pat` — Override the control plane PAT for this run only
- `--gitlab-domain` — Override the control plane domain for this run only
- `--mr-success` — Create an MR when the run succeeds
- `--mr-fail` — Create an MR when the run fails

Branch naming semantics:
- The MR source branch is always the effective target ref for the run. When `--target-ref` is provided, that value is used. When it is omitted, the node derives a default of `ploy/{run_name|run_id}` using the run name when set (e.g., batch name) or the run ID (KSUID string) otherwise.
- The base branch is whatever you pass via `--base-ref` (commonly `main`).

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
ploy run --mr-success \
  --repo https://gitlab.com/org/repo.git \
  --base-ref main \
  --target-ref workflow/upgrade

# Per-run override
ploy run --mr-success \
  --gitlab-pat glpat-xxxxxxxxxxxxxxxxxxxx \
  --gitlab-domain https://gitlab.example.com \
  --repo https://gitlab.example.com/org/repo.git \
  --base-ref main \
  --target-ref workflow/upgrade
```



## gapi

- No environment variables are active for gapi within this codebase.

## Control Plane

- (removed) `PLOY_CONTROL_PLANE_URL` — Legacy override removed. Components derive the endpoint and token from
  the default cluster descriptor under `PLOY_CONFIG_HOME` (or home default).

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
CA bundles, and API keys without embedding them in every spec file.

### Configuration via CLI

Use the `ploy config env` subcommands to manage global environment variables:

```bash
# Set CA certificates via typed config
# Sections: pre_gate, re_gate, post_gate, mig, heal, sbom, hook
ploy config ca set --file ca-bundle.pem --section pre_gate --section re_gate

# Set OpenAI API key (injected into gate and step jobs — default --on jobs)
ploy config env set --key OPENAI_API_KEY --value sk-...

# List configured variables (secret values redacted)
ploy config env list

# Show a specific variable (use --raw to reveal secret values)
ploy config env show --key OPENAI_API_KEY --raw

# Delete a variable (use --from when key exists for multiple targets)
ploy config env unset --key OLD_VAR
```

**Typed config fields** manage structured data that was previously carried as raw env keys.
Use the dedicated typed config commands:
- `ploy config ca set/unset/ls` — CA certificates (sections: pre_gate, re_gate, post_gate, mig, heal, sbom, hook)
- `ploy config home set/unset/ls` — Home-relative file mounts

### Target Semantics

**Targets** control which components receive each variable:

| Target | Components | Use Case |
|--------|------------|----------|
| `server` | Server process | Server-side credentials and configuration |
| `nodes` | Node agent processes | Node-level configuration |
| `gates` | Gate jobs (`pre_gate`, `re_gate`, `post_gate`) | Build gate credentials |
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
   (gates → pre_gate/re_gate/post_gate; steps → mig/heal).
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
| `ca` (typed) | ORW migs, build-gate, custom migs | PEM-encoded CA certificates; mounted via Hydra `ca` materialization at `/etc/ploy/ca/<hash>` |
| `home` (typed) | `codex` | File mounts relative to $HOME (replaces `CODEX_AUTH_JSON`, `CODEX_CONFIG_TOML`, `CCR_CONFIG_JSON`, `CRUSH_JSON`) |
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
| `RECIPE_VERSION` | Optional recipe artifact version (defaults to `8.74.3`) |
| `ORW_REPO_USERNAME` | Repository username (must be paired with `ORW_REPO_PASSWORD`) |
| `ORW_REPO_PASSWORD` | Repository password (must be paired with `ORW_REPO_USERNAME`) |
| `ORW_CONFIG_PATH` | Optional path to rewrite YAML config; when unset ORW uses `/out/rewrite.yml` |
| `ORW_ACTIVE_RECIPES` | Comma-separated override list of active recipes |
| `ORW_FAIL_ON_UNSUPPORTED` | Boolean flag, default `true` |
| `ORW_EXCLUDE_PATHS` | Comma-separated glob patterns excluded from ORW parsing (for example `**/*.proto`) |
| `ORW_CLI_BIN` | OpenRewrite CLI executable name/path (default: `rewrite`) |

Healing execution (custom recipe via Amata lane):

- Canonical command:
  - `heal-orw --apply --dir /workspace --out /out/orw-task`
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
- If `ORW_EXCLUDE_PATHS` is unset/empty, no exclude globs are applied.
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

**Amata image (`amata`)**: when `amata.spec` is set on a mig step or healing action,
the container runs `amata run /in/amata.yaml` (with optional `--set` flags).

The `amata` image also ships OpenRewrite healing helpers:
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
`ploy config home set` or the spec `home` field.

If `/root/.claude-code-router/config.json` exists at startup, `amata` runs:
- `ccr start`
- `eval "$(ccr activate)"`

**Build Gate images (Maven/Gradle)**: The gate executor uses the Hydra `ca` mount
entries to install CA certificates into the container trust store:
1. CA bundles are delivered as files under `/etc/ploy/ca/<hash>`.
2. The gate preamble splits the PEM bundle into individual `.crt` files.
3. Copies them to `/usr/local/share/ca-certificates/ploy/`.
4. Runs `update-ca-certificates` (on Debian/Ubuntu images).
5. Optionally imports into Java cacerts via `keytool` when available.
6. Exports `SSL_CERT_FILE`, `CURL_CA_BUNDLE`, and `GIT_SSL_CAINFO` to the resolved path.

**Build Gate Gradle images (`gate-gradle:*`)**: Ship a Gradle init script under `~/.gradle/init.d/` that enables a remote Gradle Build Cache when `PLOY_GRADLE_BUILD_CACHE_URL` is set (push behavior controlled by `PLOY_GRADLE_BUILD_CACHE_PUSH`).

Build Gate jobs also use node-local persistent tool caches under
`$PLOY_BUILDGATE_CACHE_ROOT/<language>/<tool>/<release>`.
When `PLOY_BUILDGATE_CACHE_ROOT` is unset, default root is `/var/cache/ploy/gates`
and, when not writable, the node falls back to `${TMPDIR:-/tmp}/ploy/gates`.
- Gradle gates mount that path to `/home/gradle/.gradle`.
- Maven gates mount that path to `/root/.m2`.
- On every gate execution, before mounting the cache path, the node checks free
  space on that filesystem. If free space is below `2 GiB`, it prunes entries in
  the active cache directory (`<language>/<tool>/<release>`) from oldest to newest
  until free space reaches `2 GiB` or the directory is exhausted.

**SBOM Gradle image (`sbom-gradle`)**: Uses the same Gradle init script and
centralized cache-root policy as Gradle gate jobs. Runtime mounts
`$PLOY_BUILDGATE_CACHE_ROOT/java/gradle/17` to `/home/gradle/.gradle` (with the
same fallback root and prune policy when `PLOY_BUILDGATE_CACHE_ROOT` is unset).

**ORW images (`orw-cli-maven`, `orw-cli-gradle`)**: Same Hydra `ca` materialization behavior as
build-gate, ensuring OpenRewrite can fetch dependencies from internal artifact repositories while
staying isolated from Maven/Gradle project task execution.

Both images ship a bundled `rewrite` executable (`/usr/local/bin/rewrite`) backed
by an embedded standalone runner JAR. `ORW_CLI_BIN` defaults to this bundled
binary and should only be overridden for controlled debugging. Recipes are
resolved dynamically from `RECIPE_GROUP/RECIPE_ARTIFACT` and optional `RECIPE_VERSION`; no
per-recipe image rebuild is required.

ORW jobs also support node-local persistent runtime dependency cache under
`$PLOY_ORW_CACHE_ROOT/maven/<image>`, mounted into the container at `/root/.m2`.
When `PLOY_ORW_CACHE_ROOT` is unset, default root is `/var/cache/ploy/orw` and,
when not writable, the node falls back to `${TMPDIR:-/tmp}/ploy/orw`.
The same oldest-first free-space pruning policy is applied in the active ORW
cache directory before mounting.

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
- [Deployment](../how-to/deploy.md) — Local Docker cluster

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
- `PLOY_ORW_CACHE_ROOT` — Host path root for persistent ORW runtime recipe/dependency cache.
  Default is `/var/cache/ploy/orw`; when unset and not writable, fallback is `${TMPDIR:-/tmp}/ploy/orw`.

Notes:
- Memory and disk limits accept human‑friendly suffixes; CPU uses numeric millicores only.
