# Gradle Build Cache (local deploy + gate containers)

Scope: Add a Gradle Build Cache Node service to the local Docker deployment and enable Gradle build caching for both Build Gate and `orw-gradle` executions without requiring repository changes.

Documentation: `design/gradle-build-cache.md`

Legend: [ ] todo, [x] done.

## Local Deploy
- [x] Add Gradle Build Cache Node service — Provides a shared remote cache endpoint for local gate/mod containers
  - Repository: ploy
  - Component: local docker-compose stack
  - Scope: `local/docker-compose.yml` add service `gradle-build-cache` using `gradle/build-cache-node:latest` exposing `5071`, with a named volume for the data dir
  - Snippets: N/A
  - Tests: `docker compose -f local/docker-compose.yml up -d gradle-build-cache` — container starts and listens on `5071`

## Build Gate (Gradle)
- [x] Enable build cache in Gradle gate command — Ensures caching is on even without image defaults
  - Repository: ploy
  - Component: `internal/workflow/runtime/step`
  - Scope: `internal/workflow/runtime/step/gate_docker.go` update `buildCommandForTool("gradle")` to include `--build-cache`
  - Snippets: N/A
  - Tests: `go test ./internal/workflow/runtime/step -run GateDocker` — command contains `--build-cache`

## Gate Gradle Images
- [x] Create dedicated Gradle gate images — Allows injecting `gradle.properties` and init scripts without repo changes
  - Repository: ploy
  - Component: Docker images for Build Gate
  - Scope: add new Dockerfiles under `docker/gates/gradle/` (JDK 11 + JDK 17) that:
    - start from `gradle:8.8-jdk11` / `gradle:8.8-jdk17`
    - add `~/.gradle/gradle.properties` enabling caching (`org.gradle.caching=true`, `org.gradle.daemon=false`)
    - add `~/.gradle/init.d/ploy-remote-build-cache.init.gradle` configuring remote cache from env
  - Snippets: N/A
  - Tests: `docker build` the images and run a trivial `gradle --version` smoke command

- [x] Switch default image mapping to new images — Ensures Build Gate uses the configured Gradle gate images by default
  - Repository: ploy
  - Component: Build Gate image mapping
  - Scope: `etc/ploy/gates/build-gate-images.yaml` replace `gradle:8.8-jdk11` / `gradle:8.8-jdk17` entries with the new images
  - Snippets: N/A
  - Tests: `go test ./internal/workflow/runtime/step -run GateDocker` — image resolution still succeeds for `java+gradle` expectations

## Remote Cache Wiring (env-driven)
- [x] Document and use cache env vars in gate jobs — Allows local deploy to point gate containers to the cache service
  - Repository: ploy
  - Component: docs + global env injection
  - Scope:
    - `docs/envs/README.md` document `PLOY_GRADLE_BUILD_CACHE_URL` and `PLOY_GRADLE_BUILD_CACHE_PUSH` (scope: `gate`)
    - `docs/how-to/deploy-locally.md` add instructions to set the vars via `ploy config env set`
  - Snippets: N/A
  - Tests: manual local run — set `PLOY_GRADLE_BUILD_CACHE_URL=http://gradle-build-cache:5071/cache/` and observe cache hits on second run

## orw-gradle (OpenRewrite)
- [x] Enable build cache for `orw-gradle` — Avoids repeated work for rewrite runs while keeping isolated `GRADLE_USER_HOME`
  - Repository: ploy
  - Component: `docker/mods/orw-gradle`
  - Scope: `docker/mods/orw-gradle/orw-gradle.sh` add `--build-cache` and remote cache wiring (same env vars + init script approach) to the `gradle rewriteRun` invocation
  - Snippets: N/A
  - Tests: run `mods-orw-gradle` twice on the same Gradle project; second run should show cache reuse (and/or reduced duration)
