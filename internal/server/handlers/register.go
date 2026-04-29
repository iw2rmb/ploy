package handlers

import (
	"github.com/iw2rmb/ploy/internal/blobstore"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

type routeDeps struct {
	st                  store.Store
	bs                  blobstore.Store
	bp                  *blobpersist.Service
	eventsService       *server.EventsService
	configHolder        *ConfigHolder
	tokenSecret         string
	gateProfileResolver GateProfileResolver
}

// RegisterRoutes mounts all HTTP endpoints on the given server.
func RegisterRoutes(s *server.HTTPServer, st store.Store, bs blobstore.Store, bp *blobpersist.Service, eventsService *server.EventsService, configHolder *ConfigHolder, tokenSecret string) {
	deps := routeDeps{
		st:            st,
		bs:            bs,
		bp:            bp,
		eventsService: eventsService,
		configHolder:  configHolder,
		tokenSecret:   tokenSecret,
	}
	if _, ok := st.(*store.PgStore); ok && bs != nil {
		deps.gateProfileResolver = NewDBGateProfileResolver(st, bs)
	}

	registerHealthRoutes(s, deps)
	registerConfigRoutes(s, deps)
	registerTokenRoutes(s, deps)
	registerBootstrapRoutes(s, deps)
	registerMigRoutes(s, deps)
	registerArtifactReadRoutes(s, deps)
	registerRunRoutes(s, deps)
	registerRepoRoutes(s, deps)
	registerNodeRoutes(s, deps)
	registerSpecBundleRoutes(s, deps)
	registerJobArtifactRoutes(s, deps)
	registerJobRoutes(s, deps)
}

func registerHealthRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("/health", healthHandler(deps.st))
}

func registerConfigRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("GET /v1/config/gitlab", getGitLabConfigHandler(deps.configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("PUT /v1/config/gitlab", putGitLabConfigHandler(deps.configHolder), auth.RoleCLIAdmin)

	s.RegisterRouteFunc("GET /v1/config/env", listGlobalEnvHandler(deps.configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("GET /v1/config/env/{key}", getGlobalEnvHandler(deps.configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("PUT /v1/config/env/{key}", putGlobalEnvHandler(deps.configHolder, deps.st), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("DELETE /v1/config/env/{key}", deleteGlobalEnvHandler(deps.configHolder, deps.st), auth.RoleCLIAdmin)

	s.RegisterRouteFunc("GET /v1/config/ca", listConfigCAHandler(deps.configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("GET /v1/config/ca/{section}", listConfigCABySectionHandler(deps.configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("PUT /v1/config/ca/{hash}", putConfigCAHandler(deps.configHolder, deps.st), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("DELETE /v1/config/ca/{hash}", deleteConfigCAHandler(deps.configHolder, deps.st), auth.RoleCLIAdmin)

	s.RegisterRouteFunc("GET /v1/config/home", listConfigHomeHandler(deps.configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("GET /v1/config/home/{section}", listConfigHomeBySectionHandler(deps.configHolder), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("PUT /v1/config/home", putConfigHomeHandler(deps.configHolder, deps.st), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("DELETE /v1/config/home", deleteConfigHomeHandler(deps.configHolder, deps.st), auth.RoleCLIAdmin)
}

func registerTokenRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("POST /v1/tokens", createAPITokenHandler(deps.st, deps.tokenSecret), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("GET /v1/tokens", listAPITokensHandler(deps.st), auth.RoleCLIAdmin)
	s.RegisterRouteFunc("DELETE /v1/tokens/{id}", revokeAPITokenHandler(deps.st), auth.RoleCLIAdmin)
}

func registerBootstrapRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("POST /v1/bootstrap/tokens", createBootstrapTokenHandler(deps.st, deps.tokenSecret), auth.RoleControlPlane, auth.RoleCLIAdmin)
	s.RegisterRouteFunc("POST /v1/pki/bootstrap", bootstrapCertificateHandler(deps.st, deps.tokenSecret), auth.RoleWorker)
}

func registerMigRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("POST /v1/runs", createSingleRepoRunHandler(deps.st, deps.eventsService), auth.RoleControlPlane)

	s.RegisterRouteFunc("POST /v1/migs", createMigHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/migs", listMigsHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("DELETE /v1/migs/{mig_ref}", deleteMigHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("PATCH /v1/migs/{mig_ref}/archive", archiveMigHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("PATCH /v1/migs/{mig_ref}/unarchive", unarchiveMigHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/migs/{mig_ref}/specs", setMigSpecHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFuncAllowQueryToken("GET /v1/migs/{mig_ref}/specs/latest", getMigLatestSpecHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/migs/{mig_id}/repos", addMigRepoHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/migs/{mig_id}/repos", listMigReposHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("DELETE /v1/migs/{mig_id}/repos/{repo_id}", deleteMigRepoHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/migs/{mig_id}/repos/bulk", bulkUpsertMigReposHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/migs/{mig_id}/runs", createMigRunHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/migs/{mig_id}/pull", pullMigRepoHandler(deps.st), auth.RoleControlPlane)
}

func registerArtifactReadRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("GET /v1/artifacts", listArtifactsByCIDHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/artifacts/{id}", getArtifactHandler(deps.st, deps.bs), auth.RoleControlPlane, auth.RoleWorker)
}

func registerRunRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("GET /v1/runs", listRunsHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/runs/{run_id}", getRunHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/runs/{run_id}/status", getRunStatusHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFuncAllowQueryToken("GET /v1/runs/{run_id}/logs", getRunLogsHandler(deps.st, deps.bs, deps.eventsService), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/cancel", cancelRunHandlerV1(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/start", startRunHandler(deps.st, deps.bs), auth.RoleControlPlane)

	s.RegisterRouteFunc("POST /v1/runs/{run_id}/repos", addRunRepoHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/runs/{run_id}/repos", listRunReposHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/repos/{repo_id}/restart", restartRunRepoHandler(deps.st, deps.bs), auth.RoleControlPlane)
	s.RegisterRouteFuncAllowQueryToken("GET /v1/runs/{run_id}/repos/{repo_id}/diffs", listRunRepoDiffsHandler(deps.st, deps.bs), auth.RoleControlPlane, auth.RoleWorker)
	s.RegisterRouteFuncAllowQueryToken("GET /v1/runs/{run_id}/repos/{repo_id}/logs", getRunRepoLogsHandler(deps.st, deps.bs, deps.eventsService), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/runs/{run_id}/repos/{repo_id}/artifacts", listRunRepoArtifactsHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/runs/{run_id}/repos/{repo_id}/jobs", listRunRepoJobsHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/repos/{repo_id}/cancel", cancelRunRepoHandlerV1(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/repos/{repo_id}/mr", createRunRepoMRActionHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/pull", pullRunRepoHandler(deps.st), auth.RoleControlPlane)
}

func registerRepoRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("GET /v1/repos", listReposHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/repos/{repo_id}/runs", listRunsForRepoHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/sboms/compat", sbomCompatHandler(deps.st), auth.RoleControlPlane)
}

func registerNodeRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("GET /v1/nodes", listNodesHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/nodes/{id}/drain", drainNodeHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/nodes/{id}/undrain", undrainNodeHandler(deps.st), auth.RoleControlPlane)

	s.RegisterRouteFunc("POST /v1/nodes/{id}/heartbeat", heartbeatHandler(deps.st), auth.RoleWorker)
	s.RegisterRouteFunc("POST /v1/nodes/{id}/claim", claimJobHandlerWithEvents(deps.st, deps.bs, deps.eventsService, deps.configHolder, deps.gateProfileResolver), auth.RoleWorker)
	s.RegisterRouteFunc("POST /v1/nodes/{id}/events", createNodeEventsHandler(deps.st, deps.eventsService), auth.RoleWorker)
	s.RegisterRouteFunc("POST /v1/nodes/{id}/logs", createNodeLogsHandler(deps.st, deps.bp, deps.eventsService), auth.RoleWorker)
}

func registerSpecBundleRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("HEAD /v1/spec-bundles", probeSpecBundleHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/spec-bundles", uploadSpecBundleHandler(deps.st, deps.bp), auth.RoleControlPlane)
	s.RegisterRouteFunc("GET /v1/spec-bundles/{id}", downloadSpecBundleHandler(deps.st, deps.bs), auth.RoleWorker, auth.RoleControlPlane)
}

func registerJobArtifactRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/jobs/{job_id}/artifact", createJobArtifactHandler(deps.st, deps.bp), auth.RoleWorker)
	s.RegisterRouteFunc("POST /v1/runs/{run_id}/jobs/{job_id}/diff", createJobDiffHandler(deps.st, deps.bp), auth.RoleWorker)
}

func registerJobRoutes(s *server.HTTPServer, deps routeDeps) {
	s.RegisterRouteFunc("GET /v1/jobs", listJobsHandler(deps.st), auth.RoleControlPlane)
	s.RegisterRouteFuncAllowQueryToken("GET /v1/jobs/{job_id}/logs", getJobLogsHandler(deps.st, deps.bs, deps.eventsService), auth.RoleControlPlane)
	s.RegisterRouteFunc("POST /v1/jobs/{job_id}/logs", createJobLogsHandler(deps.st, deps.bp, deps.eventsService), auth.RoleWorker)
	s.RegisterRouteFunc("POST /v1/jobs/{job_id}/complete", completeJobHandler(deps.st, deps.eventsService, deps.bp, deps.bs), auth.RoleWorker)
	s.RegisterRouteFunc("POST /v1/actions/{action_id}/complete", completeActionHandler(deps.st), auth.RoleWorker)
	s.RegisterRouteFunc("GET /v1/jobs/{job_id}/status", getJobStatusHandler(deps.st), auth.RoleWorker)
	s.RegisterRouteFunc("POST /v1/jobs/{job_id}/image", saveJobImageNameHandler(deps.st), auth.RoleWorker)
}
