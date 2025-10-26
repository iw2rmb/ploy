# Step Runner Decomposition

Design record for the step runner split that now lives in `internal/workflow/runtime/step`. This stays in sync with the merged work so future slices can extend the runner without collapsing back into a monolith.

## Status

| Date (UTC) | State | Notes |
| --- | --- | --- |
| 2025-06-09 | Completed | Split landed; runner orchestrates the collaborators defined below. |
| 2025-10-26 | Living doc | Updated to reflect the merged structure and highlight follow-on slices. |

## Context

- The original `runner.go` exceeded 400 LOC because it mixed orchestration, container contracts, workspace hydration, artifact publishing, and log streaming helpers. Any tweak required loading the entire file into reviewers' heads.
- The CLI runtime depends on this path for every workflow. Regressions block all `ploy step run` traffic, so incremental refactors must be obvious and low-risk.
- Multiple sub-teams now own pieces of the runner (workspace hydration, container runtime, artifacts, SHIFT). A clean split allows each team to iterate without merge-fighting unrelated logic.

## Design Objectives

1. Keep `Runner` as the orchestration entry point while moving supporting types and helpers into cohesive, <200 LOC files when practical.
2. Preserve the public API (`Runner`, `Request`, `Result`, and collaborator interfaces) so upstream callers/tests need no changes.
3. Document the data flow, invariants, and extension points so new runtimes, hydrators, or publishers can be added predictably.

## Execution Flow Overview

1. **Manifest validation (`runner.go`)** — `Runner.Run` validates the `contracts.StepManifest` and reports `ErrManifestInvalid` before doing any work.
2. **Workspace hydration (`workspace.go`, `workspace_fs.go`)** — `WorkspaceHydrator.Prepare` extracts snapshot/diff tarballs under a temp directory. The filesystem hydrator honours `PLOY_ARTIFACT_ROOT` (defaults to `$XDG_CACHE_HOME/ploy/artifacts`) so bundle locations remain deterministic. Missing inputs surface as `ErrWorkspaceUnavailable`.
3. **Container spec construction (`container_spec.go`)** — `buildContainerSpec` creates mounts/env/working dir from the manifest plus hydrated workspace. A read/write mount is required for diff capture; missing mounts fail fast.
4. **Container lifecycle (`container_docker.go`)** — `ContainerRuntime` is an interface; the Docker implementation handles optional image pulls, create/start/wait, and log collection. Containers run with `AutoRemove=false` so retention policies can keep them around for inspection.
5. **Log capture & streaming (`logging.go`, `runner_streams.go`)** — `LogCollector` overrides runtime log collection when present. Buffered logs are pushed to `LogStreamPublisher` with RFC3339 timestamps, followed by run status and retention hints (`logstream.RetentionHint`).
6. **Diff capture (`diff_shift.go`, `diff_fs.go`)** — `DiffGenerator.Capture` archives the writable mount into a deterministic tarball. The filesystem generator enforces the write-mount contract and streams contents via `archive/tar` + context cancellation.
7. **Artifact publication (`artifacts.go`, `artifact_fs.go`)** — `ArtifactPublisher.Publish` stores both diff tarballs and log payloads. The filesystem publisher writes deterministic CIDs (`ipfs:<sha256>`) plus digests so downstream consumers can verify content.
8. **SHIFT validation (`shift_client.go`)** — When manifests include SHIFT metadata, `ShiftClient.Validate` runs after artifact publication so reports can link to the log bundle CID. The build-gate client normalises metadata and produces actionable failure strings while filling duration defaults.
9. **Retention signal (`runner_streams.go`)** — Regardless of success, `publishRetentionHint` emits TTL and CID metadata to the log stream so the CLI can display inspection hints without re-fetching artifacts.

## Package Layout

| File | Responsibility |
| --- | --- |
| `runner.go` | Errors, `Request`, `Result`, `Runner`, and orchestration logic. |
| `runner_streams.go` | Log/status publishing helpers. |
| `workspace.go` / `workspace_fs.go` | Hydrator contracts plus filesystem implementation. |
| `container_spec.go` | Container contracts and spec builder. |
| `container_docker.go` | Docker-backed runtime implementation. |
| `diff_shift.go` | Diff + SHIFT interface definitions shared across the package. |
| `diff_fs.go` | Filesystem diff generator. |
| `shift_client.go` | Build gate-backed SHIFT client and helpers. |
| `artifacts.go` / `artifact_fs.go` | Artifact publisher interfaces plus filesystem implementation. |
| `logging.go` | `LogCollector` / `LogStreamPublisher` interfaces. |
| `runner_test.go`, `shift_client_test.go` | Regression coverage (≈92% for the package, satisfying ≥90% on workflow-critical code). |

## Interface Contracts & Extension Points

- **WorkspaceHydrator** — Must validate manifests and populate `Workspace.Inputs` for every declared input. Alternative hydrators (e.g., remote cache, snapshot streaming) can swap in as long as they respect the temp-directory contract.
- **ContainerRuntime** — Implements `Create`, `Start`, `Wait`, `Logs`. Additional runtimes (Kubernetes, containerd) plug in without touching hydration or diff code.
- **DiffGenerator** — Returns a stable path until artifacts are published. Non-filesystem implementations (e.g., snapshot deltas) should maintain the read/write mount invariant.
- **ArtifactPublisher** — Called twice per run (diff + logs). Publishers must return deterministic `PublishedArtifact` metadata so SHIFT and retention hints can refer to stable CIDs.
- **ShiftClient** — Optional; when nil the runner marks SHIFT as passed. Clients should copy the log artifact reference when available to provide richer findings.
- **LogCollector / LogStreamPublisher** — Optional. Collectors may stream logs in real time; publishers should handle duplicate hints because `publishRetentionHint` fires on both success and failure paths.

## Guardrails & Observability

- Nil collaborators are rejected with precise `step:`-prefixed errors so operators can grep logs quickly.
- `publishLogStream` timestamps each line on emission rather than trusting container output, keeping viewer ordering predictable.
- The filesystem hydrator and artifact publisher share helper utilities (`defaultArtifactRoot`, `sanitizeName`) so path derivations stay consistent. When defaults change, update both files plus `docs/envs/README.md`.
- SHIFT duration defaults to measured wall time when adapters omit it, ensuring CLI reports stay accurate across implementations.

## Testing & Coverage Plan

- `go test ./internal/workflow/runtime/step` — primary RED→GREEN loop for runner, hydrator, diff, artifact, and SHIFT units.
- `go test ./internal/workflow/runtime/...` — quick sweep to ensure interface changes do not break runtime adapters.
- `make test` — full guardrail run (lint, coverage ≥60% global). Run before PRs or whenever collaborator implementations change.
- GRID/VPS (REFACTOR phase) — once JetStream access is restored, exercise `tests/e2e` for adapters that talk to remote resources. Not required for pure file moves.

## Follow-On Opportunities

1. Lift the Docker runtime into its own subpackage (`internal/workflow/runtime/step/docker`) or adapter to simplify dependency injection and testing doubles.
2. Add an IPFS Cluster-backed `ArtifactPublisher` so filesystem drops stay optional for local dev; document the required credentials in `docs/envs/README.md`.
3. Record structured metadata (exit code, durations) inside `publishRetentionHint` so the CLI can summarise runs without log lookups.
4. Ensure `docs/envs/README.md` tracks runtime-affecting knobs such as `PLOY_ARTIFACT_ROOT` and `PLOY_RUNTIME_ADAPTER`; the absence of an entry should be logged as a TODO in any slice that introduces one.
