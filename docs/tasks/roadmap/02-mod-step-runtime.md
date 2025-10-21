# Mod Step Runtime Pipeline

## Why
- Each Mod must execute typed steps inside OCI images with cumulative diffs, SHIFT gating, and artifact capture as described in `docs/v2/README.md`.
- Removing Grid-specific runtime assumptions requires a fresh pipeline that Ploy nodes can host locally.

## Required Changes
- Implement a step runner that mounts repository snapshots plus prior step diffs, then orchestrates container lifecycle (create, run, retain for inspection).
- Integrate the SHIFT build gate as an API/service invocation rather than embedding legacy Grid CLIs; ensure failures short-circuit downstream steps.
- Define a step manifest contract (inputs, outputs, environment) that nodes validate before execution.
- Capture step outputs (diffs, logs, metrics) and hand off to the artifact publisher without referencing Grid storage.

## Definition of Done
- CLI submits Mods that expand into step manifests consumed by nodes, with container execution succeeding end-to-end on workstation nodes.
- SHIFT validation runs automatically after each step and blocks the pipeline on failure with actionable diagnostics.
- Step containers remain available for inspection via documented CLI commands, matching the retention expectations in `docs/v2/job.md`.

## Tests
- Table-driven unit tests for manifest validation, container configuration assembly, and SHIFT error handling.
- Integration tests that run representative step images locally and verify diff capture plus SHIFT invocation.
- Golden-file tests for CLI status output covering success, failure, and inspection modes.
