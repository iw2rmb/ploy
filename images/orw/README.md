# ORW Images (`orw-cli-maven`, `orw-cli-gradle`)

Consolidated reference for ORW-related behavior documented under `docs/`.

## What Lives Here

- `orw-cli-maven` (`images/orw/orw-cli-maven`) -> image name `orw-cli-maven`
- `orw-cli-gradle` (`images/orw/orw-cli-gradle`) -> image name `orw-cli-gradle`

Runners resolve as:

- `$PLOY_CONTAINER_REGISTRY/orw-cli-maven:latest`
- `$PLOY_CONTAINER_REGISTRY/orw-cli-gradle:latest`

## How ORW Images Are Selected

Migs `image` supports:

1. Universal string image (same image for all stacks)
2. Stack-specific map (`default`, `java-maven`, `java-gradle`, ...)

For stack-specific maps:

1. Exact stack key match wins (`java-maven`, `java-gradle`, etc.)
2. Fallback to `default` if no exact key exists
3. Fail if neither exact key nor `default` exists

Typical ORW mapping:

```yaml
image:
  default: $PLOY_CONTAINER_REGISTRY/orw-cli-maven:latest
  java-maven: $PLOY_CONTAINER_REGISTRY/orw-cli-maven:latest
  java-gradle: $PLOY_CONTAINER_REGISTRY/orw-cli-gradle:latest
```

Build Gate detects stack once (for example `java-maven` or `java-gradle`) and that stack is reused across subsequent mig/heal steps in the run.

## ORW CLI Runtime Contract

Required:

- `RECIPE_GROUP`
- `RECIPE_ARTIFACT`
- `RECIPE_VERSION`
- `RECIPE_CLASSNAME`

Optional:

- `ORW_REPOS` (comma-separated repo URLs)
- `ORW_REPO_USERNAME` + `ORW_REPO_PASSWORD` (must be paired)
- `ORW_ACTIVE_RECIPES` (comma-separated override list)
- `ORW_FAIL_ON_UNSUPPORTED` (default `true`)
- `ORW_EXCLUDE_PATHS` (comma-separated globs)
- `ORW_CLI_BIN` (defaults to `rewrite`)

## How ORW Images Behave

- Both ORW images use the same `PLOY_CA_CERTS` materializer pattern as Build Gate.
- Both ship bundled `rewrite` at `/usr/local/bin/rewrite` backed by an embedded standalone runner JAR.
- `ORW_CLI_BIN` defaults to the bundled binary and is intended to be overridden only for controlled debugging.
- Recipes are resolved dynamically from `RECIPE_GROUP/RECIPE_ARTIFACT/RECIPE_VERSION`; per-recipe image rebuild is not required.
- ORW images execute OpenRewrite in isolated runtime containers instead of invoking Maven/Gradle project tasks directly.

## `rewrite.yml` Behavior

When a `rewrite.yml` exists in workspace:

- `rewrite.configLocation` points to `rewrite.yml`
- Active recipes are derived from the file (docs also mention `REWRITE_ACTIVE_RECIPES` override for this path)

If no `rewrite.yml` exists, ORW falls back to class-based recipe execution using the recipe coordinates and `RECIPE_CLASSNAME`.

## ORW Report / Failure Contract

Expected report file: `/out/report.json`

Example payload:

```json
{
  "success": false,
  "error_kind": "unsupported",
  "reason": "type-attribution-unavailable",
  "message": "Type attribution is unavailable for this repository"
}
```

`error_kind` taxonomy:

- `input` (invalid/missing input)
- `resolution` (dependency/repository resolution failure)
- `execution` (OpenRewrite CLI run failure)
- `unsupported` (deterministic unsupported mode)
- `internal` (unexpected internal failure)

Constraint:

- `error_kind=unsupported` requires `reason=type-attribution-unavailable`

Propagation:

- Node propagates ORW failure metadata into run metadata:
  - `orw_error_kind`
  - `orw_reason` (if present)

## Build and Publish

Publish all runtime + mig images:

```bash
images/build-and-push.sh
```

Publish a single ORW image:

```bash
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-ghcr.io/iw2rmb/ploy}" \
  docker buildx build --platform linux/amd64 \
  -f images/orw/orw-cli-maven/Dockerfile \
  -t "${IMAGE_PREFIX}/orw-cli-maven:latest" \
  --push .
```

Verify published tags:

```bash
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/orw-cli-maven:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/orw-cli-gradle:latest"
```

## Sources Consolidated

- `docs/envs/README.md`
- `docs/migs-lifecycle.md`
- `docs/how-to/publish-migs.md`
- `docs/schemas/mig.example.yaml`
- `docs/api/components/schemas/controlplane.yaml` (run metadata keys)
