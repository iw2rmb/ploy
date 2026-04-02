# ORW Auto Compatibility Algorithm

## Goal

Provide a deterministic way to select `orw-cli` runner dependency versions for a given recipe coordinate from `tmp/j11to17/mig.yaml`, without maintaining a manual correspondence table.

## Scope

- Input recipe coordinates from migration spec (`RECIPE_GROUP`, `RECIPE_ARTIFACT`, `RECIPE_VERSION`).
- Resolve compatible OpenRewrite library versions from Maven metadata.
- Rebuild and validate both lane images:
  - `orw-cli-maven`
  - `orw-cli-gradle`
- Promote only validated images.

Out of scope:
- Backward compatibility with removed single image `images/orw/orw-cli`.
- Per-repo custom recipes in this document (same algorithm applies, different input coord).

## Why No Manual Table

Manual mapping drifts and causes runtime classpath conflicts. The authoritative source is the resolved dependency graph of the recipe artifact itself.

## Inputs

From `tmp/j11to17/mig.yaml`:

- `RECIPE_GROUP`
- `RECIPE_ARTIFACT`
- `RECIPE_VERSION`

Example current target:

- `org.openrewrite.recipe:rewrite-migrate-java:3.22.0`

## Algorithm

1. Read recipe coordinate from spec.
2. Generate probe POM that depends on exactly that coordinate.
3. Resolve dependency tree.
4. Extract effective `org.openrewrite:*` versions from the resolved tree.
5. Update runner POMs:
   - `images/orw/orw-cli-maven/rewrite-runner/pom.xml`
   - `images/orw/orw-cli-gradle/rewrite-runner/pom.xml`
6. Build both images from repo-root context.
7. Run image self-test (`MIGS_SELF_TEST=1`).
8. Run lane smoke tests against minimal fixtures:
   - Maven fixture with Java migration recipe.
   - Gradle fixture with Java migration recipe.
9. If all checks pass, push `:latest` for both images.
10. If any check fails, do not promote tags.

## Probe Resolution Contract

Use Maven as resolver source of truth:

```bash
mvn -f /tmp/orw-probe/pom.xml -q dependency:tree \
  -DoutputType=text \
  -Dincludes=org.openrewrite \
  -DoutputFile=/tmp/orw-probe/rewrite-tree.txt
```

The selected versions in runner POMs must match the resolved versions in `rewrite-tree.txt` for core/runtime modules used by the runner (`rewrite-core`, `rewrite-java`, parser modules, `rewrite-polyglot`).

## CI Job Shape

Single pipeline per recipe version bump:

1. `resolve-versions`
   - Input: recipe coord from `tmp/j11to17/mig.yaml`.
   - Output artifact: normalized JSON map of resolved rewrite versions.
2. `sync-runner-poms`
   - Apply resolved versions to both lane POMs.
3. `build-images`
   - Build `orw-cli-maven` and `orw-cli-gradle`.
4. `validate-images`
   - Self-test + functional smoke on Maven and Gradle fixtures.
5. `publish-images`
   - Push `:latest` only when validation succeeds.

## Failure Handling

- Resolution failure: stop pipeline, report unresolved artifact or TLS/certs issue.
- Build failure: stop pipeline, keep previous published images unchanged.
- Smoke failure: stop pipeline, keep previous published images unchanged.
- Cert/TLS in corporate network: provide CA bundle via existing build/runtime CA paths.

## Operational Rules

- Treat `tmp/j11to17/mig.yaml` as the recipe source of truth.
- Do not hand-edit rewrite versions in runner POMs without rerunning probe resolution.
- Do not publish partially validated lane images.
- Keep Maven and Gradle lane validation independent; both must pass for promotion.

## Expected Outcome

- No manual compatibility table required.
- Recipe version updates become repeatable and cheap.
- Runtime classpath mismatch risk is constrained by automated resolution + smoke validation.

## References

- `tmp/j11to17/mig.yaml`
- `design/orw-cli.md`
- `images/orw/orw-cli-maven/rewrite-runner/pom.xml`
- `images/orw/orw-cli-gradle/rewrite-runner/pom.xml`
