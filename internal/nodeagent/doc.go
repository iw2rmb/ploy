// Package nodeagent contains the ployd node execution agent.
//
// Responsibilities:
//   - Accept run requests from the control plane and orchestrate execution.
//   - Hydrate workspaces from Git, run Build Gate validation, execute mod containers,
//     and collect/upload artifacts, diffs, and terminal status.
//   - Implement gate-heal-regate orchestration. The healing flow is split across
//     focused files so the main orchestrator remains focused on lifecycle wiring
//     (workspace, runtimes, uploads, and status reporting).
//
// Key files:
//   - execution_orchestrator.go — high level run lifecycle and status upload.
//   - execution_healing.go — Build Gate healing orchestration entrypoints.
//   - execution_healing_loop.go — Healing loop state machine (gate → heal → re-gate).
//   - execution_healing_indir.go — /in artifacts and build-gate log persistence helpers.
//   - execution_healing_session.go — session propagation helpers (`codex-session.txt`).
//   - execution_upload.go — centralized diff/status/artifact upload helpers.
//   - execution.go — runtime factories and GitLab MR wiring helpers.
//   - manifest.go — request→manifest translation and helpers for healing manifests.
package nodeagent
