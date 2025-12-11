package runs

// Legacy note:
// Package runs previously contained a job-level InspectCommand wired to
// GET /v1/mods/{id}. That surface has been removed in favor of the run-level
// status command exposed as `ploy run status` backed by GetStatusCommand,
// which uses the batch summary view from /v1/runs/{id}.
