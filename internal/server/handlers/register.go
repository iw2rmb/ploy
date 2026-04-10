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
	var gateProfileResolver GateProfileResolver
	if _, ok := st.(*store.PgStore); ok && bs != nil {
		gateProfileResolver = NewDBGateProfileResolver(st, bs)
	}

	// Health
	s.RegisterRouteFunc("/health", healthHandler(st))

	// Config — GitLab
	s.RegisterRouteFunc("GET /v1/config/gitlab", getGitLabConfigHandler(configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("PUT /v1/config/gitlab", putGitLabConfigHandler(configHolder), auth.RoleCLIAdmin)

	// Config — Global Env (see docs/api/paths/config_env.yaml and docs/api/paths/config_env_key.yaml)
	s.RegisterRouteFunc("GET /v1/config/env", listGlobalEnvHandler(configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("GET /v1/config/env/{key}", getGlobalEnvHandler(configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("PUT /v1/config/env/{key}", putGlobalEnvHandler(configHolder, st), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("DELETE /v1/config/env/{key}", deleteGlobalEnvHandler(configHolder, st), auth.RoleCLIAdmin)

	// Config — Global CA
	s.RegisterRouteFunc("GET /v1/config/ca", listConfigCAHandler(configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("GET /v1/config/ca/{section}", listConfigCABySectionHandler(configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("PUT /v1/config/ca/{hash}", putConfigCAHandler(configHolder, st), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("DELETE /v1/config/ca/{hash}", deleteConfigCAHandler(configHolder, st), auth.RoleCLIAdmin)

	// Config — Global Home
	s.RegisterRouteFunc("GET /v1/config/home", listConfigHomeHandler(configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("GET /v1/config/home/{section}", listConfigHomeBySectionHandler(configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("PUT /v1/config/home", putConfigHomeHandler(configHolder, st), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("DELETE /v1/config/home", deleteConfigHomeHandler(configHolder, st), auth.RoleCLIAdmin)

	// Token management
	s.RegisterRouteFunc("POST /v1/tokens", createAPITokenHandler(st, tokenSecret), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("GET /v1/tokens", listAPITokensHandler(st), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("DELETE /v1/tokens/{id}", revokeAPITokenHandler(st), auth.RoleCLIAdmin)

	// Bootstrap tokens
	s.RegisterRouteFunc("POST /v1/bootstrap/tokens", createBootstrapTokenHandler(st, tokenSecret), auth.RoleControlPlane, auth.RoleCLIAdmin)
	s.RegisterRouteFunc("POST /v1/pki/bootstrap", bootstrapCertificateHandler(st, tokenSecret), auth.RoleWorker)

	// Runs — single-repo run submission (v1 API).
	// POST /v1/runs creates a single-repo run with automatic mig project creation.
	s.RegisterRouteFunc("POST /v1/runs", createSingleRepoRunHandler(st, eventsService), auth.RoleControlPlane)

	// Migs — mig project CRUD (v1 API).
	// /v1/migs endpoints handle mig project CRUD operations.
	s.RegisterRouteFunc("POST /v1/migs", createMigHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/migs", listMigsHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("DELETE /v1/migs/{mig_ref}", deleteMigHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("PATCH /v1/migs/{mig_ref}/archive", archiveMigHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("PATCH /v1/migs/{mig_ref}/unarchive", unarchiveMigHandler(st), auth.RoleControlPlane)
	// Set mig spec (append-only specs + migs.spec_id pointer).
	s.RegisterRouteFunc("POST /v1/migs/{mig_ref}/specs", setMigSpecHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFuncAllowQueryToken("GET /v1/migs/{mig_ref}/specs/latest", getMigLatestSpecHandler(st), auth.RoleControlPlane)
	// Mig repo set management (add/list/delete + bulk CSV upsert).
	s.RegisterRouteFunc("POST /v1/migs/{mig_id}/repos", addMigRepoHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/migs/{mig_id}/repos", listMigReposHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("DELETE /v1/migs/{mig_id}/repos/{repo_id}", deleteMigRepoHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/migs/{mig_id}/repos/bulk", bulkUpsertMigReposHandler(st), auth.RoleControlPlane)
	// Multi-repo run submission with repo selection (all/failed/explicit).
	s.RegisterRouteFunc("POST /v1/migs/{mig_id}/runs", createMigRunHandler(st), auth.RoleControlPlane)
	// Pull resolution for mig repos (last-succeeded/last-failed).
	s.RegisterRouteFunc("POST /v1/migs/{mig_id}/pull", pullMigRepoHandler(st), auth.RoleControlPlane)

	// Artifact download endpoints
	s.RegisterRouteFunc("GET /v1/artifacts", listArtifactsByCIDHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/artifacts/{id}", getArtifactHandler(st, bs), auth.RoleControlPlane, auth.RoleWorker)

	// Runs — batch lifecycle endpoints for listing, inspecting, cancelling, starting, and streaming logs/events.
	s.RegisterRouteFunc("GET /v1/runs", listRunsHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/runs/{id}", getRunHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/runs/{id}/status", getRunStatusHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFuncAllowQueryToken("GET /v1/runs/{id}/logs", getRunLogsHandler(st, bs, eventsService), auth.RoleControlPlane)
	// v1 API: POST /v1/runs/{id}/cancel — cancels the run, all repos (Queued/Running → Cancelled), and cancels/removes Created/Queued/Running jobs.
	s.RegisterRouteFunc("POST /v1/runs/{id}/cancel", cancelRunHandlerV1(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/runs/{id}/start", startRunHandler(st, bs), auth.RoleControlPlane)

	// RunRepo — manage repos within a batch (add/restart/list).
	s.RegisterRouteFunc("POST /v1/runs/{id}/repos", addRunRepoHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/runs/{id}/repos", listRunReposHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/runs/{id}/repos/{repo_id}/restart", restartRunRepoHandler(st, bs), auth.RoleControlPlane)
	// Repo-scoped diffs listing.
	s.RegisterRouteFuncAllowQueryToken("GET /v1/runs/{run_id}/repos/{repo_id}/diffs", listRunRepoDiffsHandler(st, bs), auth.RoleControlPlane, auth.RoleWorker)
	// Repo-scoped logs SSE stream (filtered view of GET /v1/runs/{id}/logs).
	s.RegisterRouteFuncAllowQueryToken("GET /v1/runs/{run_id}/repos/{repo_id}/logs", getRunRepoLogsHandler(st, bs, eventsService), auth.RoleControlPlane)
	// Repo-scoped artifact listing.
	s.RegisterRouteFunc("GET /v1/runs/{run_id}/repos/{repo_id}/artifacts", listRunRepoArtifactsHandler(st), auth.RoleControlPlane)
	// Repo-scoped job listing for --follow mode.
	s.RegisterRouteFunc("GET /v1/runs/{run_id}/repos/{repo_id}/jobs", listRunRepoJobsHandler(st), auth.RoleControlPlane)
	// Repo-scoped cancel (replacement for DELETE /v1/runs/{id}/repos/{repo_id}).
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/repos/{repo_id}/cancel", cancelRunRepoHandlerV1(st), auth.RoleControlPlane)
	// Manual MR-create action enqueue (idempotent per run/repo/attempt).
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/repos/{repo_id}/mr", createRunRepoMRActionHandler(st), auth.RoleControlPlane)
	// Pull resolution for run repos.
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/pull", pullRunRepoHandler(st), auth.RoleControlPlane)

	// Repos — repo-centric view: list repos and show runs for a given repo.
	s.RegisterRouteFunc("GET /v1/repos", listReposHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/repos/{repo_id}/runs", listRunsForRepoHandler(st), auth.RoleControlPlane)
	// SBOM compatibility hints for deps healing.
	s.RegisterRouteFunc("GET /v1/sboms/compat", sbomCompatHandler(st), auth.RoleControlPlane)

	// Runs (control plane) — legacy write/management endpoints
	s.RegisterRouteFunc("GET /v1/runs/{id}/timing", getRunTimingHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/runs/{id}/logs", createRunLogHandler(st, bp, eventsService), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/runs/{id}/diffs", createRunDiffHandler(st, bp), auth.RoleControlPlane)
	s.RegisterRouteFunc("DELETE /v1/runs/{id}", deleteRunHandler(st), auth.RoleControlPlane)

	// Node management endpoints
	s.RegisterRouteFunc("GET /v1/nodes", listNodesHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/nodes/{id}/drain", drainNodeHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/nodes/{id}/undrain", undrainNodeHandler(st), auth.RoleControlPlane)

	// Node worker endpoints
	s.RegisterRouteFunc("POST /v1/nodes/{id}/heartbeat", heartbeatHandler(st), auth.RoleWorker)
	// NOTE: The ack endpoint (/v1/nodes/{id}/ack) has been removed. Claim is the
	// canonical endpoint for pulling work from the unified jobs queue.
	s.RegisterRouteFunc("POST /v1/nodes/{id}/claim", claimJobHandlerWithEvents(st, bs, eventsService, configHolder, gateProfileResolver), auth.RoleWorker)
	// NOTE: Node-based completion endpoint (/v1/nodes/{id}/complete) has been removed.
	// Use the job-level endpoint POST /v1/jobs/{job_id}/complete instead.
	s.RegisterRouteFunc("POST /v1/nodes/{id}/events", createNodeEventsHandler(st, eventsService), auth.RoleWorker)
	s.RegisterRouteFunc("POST /v1/nodes/{id}/logs", createNodeLogsHandler(st, bp, eventsService), auth.RoleWorker)

	// Spec bundle upload/download/probe endpoints (CLI uploads; worker downloads during execution)
	s.RegisterRouteFunc("HEAD /v1/spec-bundles", probeSpecBundleHandler(st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/spec-bundles", uploadSpecBundleHandler(st, bp), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/spec-bundles/{id}", downloadSpecBundleHandler(st, bs), auth.RoleWorker, auth.RoleControlPlane)

	// Job artifact and diff upload endpoints (run-scoped, no node ID)
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/jobs/{job_id}/artifact", createJobArtifactHandler(st, bp), auth.RoleWorker)
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/jobs/{job_id}/diff", createJobDiffHandler(st, bp), auth.RoleWorker)

	// Jobs — global jobs listing for TUI (list across all runs with mig context).
	s.RegisterRouteFunc("GET /v1/jobs", listJobsHandler(st), auth.RoleControlPlane)

	// Job-level log endpoints — SSE stream and ingest scoped to a single job.
	s.RegisterRouteFuncAllowQueryToken("GET /v1/jobs/{job_id}/logs", getJobLogsHandler(st, bs, eventsService), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/jobs/{job_id}/logs", createJobLogsHandler(st, bp, eventsService), auth.RoleWorker)

	// Job-level completion endpoint — simplifies node → server contract by addressing jobs directly.
	// Node identity is derived from mTLS certificate; no node_id in URL or body.
	s.RegisterRouteFunc("POST /v1/jobs/{job_id}/complete", completeJobHandler(st, eventsService, bp, bs), auth.RoleWorker)
	// Action-level completion endpoint for worker-executed repo actions (e.g. MR create).
	s.RegisterRouteFunc("POST /v1/actions/{action_id}/complete", completeActionHandler(st), auth.RoleWorker)
	// Job-level status polling endpoint — allows workers to stop local execution
	// when control plane transitions a running job to Cancelled.
	s.RegisterRouteFunc("GET /v1/jobs/{job_id}/status", getJobStatusHandler(st), auth.RoleWorker)
	// Job-level runtime image persistence — nodes persist the resolved container image name
	// that will be used to execute a mig/heal job (stack-aware resolution).
	s.RegisterRouteFunc("POST /v1/jobs/{job_id}/image", saveJobImageNameHandler(st), auth.RoleWorker)

	// NOTE: HTTP Build Gate endpoints (POST /v1/buildgate/validate, GET /v1/buildgate/jobs/{id},
	// POST /v1/nodes/{id}/buildgate/*, etc.) have been removed. Gate execution now runs
	// as part of the unified jobs queue. See docs/build-gate/README.md.
}
