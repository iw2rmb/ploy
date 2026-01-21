# Stack Gate — Phase 1: Spec + Contracts + Invariants

Status: **Planned (not implemented)**

## Goal

Add explicit Stack Gate expectations to the Mods spec, thread them through the node agent manifests, and reject contradictory multi-step specs early.

## What remains unchanged

- Build Gate still detects stack via tool markers (see `docs/mods-lifecycle.md` → “## 1.2 Stack-Aware Image Selection”).
- Build Gate still selects runtime images via tool detection / env overrides (see `internal/workflow/runtime/step/gate_docker.go`).

## Compatibility impact

- None required (new fields are additive; enforcement is gated behind `stack.*.enabled`).

## Implementation steps (RED → GREEN → REFACTOR)

1. Define typed Stack Gate spec and expectations in contracts:
   - Add types under `internal/workflow/contracts/` (new file recommended, e.g. `stack_gate_spec.go`):
     - `StackExpectation` (`language`, optional `tool`, optional `release`)
     - `StackGatePhaseSpec` (`enabled`, `expect`)
     - `StackGateSpec` (`inbound`, `outbound`)
   - Add `Stack *StackGateSpec` to `internal/workflow/contracts/mods_spec.go` → `type ModStep`.
2. Parse and serialize the new spec fields:
   - Extend `internal/workflow/contracts/mods_spec_parse.go` to parse `steps[i].stack`.
   - Extend `internal/workflow/contracts/mods_spec_wire.go` to round-trip `steps[i].stack`.
3. Validate invariants at spec-parse time:
   - Extend `internal/workflow/contracts/mods_spec.go` → `func (s ModsSpec) Validate()`:
     - If `stack.<phase>.enabled == true` then `stack.<phase>.expect.language` must be non-empty.
     - If `stack.<phase>.enabled == false` then `stack.<phase>.expect` must be omitted (reject to avoid “enabled=false but expect set” ambiguity).
4. Validate multi-step inbound chaining at manifest-build time:
   - In `internal/nodeagent/manifest.go` (or the helper that builds per-step manifests), enforce:
     - `steps[0].stack.inbound.expect` must be present when inbound is enabled.
     - For `i > 0`, if `steps[i].stack.inbound.expect` is omitted and inbound is enabled, derive it from `steps[i-1].stack.outbound.expect`.
     - If provided for `i > 0`, it must equal `steps[i-1].stack.outbound.expect` (reject otherwise).
5. Thread Stack Gate config into gate execution:
   - Extend `internal/workflow/contracts/step_manifest.go` → `type StepGateSpec` to carry Stack Gate expectations for the relevant phase(s) (shape: one “effective expectation” per gate run, or separate inbound/outbound fields).
   - Update `internal/nodeagent/manifest.go` to populate the new `StepGateSpec` fields for:
     - pre-gate (inbound expectation)
     - post-gate (outbound expectation)
6. Tests:
   - Add/extend unit tests in `internal/workflow/contracts/mods_spec_test.go` for:
     - parsing + round-trip of `steps[].stack`
     - validation errors for missing `expect.language` when enabled
   - Add/extend tests around manifest building in `internal/nodeagent/agent_manifest_builder_test.go` (or closest existing tests) for chaining/derivation rules.

