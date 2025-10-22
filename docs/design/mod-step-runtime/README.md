# Mod Step Runtime Pipeline

- **Identifier**: `roadmap-mod-step-runtime`
- **Status**: Completed — 2025-10-22
- **Upstream Docs**: `../../v2/README.md`, `../../v2/mod.md`, `../../v2/job.md`, `../../v2/shift.md`

## Why

- Execute Mods steps locally on Ploy nodes without delegating to Grid.
- Guarantee deterministic container runs by hydrating snapshots plus ordered diff bundles.
- Enforce SHIFT build-gate validation after every step while keeping containers available for
  inspection.
- Publish diffs, logs, and SHIFT reports to IPFS Cluster so downstream steps can reuse artifacts.

## What to do

- Define `contracts.StepManifest` capturing image, command, env, inputs/outputs, retention, and SHIFT
  profile requirements.
- Hydrate workspaces by composing repo snapshots with prior diff bundles before launching containers.
- Use the Docker runtime to create/start/wait containers with retention disabled only by manifest
  choice.
- Capture logs and diffs, publish artifacts through the node-local IPFS client, and hand the results
  to the control plane.
- Invoke SHIFT via `internal/workflow/buildgate` to short-circuit pipelines on validation failure and
  surface actionable diagnostics.

## Where to change

- `internal/workflow/contracts` — manifest structs, validation logic.
- `internal/workflow/runtime/step` — workspace hydrator, container orchestration, diff capture,
  artifact publisher, SHIFT client.
- `internal/workflow/runtime` — adapter registration and wiring to the control plane job payload.
- `internal/workflow/runner` — use the new runtime and persist job results (container IDs, artifact
  CIDs, SHIFT reports).
- `docs/v2/mod.md`, `docs/v2/shift.md`, `docs/v2/ipfs.md` — keep examples and operator guidance in
  sync with the runtime behaviour.

## How to test

- `go test ./internal/workflow/runtime/...` — unit coverage for workspace hydration, container spec
  assembly, diff capture, and SHIFT error propagation.
- Integration: run sample images (`busybox`, `ghcr.io/ploy/mods-dev`) via the step runner while
  asserting diff/log artifact publication and SHIFT outcomes.
- CLI smoke: `make build && dist/ploy mod run --dry-run ...` to ensure manifest compilation and job
  submission exercise the new runtime bindings.
