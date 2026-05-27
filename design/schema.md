# Mig Spec Schema Validation

## Summary

Mig spec validation moves to one embedded JSON Schema contract checked by Go
code everywhere. Replace `github.com/xeipuuv/gojsonschema` with
`github.com/santhosh-tekuri/jsonschema/v6`, remove the obsolete integration
manifest schema and CLI surface, tighten the mig schema, and add
`ploy spec schema` plus `ploy spec validate`.

The same embedded schema and the same validation library must be used by the
CLI before submitting specs and by the server before accepting or executing
specs.

## Scope

In scope:

- Replace `github.com/xeipuuv/gojsonschema` with
  `github.com/santhosh-tekuri/jsonschema/v6`.
- Keep `internal/workflow/contracts/schemas/mig.schema.json` as the embedded
  source of truth for mig specs.
- Update `internal/workflow/contracts/schemas/mig.schema.json` to match the
  current mig contract and reject stale or unknown fields.
- Align `docs/schemas/mig.example.yaml` with the updated schema.
- Remove `docs/schemas/integration_manifest.schema.json`.
- Remove integration manifest related paths, validation, tests, docs, and help.
- Remove `ploy manifest ...` machinery and command wiring.
- Introduce `ploy spec schema` and `ploy spec validate`.
- Validate specs locally for `ploy mig ...` and `ploy run ...` paths that accept
  or submit a mig spec.
- Validate specs server-side before storage and before execution.

Out of scope:

- No validation CLI dependency such as `ajv-cli`, `check-jsonschema`, or a custom
  external schema command.
- No remaining `ploy manifest ...` compatibility path.
- No backward compatibility for stale spec shapes.
- No separate client/server schema implementations.
- No removal of active runtime job data structures merely because their names
  include `manifest`, unless they are part of the obsolete integration manifest
  surface.

## Current Baseline

- `internal/workflow/contracts/mig_schema.go` uses
  `github.com/xeipuuv/gojsonschema`.
- `internal/workflow/contracts/schemas/mig.schema.json` is the embedded mig
  schema, but it must be tightened to the current contract.
- `docs/schemas/mig.example.yaml` is the public example and must remain valid
  against the embedded schema.
- `docs/schemas/integration_manifest.schema.json` and `ploy manifest ...` are
  obsolete because integration manifests are no longer in use.
- CLI and server paths already parse mig specs through workflow contracts in
  several places, but validation must be made explicit and consistent for every
  spec submission and execution path.

## Target Contract

Mig specs are validated by:

```text
github.com/santhosh-tekuri/jsonschema/v6
```

Rules:

- The embedded file
  `internal/workflow/contracts/schemas/mig.schema.json` is the only JSON Schema
  source for mig specs.
- The contracts package exposes narrow helpers for:
  - returning the embedded schema bytes for CLI output;
  - validating raw mig spec JSON bytes;
  - parsing a validated spec into typed Go contracts.
- YAML input is converted into JSON-compatible data before schema validation.
- JSON and YAML-origin specs use the same embedded schema and the same validator.
- Typed Go validation remains after schema validation for semantic checks that
  JSON Schema should not own, such as cross-field constraints.
- Unknown fields are rejected by the schema wherever the current contract shape
  is known.
- Stale fields such as old direct build-gate phase keys are rejected unless they
  are part of an explicitly defined current schema object.

## CLI Contract

Add:

```text
ploy spec schema
ploy spec validate <path> [<path>...]
```

Behavior:

- `ploy spec schema` prints the exact embedded mig schema JSON.
- `ploy spec validate` accepts YAML or JSON spec files and validates them using
  the embedded schema through `github.com/santhosh-tekuri/jsonschema/v6`.
- Validation failures return a non-zero exit code and include concise path-aware
  error output when the library provides it.
- Every local `ploy mig ...` and `ploy run ...` command path that accepts or
  submits a mig spec validates it before making the server request.
- Remove all `ploy manifest ...` commands, help text, wiring, and tests.

## Server Contract

The server validates mig specs with the same embedded schema and library before:

- accepting direct run submit payloads;
- accepting mig spec create/update payloads;
- storing any new mig spec row;
- materializing or executing a run from an already stored spec.

The final check before execution is required so old invalid database rows cannot
silently run after the schema becomes strict.

## Implementation Notes

- Replace the validator in `internal/workflow/contracts/mig_schema.go` with a
  compiled-schema helper backed by `github.com/santhosh-tekuri/jsonschema/v6`.
- Update `go.mod` and `go.sum` to remove
  `github.com/xeipuuv/gojsonschema` and add
  `github.com/santhosh-tekuri/jsonschema/v6`.
- Keep schema embedding in the contracts package, not in CLI or server packages.
- Add a `spec` CLI package or command module matching existing CLI structure.
- Reuse the contracts package from both CLI and server; do not duplicate schema
  loading or validation logic.
- Update `internal/workflow/contracts/schemas/mig.schema.json` from current Go
  contracts and valid fixtures.
- Update `docs/schemas/mig.example.yaml` until it passes
  `ploy spec validate docs/schemas/mig.example.yaml`.
- Delete `docs/schemas/integration_manifest.schema.json` and obsolete
  integration manifest code paths.

## Acceptance Criteria

- `go.mod` no longer references `github.com/xeipuuv/gojsonschema`.
- `go.mod` references `github.com/santhosh-tekuri/jsonschema/v6`.
- No external validation CLI such as `ajv-cli` or `check-jsonschema` is added.
- `docs/schemas/integration_manifest.schema.json` is removed.
- `ploy manifest ...` is gone from command registration and root help.
- `internal/workflow/contracts/schemas/mig.schema.json` rejects unknown fields
  for the current mig contract.
- `docs/schemas/mig.example.yaml` validates successfully.
- `ploy spec schema` prints the embedded mig schema.
- `ploy spec validate docs/schemas/mig.example.yaml` succeeds.
- Invalid specs are rejected locally by `ploy mig ...` and `ploy run ...`
  commands before HTTP submission.
- Invalid specs are rejected server-side before storage.
- Invalid stored specs are rejected server-side before execution.
- CLI and server validation both use the embedded schema through
  `github.com/santhosh-tekuri/jsonschema/v6`.

## Risks

- Tightening `additionalProperties` can reject active but undocumented fields if
  the schema is incomplete. Mitigation: derive the schema from current Go
  contracts and existing valid fixtures before enforcing strict rejection.
- Stored invalid specs will stop running. This is intended because there is no
  manifest or stale-spec compatibility requirement.
- Validation error formatting will change with the library replacement. Keep the
  messages concise and preserve failing paths where available.
