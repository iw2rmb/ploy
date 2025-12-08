package handlers

import (
	"log/slog"
	"net/http"

	"github.com/iw2rmb/ploy/internal/store"
)

const buildGateDeprecatedMessage = "HTTP Build Gate API has been removed; gate now runs as part of the unified jobs queue. See ROADMAP.md for details."

// buildGateDeprecatedHandler returns a handler that reports the removal of the
// HTTP Build Gate API. It keeps the routes mounted so OpenAPI coverage tests
// continue to pass, but signals callers to migrate to the jobs-based pipeline.
func buildGateDeprecatedHandler(_ store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Warn("HTTP Build Gate endpoint called after deprecation",
			"method", r.Method,
			"path", r.URL.Path,
		)

		http.Error(w, buildGateDeprecatedMessage, http.StatusGone)
	}
}

// validateBuildGateHandler handles POST /v1/buildgate/validate.
// The HTTP Build Gate API has been removed; this endpoint now returns 410 Gone.
func validateBuildGateHandler(st store.Store) http.HandlerFunc {
	return buildGateDeprecatedHandler(st)
}

// getBuildGateJobStatusHandler handles GET /v1/buildgate/jobs/{id}.
// The HTTP Build Gate API has been removed; this endpoint now returns 410 Gone.
func getBuildGateJobStatusHandler(st store.Store) http.HandlerFunc {
	return buildGateDeprecatedHandler(st)
}

// claimBuildGateJobHandler handles POST /v1/nodes/{id}/buildgate/claim.
// The HTTP Build Gate API has been removed; this endpoint now returns 410 Gone.
func claimBuildGateJobHandler(st store.Store) http.HandlerFunc {
	return buildGateDeprecatedHandler(st)
}

// completeBuildGateJobHandler handles POST /v1/nodes/{id}/buildgate/{job_id}/complete.
// The HTTP Build Gate API has been removed; this endpoint now returns 410 Gone.
func completeBuildGateJobHandler(st store.Store) http.HandlerFunc {
	return buildGateDeprecatedHandler(st)
}

// ackBuildGateJobStartHandler handles POST /v1/nodes/{id}/buildgate/{job_id}/ack.
// The HTTP Build Gate API has been removed; this endpoint now returns 410 Gone.
func ackBuildGateJobStartHandler(st store.Store) http.HandlerFunc {
	return buildGateDeprecatedHandler(st)
}
