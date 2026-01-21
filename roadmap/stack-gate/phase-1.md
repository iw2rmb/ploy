# Stack Gate — Phase 1: Spec + Contracts + Invariants

Scope: Add explicit stack expectations to the Mods spec and gate manifests, and reject contradictory multi-step runs (inbound/outbound chaining) before execution.

Documentation: `design/stack-gate.md`, `internal/workflow/contracts/mods_spec.go`, `internal/workflow/contracts/mods_spec_parse.go`, `internal/workflow/contracts/mods_spec_wire.go`, `internal/workflow/contracts/step_manifest.go`, `internal/nodeagent/manifest.go`.

Legend: [ ] todo, [x] done.

## Contracts and parsing
- [x] Define Stack Gate types in contracts — Enables a typed schema for `steps[].stack.{inbound,outbound}`.
  - Repository: ploy
  - Component: `internal/workflow/contracts`
  - Scope: Add `StackExpectation`, `StackGatePhaseSpec`, `StackGateSpec` (new `internal/workflow/contracts/stack_gate_spec.go`).
  - Snippets: `stack: { inbound: { enabled: true, expect: { language: java, tool: maven, release: "11" } } }`
  - Tests: `go test ./internal/workflow/contracts -run StackGate` — new types round-trip and validate.
- [x] Add `steps[].stack` to typed Mods spec — Makes Stack Gate expectations available to node agent.
  - Repository: ploy
  - Component: `internal/workflow/contracts`
  - Scope: `internal/workflow/contracts/mods_spec.go` (`type ModStep`), `internal/workflow/contracts/mods_spec_parse.go`, `internal/workflow/contracts/mods_spec_wire.go`.
  - Snippets: `steps: [{ name: s0, stack: { inbound: ..., outbound: ... } }]`
  - Tests: `go test ./internal/workflow/contracts -run ModsSpec` — parse + wire round-trip.

## Validation and invariants
- [x] Validate Stack Gate phase toggles and required fields — Prevents ambiguous specs (enabled without expectation, or expectation with enabled=false).
  - Repository: ploy
  - Component: `internal/workflow/contracts`
  - Scope: `internal/workflow/contracts/mods_spec.go` (`func (s ModsSpec) Validate()`).
  - Snippets: Reject: `enabled: false` + `expect: {...}`.
  - Tests: `go test ./internal/workflow/contracts -run StackGate` — invalid specs fail with stable errors.
- [x] Enforce multi-step inbound/outbound chaining in manifest build — Prevents contradictory step graphs.
  - Repository: ploy
  - Component: `internal/nodeagent`
  - Scope: `internal/nodeagent/manifest.go` (derive inbound for `i>0` from `steps[i-1].stack.outbound`, reject mismatch when explicitly provided).
  - Snippets: Derive: `steps[1].stack.inbound.expect := steps[0].stack.outbound.expect` when omitted.
  - Tests: `go test ./internal/nodeagent -run Manifest.*StackGate` — multi-step chaining derivation and rejection.

## Gate threading
- [x] Thread “effective expectation” into gate execution spec — Makes the next phases able to enforce expectations per gate run.
  - Repository: ploy
  - Component: `internal/workflow/contracts`, `internal/nodeagent`
  - Scope: Extend `internal/workflow/contracts/step_manifest.go` (`type StepGateSpec`) and populate it from `internal/nodeagent/manifest.go` for pre-gate (inbound) and post-gate (outbound).
  - Snippets: Add `StackGate` fields under `StepGateSpec` (shape defined by Phase 1).
  - Tests: `go test ./internal/nodeagent -run GateManifest.*StackGate` — pre/post gate manifests carry expected values.
