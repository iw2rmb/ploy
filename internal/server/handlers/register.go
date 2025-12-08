package handlers

import (
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/events"
	httpapi "github.com/iw2rmb/ploy/internal/server/http"
	"github.com/iw2rmb/ploy/internal/store"
)

// RegisterRoutes mounts all HTTP endpoints on the given server.
func RegisterRoutes(s *httpapi.Server, st store.Store, eventsService *events.Service, configHolder *ConfigHolder, tokenSecret string) {
	// Health
	s.HandleFunc("/health", healthHandler)

	// Config
	s.HandleFunc("GET /v1/config/gitlab", getGitLabConfigHandler(configHolder), auth.RoleCLIAdmin)
	s.HandleFunc("PUT /v1/config/gitlab", putGitLabConfigHandler(configHolder), auth.RoleCLIAdmin)

	// Token management
	s.HandleFunc("POST /v1/tokens", createAPITokenHandler(st, tokenSecret), auth.RoleCLIAdmin)
	s.HandleFunc("GET /v1/tokens", listAPITokensHandler(st), auth.RoleCLIAdmin)
	s.HandleFunc("DELETE /v1/tokens/{id}", revokeAPITokenHandler(st), auth.RoleCLIAdmin)

	// Bootstrap tokens
	s.HandleFunc("POST /v1/bootstrap/tokens", createBootstrapTokenHandler(st, tokenSecret), auth.RoleControlPlane, auth.RoleCLIAdmin)
	s.HandleFunc("POST /v1/pki/bootstrap", bootstrapCertificateHandler(st, tokenSecret), auth.RoleWorker)

	// Mods run submission (simplified API for single-repo runs).
	s.HandleFunc("POST /v1/mods", submitRunHandler(st, eventsService), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/mods/{id}/events", getModEventsHandler(st, eventsService), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/mods/{id}/graph", getModGraphHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/mods/{id}", getRunStatusHandler(st), auth.RoleControlPlane)
	// Mods run cancellation.
	s.HandleFunc("POST /v1/mods/{id}/cancel", cancelRunHandler(st, eventsService), auth.RoleControlPlane)
	// Mods run resume (for failed/canceled runs).
	s.HandleFunc("POST /v1/mods/{id}/resume", resumeRunHandler(st, eventsService), auth.RoleControlPlane)
	// Diffs listing and download (Worker role for multi-node rehydration C2, ControlPlane for CLI access)
	s.HandleFunc("GET /v1/mods/{id}/diffs", listRunDiffsHandler(st), auth.RoleControlPlane, auth.RoleWorker)
	s.HandleFunc("GET /v1/diffs/{id}", getDiffHandler(st), auth.RoleControlPlane, auth.RoleWorker)
	s.HandleFunc("POST /v1/mods/{id}/artifact_bundles", createRunArtifactBundleHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/mods/{id}/logs", createRunLogHandler(st, eventsService), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/mods/{id}/diffs", createRunDiffHandler(st), auth.RoleControlPlane)

	// Artifact download endpoints
	s.HandleFunc("GET /v1/artifacts", listArtifactsByCIDHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/artifacts/{id}", getArtifactHandler(st), auth.RoleControlPlane)

	// Runs — batch lifecycle endpoints for listing, inspecting, stopping, and starting batched runs.
	s.HandleFunc("GET /v1/runs", listRunsHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/runs/{id}", getRunHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/runs/{id}/stop", stopRunHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/runs/{id}/start", startRunHandler(st), auth.RoleControlPlane)

	// RunRepo — manage repos within a batch (add/remove/restart/list).
	s.HandleFunc("POST /v1/runs/{id}/repos", addRunRepoHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/runs/{id}/repos", listRunReposHandler(st), auth.RoleControlPlane)
	s.HandleFunc("DELETE /v1/runs/{id}/repos/{repo_id}", deleteRunRepoHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/runs/{id}/repos/{repo_id}/restart", restartRunRepoHandler(st), auth.RoleControlPlane)

	// Repos — repo-centric view: list repos and show runs for a given repo.
	s.HandleFunc("GET /v1/repos", listReposHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/repos/{repo_id}/runs", listRunsForRepoHandler(st), auth.RoleControlPlane)

	// Runs (control plane) — legacy write/management endpoints
	s.HandleFunc("GET /v1/runs/{id}/timing", getRunTimingHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/runs/{id}/logs", createRunLogHandler(st, eventsService), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/runs/{id}/diffs", createRunDiffHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/runs/{id}/artifact_bundles", createRunArtifactBundleHandler(st), auth.RoleControlPlane)
	s.HandleFunc("DELETE /v1/runs/{id}", deleteRunHandler(st), auth.RoleControlPlane)

	// Node management endpoints
	s.HandleFunc("GET /v1/nodes", listNodesHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/nodes/{id}/drain", drainNodeHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/nodes/{id}/undrain", undrainNodeHandler(st), auth.RoleControlPlane)

	// Node worker endpoints
	s.HandleFunc("POST /v1/nodes/{id}/heartbeat", heartbeatHandler(st), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/claim", claimJobHandler(st, configHolder), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/ack", ackRunStartHandler(st, eventsService), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/complete", completeRunHandler(st, eventsService), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/events", createNodeEventsHandler(st, eventsService), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/logs", createNodeLogsHandler(st, eventsService), auth.RoleWorker)

	// Job artifact and diff upload endpoints (run-scoped, no node ID)
	s.HandleFunc("POST /v1/runs/{run_id}/jobs/{job_id}/artifact", createJobArtifactHandler(st), auth.RoleWorker)
	s.HandleFunc("POST /v1/runs/{run_id}/jobs/{job_id}/diff", createJobDiffHandler(st), auth.RoleWorker)

	// Job-level completion endpoint — simplifies node → server contract by addressing jobs directly.
	// Node identity is derived from mTLS certificate; no node_id in URL or body.
	s.HandleFunc("POST /v1/jobs/{job_id}/complete", completeJobHandler(st, eventsService), auth.RoleWorker)

	// NOTE: HTTP Build Gate endpoints (POST /v1/buildgate/validate, GET /v1/buildgate/jobs/{id},
	// POST /v1/nodes/{id}/buildgate/*, etc.) have been removed. Gate execution now runs
	// as part of the unified jobs queue. See ROADMAP.md for details.
}
