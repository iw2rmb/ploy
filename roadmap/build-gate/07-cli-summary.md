# CLI Build Gate Summary
- [x] Completed 2025-10-07

## Why / What For
Surface build gate outcomes inside the workstation CLI so developers immediately see static check failures and knowledge base guidance without digging through checkpoints or Grid logs.

## Required Changes
- Expose build gate log findings and static check metadata through workflow checkpoints.
- Render build gate summaries after `ploy workflow run` completes, including actionable knowledge base findings and log digests.
- Expand documentation and roadmap records to capture the new CLI behaviour.

## Definition of Done
- Build gate metadata published with checkpoints includes log findings alongside static check reports.
- `ploy workflow run` prints a build gate summary showing static check status, failing diagnostics, knowledge base findings, and the associated log digest when available.
- Documentation and the SHIFT tracker record the milestone with cross references to the design record.

## Implementation Notes
- Added `BuildGateLogFinding` to the workflow contracts so sanitised log ingestion results flow into checkpoints.
- Updated the runner to propagate log findings and static check results into checkpoint metadata.
- Extended the CLI workflow summary to read checkpoints from the in-memory bus and display build gate diagnostics with severity, rule identifiers, and evidence lines.

## Tests
- `cmd/ploy/workflow_run_test.go` covers the CLI summary output with representative static check and knowledge base findings.
- `internal/workflow/runner/runner_events_test.go` verifies build gate metadata sanitation for log findings.
- `go test ./...` ensures the workflow runner, contracts, and CLI coverage remains intact.

## References
- Build gate design (`docs/design/build-gate/README.md`).
- SHIFT roadmap slice (`roadmap/shift/21-build-gate-reboot.md`).
