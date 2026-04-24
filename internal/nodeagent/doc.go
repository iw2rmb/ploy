// Package nodeagent contains the ployd node execution agent.
//
// Responsibilities:
//   - Accept run requests from the control plane and orchestrate execution.
//   - Hydrate workspaces from Git, run Build Gate validation, execute mig containers,
//     and collect/upload artifacts, diffs, and terminal status.
//   - Execute discrete job types from the unified queue (gate, mig, sbom, mr).
//
// Key files:
//   - execution_orchestrator.go — high level run lifecycle and status upload.
//   - execution_orchestrator_gate.go — gate job execution and failure context persistence.
//   - execution_orchestrator_jobs.go — mig/sbom/mr job execution and shared helpers.
//   - execution_upload.go — centralized diff/status/artifact upload helpers.
//   - execution.go — runtime factories, rehydration helpers, workspace/file utilities.
//   - manifest.go — request→manifest translation helpers.
//   - job.go — job status types, image name persistence.
//   - http.go — base HTTP client, URL builders, compression helpers.
package nodeagent
