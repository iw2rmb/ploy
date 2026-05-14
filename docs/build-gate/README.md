# Build Gate

## Current Contract

Build Gate is image-driven:

- Image is resolved by stack selector from `gates/gates.yaml` (or `build_gate.images` overrides in spec).
- Command is owned by the gate image (`CMD`/entrypoint inside image).
- Ploy no longer injects per-phase target/command overrides into Build Gate execution.
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

`build_gate.pre.stack` and `build_gate.post.stack` remain supported for stack-detect policy.

## Runtime Paths

- Host out dir: `.ploy-gate-out`
- Container out dir: `/out`
- Optional host in dir: `.ploy-gate-in`
- Container in dir: `/in`
