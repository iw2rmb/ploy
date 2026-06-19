# Build Gate

## Current Contract

Build Gate is image-driven:

- Image is resolved by stack selector from `gates/gates.yaml` (or `build_gate.images` overrides in spec).
- Command is owned by the gate image (`CMD`/entrypoint inside image).
- Gate profiles are not resolved/persisted by the server and are not promoted from successful gates.

## Catalog Format

`gates/gates.yaml`:

```yaml
gates:
  - lang: java
    tool: maven
    release: "17"
    image: $PLOY_CONTAINER_REGISTRY/gate-${stack.tool}:jdk${stack.release}
```

Rules:

- `lang` required
- `release` required (string or number in YAML)
- `tool` optional
- `image` required

## Resolution Order

1. `build_gate.images[]` from run spec (highest precedence)
2. `gates/gates.yaml` (default)

Most-specific match wins (language+release+tool beats language+release).

## Phase Options

`build_gate.pre.stack` and `build_gate.post.stack` configure stack-detect policy:

```yaml
build_gate:
  pre:
    stack:
      mode: strict
      language: java
      tool: maven
      release: "17"
```

Modes:

- `forced`: skip detection and use the configured stack.
- `strict`: run detection and fail when detected values differ from the configured stack fields.
- `fallback`: use complete detection, otherwise use the configured stack.

`forced` and `fallback` require `language`, `tool`, and `release`, because the
configured stack can become the runtime stack. `strict` requires at least one of
those fields and treats omitted fields as "any"; successful strict detection
still uses the complete detected stack for image and command resolution. An
absent or empty `stack` object keeps normal auto-detection.

Set `build_gate.disabled: true` when no Build Gate jobs should be created.

## Runtime Paths

- Host out dir: `.ploy-gate-out`
- Container out dir: `/out`
- Optional host in dir: `.ploy-gate-in`
- Container in dir: `/in`

## Log Preservation

Build Gate preserves captured gate logs as execution metadata. It does not run
Maven, Gradle, or other tool-specific log processors.

On failed gate execution, `BuildGateStageMetadata.LogFindings[0]` contains:

- `severity: error`
- `message`: the raw canonical container logs, capped at 10 MiB
- no structured `evidence`

The same capped log text is stored in `BuildGateStageMetadata.LogsText`, and
`LogDigest` is computed from that capped text.

Successful Gradle gates may still add an informational `GRADLE_BUILD_CACHE_HIT`
finding when the gate image reports cache-hit tasks.
