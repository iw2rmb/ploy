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

	// PKI
	s.HandleFunc("POST /v1/pki/sign", pkiSignHandler(st), auth.RoleCLIAdmin)
	s.HandleFunc("POST /v1/pki/sign/admin", pkiSignAdminHandler(), auth.RoleCLIAdmin)
	s.HandleFunc("POST /v1/pki/sign/client", pkiSignClientHandler(), auth.RoleCLIAdmin)

	// Mods ticket submission (new simplified API)
	s.HandleFunc("POST /v1/mods", submitTicketHandler(st, eventsService), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/mods/{id}/events", getModEventsHandler(st, eventsService), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/mods/{id}", getTicketStatusHandler(st), auth.RoleControlPlane)
	// Mods ticket cancellation
	s.HandleFunc("POST /v1/mods/{id}/cancel", cancelTicketHandler(st, eventsService), auth.RoleControlPlane)
	// Diffs listing and download
	s.HandleFunc("GET /v1/mods/{id}/diffs", listRunDiffsHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/diffs/{id}", getDiffHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/mods/{id}/artifact_bundles", createRunArtifactBundleHandler(st), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/mods/{id}/logs", createRunLogHandler(st, eventsService), auth.RoleControlPlane)
	s.HandleFunc("POST /v1/mods/{id}/diffs", createRunDiffHandler(st), auth.RoleControlPlane)

	// Artifact download endpoints
	s.HandleFunc("GET /v1/artifacts", listArtifactsByCIDHandler(st), auth.RoleControlPlane)
	s.HandleFunc("GET /v1/artifacts/{id}", getArtifactHandler(st), auth.RoleControlPlane)

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
	s.HandleFunc("POST /v1/nodes/{id}/claim", claimRunHandler(st, configHolder), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/ack", ackRunStartHandler(st, eventsService), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/complete", completeRunHandler(st, eventsService), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/events", createNodeEventsHandler(st, eventsService), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/logs", createNodeLogsHandler(st, eventsService), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/stage/{stage}/diff", createDiffHandler(st), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/stage/{stage}/artifact", createArtifactBundleHandler(st), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/buildgate/claim", claimBuildGateJobHandler(st), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/buildgate/{job_id}/ack", ackBuildGateJobStartHandler(st), auth.RoleWorker)
	s.HandleFunc("POST /v1/nodes/{id}/buildgate/{job_id}/complete", completeBuildGateJobHandler(st), auth.RoleWorker)

	// Build gate endpoints
	// Allow both control-plane and worker roles so healing containers running on nodes
	// can submit validation requests using the node's client certificate.
	s.HandleFunc("POST /v1/buildgate/validate", validateBuildGateHandler(st), auth.RoleControlPlane, auth.RoleWorker)
	s.HandleFunc("GET /v1/buildgate/jobs/{id}", getBuildGateJobStatusHandler(st), auth.RoleControlPlane, auth.RoleWorker)
}
