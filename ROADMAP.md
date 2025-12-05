# Mods stack-specific images and ORW split

Scope: Allow Mods specs to select per-stack images based on the Build Gate stack detection, while still supporting a universal image path when needed. Split `docker/mods/mod-orw` into dedicated Maven and Gradle images with focused entrypoints, and wire them into the new stack-aware image selection.

Documentation: `docs/schemas/mod.example.yaml`, `docs/mods-lifecycle.md`, `docs/how-to/publish-mods.md`, `docs/build-gate/README.md`, `docs/envs/README.md`, `ROADMAP_DAG.md`, `ROADMAP_GATE.md`.

Legend: [ ] todo, [x] done.

## Stack-aware Mods images
- [x] Extend Mods spec to support stack-aware images — Teach `mod.image` / `mods[].image` to accept either a string (universal image) or a map keyed by stack, with `default` as the fallback, and preserve existing single-string behavior.
  - Repository: ploy
  - Component: `internal/workflow/contracts`, `internal/workflow/runtime/step`, `cmd/ploy`, `internal/cli/mods`, `tests/e2e/mods`
  - Scope: 
    - Update the Mods run spec contract to model `image` as a union: string (universal) or `map[string]string` keyed by stack (e.g., `java-maven`, `java-gradle`, `default`).
    - Thread Build Gate stack detection into Mods execution so each step sees the resolved stack (e.g., `java-maven`, `java-gradle`, `java`, `unknown`) that the gate used for validation.
    - Implement image resolution rules in the runner:
      - If `image` is a string, treat it as the universal image for any stack (backward compatible behavior).
      - If `image` is a map:
        - Prefer an exact stack key match (e.g., `java-maven`, `java-gradle`).
        - Fall back to `default` when present.
        - Fail fast with a clear error when neither a matching key nor `default` is present.
    - Ensure the same rules apply to the top-level `mod.image` (single-step) and each `mods[].image` (multi-step).
  - Snippets:
    - Example `mod` with universal image (legacy shape):
      ```yaml
      mod:
        image: docker.io/your-dh-user/mods-openrewrite:latest
      ```
    - Example `mod` with stack-specific images (overloaded map form):
      ```yaml
      mod:
        image:
          default: docker.io/your-dh-user/mods-openrewrite:latest
          java-maven: docker.io/your-dh-user/mods-orw-maven:latest
          java-gradle: docker.io/your-dh-user/mods-orw-gradle:latest
      ```
  - Tests: 
    - Add unit tests for image resolution under `internal/workflow/runtime/step` (or a new helper package) that cover:
      - String `image` with any stack → universal image selected.
      - Map `image` with exact stack key present → stack-specific image selected.
      - Map `image` with no exact key but `default` present → `default` image selected.
      - Map `image` with neither stack key nor `default` → run fails with a clear, actionable error.

- [x] Wire Build Gate stack into Mods planner/runtime — Make the Build Gate stack available to each Mods step so stack-aware image selection is deterministic and observable.
  - Repository: ploy
  - Component: `internal/workflow/contracts`, `internal/workflow/planner`, `internal/workflow/runtime/step`, `internal/nodeagent`
  - Scope:
    - Extend the Build Gate metadata and/or run context to carry the resolved stack identifier used by `dockerGateExecutor` (e.g., `java-maven`, `java-gradle`, `java`).
    - When constructing Mods workflow steps (both single-step and multi-step), propagate this stack value into the step manifest so the executor can choose the correct image.
    - Define behavior for ambiguous or unsupported workspaces:
      - If no Maven/Gradle/JDK markers are present, treat the stack as `java` (or `unknown`) and rely on `default` in stack maps.
      - Document that non-Java stacks still use the universal image path.
    - Ensure that stack values are stable across re-gates and healing so a run does not see inconsistent stack identifiers for the same workspace.
  - Snippets:
    - Pseudocode for stack propagation:
      ```go
      type ModsStack string
      // examples: "java-maven", "java-gradle", "java", "unknown"

      type StepContext struct {
        Stack ModsStack
        // ...
      }
      ```
  - Tests:
    - Add planner/runtime tests asserting that a run with a Maven workspace yields `Stack == "java-maven"` on Mods steps, and a Gradle-only workspace yields `Stack == "java-gradle"`.
    - Add tests that verify the same stack value is visible both before and after healing-induced re-gates for a given ticket.

- [x] Document stack-aware Mods images in schemas and lifecycle docs — Make the new `image` map form discoverable and describe stack resolution rules.
  - Repository: ploy
  - Component: `docs/schemas`, `docs/mods-lifecycle.md`, `tests/e2e/mods`
  - Scope:
    - Update `docs/schemas/mod.example.yaml`:
      - Document that `mod.image` (and `mods[].image`) accepts either:
        - A string (universal image), or
        - A map keyed by stack (`default`, `java-maven`, `java-gradle`, etc.).
      - Include a concrete example that mirrors the roadmap snippets and references stack names used by the Build Gate (`java-maven`, `java-gradle`).
    - Extend `docs/mods-lifecycle.md` to describe:
      - How the Build Gate detects the stack for Java projects.
      - How Mods steps resolve images from the `image` map based on the detected stack.
      - The fallback behavior when no stack-specific key is present.
    - Add or extend an e2e scenario under `tests/e2e/mods` (e.g., new `scenario-stack-aware-images`) that:
      - Uses a `mod.yaml` with stack-specific `image` map.
      - Asserts that Maven and Gradle workspaces select the expected images in logs or artifacts.
  - Snippets:
    - Example schema excerpt:
      ```yaml
      mod:
        # image can be a string or a map keyed by stack.
        image:
          default: docker.io/your-dh-user/mods-openrewrite:latest
          java-maven: docker.io/your-dh-user/mods-orw-maven:latest
          java-gradle: docker.io/your-dh-user/mods-orw-gradle:latest
      ```
  - Tests:
    - Run the new e2e scenario and validate that:
      - The node agent logs show the selected image per stack.
      - The run fails with a clear error when the stack is `java-gradle` and only `java-maven` is specified without `default`.

## Split OpenRewrite ORW image by stack
- [x] Introduce `docker/mods/orw-maven` and `docker/mods/orw-gradle` images — Create dedicated, minimal images for Maven and Gradle OpenRewrite application, derived from the current `mod-orw` behavior.
  - Repository: ploy
  - Component: `docker/mods`, `tests/integration/mods`, `scripts/docker`
  - Scope:
    - Create `docker/mods/orw-maven/`:
      - Base image: Maven + JDK (reuse the current `mod-orw` base).
      - Entry script (e.g., `orw-maven.sh`) focused solely on Maven projects:
        - Require `pom.xml` in the workspace.
        - Invoke `rewrite-maven-plugin` with recipe coordinates from env (`RECIPE_GROUP`, `RECIPE_ARTIFACT`, `RECIPE_VERSION`, `RECIPE_CLASSNAME`, `MAVEN_PLUGIN_VERSION`).
        - Preserve TLS CA injection logic from the existing `mod-orw` script.
      - CMD/ENTRYPOINT wired to the Maven-focused script (`mods-orw-maven`).
    - Create `docker/mods/orw-gradle/`:
      - Base image: Gradle + JDK (reuse the explicit Gradle distribution logic from `mod-orw`).
      - Entry script (e.g., `orw-gradle.sh`) focused solely on Gradle projects:
        - Require `build.gradle` or `build.gradle.kts` in the workspace.
        - Prefer `gradle` in PATH, then `./gradlew` as today.
        - Support Kotlin DSL injection for the OpenRewrite Gradle plugin and recipe dependency (mirroring current `mod-orw` Gradle path).
        - Preserve TLS CA injection logic where applicable.
      - CMD/ENTRYPOINT wired to the Gradle-focused script (`mods-orw-gradle`).
    - Keep image naming consistent with existing publish guidance:
      - `docker/mods/orw-maven` → `mods-orw-maven` (or similar; to be finalized alongside `docs/how-to/publish-mods.md` update).
      - `docker/mods/orw-gradle` → `mods-orw-gradle`.
  - Snippets:
    - Example Maven image Dockerfile skeleton:
      ```dockerfile
      FROM --platform=$TARGETOS/$TARGETARCH maven:3.9.11-eclipse-temurin-17
      WORKDIR /workspace
      COPY --chmod=755 orw-maven.sh /usr/local/bin/mods-orw-maven
      ENTRYPOINT ["mods-orw-maven"]
      CMD ["--apply", "--dir", "/workspace", "--out", "/out"]
      ```
  - Tests:
    - Add integration tests in `tests/integration/mods` to cover:
      - `orw-maven` against a sample Maven workspace (e.g., using the existing ORW scenario repo) with a real `mvn` invocation.
      - `orw-gradle` against a minimal Gradle workspace using a stubbed `gradle`/`gradlew` that confirms the expected command line (similar to `TestModORW_GradleWorkspace_UsesGradleWrapper`).

- [ ] Remove `docker/mods/mod-orw` in favor of split images — Eliminate `mod-orw` usage entirely and rely solely on `orw-maven` / `orw-gradle`.
  - Repository: ploy
  - Component: `docker/mods/mod-orw`, `scripts/docker/build-and-push-mods.sh`, `docs/how-to/publish-mods.md`, `tests/integration/mods`
  - Scope:
    - Update `scripts/docker/build-and-push-mods.sh`:
      - Discover `orw-maven` and `orw-gradle` directories along with existing mods.
      - Map `orw-maven` / `orw-gradle` to appropriate repo names (e.g., `mods-orw-maven`, `mods-orw-gradle`) and remove the special-case mapping for `mod-orw` → `mods-openrewrite`.
      - Stop building or pushing any `mod-orw` image.
    - Update `docs/how-to/publish-mods.md`:
      - Document only the new `orw-maven` and `orw-gradle` images and their intended usage with stack-aware `image` maps in `mod.yaml`.
      - Remove references to `mod-orw` as a published or supported image.
      - Provide example `docker buildx` commands for publishing the new images.
    - Remove `docker/mods/mod-orw`:
      - Delete the `docker/mods/mod-orw/` directory (Dockerfile and `mod-orw.sh`) once all references are migrated to `orw-maven` / `orw-gradle`.
      - Ensure no remaining code paths or docs expect `mod-orw` to exist.
    - Update tests to stop using `mod-orw`:
      - Adjust `tests/integration/mods/mod_orw_test.go` to target the split images (or replace it with new `orw-maven` / `orw-gradle` tests).
      - Remove any remaining references to `mod-orw` from integration and e2e tests.
  - Snippets:
    - Example note in `docs/how-to/publish-mods.md`:
      ```markdown
      - `orw-maven` — OpenRewrite apply for Maven-only workspaces.
      - `orw-gradle` — OpenRewrite apply for Gradle-only workspaces.
      ```
  - Tests:
    - Run `scripts/docker/build-and-push-mods.sh` in a dry-run or test environment to verify that:
      - `orw-maven` and `orw-gradle` are discovered and mapped correctly.
      - No `mod-orw` image is built or pushed, and no references to it remain.
