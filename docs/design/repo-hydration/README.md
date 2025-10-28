# Repository Hydration Slice

## Scope
Roadmap item 2.4 requires nodes to hydrate repositories without depending on pre-staged artifacts. The current filesystem hydrator only unpacks tarballs already present under `PLOY_ARTIFACT_ROOT`, so cold nodes cannot execute steps when snapshot/diff CIDs have not been cached locally. We must enable workers to fetch snapshots and ordered diffs from IPFS Cluster and fall back to cloning GitLab repositories with credentials managed in etcd, ensuring consistent repo state for every step.

## Solution
- Extend the step manifest contract with an explicit hydration plan that lists a base snapshot CID plus an ordered list of diff CIDs, keeping backward compatibility with existing `SnapshotCID`/`DiffCID` fields.
- Upgrade the filesystem hydrator into a remote-aware component that streams artifacts from IPFS Cluster when missing locally, verifies SHA-256 digests before extraction, and caches the tarballs under `snapshots/` and `diffs/` for reuse.
- When the control plane only supplies a repository reference (URL + branch/commit) and no CIDs, perform a shallow Git clone with the issued token, record the exact commit/branch resolved, archive that tree into an IPFS snapshot, and reuse the published CID for subsequent steps.
- Guard hydration with concurrency-safe caching (lock per CID) and deterministic extraction ordering so multiple inputs sharing the same base snapshot do not corrupt each other during parallel hydration.

## Changes required
1. **Manifest contract** (`internal/workflow/contracts/step_manifest.go`, tests): add `Hydration` metadata to `StepInput` (base snapshot CID, ordered diff CIDs, optional repo hint) and update validation to allow either the legacy single `SnapshotCID`/`DiffCID` or the new structured plan. Extend fixtures in `internal/workflow/contracts/step_manifest_test.go`.
2. **Artifact retrieval** (`internal/workflow/artifacts/cluster_client_fetch.go`, new helper): add a streaming `FetchToFile` path that downloads via `GET /ipfs/{cid}` into a temporary file, verifies digest, and atomically renames into the artifact cache.
3. **Workspace hydrator** (`internal/workflow/runtime/step/workspace_fs.go`, new file `workspace_remote.go`): refactor to (a) resolve hydration plans, (b) fetch/cache missing tarballs via the cluster client or fallback clone, and (c) apply base snapshot plus diff chain in order using temporary staging directories with per-CID locks.
4. **Git fallback** (`internal/node/worker/step/executor.go`, new `internal/node/worker/hydration/gitfetcher.go`): wire the hydrator with a Git fetcher that requests tokens through `/v1/gitlab/signer/tokens`, runs shallow clones with the issued token, captures the resolved commit/branch metadata, archives the tree, and publishes the snapshot back through `artifacts.ClusterPublisher`.
5. **Tests and fixtures**: add unit coverage for hydration plans (snapshot-only, snapshot+diff chain, clone fallback) in `internal/workflow/runtime/step/workspace_fs_test.go`, extend `internal/workflow/artifacts` tests for streaming fetch, and add executor-level tests in `internal/node/worker/step/executor_test.go` to assert clone fallback invokes the publisher.

## COSMIC evaluation
| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Hydrate repository input from manifest (resolve plan → fetch snapshot/diffs → cache/publish) | 1 | 0 | 2 | 1 | 4 |
| **TOTAL** | **1** | **0** | **2** | **1** | **4** |

Assumptions: (1) manifest hydration metadata delivers the repo reference required for Git fallback; (2) publishing the cloned snapshot uses the existing artifact publisher and does not introduce additional external processes.

## What to expect / How to test
- `go test ./internal/workflow/runtime/step` — validates hydration sequencing and cache paths.
- `go test ./internal/workflow/artifacts` — covers streaming fetch and digest verification.
- `go test ./internal/node/worker/step` — ensures executor wiring triggers fallback cloning and publishes new snapshots.
- Optional manual check: run a worker with empty artifact cache against a control-plane job referencing known CIDs to confirm automatic fetch and clone fallback.

## Out of scope
- CLI UX changes (e.g., new flags for hydration tuning).
- Control-plane scheduler changes beyond supplying the hydration plan.
- Artifact garbage-collection or retention policy adjustments.

## Open questions
None.

## Web search
- IPFS gateways serve `GET /ipfs/{cid}` responses as streaming downloads, which we can use to hydrate large snapshots without buffering them entirely in memory. citeturn1search1
- IPFS Cluster coordinates distributed pinning across peers and exposes HTTP APIs for clients, supporting the decision to rely on cluster fetches and re-publish cloned snapshots for reuse. citeturn1search9
- GitLab repositories accept personal access tokens with the `read_repository` scope for HTTPS clones, matching the fallback token we need from the signer. citeturn2search1

## Debugging
Not started; no issues encountered yet.
