# SHIFT Integration (Slice 1)
> Keeping SHIFT enforcement wired into the step runtime prevents regressions from skipping build gate checks. This slice focuses on replacing the noop executor with the real SHIFT runner; static-check adapters and artifact publishing follow in later slices.

## Why
> Implement roadmap item 2.3 from `docs/next/roadmap.md`: replace the noop SHIFT executor with a real integration that shells out to the standalone SHIFT module, runs static analysis, and stores structured reports so failures are actionable.

## How
> Key changes for slice 1:
> 1. Extend the build gate contract so sandbox executions know which hydrated workspace to analyse.
> 2. Replace `noopSandboxExecutor` with a SHIFT-backed executor that shells out to the standalone `shift` CLI, captures logs, and maps exit codes to failure metadata.
> 3. Update the STEP client wiring (CLI + worker) to use the new executor and feed workspace/profile data through `SandboxSpec`.
> 4. Leave static-check adapters + structured artifact publication for the next slice (tracked separately).

## What & Where
> - Add `Workspace` (and helper accessors) to `internal/workflow/buildgate.SandboxSpec`; adjust sandbox runner tests to assert passthrough.
> - Create `internal/workflow/buildgate/shift/executor.go`:
>   - Accepts `SandboxSpec` with workspace + profile env, spawns the `shift run` CLI (`--path`, `--lane`, `--output json`).
>   - Streams SHIFT stdout/stderr into buffers, computes SHA-256 digest for metadata propagation, and translates exit codes into `SandboxBuildResult` (success/cache hit, failure reason/detail).
>   - Surfaces `Metadata.LogDigest` by reading the buffered output; metadata for static checks stays empty this slice.
> - Update `step.BuildGateShiftClient.Validate`:
>   - Resolve the hydrated read-write workspace path from `ShiftRequest.Workspace.Inputs`.
>   - Populate `SandboxSpec.Workspace`, propagate manifest profile / timeout env overrides, keep log artifact wiring unchanged.
> - Replace `noopSandboxExecutor` in both `cmd/ploy/dependencies_runtime_local.go` and `internal/node/worker/step/executor.go` with the shared SHIFT executor factory (`internal/workflow/buildgate/shift/factory.go`).
> - Adjust `internal/workflow/buildgate/runner_test.go` / CLI runtime tests to assert the executor receives workspace + env.
> - Document follow-up slices for static-check adapters & SHIFT artifact publishing (tracked in docs queue).

## COSMIC evaluation
> Scope: single functional process (SHIFT validation inside the runtime) that accepts a manifest/workspace entry, reads hydrated workspace metadata, writes failure details, and exits with structured outcome.

| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Execute SHIFT build gate via sandbox executor | 1 | 1 | 1 | 1 | 4 |
| **TOTAL** | **1** | **1** | **1** | **1** | **4** |

## What to expect / How to test
> - Unit tests covering:
>   - `shift.Executor` success/failure paths (mock SHIFT runner, assert `SandboxBuildResult` fields, log digest propagation, failure mapping).
>   - `BuildGateShiftClient.Validate` populating workspace/profile env and forwarding to executor.
>   - CLI + worker constructors wiring the new executor (ensure tests observe workspace forwarded).
> - End-to-end smoke (deferred) once static-check + artifact slices land.
> - `go test ./...` remains green.
