// Package nodeagent contains the ployd node execution agent.
//
// Responsibilities:
//   - Accept run requests from the control plane and orchestrate execution.
//   - Hydrate workspaces from Git, run Build Gate validation, execute mod containers,
//     and collect/upload artifacts, diffs, and terminal status.
//   - Implement gate-heal-regate orchestration. The healing loop is isolated in
//     execution_healing.go so the main orchestrator remains focused on lifecycle
//     wiring (workspace, runtimes, uploads, and status reporting).
//
// Key files:
//   - execution_orchestrator.go — high level run lifecycle and status upload.
//   - execution_healing.go — Build Gate healing loop (gate → heal → re-gate → main).
//   - execution.go — runtime factories and GitLab MR wiring helpers.
//   - manifest.go — request→manifest translation and helpers for healing manifests.
package nodeagent
