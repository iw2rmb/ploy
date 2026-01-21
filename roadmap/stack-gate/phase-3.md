# Stack Gate — Phase 3: Build Gate Image Mapping Resolver

Status: **Planned (not implemented)**

## Goal

Implement explicit stack→image mapping resolution (no implicit defaults) with well-defined precedence and “most specific match wins”.

## What remains unchanged

- Build Gate still uses tool detection defaults and `PLOY_BUILDGATE_*` env overrides until Phase 4 (`internal/workflow/runtime/step/gate_docker.go`).

## Compatibility impact

- None required (mapping is only used when Stack Gate is enabled in Phase 4).

## Implementation steps (RED → GREEN → REFACTOR)

1. Define mapping schema (typed):
   - Add a type like `BuildGateImageRule` under `internal/workflow/contracts/` or a dedicated package:
     - `stack: {language, tool?, release?}`
     - `image: string`
2. Implement loader + merge:
   - Load default rules from `/etc/ploy/gates/build-gate-images.yaml`.
   - Merge/override with:
     - cluster/global inline rules (`gates.build_gate.images`) when that config exists in the repo config model
     - mod-level overrides (`build_gate.images` in the Mods spec)
3. Implement resolver:
   - Input: expected `StackExpectation`
   - Output: selected `image`
   - Rules:
     - `language+tool+release` > `language+release`
     - equal specificity ties at same precedence level → reject (configuration error)
4. Tests:
   - Add unit tests for:
     - specificity ordering
     - precedence ordering
     - tie/conflict detection

