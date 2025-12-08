package contracts

// This file previously contained HTTP Build Gate request/response types for
// the `/v1/buildgate/*` API. That API and the backing `buildgate_jobs` table
// have been removed in favor of the unified jobs queue model documented in
// ROADMAP.md and docs/build-gate/README.md. The remaining gate contract types
// (e.g., BuildGateStageMetadata) are defined in their respective files and are
// used by the Docker-based gate executor and nodeagent.
