# Environment Variables

This reference tracks the environment variables that the workstation CLI
inspects today and notes the current local values. Update this file whenever a
new variable is introduced, defaults change, or components adopt additional
configuration.

## Dependencies

- [cmd/ploy/dependencies.go](../../cmd/ploy/dependencies.go) — runtime factories
  resolving control-plane and IPFS endpoints.
- [cmd/ploy/feature_flags.go](../../cmd/ploy/feature_flags.go) — feature flag
  inspection for the Aster integration.

## CLI

- `PLOY_RUNTIME_ADAPTER` — Optional runtime adapter selector. Defaults to
  `local-step`. Other adapters (e.g., `k8s`, `nomad`) can plug in here; the CLI
  fails fast when an unknown adapter name is provided.
- `PLOY_ASTER_ENABLE` — Opt-in switch for the experimental Aster bundle
  integration. Current default: `unset` (Aster toggles stay disabled).
- `PLOY_CONTROL_PLANE_URL` — Optional override for the control-plane base URL when cached descriptors do not yet
  embed the endpoint (new workstation) or you need to target a secondary cluster explicitly. Descriptors discovered via
  `ploy cluster add` remain the default for CLI calls such as `ploy upload` and `ploy report`.
- `PLOY_IPFS_CLUSTER_API` — Base URL for the IPFS Cluster REST API used by the
  step runtime and the control-plane artifact publisher. Workstations still
  read this value when executing Mods locally, but `ploy artifact *`, `ploy upload`,
  and `ploy report` routes now talk to the control plane instead of hitting the
  cluster directly. **Required on ployd worker nodes** so the step executor can
  publish diff/log bundles after each job.
- `PLOY_IPFS_CLUSTER_TOKEN` — Optional bearer token passed to the cluster when
  authenticating artifact requests.
- `PLOY_IPFS_CLUSTER_USERNAME` / `PLOY_IPFS_CLUSTER_PASSWORD` — Optional
  basic-auth credentials used when a bearer token is not available. Username
  and password must be provided together.
- `PLOY_IPFS_CLUSTER_REPL_MIN` — Optional override for the minimum replication
  factor applied to artifact pins. Defaults to the cluster-defined value when
  unset or zero.
- `PLOY_IPFS_CLUSTER_REPL_MAX` — Optional override for the maximum replication
  factor applied to artifact pins. Defaults to the cluster-defined value when
  unset or zero.

  Additional worker guards (for unstable clusters):
  - `PLOY_IPFS_CLUSTER_LOCAL` — When `true`/`1`, workers publish artifacts with
    `local=true` to prefer the local IPFS daemon and reduce cross‑peer pressure.
  - `PLOY_HYDRATION_PUBLISH_SNAPSHOT` — When `false`/`0`, workers skip publishing
    the repo hydration snapshot to IPFS Cluster and hydrate directly from the
    local tarball.
  - `PLOY_ARTIFACT_PUBLISH` — When `false`/`0`, workers skip publishing diff/log
    artifacts entirely. Live logs still stream via SSE.
- `PLOY_IPFS_GATEWAY` — Optional IPFS HTTP gateway base URL used for artifact
  uploads from the workstation. Not required on nodes (they use IPFS Cluster directly).
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
- `PLOY_ETCD_USERNAME` / `PLOY_ETCD_PASSWORD` — Optional etcd basic-auth credentials applied when
  ployd connects to the local etcd instance.
- `PLOY_ETCD_TLS_CA` — Path to a PEM bundle used to trust etcd server certificates. Optional.
- `PLOY_ETCD_TLS_CERT` / `PLOY_ETCD_TLS_KEY` — Optional client certificate pair presented to etcd
  when mutual TLS is required. Both values must be provided together.
- `PLOY_ETCD_TLS_SKIP_VERIFY` — When set to `true`, disables server certificate verification. Use
  only for local development.
- `PLOYD_CONFIG_PATH` — When set during bootstrap, overrides the generated ployd configuration file
  location (default `/etc/ploy/ployd.yaml`).
- `PLOYD_HTTP_LISTEN` — Optional address override for the ployd HTTP API listener when bootstrap
  generates the initial configuration (default `0.0.0.0:8443`).
- `PLOYD_METRICS_LISTEN` — Optional override for the ployd Prometheus metrics listener (defaults to
  `:9100`).
- `PLOY_SSH_USER` — SSH username applied when establishing control-plane tunnels (default `ploy`).
- `PLOY_SSH_IDENTITY` — Path to the SSH private key used for tunnel authentication (default `~/.ssh/id_rsa`).
- `PLOY_SSH_SOCKET_DIR` — Override for the directory holding SSH control sockets (default `~/.ploy/tunnels`).
- `PLOY_CACHE_HOME` — Optional base directory for CLI cache artifacts such as tunnel node assignments.
- `PLOY_ARTIFACT_ROOT` — Optional override for the local artifact cache used by the step workspace hydrator and filesystem artifact publisher. Defaults to `$XDG_CACHE_HOME/ploy/artifacts` (or the OS cache dir fallback) when unset.
  

## Worker Nodes

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

- `PLOY_GITLAB_SIGNER_AES_KEY` — Required base64-encoded AES key used by the signer
  to encrypt GitLab API keys before persisting them in etcd. The decoded key must be
  16, 24, or 32 bytes to satisfy AES-GCM requirements.
- `PLOY_CLUSTER_ID` — Optional override for the cluster identifier the control plane writes inside
  etcd prefixes (defaults to the value recorded in `/etc/ploy/cluster-id`). Set this when running
  multiple clusters from the same environment and `/etc/ploy/cluster-id` is unavailable.
- `PLOY_CONTROL_PLANE_URL` — Optional control-plane base URL override used by `ploy config gitlab`, worker onboarding,
  and CLI log/streaming commands. When unset, the CLI derives the endpoint plus CA bundle from the cached cluster descriptor.
- `PLOY_GITLAB_SIGNER_DEFAULT_TTL` — Optional duration (e.g., `15m`) applied when
  callers omit a TTL while requesting short-lived GitLab tokens.
- `PLOY_GITLAB_SIGNER_MAX_TTL` — Optional duration that caps the maximum issued TTL.
  Requests above this threshold are rejected. Defaults to `12h` when unset.
- `PLOY_GITLAB_API_BASE_URL` — Base URL for GitLab API requests when revoking
  stale runner tokens during credential rotations.
- `PLOY_GITLAB_ADMIN_TOKEN` — Admin or automation token presented to GitLab when
  calling the revocation API. Required for the rotation revocation workflow to
  disable stale tokens across nodes.
- `PLOY_TRANSFERS_BASE_DIR` — Optional override for the SSH slot staging root on control-plane
  nodes. Defaults to `/var/lib/ploy/ssh-artifacts` and is referenced by the slot guard wrapper that
  bootstrap installs.

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

## Related Docs

- [docs/design/overview/README.md](../design/overview/README.md)
- [docs/design/workflow-rpc-alignment/README.md](../design/workflow-rpc-alignment/README.md)
- [docs/design/ipfs-artifacts/README.md](../design/ipfs-artifacts/README.md)
 
