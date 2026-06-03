// Package nodeagent contains the ployd node execution agent.
//
// Responsibilities:
//   - Accept run requests from the control plane and orchestrate execution.
//   - Hydrate workspaces from Git, run Build Gate validation, execute mig containers,
//     and collect/upload artifacts, diffs, and terminal status.
//   - Execute discrete job types from the unified queue (gate, mig).
//
// Key files:
//   - execution.go — high level run lifecycle and runtime factories.
//   - gate_job.go — gate job execution and failure context persistence.
//   - container_job.go — mig job execution and shared container lifecycle.
//   - job_reporting.go — centralized diff/status upload helpers.
//   - workspace.go — workspace/file utilities.
//   - manifest.go — request→manifest translation helpers.
//   - job.go — job status types, image name persistence.
//   - http.go — base HTTP client, URL builders, compression helpers.
package nodeagent
