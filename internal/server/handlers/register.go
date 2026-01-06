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

	// Config — GitLab
	s.HandleFunc("GET /v1/config/gitlab", getGitLabConfigHandler(configHolder), auth.RoleCLIAdmin)
	s.HandleFunc("PUT /v1/config/gitlab", putGitLabConfigHandler(configHolder), auth.RoleCLIAdmin)

	// Config — Global Env (ROADMAP.md line 47: /v1/config/env endpoints)
	s.HandleFunc("GET /v1/config/env", listGlobalEnvHandler(configHolder), auth.RoleCLIAdmin)
	s.HandleFunc("GET /v1/config/env/{key}", getGlobalEnvHandler(configHolder), auth.RoleCLIAdmin)
	s.HandleFunc("PUT /v1/config/env/{key}", putGlobalEnvHandler(configHolder, st), auth.RoleCLIAdmin)
	s.HandleFunc("DELETE /v1/config/env/{key}", deleteGlobalEnvHandler(configHolder, st), auth.RoleCLIAdmin)

	// Token management
	s.HandleFunc("POST /v1/tokens", createAPITokenHandler(st, tokenSecret), auth.RoleCLIAdmin)
	s.HandleFunc("GET /v1/tokens", listAPITokensHandler(st), auth.RoleCLIAdmin)
	s.HandleFunc("DELETE /v1/tokens/{id}", revokeAPITokenHandler(st), auth.RoleCLIAdmin)

	// Bootstrap tokens
	s.HandleFunc("POST /v1/bootstrap/tokens", createBootstrapTokenHandler(st, tokenSecret), auth.RoleControlPlane, auth.RoleCLIAdmin)
	s.HandleFunc("POST /v1/pki/bootstrap", bootstrapCertificateHandler(st, tokenSecret), auth.RoleWorker)

	// Runs — single-repo run submission (v1 API).
	// v1 change (roadmap/v1/scope.md:66, roadmap/v1/api.md:104): POST /v1/runs for single-repo submission.
	s.HandleFunc("POST /v1/runs", createSingleRepoRunHandler(st, eventsService), auth.RoleControlPlane)

	// Mods — mod project CRUD (v1 API).
	// v1 change (roadmap/v1/api.md:5-91): Repurpose /v1/mods for mod project CRUD.
	s.HandleFunc("POST /v1/mods", createModHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/mods", listModsHandler(st), auth.RoleControlPlane)
	s.HandleFunc("DELETE /v1/mods/{mod_id}", deleteModHandler(st), auth.RoleControlPlane)
	s.HandleFunc("PATCH /v1/mods/{mod_id}/archive", archiveModHandler(st), auth.RoleControlPlane)
	s.HandleFunc("PATCH /v1/mods/{mod_id}/unarchive", unarchiveModHandler(st), auth.RoleControlPlane)
	// v1 change (roadmap/v1/api.md:140-151, roadmap/v1/scope.md:8-9): Set mod spec (append-only specs + mods.spec_id pointer).
	s.HandleFunc("POST /v1/mods/{mod_id}/specs", setModSpecHandler(st), auth.RoleControlPlane)
	// v1 change (roadmap/v1/api.md:154-198, roadmap/v1/scope.md:10): Mod repo set management (add/list/delete + bulk CSV upsert).
	s.HandleFunc("POST /v1/mods/{mod_id}/repos", addModRepoHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/mods/{mod_id}/repos", listModReposHandler(st), auth.RoleControlPlane)
	s.HandleFunc("DELETE /v1/mods/{mod_id}/repos/{repo_id}", deleteModRepoHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/mods/{mod_id}/repos/bulk", bulkUpsertModReposHandler(st), auth.RoleControlPlane)

	// Legacy routes under /v1/mods/{id}/* for run operations.
	// These routes remain for backward compatibility but should eventually move to /v1/runs/{id}/*.
	// See roadmap/v1/scope.md:66-73 for the v1 migration plan.
	// Note: POST /v1/mods/{id}/cancel has been moved to POST /v1/runs/{id}/cancel (v1).
	// Note: POST /v1/mods/{id}/resume has been removed; v1 uses repo-level POST /v1/runs/{id}/repos/{repo_id}/restart.
	s.HandleFunc("GET /v1/mods/{id}/graph", getModGraphHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/mods/{id}/diffs", listRunDiffsHandler(st), auth.RoleControlPlane, auth.RoleWorker)
	s.HandleFunc("GET /v1/diffs/{id}", getDiffHandler(st), auth.RoleControlPlane, auth.RoleWorker)
	s.HandleFunc("POST /v1/mods/{id}/logs", createRunLogHandler(st, eventsService), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/mods/{id}/diffs", createRunDiffHandler(st), auth.RoleControlPlane)

	// Artifact download endpoints
	s.HandleFunc("GET /v1/artifacts", listArtifactsByCIDHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/artifacts/{id}", getArtifactHandler(st), auth.RoleControlPlane)

	// Runs — batch lifecycle endpoints for listing, inspecting, cancelling, starting, and streaming logs/events.
	s.HandleFunc("GET /v1/runs", listRunsHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/runs/{id}", getRunHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/runs/{id}/status", getRunStatusHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/runs/{id}/logs", getRunLogsHandler(st, eventsService), auth.RoleControlPlane)
	// v1 API: POST /v1/runs/{id}/cancel — cancels the run, all repos (Queued/Running → Cancelled), and cancels/removes Created/Queued/Running jobs.
	// Required by roadmap/v1/scope.md:72 and roadmap/v1/statuses.md:177-184.
	s.HandleFunc("POST /v1/runs/{id}/cancel", cancelRunHandlerV1(st), auth.RoleControlPlane)
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
	s.HandleFunc("DELETE /v1/runs/{id}", deleteRunHandler(st), auth.RoleControlPlane)

	// Node management endpoints
	s.HandleFunc("GET /v1/nodes", listNodesHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/nodes/{id}/drain", drainNodeHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/nodes/{id}/undrain", undrainNodeHandler(st), auth.RoleControlPlane)

	// Node worker endpoints
	s.HandleFunc("POST /v1/nodes/{id}/heartbeat", heartbeatHandler(st), auth.RoleWorker)
	// claimJobHandler now publishes SSE "running" events directly when the run
	// transitions from 'queued' to 'running'. The separate ackRunStartHandler
	// endpoint has been removed — claim is the canonical place for run lifecycle events.
	s.HandleFunc("POST /v1/nodes/{id}/claim", claimJobHandler(st, configHolder, eventsService), auth.RoleWorker)
	// NOTE: Node-based completion endpoint (/v1/nodes/{id}/complete) has been removed.
	// Use the job-level endpoint POST /v1/jobs/{job_id}/complete instead.
	// NOTE: The ack endpoint (/v1/nodes/{id}/ack) has been removed. Run status
	// transitions and SSE events are now handled in claimJobHandler.
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
