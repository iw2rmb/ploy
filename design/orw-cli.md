# ORW CLI Replacement (`orw-cli`)

## Goal

Replace the current OpenRewrite execution model with a single isolated runtime image: `orw-cli`.

The new model must:
- apply OpenRewrite recipes without executing repository build tasks
- be repo-agnostic and not depend on project-specific task names or plugins
- remove current plugin-driven runners (`orw-gradle`, `orw-maven`)

## Scope

In scope:
- runtime image replacement for Java OpenRewrite steps
- workflow wiring changes to use `orw-cli`
- removal of build-tool plugin invocation from migration execution
- docs and tests updates for the new runtime contract

Out of scope:
- backward compatibility layer for `migs-orw-*`
- migration of historical data
- mixed runtime mode where both old and new ORW paths coexist

## Current Implementation (As-Is)

Current ORW runners execute through build tools:
- Gradle path: `orw-gradle` invokes `gradle rewriteRun`
- Maven path: `orw-maven` invokes `mvn org.openrewrite.maven:rewrite-maven-plugin:run`

Relevant scripts:
- `deploy/images/migs/orw-gradle/orw-gradle.sh`
- `deploy/images/migs/orw-maven/orw-maven.sh`

Current model properties:
- runs inside full project build lifecycle
- loads repository plugins and build logic
- allows unrelated build tasks/plugins to execute during rewrite run

Observed failure class:
- unrelated plugin task crashes (example: `:autoLintGradle`) fail rewrite job even when recipe setup is valid

## Target Model

`orw-cli` becomes the only OpenRewrite runtime for Java repositories.

Key properties:
- one runtime image name: `orw-cli`
- one execution engine: OpenRewrite CLI (standalone), not Gradle/Maven plugin goals
- bundled CLI binary `rewrite` built from in-image `orw-cli-runner` (pinned OpenRewrite libs:
  `rewrite-core/rewrite-java/rewrite-maven/rewrite-gradle` `8.67.0`, `rewrite-polyglot` `2.9.6`)
- no project task execution by migration runtime
- stable `/workspace` and `/out` contract unchanged

## Universal Isolation Contract

Isolation rules:
- `orw-cli` must not invoke `gradle`, `./gradlew`, `mvn`, or `./mvnw` for rewrite execution
- `orw-cli` must not execute repository-defined tasks, hooks, or plugin lifecycle callbacks
- rewrite application is driven by CLI config and source tree only

Result:
- repo plugin task failures cannot break ORW run
- execution behavior is independent from repo-specific task graph design

## Execution Flow

1. Input validation
- require recipe coordinates and active recipe identity
- validate workspace and writable output directory

2. Rewrite configuration
- prefer repository `rewrite.yml` when present
- otherwise generate deterministic temporary config from requested recipe

3. Gradle static-import normalization (Java 17 upgrade recipes only)
- before recipe execution, normalize `build.gradle` assignments from:
  - `sourceCompatibility = VERSION_<n>`
  - `targetCompatibility = VERSION_<n>`
  to:
  - `sourceCompatibility = JavaVersion.VERSION_<n>`
  - `targetCompatibility = JavaVersion.VERSION_<n>`
- remove `import static org.gradle.api.JavaVersion.VERSION_<n>` lines
- this pre-step is applied only for `UpgradeToJava17` / `UpgradeBuildToJava17`

4. Artifact resolution
- resolve recipe artifacts from configured Maven repositories
- use explicit credential and CA inputs from env

5. CLI run
- execute OpenRewrite CLI directly against `/workspace`
- emit full transform logs to `/out/transform.log`

6. Output contract
- write `/out/report.json`
- success payload includes recipe and coordinates
- failure payload includes explicit `error_kind`, message, and non-zero exit code

7. Cleanup
- remove temporary config/cache paths created by runtime
- keep workspace content only as transformed source state

## Dependency and Type Attribution Policy

`orw-cli` must be explicit about attribution quality:
- if recipe can run with available classpath inputs, run normally
- if required classpath cannot be resolved without executing build lifecycle, fail with structured unsupported reason

Failure reason contract:
- `error_kind=unsupported`
- `reason=type-attribution-unavailable`
- deterministic message in `report.json` and `transform.log`

This keeps behavior universal and deterministic without falling back to build tool task execution.

## Runtime Contract

Required env (existing semantics retained where possible):
- `RECIPE_GROUP`
- `RECIPE_ARTIFACT`
- `RECIPE_VERSION`
- `RECIPE_CLASSNAME`

New or formalized env for CLI runtime:
- `ORW_REPOS` (comma-separated Maven repository URLs)
- `ORW_REPO_USERNAME`
- `ORW_REPO_PASSWORD`
- `ORW_ACTIVE_RECIPES` (optional override)
- `ORW_FAIL_ON_UNSUPPORTED` (default `true`)

Security and trust:
- keep `CA_CERTS_PEM_BUNDLE` support
- never print credentials in logs

## Control Plane and Workflow Changes

### Image mapping

Replace Java ORW stack mappings:
- `java-gradle -> orw-cli`
- `java-maven -> orw-cli`

Remove stack mappings to:
- `migs-orw-gradle`
- `migs-orw-maven`

### Contracts

Keep manifest step contract shape stable:
- `--apply --dir /workspace --out /out`
- same artifact upload behavior (`mig-out`)

Update runtime metadata:
- `job_image` reflects `orw-cli`
- failure taxonomy includes explicit unsupported attribution failures

## File-Level Change Plan

Create:
- `deploy/images/orw/orw-cli-gradle/Dockerfile`
- `deploy/images/orw/orw-cli-gradle/orw-cli.sh`
- `deploy/images/orw/orw-cli-maven/Dockerfile`
- `deploy/images/orw/orw-cli-maven/orw-cli.sh`
- tests for CLI runtime behavior under `tests/integration/migs/` (renamed path in same slice if required)

Update:
- stack/image resolution config and defaults
- workflow contracts for image names
- docs that reference `migs-orw-gradle` and `migs-orw-maven`

Remove:
- `deploy/images/migs/orw-gradle/`
- `deploy/images/migs/orw-maven/`
- docs and tests that assert plugin-based ORW behavior

## Testing Plan

Unit tests:
- env parsing and validation for `orw-cli`
- report contract serialization for success/fail/unsupported
- credential redaction in logs and errors

Integration tests:
- recipe applies in Gradle repo without invoking Gradle tasks
- recipe applies in Maven repo without invoking Maven goals
- unsupported attribution path returns deterministic structured failure

E2E tests:
- stack-aware run selects `orw-cli`
- diff artifact generation remains unchanged
- downstream step cancellation semantics remain unchanged on ORW failure

Validation commands:
- `make test`
- `make vet`
- `make staticcheck`

## Rollout Strategy

Single-cut replacement in one release slice:
- ship `orw-cli`
- switch all Java ORW mappings to new image
- remove old ORW plugin images and references in same slice

No compatibility mode and no fallback path.

## Risks

- some recipes may require richer classpath than available in isolated mode
- teams may observe new `unsupported` failures where plugin path previously attempted execution
- repository-specific custom recipe environments may need explicit repository credential/env wiring

## Open Questions

- Should `ORW_REPOS` default to Maven Central only, or require explicit repository configuration per cluster?
- Should unsupported attribution be terminal by default in all environments, or configurable only in local dev?

## References

- `deploy/images/migs/orw-gradle/orw-gradle.sh`
- `deploy/images/migs/orw-maven/orw-maven.sh`
- `docs/migs-lifecycle.md`
- `docs/envs/README.md`
- `design/gate-stack.md`
