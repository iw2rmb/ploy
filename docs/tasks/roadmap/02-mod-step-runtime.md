# roadmap-mod-step-runtime-02 – Mod Step Runtime Pipeline

- **Identifier**: `roadmap-mod-step-runtime-02`
- [x] **Status**: Completed (2025-10-21)
- **Blocked by**:
  - `docs/design/mod-step-runtime/README.md`
  - `docs/design/control-plane/README.md`
- **Unblocks**:
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
  - `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`
  - `docs/tasks/roadmap/07-job-observability.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 11

| Functional process                  | E   | X   | R   | W   | CFP |
| ----------------------------------- | --- | --- | --- | --- | --- |
| Step manifest contract & validation | 1   | 1   | 1   | 0   | 3   |
| Container runtime orchestration     | 2   | 1   | 1   | 1   | 5   |
| SHIFT integration & CLI outputs     | 1   | 1   | 1   | 0   | 3   |
| **TOTAL**                           | 4   | 3   | 3   | 1   | 11  |

- Assumptions / notes: CFP assumes reuse of existing build gate runner and
  Docker client libraries; integration harness limited to local workstation
  nodes.

- **Why**
  - Replace Grid-dependent Mods execution with node-local container runtime per
    `docs/design/mod-step-runtime/README.md`.
  - Enforce SHIFT build gate automatically after each step to block regressions
    and surface actionable diagnostics.
  - Capture diffs, logs, and metrics so artifact publishing no longer depends on
    Grid storage.

- **How / Approach**
  - Introduce `StepManifest` contracts in `internal/workflow/contracts` plus
    validation utilities and table-driven tests.
- Implement step runner in `internal/workflow/runtime/step` that hydrates repo
  snapshots + diffs, launches OCI containers, streams logs, and retains
  containers.
  - Integrate `internal/workflow/buildgate` via a new SHIFT client interface
    invoked post-step with halt-on-failure semantics.
  - Update `internal/workflow/runner` and CLI submission paths
    (`cmd/ploy/mod_run.go`) to emit/consume manifest-driven jobs.
- Prepare diff/log capture outputs for artifact publishing while deferring IPFS
  wiring to `roadmap-mod-step-runtime-03`; wire CLI inspection commands to
  retained containers.

- **Changes Needed**
  - `internal/workflow/contracts/*` – define step manifest structs, validation,
    JSON schema.
- `internal/workflow/runtime/step/*` – container runtime, hydration planner,
  diff capture (tarball generation), log streaming.
  - `internal/workflow/runtime/registry.go` & adapters – add local runner,
    retire Grid-only assumptions.
  - `internal/workflow/buildgate/*` – expose SHIFT API client (sandbox only for
    now), error propagation helpers.
- `internal/workflow/runner/*` – consume control plane jobs, materialise
  manifests, trigger SHIFT hand-off.
  - `cmd/ploy/mod_run.go`, `cmd/ploy/mod_run_*.go` – CLI submission updates,
    golden output fixtures.
  - `docs/design/mod-step-runtime/README.md` – maintain status and verification
    notes.
- `docs/v2/job.md`, `docs/v2/mod.md`, `docs/v2/shift.md`,
  `docs/workflow/README.md` – align narrative with implementation.
  - `CHANGELOG.md` – record verification evidence and doc sync.

- **Definition of Done**
  - CLI submits Mods that expand into validated step manifests consumed by
    nodes.
- Node step runner executes containers locally with hydrated snapshots + diffs,
  retains containers for inspection, and captures diff/log artifacts as tarballs
  ready for publishing.
- SHIFT validation executes automatically after each step using sandbox
  enforcement (static checks deferred) and blocks downstream stages on failure
  while returning diagnostics to CLI.
  - CLI inspection/status commands reflect container retention, SHIFT results,
    and artifact availability placeholders.

- **Tests To Add / Fix**
  - Unit: `internal/workflow/contracts/step_manifest_test.go` (validation),
    `internal/workflow/runtime/step/runner_test.go` (container config assembly,
    SHIFT error handling).
- Integration: `tests/integration/runtime/step_runner_test.go` executing
  representative step images locally, verifying diff capture and SHIFT
  invocation (with stubbed SHIFT service).
  - Golden: `cmd/ploy/mod_run_output_test.go` covering success, failure, and
    inspection modes.
  - Update existing runner tests to assert control plane job payload
    compatibility.

- **Dependencies & Blockers**
  - Requires control plane scheduler (roadmap-control-plane design) to deliver
    job payloads.
  - Depends on SHIFT library simplification per `docs/v2/shift.md`.
  - Needs Docker Engine availability on workstation nodes.

- **Verification Steps**
  - `go test ./internal/workflow/contracts/... ./internal/workflow/runtime/...`
  - `go test -tags integration ./tests/integration/runtime/...`
  - `go test ./cmd/ploy -run TestModRun*`
  - `make lint-md`

- **Changelog / Docs Impact**
- Append dated entry describing runtime pipeline implementation, tests executed,
  and documentation refresh.
- Update `docs/v2/job.md`, `docs/v2/mod.md`, `docs/v2/shift.md`,
  `docs/workflow/README.md` with retention + inspection guidance.

- **Notes**
  - Evaluate container runtime abstraction for Podman compatibility (future
    follow-up).
  - Track IPFS publisher wiring in `roadmap-mod-step-runtime-03` once artifact
    interfaces are available.

## Progress Log

- 2025-10-21 — Added a node-local step runtime client
  (`internal/workflow/runtime/local_client`) plus a build gate-backed
  `ShiftClient` adapter (`internal/workflow/runtime/step/shift_client.go`);
  introduced unit coverage to lock down SHIFT diagnostics and stage outcome
  shaping:
  - `go test ./internal/workflow/runtime/...`
  - `go test ./internal/workflow/runtime/step/...`
- 2025-10-21 — Defaulted the CLI to the `local-step` runtime adapter, wired
  Docker-backed execution, filesystem workspace hydration, diff tarball capture,
  and artifact staging. Updated Mods docs and fixtures to reflect
  manifest-driven runs and staged artifacts.
  - `go test ./internal/workflow/runtime/...`
  - `go test ./cmd/...`
