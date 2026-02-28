package handlers

import (
	"github.com/iw2rmb/ploy/internal/blobstore"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// RegisterRoutes mounts all HTTP endpoints on the given server.
func RegisterRoutes(s *server.HTTPServer, st store.Store, bs blobstore.Store, bp *blobpersist.Service, eventsService *server.EventsService, configHolder *ConfigHolder, tokenSecret string) {
	// Health
	s.HandleFunc("/health", healthHandler)

	// Config — GitLab
	s.HandleFunc("GET /v1/config/gitlab", getGitLabConfigHandler(configHolder), auth.RoleCLIAdmin)
	s.HandleFunc("PUT /v1/config/gitlab", putGitLabConfigHandler(configHolder), auth.RoleCLIAdmin)

	// Config — Global Env (see docs/api/paths/config_env.yaml and docs/api/paths/config_env_key.yaml)
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
	// POST /v1/runs creates a single-repo run with automatic mig project creation.
	s.HandleFunc("POST /v1/runs", createSingleRepoRunHandler(st, eventsService), auth.RoleControlPlane)

	// Migs — mig project CRUD (v1 API).
	// /v1/migs endpoints handle mig project CRUD operations.
	s.HandleFunc("POST /v1/migs", createMigHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/migs", listMigsHandler(st), auth.RoleControlPlane)
	s.HandleFunc("DELETE /v1/migs/{mig_ref}", deleteMigHandler(st), auth.RoleControlPlane)
	s.HandleFunc("PATCH /v1/migs/{mig_ref}/archive", archiveMigHandler(st), auth.RoleControlPlane)
	s.HandleFunc("PATCH /v1/migs/{mig_ref}/unarchive", unarchiveMigHandler(st), auth.RoleControlPlane)
	// Set mig spec (append-only specs + migs.spec_id pointer).
	s.HandleFunc("POST /v1/migs/{mig_ref}/specs", setMigSpecHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/migs/{mig_ref}/specs/latest", getMigLatestSpecHandler(st), auth.RoleControlPlane)
	// Mig repo set management (add/list/delete + bulk CSV upsert).
	s.HandleFunc("POST /v1/migs/{mig_id}/repos", addMigRepoHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/migs/{mig_id}/repos", listMigReposHandler(st), auth.RoleControlPlane)
	s.HandleFunc("DELETE /v1/migs/{mig_id}/repos/{repo_id}", deleteMigRepoHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/migs/{mig_id}/repos/bulk", bulkUpsertMigReposHandler(st), auth.RoleControlPlane)
	// Multi-repo run submission with repo selection (all/failed/explicit).
	s.HandleFunc("POST /v1/migs/{mig_id}/runs", createMigRunHandler(st), auth.RoleControlPlane)
	// Pull resolution for mig repos (last-succeeded/last-failed).
	s.HandleFunc("POST /v1/migs/{mig_id}/pull", pullMigRepoHandler(st), auth.RoleControlPlane)

	// Artifact download endpoints
	s.HandleFunc("GET /v1/artifacts", listArtifactsByCIDHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/artifacts/{id}", getArtifactHandler(st, bs), auth.RoleControlPlane)

	// Runs — batch lifecycle endpoints for listing, inspecting, cancelling, starting, and streaming logs/events.
	s.HandleFunc("GET /v1/runs", listRunsHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/runs/{id}", getRunHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/runs/{id}/status", getRunStatusHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/runs/{id}/logs", getRunLogsHandler(st, eventsService), auth.RoleControlPlane)
	// v1 API: POST /v1/runs/{id}/cancel — cancels the run, all repos (Queued/Running → Cancelled), and cancels/removes Created/Queued/Running jobs.
	s.HandleFunc("POST /v1/runs/{id}/cancel", cancelRunHandlerV1(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/runs/{id}/start", startRunHandler(st), auth.RoleControlPlane)

	// RunRepo — manage repos within a batch (add/restart/list).
	s.HandleFunc("POST /v1/runs/{id}/repos", addRunRepoHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/runs/{id}/repos", listRunReposHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/runs/{id}/repos/{repo_id}/restart", restartRunRepoHandler(st), auth.RoleControlPlane)
	// Repo-scoped diffs listing.
	s.HandleFunc("GET /v1/runs/{run_id}/repos/{repo_id}/diffs", listRunRepoDiffsHandler(st, bs), auth.RoleControlPlane, auth.RoleWorker)
	// Repo-scoped logs SSE stream (filtered view of GET /v1/runs/{id}/logs).
	s.HandleFunc("GET /v1/runs/{run_id}/repos/{repo_id}/logs", getRunRepoLogsHandler(st, eventsService), auth.RoleControlPlane)
	// Repo-scoped artifact listing.
	s.HandleFunc("GET /v1/runs/{run_id}/repos/{repo_id}/artifacts", listRunRepoArtifactsHandler(st), auth.RoleControlPlane)
	// Repo-scoped job listing for --follow mode.
	s.HandleFunc("GET /v1/runs/{run_id}/repos/{repo_id}/jobs", listRunRepoJobsHandler(st), auth.RoleControlPlane)
	// Repo-scoped cancel (replacement for DELETE /v1/runs/{id}/repos/{repo_id}).
	s.HandleFunc("POST /v1/runs/{run_id}/repos/{repo_id}/cancel", cancelRunRepoHandlerV1(st), auth.RoleControlPlane)
	// Pull resolution for run repos.
	s.HandleFunc("POST /v1/runs/{run_id}/pull", pullRunRepoHandler(st), auth.RoleControlPlane)

	// Repos — repo-centric view: list repos and show runs for a given repo.
	s.HandleFunc("GET /v1/repos", listReposHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/repos/{repo_id}/runs", listRunsForRepoHandler(st), auth.RoleControlPlane)

	// Runs (control plane) — legacy write/management endpoints
	s.HandleFunc("GET /v1/runs/{id}/timing", getRunTimingHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/runs/{id}/logs", createRunLogHandler(st, bp, eventsService), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/runs/{id}/diffs", createRunDiffHandler(st, bp), auth.RoleControlPlane)
	s.HandleFunc("DELETE /v1/runs/{id}", deleteRunHandler(st), auth.RoleControlPlane)

	// Node management endpoints
	s.HandleFunc("GET /v1/nodes", listNodesHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/nodes/{id}/drain", drainNodeHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/nodes/{id}/undrain", undrainNodeHandler(st), auth.RoleControlPlane)

	// Node worker endpoints
	s.HandleFunc("POST /v1/nodes/{id}/heartbeat", heartbeatHandler(st), auth.RoleWorker)
	// NOTE: The ack endpoint (/v1/nodes/{id}/ack) has been removed. Claim is the
	// canonical endpoint for pulling work from the unified jobs queue.
	s.HandleFunc("POST /v1/nodes/{id}/claim", claimJobHandler(st, configHolder, eventsService), auth.RoleWorker)
	// NOTE: Node-based completion endpoint (/v1/nodes/{id}/complete) has been removed.
	// Use the job-level endpoint POST /v1/jobs/{job_id}/complete instead.
	s.HandleFunc("POST /v1/nodes/{id}/events", createNodeEventsHandler(st, eventsService), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/logs", createNodeLogsHandler(st, bp, eventsService), auth.RoleWorker)

	// Job artifact and diff upload endpoints (run-scoped, no node ID)
	s.HandleFunc("POST /v1/runs/{run_id}/jobs/{job_id}/artifact", createJobArtifactHandler(st, bp), auth.RoleWorker)
	s.HandleFunc("POST /v1/runs/{run_id}/jobs/{job_id}/diff", createJobDiffHandler(st, bp), auth.RoleWorker)

	// Job-level completion endpoint — simplifies node → server contract by addressing jobs directly.
	// Node identity is derived from mTLS certificate; no node_id in URL or body.
	s.HandleFunc("POST /v1/jobs/{job_id}/complete", completeJobHandler(st, eventsService, bp), auth.RoleWorker)
	// Job-level runtime image persistence — nodes persist the resolved container image name
	// that will be used to execute a mig/heal job (stack-aware resolution).
	s.HandleFunc("POST /v1/jobs/{job_id}/image", saveJobImageNameHandler(st), auth.RoleWorker)

	// NOTE: HTTP Build Gate endpoints (POST /v1/buildgate/validate, GET /v1/buildgate/jobs/{id},
	// POST /v1/nodes/{id}/buildgate/*, etc.) have been removed. Gate execution now runs
	// as part of the unified jobs queue. See docs/build-gate/README.md.
}
