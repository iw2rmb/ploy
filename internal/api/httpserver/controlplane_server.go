package httpserver

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	httpsecurity "github.com/iw2rmb/ploy/internal/api/httpserver/security"
	"github.com/iw2rmb/ploy/internal/config/gitlab"
	controlplaneartifacts "github.com/iw2rmb/ploy/internal/controlplane/artifacts"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
	"github.com/iw2rmb/ploy/internal/controlplane/config"
	"github.com/iw2rmb/ploy/internal/controlplane/events"
	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/store"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	artifactRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ploy_artifact_http_requests_total",
		Help: "Count of control-plane artifact API requests.",
	}, []string{"method", "status"})
	artifactPayloadBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ploy_artifact_payload_bytes_total",
		Help: "Bytes processed by artifact API payloads.",
	}, []string{"operation"})
	configRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ploy_config_http_requests_total",
		Help: "Count of control-plane config API requests.",
	}, []string{"method", "status"})
	configUpdatesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ploy_config_updates_total",
		Help: "Count of persisted configuration updates.",
	}, []string{"cluster"})
	beaconRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ploy_beacon_http_requests_total",
		Help: "Count of beacon discovery API requests.",
	}, []string{"resource", "method", "status"})
)

func init() {
	prometheus.MustRegister(artifactRequestsTotal, artifactPayloadBytes)
	prometheus.MustRegister(configRequestsTotal, configUpdatesTotal, beaconRequestsTotal)
}

// Server exposes the control-plane scheduler over HTTP.
type controlPlaneServer struct {
	scheduler         *scheduler.Scheduler
	signer            *gitlab.Signer
	rotations         *events.RotationHub
	streams           *logstream.Hub
	etcd              *clientv3.Client
	mods              *controlplanemods.Service
	auth              *httpsecurity.Manager
	roles             *auth.Authorizer
	gatherer          prometheus.Gatherer
	cfgStore          *config.Store
	transfers         *transfers.Manager
	artifacts         *controlplaneartifacts.Store
	artifactPublisher artifactPublisher
	store             store.Store
}

type artifactPublisher interface {
	Add(ctx context.Context, req workflowartifacts.AddRequest) (workflowartifacts.AddResponse, error)
	Fetch(ctx context.Context, cid string) (workflowartifacts.FetchResult, error)
}

// ControlPlaneOptions configure the HTTP server handlers for the control plane.
type ControlPlaneOptions struct {
	Scheduler         *scheduler.Scheduler
	Signer            *gitlab.Signer
	Streams           *logstream.Hub
	Gatherer          prometheus.Gatherer
	Etcd              *clientv3.Client
	ClusterID         string
	Rotations         *events.RotationHub
	Mods              *controlplanemods.Service
	Auth              *httpsecurity.Manager
	AuthVerifier      httpsecurity.TokenVerifier
	Authorizer        *auth.Authorizer
	Transfers         *transfers.Manager
	ArtifactStore     *controlplaneartifacts.Store
	ArtifactPublisher artifactPublisher
	Store             store.Store
}

// NewControlPlaneHandler returns an HTTP handler rooted at /v1 for control-plane routes.
func NewControlPlaneHandler(opts ControlPlaneOptions) http.Handler {
	mux := http.NewServeMux()

	var authManager *httpsecurity.Manager
	switch {
	case opts.Auth != nil:
		authManager = opts.Auth
	case opts.AuthVerifier != nil:
		authManager = httpsecurity.NewManager(opts.AuthVerifier)
	}

	gatherer := opts.Gatherer
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}

	roleManager := opts.Authorizer
	if roleManager == nil {
		roleManager = auth.NewAuthorizer(auth.Options{})
	}

	var cfgStore *config.Store
	if opts.Etcd != nil {
		if store, err := config.NewStore(opts.Etcd); err == nil {
			cfgStore = store
		}
	}

	var artStore *controlplaneartifacts.Store
	if opts.ArtifactStore != nil {
		artStore = opts.ArtifactStore
	} else if opts.Etcd != nil {
		if store, err := controlplaneartifacts.NewStore(opts.Etcd, controlplaneartifacts.StoreOptions{}); err == nil {
			artStore = store
		}
	}

	h := &controlPlaneServer{
		scheduler:         opts.Scheduler,
		signer:            opts.Signer,
		streams:           opts.Streams,
		etcd:              opts.Etcd,
		rotations:         opts.Rotations,
		mods:              opts.Mods,
		auth:              authManager,
		roles:             roleManager,
		gatherer:          gatherer,
		cfgStore:          cfgStore,
		transfers:         opts.Transfers,
		artifacts:         artStore,
		artifactPublisher: opts.ArtifactPublisher,
		store:             opts.Store,
	}
	if h.rotations == nil && opts.Signer != nil {
		h.rotations = events.NewRotationHub(context.Background(), opts.Signer)
	}

	var slotStore *transfers.SlotStore
	if opts.Transfers == nil && opts.Etcd != nil {
		store, err := transfers.NewSlotStore(opts.Etcd, transfers.SlotStoreOptions{
			ClusterID: opts.ClusterID,
		})
		if err != nil {
			log.Printf("control-plane: slot store disabled: %v", err)
		} else {
			slotStore = store
		}
	}

	if h.transfers == nil {
		h.transfers = transfers.NewManager(transfers.Options{
			SlotStore: slotStore,
			Store:     artStore,
			Publisher: opts.ArtifactPublisher,
		})
	}
	h.registerRoute(mux, "", "/v1/jobs", h.handleJobs)
	h.registerRoute(mux, http.MethodPost, "/v1/jobs/claim", h.handleClaim)
	h.registerRoute(mux, "", "/v1/jobs/", h.handleJobSubpath)
	h.registerRoute(mux, http.MethodGet, "/v1/health", h.handleHealth)
	h.registerRoute(mux, http.MethodPut, "/v1/gitlab/signer/secrets", h.handleSignerSecrets, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodPost, "/v1/gitlab/signer/tokens", h.handleSignerTokens, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodGet, "/v1/gitlab/signer/rotations", h.handleSignerRotations, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, "", "/v1/nodes", h.handleNodes)
	// Subpath handler for node-specific actions (e.g., PATCH /v1/nodes/{id}).
	h.registerRoute(mux, "", "/v1/nodes/", h.handleNodeSubpath)
	h.registerRoute(mux, "", "/v1/config/gitlab", h.handleGitLabConfig, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, "", "/v1/config", h.handleClusterConfig, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodGet, "/v1/status", h.handleStatusSummary, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodGet, "/v1/security/ca", h.handleSecurityCA, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodPost, "/v1/security/certificates/control-plane", h.handleControlPlaneCertificate, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodPost, "/v1/pki/sign", h.handlePKISign, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodGet, "/v1/version", h.handleVersion)
	if h.mods != nil {
		h.registerRoute(mux, http.MethodPost, "/v1/mods", h.handleModsSubmit, httpsecurity.ScopeMods)
		h.registerRoute(mux, "", "/v1/mods/", h.handleModsSubpath, httpsecurity.ScopeMods)
		h.registerRoute(mux, http.MethodPost, "/v1/mods/tickets", h.handleModsTickets, httpsecurity.ScopeMods)
		h.registerRoute(mux, "", "/v1/mods/tickets/", h.handleModsTicketSubpath, httpsecurity.ScopeMods)
	}
	// Core control-plane CRUD endpoints for repos, mods, runs
	if h.store != nil {
		h.registerRoute(mux, "", "/v1/repos", h.handleRepos)
		h.registerRoute(mux, "", "/v1/repos/", h.handleReposSubpath)
		h.registerRoute(mux, "", "/v1/mods/crud", h.handleModsCRUD)
		h.registerRoute(mux, "", "/v1/mods/crud/", h.handleModsCRUDSubpath)
		h.registerRoute(mux, "", "/v1/runs", h.handleRuns)
		h.registerRoute(mux, "", "/v1/runs/", h.handleRunsSubpath)
	}
	h.registerRoute(mux, http.MethodPost, "/v1/artifacts/upload", h.handleArtifactsUpload, httpsecurity.ScopeArtifactsWrite)
	h.registerRoute(mux, http.MethodGet, "/v1/artifacts", h.handleArtifactsList, httpsecurity.ScopeArtifactsRead)
	h.registerRoute(mux, "", "/v1/artifacts/", h.handleArtifactsSubpath)
	// v2 artifacts (HTTPS direct uploads): alias v2 to v1 handlers during migration.
	h.registerRoute(mux, http.MethodPost, "/v2/artifacts/upload", h.handleArtifactsUpload, httpsecurity.ScopeArtifactsWrite)
	h.registerRoute(mux, http.MethodGet, "/v2/artifacts", h.handleArtifactsList, httpsecurity.ScopeArtifactsRead)
	h.registerRoute(mux, "", "/v2/artifacts/", h.handleArtifactsSubpath)
	h.registerRoute(mux, http.MethodPost, "/v1/transfers/upload", h.handleTransfersUpload, httpsecurity.ScopeArtifactsWrite)
	h.registerRoute(mux, http.MethodPost, "/v1/transfers/download", h.handleTransfersDownload, httpsecurity.ScopeArtifactsRead)
	h.registerRoute(mux, http.MethodPost, "/v1/transfers/", h.handleTransfersSlotAction, httpsecurity.ScopeArtifactsWrite)
	mux.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	return mux
}

func (s *controlPlaneServer) registerRoute(mux *http.ServeMux, method, path string, handler http.HandlerFunc, scopes ...string) {
	var final http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if method != "" && r.Method != method {
			w.Header().Set("Allow", method)
			writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		handler(w, r)
	})
	if s.roles != nil {
		if roles := s.routeRoles(path); len(roles) > 0 {
			final = s.roles.Middleware(roles...)(final)
		}
	}
	if s.auth != nil {
		final = s.auth.Middleware(scopes...)(final)
	}
	mux.Handle(path, final)
}

func (s *controlPlaneServer) routeRoles(path string) []string {
	switch path {
	case "/v1/nodes":
		return []string{auth.RoleControlPlane, auth.RoleCLIAdmin}
	case "/v1/nodes/":
		return []string{auth.RoleControlPlane, auth.RoleCLIAdmin}
	case "/v1/status":
		return []string{auth.RoleControlPlane, auth.RoleCLIAdmin, auth.RoleWorker}
	case "/v1/security/ca":
		return []string{auth.RoleControlPlane, auth.RoleCLIAdmin}
	default:
		return nil
	}
}

func (s *controlPlaneServer) ensureEtcd(w http.ResponseWriter) bool {
	if s.etcd == nil {
		http.Error(w, "etcd unavailable", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (s *controlPlaneServer) ensureMods(w http.ResponseWriter) bool {
	if s.mods == nil {
		http.Error(w, "mods orchestrator unavailable", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (s *controlPlaneServer) requireScope(w http.ResponseWriter, r *http.Request, scope string) bool {
	if s.auth == nil {
		return true
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return true
	}
	principal, ok := s.principal(r)
	if !ok {
		writeErrorMessage(w, http.StatusForbidden, "principal missing")
		return false
	}
	if !principal.HasScope(scope) {
		writeErrorMessage(w, http.StatusForbidden, "insufficient scope")
		return false
	}
	return true
}

func (s *controlPlaneServer) principal(r *http.Request) (httpsecurity.Principal, bool) {
	return httpsecurity.PrincipalFromContext(r.Context())
}

func (s *controlPlaneServer) configStore() (*config.Store, error) {
	if s.cfgStore != nil {
		return s.cfgStore, nil
	}
	if s.etcd == nil {
		return nil, errors.New("config: etcd unavailable")
	}
	store, err := config.NewStore(s.etcd)
	if err != nil {
		return nil, err
	}
	s.cfgStore = store
	return store, nil
}

func (s *controlPlaneServer) ensureScheduler(w http.ResponseWriter) bool {
	if s.scheduler == nil {
		http.Error(w, "scheduler unavailable", http.StatusServiceUnavailable)
		return false
	}
	return true
}
