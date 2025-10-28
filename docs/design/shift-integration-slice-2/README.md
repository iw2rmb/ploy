# SHIFT Integration Slice 2
Persist SHIFT reports and surface actionable metadata for roadmap item 2.3.

## Scope
Complete the remainder of roadmap step 2.3 by enriching SHIFT validation outputs. This slice delivers structured SHIFT metadata into job results and stores the CLI JSON report in IPFS so operators and automation can audit build-gate runs. Static check adapters stay out of scope per the updated requirements.

## Solution
Key decisions:
- Parse the `shift run --output json` payload emitted by the standalone CLI to recover run status, diagnostics, lane, and artifact paths without depending on raw stdout strings.
- Map SHIFT diagnostics into `buildgate.Metadata.LogFindings`, preserving severity and codes, so downstream consumers (CLI, control plane, metrics) can reason about failures beyond exit codes.
- Publish the structured SHIFT report as a dedicated step artifact (new artifact kind) alongside existing diff/log bundles, enabling IPFS-backed retention and later downloads for both successful and failed runs.
- Keep the executor contract tolerant: if parsing fails we fall back to the current exit-code based behaviour instead of aborting the build gate.

## Changes required
1. **SHIFT executor parsing** (`internal/workflow/buildgate/shift/executor.go`, new helper in same package):
   - Parse stdout into the CLI `ExecutionSummary` schema (import `github.com/iw2rmb/shift/pkg/cli/output` or mirror its shape) when `--output json` is requested.
   - Derive `SandboxBuildResult` details from the summary (status-driven failure reason/detail, duration capture).
   - Extract diagnostics and lane metadata for later mapping; guard parsing with graceful degradation when stdout is empty or invalid JSON.
2. **Metadata enrichment** (`internal/workflow/runtime/step/shift_client.go`):
   - Translate parsed SHIFT diagnostics into `buildgate.LogFindings`.
   - Include lane and executor information in findings evidence for quick triage.
   - Ensure `ShiftResult.Report` embeds the enriched metadata object so existing consumers (`parseShiftMetadata`) see structured content.
3. **SHIFT report artifact**:
   - Extend `step.ArtifactKind` with `ArtifactKindShiftReport` and document intent (`internal/workflow/runtime/step/artifacts.go`).
   - Teach the cluster publisher to name JSON payloads deterministically (for example, `shift-report-<timestamp>.json`) and accept buffer input (`internal/workflow/artifacts/publisher.go`).
   - Update the step runner to publish the SHIFT report for all runs and capture the resulting CID/digest in `Result` (`internal/workflow/runtime/step/runner.go`).
   - Surface the artifact through node executor plumbing and assignment results, and include it in retention/bundle metadata so downstream steps can reference the report (`internal/node/worker/step/executor.go`, `internal/workflow/runtime/local_client.go`, `internal/api/controlplane/client.go` serializers/tests).
4. **Go module wiring**:
   - Add `github.com/iw2rmb/shift` as a module dependency (with a `replace` to `../shift` for local development) so parsing code can reuse shared types.
5. **Tests and fixtures**:
   - Extend `internal/workflow/buildgate/shift/executor_test.go` with success/failure JSON samples.
   - Cover metadata mapping in `internal/workflow/runtime/step/shift_client_test.go`.
   - Add runner/executor assertions that the SHIFT artifact is published, retained, and reported (`internal/workflow/runtime/step/runner_test.go`, `internal/node/worker/step/executor_test.go`).
  - Verify control-plane client payloads include the new artifact keys and bundling metadata (`internal/api/controlplane/client_test.go`).

## COSMIC evaluation
| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Persist SHIFT metadata + report artifact | 1 | 1 | 1 | 1 | 4 |
| **TOTAL** | **1** | **1** | **1** | **1** | **4** |

## What to expect / How to test
- `go test ./internal/workflow/buildgate/...` — updated executor and metadata coverage.
- `go test ./internal/workflow/runtime/...` — step runner and client wiring.
- `go test ./internal/node/worker/...` — ensures assignment payload includes SHIFT artifact and metrics.
- `go test ./internal/api/controlplane/...` — verifies API marshals new artifact fields.
- Optional: manual `shift run --output json --path <fixture>` inside a hydrated workspace to sanity-check report parsing.

## Open questions
None; prior questions resolved by stakeholder guidance.

## Out of scope
- Enabling/disabling static check adapters or ingesting their outputs.
- Restructuring bundle retention logic or scheduler GC rules.
- CLI/operator UX enhancements beyond the new artifact visibility.

## Web search
- IPFS Proxy intercepts `/add` and pin operations, ensuring uploads through the cluster endpoint are automatically pinned, which matches the planned artifact publication flow. citeturn0search0
- The Cluster REST API `/add` endpoint replicates data across allocated peers, validating that SHIFT reports added via the publisher inherit cluster-wide redundancy. citeturn0search1
- IPFS Cluster reference material reiterates distributed pin management, supporting the decision to rely on cluster-managed retention rather than bespoke handling. citeturn0search2
