# Runner JobSpec Composition
- [ ] Pending

## Why / What For
Construct `workflowrpc.JobSpec` payloads from lane definitions so Grid receives the required `image`, `command`, `env`, and `resources` fields for every submitted stage.

## Required Changes
- Extend lane specifications (and loader) to expose job configuration pieces (image, command, env, resources) enforced by the design's Job Spec Schema.
- Update the workflow runner to translate each stage into a `JobSpec`, merging lane defaults with manifest/Aster overrides.
- Attach cache key, lane, and priority metadata to `JobSpec.Metadata` for scheduler consumption.

## Definition of Done
- Runner submits stages with fully populated `JobSpec` payloads that pass SDK validation (image/command/env/resources present even when lanes rely on defaults).
- Cache keys and lane metadata continue to appear in checkpoints and job metadata.
- Tests confirm missing job data surfaces actionable errors.

## Tests
- Runner unit tests validating `JobSpec` composition across sample lanes (Go, Node, Java) with snapshots for env/resources.
- Lane loader tests covering new fields and error paths.

## References
- Ploy Workflow RPC Alignment design (`docs/design/workflow-rpc-alignment/README.md`).
- Grid Workflow RPC helper guide (`../grid/sdk/workflowrpc/README.md`) for expectations on builder defaults.
- Grid Workflow RPC types (`../grid/sdk/workflowrpc/go/types.go`).
