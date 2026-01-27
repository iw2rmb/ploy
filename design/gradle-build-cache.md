# Gradle Build Cache (local deploy + gate containers)

Status: draft (design only)

Roadmap: `roadmap/gradle-build-cache.md`

## Problem

Ploy runs Gradle frequently inside short‑lived Docker containers (Build Gate, and some Mods images). Today those containers have **no persistent Gradle state**, so each run re-downloads dependencies and redoes work that Gradle can safely reuse via the Build Cache.

In local deployment (`./scripts/deploy-locally.sh`), we also do not start any Gradle cache service that containers could share.

## Goals

- Start a Gradle build cache service in the **local Docker deployment**.
- When Build Gate uses Gradle, enable Gradle Build Cache:
  - via `--build-cache`, and/or
  - by baking a default `~/.gradle/gradle.properties` into the Gradle gate image(s).
- Make cache usage **repo-agnostic** (no required changes to the checked-out repository).

## Non-goals

- Production rollout details (auth, multi-tenant isolation, retention policies) beyond a minimal “how we’d extend it”.
- Backward compatibility with any removed/legacy gate modes.

## Current State (HEAD)

- Local deploy starts only `db`, `server`, `node` (`local/docker-compose.yml`).
- The node attaches runtime containers to `PLOY_DOCKER_NETWORK` (local default: `local_default`). See `docs/envs/README.md` (`PLOY_DOCKER_NETWORK`).
- Build Gate runs Gradle as:
  - `gradle -q --stacktrace test -p /workspace` (see `internal/workflow/runtime/step/gate_docker.go` → `buildCommandForTool("gradle")`).
- The default Build Gate Gradle images are upstream `gradle:8.8-jdk11` / `gradle:8.8-jdk17` (see `etc/ploy/gates/build-gate-images.yaml`).

## Proposed Design

### 1) Local Deploy: add a Gradle Build Cache service

Add a new service to `local/docker-compose.yml`:

- Service name: `gradle-build-cache` (stable DNS name on `local_default`).
- Image: Gradle Build Cache Node (official).
- Port: `5071` (HTTP). Optional: expose `6011` if we later enable cache-node clustering/replication.
- Base path: default `/` (cache endpoint is `/cache`).
- Persistence: mount a named Docker volume for cache data.
- Healthcheck: optional HTTP probe.

Notes:

- Because the node already runs gate containers on `local_default`, any gate container can reach the service as `http://gradle-build-cache:5071/...` without additional networking work.
- This service is **local-only** and can run over plain HTTP.

Implementation artifacts (expected):

- `local/docker-compose.yml`: new `gradle-build-cache` service + `volumes:` entry.
- `docs/how-to/deploy-locally.md`: list the new service and optional port.

### 2) Gate containers: enable Build Cache for Gradle runs

This has two distinct knobs:

#### 2a) Always pass `--build-cache` for Gradle Build Gate

Change Build Gate’s Gradle command to include `--build-cache`:

- From: `gradle -q --stacktrace test -p /workspace`
- To: `gradle -q --stacktrace --build-cache test -p /workspace`

This enables the build cache for the invocation (local cache at minimum).

Implementation artifacts (expected):

- `internal/workflow/runtime/step/gate_docker.go`: update `buildCommandForTool("gradle")`.
- Unit test update: `internal/workflow/runtime/step/gate_docker_test.go` asserting the Gradle command contains `--build-cache`.

#### 2b) Bake a default `~/.gradle/gradle.properties` into the Gradle gate image(s)

Add a default Gradle user home configuration inside Build Gate Gradle images so caching is enabled even if callers forget `--build-cache`.

Minimal `~/.gradle/gradle.properties` contents:

```properties
org.gradle.caching=true
org.gradle.daemon=false
org.gradle.parallel=true
org.gradle.configureondemand=true
```

Notes:

- `org.gradle.caching=true` enables local build cache without needing the CLI flag.
- `daemon=false` matches container usage expectations (short-lived container).
- The other flags are optional and should be validated against common projects; they can be omitted if we want the narrowest change.

Implementation artifacts (expected):

- New Dockerfiles for gate Gradle images (one per JDK release we support), e.g.:
  - `docker/gates/gradle/Dockerfile.jdk11`
  - `docker/gates/gradle/Dockerfile.jdk17`
- Update default mapping in `etc/ploy/gates/build-gate-images.yaml` to point to the new images (breaking change is acceptable per repo policy).

### 3) Make the cache service actually used (remote cache)

Starting a build-cache service is not sufficient unless Gradle is configured to use it as a **remote build cache**.

We want this to work without modifying user repos, so the configuration must be injected via one of:

- a Gradle init script in the image (`~/.gradle/init.d/*.gradle`), or
- `-I <init.gradle>` injected into the Gradle command.

Recommended: bake an init script into the Gradle gate images, controlled by env vars.

Example init script (`~/.gradle/init.d/ploy-remote-build-cache.init.gradle`):

```groovy
// Enable remote build cache without requiring repo changes.
settingsEvaluated { settings ->
  def url = System.getenv("PLOY_GRADLE_BUILD_CACHE_URL")
  if (url == null || url.trim().isEmpty()) return

  settings.buildCache {
    local { enabled = true }
    remote(HttpBuildCache) {
      this.url = new URI(url)
      push = (System.getenv("PLOY_GRADLE_BUILD_CACHE_PUSH") ?: "true").toBoolean()
      allowInsecureProtocol = true
    }
  }
}
```

Local deploy would set (via global env, scope `gate`):

- `PLOY_GRADLE_BUILD_CACHE_URL=http://gradle-build-cache:5071/cache/`
- `PLOY_GRADLE_BUILD_CACHE_PUSH=true`

Implementation artifacts (expected):

- Gate Gradle image(s): add init script at `~/.gradle/init.d/...`.
- `docs/envs/README.md`: document the two env vars above (scope: `gate`).
- `docs/how-to/deploy-locally.md`: show how to set them via `ploy config env set`.

### 4) Extend to Mods Gradle images (OpenRewrite)

`docker/mods/orw-gradle/orw-gradle.sh` currently uses an isolated `GRADLE_USER_HOME` per run (by design) and does not use the build cache.

We will enable build cache there too:

- Keep isolated user home (to avoid concurrent runner conflicts).
- Add the same init script mechanism + `--build-cache` to the `gradle rewriteRun` invocation so it can read/write the remote cache service.

## Security / Isolation Notes

- Local deploy can run the cache over HTTP on the Docker network.
- For non-local clusters, remote build cache is a cross-repo data surface (compiled outputs). If we ever enable it outside local dev, we should add:
  - auth (basic auth / token) and TLS,
  - per-tenant cache namespaces, or separate cache nodes per tenant/cluster.

## Rollout Plan (phased)

1) Add local `gradle-build-cache` service (no behavior change yet).
2) Add `--build-cache` to Build Gate Gradle command.
3) Introduce gate Gradle image(s) with:
   - `~/.gradle/gradle.properties` defaults, and
   - init script for remote build cache (env-driven).
4) Extend to `orw-gradle`.

## Validation

- Unit:
  - `go test ./internal/workflow/runtime/step -run GateDocker` asserts Gradle uses `--build-cache`.
- Local E2E:
  - `./scripts/deploy-locally.sh`
  - Configure env:
    - `ploy config env set --key PLOY_GRADLE_BUILD_CACHE_URL --value http://gradle-build-cache:5071/cache/ --scope gate`
    - `ploy config env set --key PLOY_GRADLE_BUILD_CACHE_PUSH --value true --scope gate`
  - Run a Gradle-based mod twice and compare:
    - first run: cache misses; second run: cache hits (verify via Gradle logs or build-cache-node metrics).
