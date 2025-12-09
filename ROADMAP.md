# ploy config env global integration

Scope: Introduce a unified `ploy config env` surface and control-plane storage for global environment variables (GitLab domain/PAT, CA cert bundles, Codex auth JSON, OpenAI API keys). Ensure these globals are injected into all relevant jobs (mods, healing, gate workers) in a consistent way and consumed by official images (e.g., mod-codex, ORW, build-gate images).

Documentation: /Users/vk/@iw2rmb/auto/ROADMAP.md, ROADMAP.md, cmd/ploy/README.md, docs/envs/README.md, docs/mods-lifecycle.md, docs/api/OpenAPI.yaml, internal/server/config, internal/server/handlers/config_gitlab.go, internal/server/handlers/nodes_claim.go, internal/server/handlers/spec_utils.go, internal/nodeagent/claimer_spec.go, internal/nodeagent/manifest.go, internal/workflow/runtime/step/gate_docker.go, docker/mods/mod-codex/mod-codex.sh, docker/mods/orw-maven/orw-maven.sh, docker/mods/orw-gradle/orw-gradle.sh.

Legend: [ ] todo, [x] done.

## Server Storage & Wiring
- [x] Add `config_env` table and wire into server config load — Persist global env entries (including secrets) in the control-plane database with scope metadata.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/store (migrations + store API), internal/server/config
  - Scope:
    - Add a new migration to create table `config_env` with columns: `key TEXT PRIMARY KEY`, `value TEXT NOT NULL`, `scope TEXT NOT NULL`, `secret BOOLEAN NOT NULL DEFAULT true`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`. Use `scope` for selection semantics (e.g., `mods`, `heal`, `gate`, `all`) and `secret` to control redaction at the CLI/HTTP layer.
    - Extend `internal/store` with a small CRUD interface for global envs, e.g.:
      ```go
      type GlobalEnv struct {
        Key     string
        Value   string
        Scope   string
        Secret  bool
        Updated time.Time
      }

      type Store interface {
        ListGlobalEnv(ctx context.Context) ([]GlobalEnv, error)
        GetGlobalEnv(ctx context.Context, key string) (GlobalEnv, error)
        UpsertGlobalEnv(ctx context.Context, env GlobalEnv) error
        DeleteGlobalEnv(ctx context.Context, key string) error
      }
      ```
    - Keep GitLab YAML config (`cfg.GitLab`) for backward compatibility; global env storage is additive and can later supersede inline GitLab config when desired.
  - Snippets:
    - Migration table sketch:
      ```sql
      CREATE TABLE config_env (
        key         TEXT PRIMARY KEY,
        value       TEXT NOT NULL,
        scope       TEXT NOT NULL,
        secret      BOOLEAN NOT NULL DEFAULT TRUE,
        updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
      );
      ```
  - Tests: Add store-level unit tests to verify CRUD semantics against a test database or pgxpool mock — expect round-trip of `GlobalEnv` entries and primary-key enforcement on `key`.

## ConfigHolder & Config Env HTTP API
- [x] Extend ConfigHolder and register `/v1/config/env` endpoints — Provide in-memory access to global env entries and a typed HTTP surface used by the CLI.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/server/handlers, cmd/ployd/server.go
  - Scope:
    - Extend `internal/server/handlers/config_gitlab.go`’s `ConfigHolder` or introduce a sibling struct to track both GitLab settings and global env map:
      ```go
      type GlobalEnvVar struct {
        Value  string
        Scope  string
        Secret bool
      }

      type ConfigHolder struct {
        mu        sync.RWMutex
        gitlab    config.GitLabConfig
        globalEnv map[string]GlobalEnvVar
      }
      ```
    - Initialize `ConfigHolder` in `cmd/ployd/server.go` with GitLab config (existing) plus a `map[string]GlobalEnvVar` loaded from `store.ListGlobalEnv(ctx)` at startup.
    - Add methods:
      ```go
      func (h *ConfigHolder) GetGlobalEnv() map[string]GlobalEnvVar
      func (h *ConfigHolder) GetGlobalEnvVar(key string) (GlobalEnvVar, bool)
      func (h *ConfigHolder) SetGlobalEnvVar(key string, v GlobalEnvVar)
      func (h *ConfigHolder) DeleteGlobalEnvVar(key string)
      ```
    - Implement new handlers under `internal/server/handlers`, e.g. `config_env.go`:
      - `GET /v1/config/env` → list keys with scope/secret flags, no values for secrets.
      - `GET /v1/config/env/{key}` → return one entry (value included; rely on mTLS + admin role).
      - `PUT /v1/config/env/{key}` → upsert value/scope/secret and update `ConfigHolder` + `store.UpsertGlobalEnv`.
      - `DELETE /v1/config/env/{key}` → delete from store and `ConfigHolder`.
    - Wire endpoints in `RegisterRoutes` (Config tag) with same auth model as GitLab config.
  - Snippets:
    - Example response shape:
      ```json
      {
        "key": "CA_CERTS_PEM_BUNDLE",
        "value": "-----BEGIN CERTIFICATE-----\n...",
        "scope": "all",
        "secret": true
      }
      ```
  - Tests: Add `config_env_authz_test.go` to assert only cli-admin can access these endpoints and `config_env_test.go` to verify round-trip between HTTP, `ConfigHolder`, and `store` — expect that PUT then GET returns identical JSON payload (modulo redaction in list view).

## CLI Surface: `ploy config env`
- [ ] Add `ploy config env` subcommands — Provide a single CLI entrypoint for managing all global env vars (GitLab, CA bundles, Codex auth JSON, OpenAI keys).
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy
  - Scope:
    - Extend `handleConfig` in `cmd/ploy/config_command.go`:
      ```go
      func handleConfig(args []string, stderr io.Writer) error {
        // ...
        switch args[0] {
        case "gitlab":
          return handleConfigGitLab(args[1:], stderr)
        case "env":
          return handleConfigEnv(args[1:], stderr)
        default:
          // ...
        }
      }
      ```
    - Implement `handleConfigEnv(args []string, stderr io.Writer) error` and usage helpers in a new file `cmd/ploy/config_env_command.go` with subcommands:
      - `ploy config env list`
      - `ploy config env show --key <NAME> [--raw]`
      - `ploy config env set --key <NAME> (--value <STRING> | --file <PATH>) [--scope mods|heal|gate|all] [--secret=true|false]`
      - `ploy config env unset --key <NAME>`
    - Use the same HTTP resolution helper as GitLab config (`resolveControlPlaneHTTP`) and the new `/v1/config/env*` routes.
    - For secret values, default `--secret=true` and redact output in `list`/`show` unless `--raw` is explicitly passed.
  - Snippets:
    - Flag parsing sketch:
      ```go
      fs := flag.NewFlagSet("config env set", flag.ContinueOnError)
      var key, value, file, scope string
      var secret bool
      fs.StringVar(&key, "key", "", "Environment variable name (e.g., CA_CERTS_PEM_BUNDLE)")
      fs.StringVar(&value, "value", "", "Inline value (mutually exclusive with --file)")
      fs.StringVar(&file, "file", "", "Path to file containing value")
      fs.StringVar(&scope, "scope", "all", "Scope: mods, heal, gate, all")
      fs.BoolVar(&secret, "secret", true, "Mark value as secret (redacted by default)")
      ```
  - Tests: Add `cmd/ploy/config_env_command_flags_test.go` to cover flag/usage errors and `cmd/ploy/config_env_command_files_test.go` to exercise list/show/set/unset flows against a fake HTTP server — expect correct HTTP methods/paths and redaction behavior for secrets.

## Spec Merge on Job Claim
- [ ] Merge global env into job spec env on claim — Ensure every job spec carries the right global env vars before it reaches the node agent.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/server/handlers
  - Scope:
    - Extend `internal/server/handlers/spec_utils.go` with:
      ```go
      func mergeGlobalEnvIntoSpec(spec json.RawMessage, env map[string]GlobalEnvVar, jobType string) json.RawMessage {
        if len(env) == 0 {
          return spec
        }
        var m map[string]any
        if len(spec) > 0 && json.Valid(spec) {
          _ = json.Unmarshal(spec, &m)
        }
        if m == nil {
          m = map[string]any{}
        }
        em, _ := m["env"].(map[string]any)
        if em == nil {
          em = map[string]any{}
        }
        for k, v := range env {
          if !scopeMatches(jobType, v.Scope) {
            continue
          }
          if _, exists := em[k]; exists {
            continue // per-run env wins over global
          }
          em[k] = v.Value
        }
        m["env"] = em
        b, _ := json.Marshal(m)
        return json.RawMessage(b)
      }
      ```
    - In `buildAndSendJobClaimResponse` (`internal/server/handlers/nodes_claim.go`), after `mergeGitLabConfigIntoSpec`, call:
      ```go
      mergedSpec = mergeGlobalEnvIntoSpec(mergedSpec, configHolder.GetGlobalEnv(), job.ModType)
      ```
      so that claimed jobs always include `CA_CERTS_PEM_BUNDLE`, `CODEX_AUTH_JSON`, `OPENAI_API_KEY`, etc. when configured.
    - Implement `scopeMatches(jobType, scope string) bool` to map scopes like `mods`, `heal`, `gate`, `all` to job types (`mod`, `heal`, `pre_gate`, `re_gate`, `post_gate`).
  - Snippets:
    - Minimal scope matcher:
      ```go
      func scopeMatches(jobType, scope string) bool {
        switch scope {
        case "all":
          return true
        case "mods":
          return jobType == "mod" || jobType == "post_gate"
        case "heal":
          return jobType == "heal" || jobType == "re_gate"
        case "gate":
          return jobType == "pre_gate" || jobType == "re_gate" || jobType == "post_gate"
        default:
          return false
        }
      }
      ```
  - Tests: Add `internal/server/handlers/spec_utils_global_env_test.go` to cover merge semantics (per-run env override, scope filtering, empty spec) and extend `server_runs_claim_test.go` to assert that a claimed job contains merged `env["CA_CERTS_PEM_BUNDLE"]` / `env["CODEX_AUTH_JSON"]` when the holder is pre-populated.

## Node Agent Propagation
- [ ] Keep env propagation from spec → StartRunRequest → manifests generic — Confirm that global env vars injected into specs arrive intact in containers and gate jobs.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/nodeagent
  - Scope:
    - Verify `parseSpec` in `internal/nodeagent/claimer_spec.go` already populates `env` from `m["env"].(map[string]any)`; no new keys are needed for global env.
    - Confirm `StartRunRequest` (`internal/nodeagent/handlers.go`) keeps `Env map[string]string` and callers do not filter keys.
    - Ensure `buildManifestFromRequestWithStack` (`internal/nodeagent/manifest.go`) copies `req.Env` into `manifest.Env` exactly once per step:
      ```go
      env := make(map[string]string, len(req.Env))
      for k, v := range req.Env {
        env[k] = v
      }
      // ... merge step-specific env where applicable
      ```
    - For gate jobs, ensure `buildGateManifestFromRequest` preserves `Env` from `StartRunRequest` and that gate executors pass `Gate.Env` through unchanged.
  - Snippets:
    - Example `Env` copy:
      ```go
      env := make(map[string]string, len(req.Env))
      for k, v := range req.Env {
        env[k] = v
      }
      manifest.Env = env
      ```
  - Tests: Extend `internal/nodeagent/claimer_gitlab_config_test.go` or add `claimer_global_env_test.go` to assert that when spec includes `env: { "CODEX_AUTH_JSON": "..." }`, the resulting `StartRunRequest.Env` passes through to manifests and that gate manifests also expose the same key when built via `buildGateManifestFromRequest`.

## Container Runtime & Gate Integration
- [ ] Ensure container specs pass env through to Docker — Confirm that the Docker runtime receives all env keys from manifests, ready for image-level startup hooks to consume.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/workflow/runtime/step
  - Scope:
    - In `internal/workflow/runtime/step/container_spec.go`, verify `ContainerSpec.Env` is populated from the manifest without filtering:
      ```go
      return ContainerSpec{
        Image:      manifest.Image,
        Command:    append([]string{}, manifest.Command...),
        WorkingDir: wd,
        Env:        manifest.Env,
        // ...
      }, nil
      ```
    - In `gate_docker.go`, change the executor to use `spec.Env` when creating the gate container:
      ```go
      env := map[string]string{}
      for k, v := range spec.Env {
        env[k] = v
      }
      specC := ContainerSpec{
        Image:            image,
        Command:          cmd,
        WorkingDir:       "/workspace",
        Mounts:           mounts,
        Env:              env,
        LimitMemoryBytes: limitMem,
        // ...
      }
      ```
    - Confirm Docker runtime (`container_docker.go`) already converts `ContainerSpec.Env` into the moby API’s `Env: []string{"KEY=value"}` list.
  - Snippets:
    - Env wiring inside Docker runtime:
      ```go
      envList := make([]string, 0, len(spec.Env))
      for k, v := range spec.Env {
        envList = append(envList, fmt.Sprintf("%s=%s", k, v))
      }
      config.Env = envList
      ```
  - Tests: Extend `internal/workflow/runtime/step/container_docker_test.go` or add a focused test to assert that when `ContainerSpec.Env["CA_CERTS_PEM_BUNDLE"]` is set, the mock docker client sees an entry `CA_CERTS_PEM_BUNDLE=...` in `ContainerCreateOptions.Config.Env`.

## Image Startup Hooks (Codex, ORW, Build Gate)
- [ ] Implement startup use of global env in official images — Make sure Codex and build-gate images actually consume `CODEX_AUTH_JSON` and `CA_CERTS_PEM_BUNDLE` injected via the global config.
  - Repository: github.com/iw2rmb/ploy
  - Component: docker/mods/*, internal/workflow/runtime/step/gate_docker.go
  - Scope:
    - Codex:
      - In `docker/mods/mod-codex/mod-codex.sh`, treat global `CODEX_AUTH_JSON` exactly as today (the script already writes it into `/root/.codex/auth.json` when set). No new behavior is required beyond ensuring the env is present via global config.
      - Optionally, add a short comment near the auth section to note that `CODEX_AUTH_JSON` may come from `ploy config env`.
    - CA bundles for build-gate:
      - In `gate_docker.go`, prepend a CA-install preamble to the Maven/Gradle/Java scripts so that `CA_CERTS_PEM_BUNDLE` is honored inside gate containers:
        ```bash
        if [ -n "${CA_CERTS_PEM_BUNDLE:-}" ]; then
          pem_file="$(mktemp)"
          printf '%s\n' "${CA_CERTS_PEM_BUNDLE}" > "${pem_file}"
          pem_dir="$(mktemp -d)"
          awk '/-----BEGIN CERTIFICATE-----/{n++} {print > (d"/cert" n ".crt")}' d="${pem_dir}" "${pem_file}"
          if command -v update-ca-certificates >/dev/null 2>&1; then
            sys_ca_dir="/usr/local/share/ca-certificates/ploy"
            mkdir -p "$sys_ca_dir"
            cp "${pem_dir}"/*.crt "$sys_ca_dir"/ || true
            update-ca-certificates >/dev/null 2>&1 || true
          fi
          # Optionally import into Java cacerts with keytool when available
        fi
        ```
      - Embed this into the shell commands built by `chooseMaven`, `chooseGradle`, and `chooseJava` before invoking `mvn`, `gradle`, or `javac`.
    - ORW images:
      - `docker/mods/orw-maven/orw-maven.sh` and `docker/mods/orw-gradle/orw-gradle.sh` already support `CA_CERTS_PEM_BUNDLE`; verify behavior and keep it consistent with the gate preamble.
  - Snippets:
    - Example Go-embedded script (simplified):
      ```go
      script := `set -e
if [ -n "${CA_CERTS_PEM_BUNDLE:-}" ]; then
  # ... install bundle ...
fi
mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install
`
      cmd = []string{"/bin/sh", "-lc", script}
      ```
  - Tests: Add integration-style tests for `gate_docker` that set `spec.Env["CA_CERTS_PEM_BUNDLE"]` and verify via logs or a dummy `update-ca-certificates`/`keytool` stub that the preamble runs; for Codex and ORW images, rely on existing smoke tests plus a small self-test that asserts no error when `CA_CERTS_PEM_BUNDLE` / `CODEX_AUTH_JSON` are present.

## Documentation & OpenAPI
- [ ] Document global config env and align OpenAPI — Make the new `config env` surface and global env semantics discoverable and consistent across docs.
  - Repository: github.com/iw2rmb/ploy
  - Component: docs, docs/api
  - Scope:
    - Update `cmd/ploy/README.md`:
      - Add `config env` to the CLI command list with examples:
        ```bash
        ploy config env set --key CA_CERTS_PEM_BUNDLE --file ca-bundle.pem --scope all
        ploy config env set --key CODEX_AUTH_JSON --file ~/.codex/auth.json --scope mods
        ploy config env set --key OPENAI_API_KEY --value sk-... --scope all
        ```
    - Extend `docs/envs/README.md`:
      - Introduce a “Global Env Configuration” section describing how `config env` maps into job `env` and which keys are consumed by official images:
        - `CA_CERTS_PEM_BUNDLE` → ORW mods, build-gate, custom mods.
        - `CODEX_AUTH_JSON` → `mod-codex`.
        - `OPENAI_API_KEY` → any future OpenAI-integrated mods.
    - Update `docs/mods-lifecycle.md` to mention that server-injected env now includes these global keys for mods/healing/gate phases.
    - Extend `docs/api/OpenAPI.yaml`:
      - Add `/v1/config/env` and `/v1/config/env/{key}` under the Config tag, referencing new path docs (e.g., `docs/api/paths/config_env.yaml`).
      - Define schemas for `GlobalEnvVar` and list responses in `docs/api/components/schemas/config.yaml` or an equivalent component file.
  - Snippets:
    - Example OpenAPI path stub:
      ```yaml
      /v1/config/env:
        get:
          tags: [Config]
          summary: List global environment variables
          responses:
            '200':
              description: Global env list
              content:
                application/json:
                  schema:
                    type: array
                    items:
                      $ref: '#/components/schemas/GlobalEnvVar'
      ```
  - Tests: Run existing OpenAPI validation tests (if present) and `make test` to ensure documentation references and schemas remain in sync with the code.

