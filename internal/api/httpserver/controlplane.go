package httpserver

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"

	gonanoid "github.com/matoous/go-nanoid/v2"

	httpsecurity "github.com/iw2rmb/ploy/internal/api/httpserver/security"
	"github.com/iw2rmb/ploy/internal/config/gitlab"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
	"github.com/iw2rmb/ploy/internal/controlplane/config"
	"github.com/iw2rmb/ploy/internal/controlplane/events"
	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	cpsecurity "github.com/iw2rmb/ploy/internal/controlplane/security"
	"github.com/iw2rmb/ploy/internal/deploy"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/version"
)

const (
	errorCodeArtifactUpload   = "ARTIFACT_UPLOAD_UNIMPLEMENTED"
	errorCodeArtifactDelete   = "ARTIFACT_DELETE_UNIMPLEMENTED"
	errorCodeRegistryUpload   = "REGISTRY_UPLOAD_UNIMPLEMENTED"
	errorCodeRegistryManifest = "REGISTRY_MANIFEST_UNIMPLEMENTED"
	errorCodeRegistryBlob     = "REGISTRY_BLOB_UNIMPLEMENTED"
	errorCodeRegistryTags     = "REGISTRY_TAGS_UNIMPLEMENTED"
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
	registryRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ploy_registry_http_requests_total",
		Help: "Count of control-plane registry API requests.",
	}, []string{"resource", "method", "status"})
	registryPayloadBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ploy_registry_payload_bytes_total",
		Help: "Bytes processed by registry API payloads.",
	}, []string{"resource", "operation"})
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
	prometheus.MustRegister(artifactRequestsTotal, artifactPayloadBytes, registryRequestsTotal, registryPayloadBytes)
	prometheus.MustRegister(configRequestsTotal, configUpdatesTotal, beaconRequestsTotal)
}

// Server exposes the control-plane scheduler over HTTP.
type controlPlaneServer struct {
	scheduler *scheduler.Scheduler
	signer    *gitlab.Signer
	rotations *events.RotationHub
	streams   *logstream.Hub
	etcd      *clientv3.Client
	mods      *controlplanemods.Service
	auth      *httpsecurity.Manager
	roles     *auth.Authorizer
	gatherer  prometheus.Gatherer
	cfgStore  *config.Store
}

// Options configure the HTTP server handlers.
type ControlPlaneOptions struct {
	Scheduler    *scheduler.Scheduler
	Signer       *gitlab.Signer
	Streams      *logstream.Hub
	Gatherer     prometheus.Gatherer
	Etcd         *clientv3.Client
	Rotations    *events.RotationHub
	Mods         *controlplanemods.Service
	Auth         *httpsecurity.Manager
	AuthVerifier httpsecurity.TokenVerifier
	Authorizer   *auth.Authorizer
}

// New returns an HTTP handler rooted at /v1.
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

	h := &controlPlaneServer{
		scheduler: opts.Scheduler,
		signer:    opts.Signer,
		streams:   opts.Streams,
		etcd:      opts.Etcd,
		rotations: opts.Rotations,
		mods:      opts.Mods,
		auth:      authManager,
		roles:     roleManager,
		gatherer:  gatherer,
		cfgStore:  cfgStore,
	}
	if h.rotations == nil && opts.Signer != nil {
		h.rotations = events.NewRotationHub(context.Background(), opts.Signer)
	}
	h.registerRoute(mux, "", "/v1/jobs", h.handleJobs)
	h.registerRoute(mux, http.MethodPost, "/v1/jobs/claim", h.handleClaim)
	h.registerRoute(mux, "", "/v1/jobs/", h.handleJobSubpath)
	h.registerRoute(mux, http.MethodGet, "/v1/health", h.handleHealth)
	h.registerRoute(mux, http.MethodPut, "/v1/gitlab/signer/secrets", h.handleSignerSecrets, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodPost, "/v1/gitlab/signer/tokens", h.handleSignerTokens, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodGet, "/v1/gitlab/signer/rotations", h.handleSignerRotations, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, "", "/v1/nodes", h.handleNodes)
	h.registerRoute(mux, "", "/v1/config/gitlab", h.handleGitLabConfig, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, "", "/v1/config", h.handleClusterConfig, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodGet, "/v1/status", h.handleStatusSummary, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodGet, "/v1/security/ca", h.handleSecurityCA, httpsecurity.ScopeAdmin)
	h.registerRoute(mux, http.MethodGet, "/v1/version", h.handleVersion)
	if h.mods != nil {
		h.registerRoute(mux, http.MethodPost, "/v1/mods", h.handleModsSubmit, httpsecurity.ScopeMods)
		h.registerRoute(mux, "", "/v1/mods/", h.handleModsSubpath, httpsecurity.ScopeMods)
		h.registerRoute(mux, http.MethodPost, "/v1/mods/tickets", h.handleModsTickets, httpsecurity.ScopeMods)
		h.registerRoute(mux, "", "/v1/mods/tickets/", h.handleModsTicketSubpath, httpsecurity.ScopeMods)
	}
	h.registerRoute(mux, http.MethodPost, "/v1/artifacts/upload", h.handleArtifactsUpload, httpsecurity.ScopeArtifactsWrite)
	h.registerRoute(mux, http.MethodGet, "/v1/artifacts", h.handleArtifactsList, httpsecurity.ScopeArtifactsRead)
	h.registerRoute(mux, "", "/v1/artifacts/", h.handleArtifactsSubpath)
	h.registerRoute(mux, "", "/v1/registry/", h.handleRegistry)
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
	case "/v1/status":
		return []string{auth.RoleControlPlane, auth.RoleCLIAdmin, auth.RoleWorker}
	case "/v1/security/ca":
		return []string{auth.RoleControlPlane, auth.RoleCLIAdmin}
	default:
		return nil
	}
}

func (s *controlPlaneServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *controlPlaneServer) handleStatusSummary(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	status := http.StatusOK

	clusterID := strings.TrimSpace(r.URL.Query().Get("cluster_id"))
	if clusterID == "" {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "cluster_id query parameter required")
		return
	}

	queueDepths := s.collectQueueDepths()
	totalDepth := 0.0
	priorities := make([]map[string]any, 0, len(queueDepths))
	for priority, depth := range queueDepths {
		priorities = append(priorities, map[string]any{
			"priority": priority,
			"depth":    depth,
		})
		totalDepth += depth
	}
	sort.Slice(priorities, func(i, j int) bool {
		pi := fmt.Sprintf("%v", priorities[i]["priority"])
		pj := fmt.Sprintf("%v", priorities[j]["priority"])
		return pi < pj
	})

	workerStats := map[string]any{
		"total": 0,
		"phases": map[string]int{
			"ready":       0,
			"registering": 0,
			"error":       0,
			"unknown":     0,
		},
	}

	reg, err := registry.NewWorkerRegistry(s.etcd, clusterID)
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}
	descriptors, err := reg.List(r.Context())
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}

	phases := workerStats["phases"].(map[string]int)
	for _, descriptor := range descriptors {
		phase := strings.TrimSpace(descriptor.Status.Phase)
		if phase == "" {
			phase = "unknown"
		}
		phases[phase]++
	}
	workerStats["total"] = len(descriptors)

	payload := map[string]any{
		"cluster_id": clusterID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"queue": map[string]any{
			"total_depth": totalDepth,
			"priorities":  priorities,
		},
		"workers": workerStats,
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, payload)
}

func (s *controlPlaneServer) handleSecurityCA(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	clusterID := strings.TrimSpace(r.URL.Query().Get("cluster_id"))
	if clusterID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "cluster_id query parameter required")
		return
	}
	manager, err := deploy.NewCARotationManager(s.etcd, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	state, err := manager.State(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, deploy.ErrPKINotBootstrapped):
			writeErrorMessage(w, http.StatusNotFound, "cluster PKI not bootstrapped")
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	trustHash := ""
	if store, err := cpsecurity.NewTrustStore(s.etcd, clusterID); err == nil {
		if bundle, _, err := store.Current(r.Context()); err == nil {
			trustHash = bundle.CABundleHash
		}
	}
	current := map[string]any{
		"version":       state.CurrentCA.Version,
		"serial_number": state.CurrentCA.SerialNumber,
	}
	if !state.CurrentCA.IssuedAt.IsZero() {
		current["issued_at"] = state.CurrentCA.IssuedAt.UTC().Format(time.RFC3339Nano)
	}
	if !state.CurrentCA.ExpiresAt.IsZero() {
		current["expires_at"] = state.CurrentCA.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	response := map[string]any{
		"cluster_id": clusterID,
		"current_ca": current,
		"workers": map[string]any{
			"total": len(state.Nodes.Workers),
		},
	}
	if len(state.Nodes.Beacons) > 0 {
		response["control_plane"] = map[string]any{
			"total": len(state.Nodes.Beacons),
		}
	}
	if trustHash != "" {
		response["trust_bundle_hash"] = trustHash
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *controlPlaneServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	payload := map[string]any{
		"version":  strings.TrimSpace(version.Version),
		"commit":   strings.TrimSpace(version.Commit),
		"built_at": strings.TrimSpace(version.BuiltAt),
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, payload)
}

func (s *controlPlaneServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleNodeJoin(w, r)
	case http.MethodGet:
		s.handleNodeList(w, r)
	case http.MethodDelete:
		s.handleNodeDelete(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *controlPlaneServer) handleClusterConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleConfigGet(w, r)
	case http.MethodPut:
		s.handleConfigPut(w, r)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *controlPlaneServer) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	status := http.StatusOK
	method := http.MethodGet
	defer func() {
		recordConfigRequest(method, status)
	}()

	clusterID := strings.TrimSpace(r.URL.Query().Get("cluster_id"))
	if clusterID == "" {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "cluster_id query parameter required")
		return
	}

	store, err := s.configStore()
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}

	doc, revision, err := store.Load(r.Context(), clusterID)
	if err != nil {
		switch {
		case errors.Is(err, config.ErrNotFound):
			status = http.StatusNotFound
			w.Header().Set("Cache-Control", "no-store")
			writeErrorMessage(w, status, "configuration not found")
		default:
			status = http.StatusInternalServerError
			writeError(w, status, err)
		}
		return
	}

	response := map[string]any{
		"cluster_id": clusterID,
		"config":     cloneAnyMap(doc.Data),
		"revision":   revision,
	}
	if strings.TrimSpace(doc.VersionTag) != "" {
		response["version_tag"] = doc.VersionTag
	}
	if !doc.UpdatedAt.IsZero() {
		response["updated_at"] = doc.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(doc.UpdatedBy) != "" {
		response["updated_by"] = doc.UpdatedBy
	}

	w.Header().Set("ETag", strconv.FormatInt(revision, 10))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, response)
}

func (s *controlPlaneServer) handleConfigPut(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	status := http.StatusOK
	method := http.MethodPut
	defer func() {
		recordConfigRequest(method, status)
	}()

	var req struct {
		ClusterID  string         `json:"cluster_id"`
		Config     map[string]any `json:"config"`
		VersionTag string         `json:"version_tag"`
		UpdatedBy  string         `json:"updated_by"`
	}
	if err := decodeJSON(r, &req); err != nil {
		status = http.StatusBadRequest
		writeError(w, status, err)
		return
	}

	clusterID := strings.TrimSpace(req.ClusterID)
	if clusterID == "" {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "cluster_id is required")
		return
	}

	rawMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	if rawMatch == "" {
		status = http.StatusPreconditionRequired
		writeErrorMessage(w, status, "If-Match header required")
		return
	}
	expectedRevision, err := parseRevisionHeader(rawMatch)
	if err != nil {
		status = http.StatusBadRequest
		writeError(w, status, err)
		return
	}

	store, err := s.configStore()
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}

	doc := config.Document{
		Data:       cloneAnyMap(req.Config),
		VersionTag: strings.TrimSpace(req.VersionTag),
		UpdatedBy:  strings.TrimSpace(req.UpdatedBy),
		UpdatedAt:  time.Now().UTC(),
	}

	saved, revision, err := store.Save(r.Context(), clusterID, expectedRevision, doc)
	if err != nil {
		switch {
		case errors.Is(err, config.ErrConflict):
			status = http.StatusPreconditionFailed
			writeErrorMessage(w, status, "revision mismatch")
		case errors.Is(err, config.ErrNotFound):
			status = http.StatusNotFound
			writeErrorMessage(w, status, "configuration not found")
		default:
			status = http.StatusInternalServerError
			writeError(w, status, err)
		}
		return
	}

	response := map[string]any{
		"cluster_id": clusterID,
		"config":     cloneAnyMap(saved.Data),
		"revision":   revision,
	}
	if strings.TrimSpace(saved.VersionTag) != "" {
		response["version_tag"] = saved.VersionTag
	}
	if !saved.UpdatedAt.IsZero() {
		response["updated_at"] = saved.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(saved.UpdatedBy) != "" {
		response["updated_by"] = saved.UpdatedBy
	}

	w.Header().Set("ETag", strconv.FormatInt(revision, 10))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, response)
	configUpdatesTotal.WithLabelValues(clusterID).Inc()
}

func (s *controlPlaneServer) handleGitLabConfig(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGitLabConfigGet(w, r)
	case http.MethodPut:
		s.handleGitLabConfigPut(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *controlPlaneServer) handleArtifactsList(w http.ResponseWriter, r *http.Request) {
	if !s.requireScope(w, r, httpsecurity.ScopeArtifactsRead) {
		recordArtifactRequest(r.Method, http.StatusForbidden)
		return
	}
	recordArtifactRequest(r.Method, http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": []any{}})
}

func (s *controlPlaneServer) handleArtifactsUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireScope(w, r, httpsecurity.ScopeArtifactsWrite) {
		recordArtifactRequest(r.Method, http.StatusForbidden)
		return
	}
	if r.Body == nil {
		recordArtifactRequest(r.Method, http.StatusBadRequest)
		writeErrorMessage(w, http.StatusBadRequest, "request body required")
		return
	}
	defer func() {
		_ = r.Body.Close()
	}()
	bytesCopied, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		recordArtifactRequest(r.Method, http.StatusInternalServerError)
		writeError(w, http.StatusInternalServerError, fmt.Errorf("read upload payload: %w", err))
		return
	}
	recordArtifactPayload("upload", bytesCopied)
	recordArtifactRequest(r.Method, http.StatusNotImplemented)
	writeErrorWithCode(w, http.StatusNotImplemented, errorCodeArtifactUpload, "artifact upload pending persistence backends")
}

func (s *controlPlaneServer) handleArtifactsSubpath(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/artifacts/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		recordArtifactRequest(r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.requireScope(w, r, httpsecurity.ScopeArtifactsRead) {
			recordArtifactRequest(r.Method, http.StatusForbidden)
			return
		}
		recordArtifactRequest(r.Method, http.StatusNotFound)
		writeErrorMessage(w, http.StatusNotFound, "artifact not found")
	case http.MethodDelete:
		if !s.requireScope(w, r, httpsecurity.ScopeArtifactsWrite) {
			recordArtifactRequest(r.Method, http.StatusForbidden)
			return
		}
		recordArtifactRequest(r.Method, http.StatusNotImplemented)
		writeErrorWithCode(w, http.StatusNotImplemented, errorCodeArtifactDelete, "artifact delete pending persistence backends")
	default:
		recordArtifactRequest(r.Method, http.StatusMethodNotAllowed)
		w.Header().Set("Allow", "GET, DELETE")
		writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *controlPlaneServer) handleRegistry(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/registry/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		recordRegistryRequest("root", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		recordRegistryRequest("root", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	repo := strings.TrimSpace(parts[0])
	if repo == "" {
		recordRegistryRequest("root", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "manifests":
		s.handleRegistryManifest(w, r, repo, parts[2:])
	case "blobs":
		s.handleRegistryBlobs(w, r, repo, parts[2:])
	case "tags":
		s.handleRegistryTags(w, r, repo, parts[2:])
	default:
		recordRegistryRequest("unknown", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
	}
}

func (s *controlPlaneServer) handleRegistryManifest(w http.ResponseWriter, r *http.Request, repo string, parts []string) {
	_ = repo
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		recordRegistryRequest("manifests", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.requireScope(w, r, httpsecurity.ScopeRegistryPull) {
			recordRegistryRequest("manifests", r.Method, http.StatusForbidden)
			return
		}
		recordRegistryRequest("manifests", r.Method, http.StatusNotImplemented)
		writeErrorWithCode(w, http.StatusNotImplemented, errorCodeRegistryManifest, "registry manifest retrieval pending persistence backends")
	case http.MethodPut:
		if !s.requireScope(w, r, httpsecurity.ScopeRegistryPush) {
			recordRegistryRequest("manifests", r.Method, http.StatusForbidden)
			return
		}
		body := r.Body
		if body == nil {
			body = http.NoBody
		}
		defer func() {
			_ = body.Close()
		}()
		bytesCopied, err := io.Copy(io.Discard, body)
		if err != nil {
			recordRegistryRequest("manifests", r.Method, http.StatusInternalServerError)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("read manifest payload: %w", err))
			return
		}
		recordRegistryPayload("manifests", "write", bytesCopied)
		recordRegistryRequest("manifests", r.Method, http.StatusNotImplemented)
		writeErrorWithCode(w, http.StatusNotImplemented, errorCodeRegistryManifest, "registry manifest write pending persistence backends")
	case http.MethodDelete:
		if !s.requireScope(w, r, httpsecurity.ScopeRegistryPush) {
			recordRegistryRequest("manifests", r.Method, http.StatusForbidden)
			return
		}
		recordRegistryRequest("manifests", r.Method, http.StatusNotImplemented)
		writeErrorWithCode(w, http.StatusNotImplemented, errorCodeRegistryManifest, "registry manifest delete pending persistence backends")
	default:
		recordRegistryRequest("manifests", r.Method, http.StatusMethodNotAllowed)
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *controlPlaneServer) handleRegistryBlobs(w http.ResponseWriter, r *http.Request, repo string, parts []string) {
	_ = repo
	if len(parts) == 0 {
		recordRegistryRequest("blobs", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	if parts[0] == "uploads" {
		if len(parts) == 1 {
			s.handleRegistryUploadStart(w, r, repo)
			return
		}
		s.handleRegistryUploadSession(w, r, repo, parts[1])
		return
	}
	digest := strings.TrimSpace(parts[0])
	if digest == "" {
		recordRegistryRequest("blobs", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	s.handleRegistryBlob(w, r, repo, digest)
}

func (s *controlPlaneServer) handleRegistryUploadStart(w http.ResponseWriter, r *http.Request, repo string) {
	_ = repo
	if r.Method != http.MethodPost {
		recordRegistryRequest("uploads", r.Method, http.StatusMethodNotAllowed)
		w.Header().Set("Allow", http.MethodPost)
		writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireScope(w, r, httpsecurity.ScopeRegistryPush) {
		recordRegistryRequest("uploads", r.Method, http.StatusForbidden)
		return
	}
	recordRegistryRequest("uploads", r.Method, http.StatusNotImplemented)
	writeErrorWithCode(w, http.StatusNotImplemented, errorCodeRegistryUpload, "registry upload session pending persistence backends")
}

func (s *controlPlaneServer) handleRegistryUploadSession(w http.ResponseWriter, r *http.Request, repo, sessionID string) {
	_ = repo
	_ = sessionID
	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		if !s.requireScope(w, r, httpsecurity.ScopeRegistryPush) {
			recordRegistryRequest("uploads", r.Method, http.StatusForbidden)
			return
		}
		body := r.Body
		if body == nil {
			body = http.NoBody
		}
		defer func() {
			_ = body.Close()
		}()
		bytesCopied, err := io.Copy(io.Discard, body)
		if err != nil {
			recordRegistryRequest("uploads", r.Method, http.StatusInternalServerError)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("read upload payload: %w", err))
			return
		}
		operation := strings.ToLower(r.Method)
		recordRegistryPayload("uploads", operation, bytesCopied)
		recordRegistryRequest("uploads", r.Method, http.StatusNotImplemented)
		writeErrorWithCode(w, http.StatusNotImplemented, errorCodeRegistryUpload, "registry upload session pending persistence backends")
	default:
		recordRegistryRequest("uploads", r.Method, http.StatusMethodNotAllowed)
		w.Header().Set("Allow", "PATCH, PUT")
		writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *controlPlaneServer) handleRegistryBlob(w http.ResponseWriter, r *http.Request, repo, digest string) {
	_ = repo
	switch r.Method {
	case http.MethodGet:
		if !s.requireScope(w, r, httpsecurity.ScopeRegistryPull) {
			recordRegistryRequest("blobs", r.Method, http.StatusForbidden)
			return
		}
		recordRegistryRequest("blobs", r.Method, http.StatusNotImplemented)
		writeErrorWithCode(w, http.StatusNotImplemented, errorCodeRegistryBlob, "registry blob retrieval pending persistence backends")
	case http.MethodDelete:
		if !s.requireScope(w, r, httpsecurity.ScopeRegistryPush) {
			recordRegistryRequest("blobs", r.Method, http.StatusForbidden)
			return
		}
		recordRegistryRequest("blobs", r.Method, http.StatusNotImplemented)
		writeErrorWithCode(w, http.StatusNotImplemented, errorCodeRegistryBlob, "registry blob delete pending persistence backends")
	default:
		recordRegistryRequest("blobs", r.Method, http.StatusMethodNotAllowed)
		w.Header().Set("Allow", "GET, DELETE")
		writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *controlPlaneServer) handleRegistryTags(w http.ResponseWriter, r *http.Request, repo string, parts []string) {
	_ = repo
	if len(parts) == 0 || strings.TrimSpace(parts[0]) != "list" {
		recordRegistryRequest("tags", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		recordRegistryRequest("tags", r.Method, http.StatusMethodNotAllowed)
		w.Header().Set("Allow", http.MethodGet)
		writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireScope(w, r, httpsecurity.ScopeRegistryPull) {
		recordRegistryRequest("tags", r.Method, http.StatusForbidden)
		return
	}
	recordRegistryRequest("tags", r.Method, http.StatusNotImplemented)
	writeErrorWithCode(w, http.StatusNotImplemented, errorCodeRegistryTags, "registry tag listing pending persistence backends")
}

func (s *controlPlaneServer) handleModsSubpath(w http.ResponseWriter, r *http.Request) {
	if !s.ensureMods(w) {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/mods/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(trimmed, "/")
	ticketID := strings.TrimSpace(parts[0])
	if ticketID == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		s.handleModsTicketStatus(w, r, ticketID)
		return
	}
	switch parts[1] {
	case "resume":
		s.handleModsResume(w, r, ticketID)
	case "cancel":
		s.handleModsCancel(w, r, ticketID)
	case "logs":
		s.handleModsLogs(w, r, ticketID, parts[2:])
	case "events":
		s.handleModsEvents(w, r, ticketID)
	default:
		http.NotFound(w, r)
	}
}

func (s *controlPlaneServer) handleModsTickets(w http.ResponseWriter, r *http.Request) {
	if !s.ensureMods(w) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleModsSubmit(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *controlPlaneServer) handleModsTicketSubpath(w http.ResponseWriter, r *http.Request) {
	if !s.ensureMods(w) {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/mods/tickets/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(trimmed, "/")
	ticketID := strings.TrimSpace(parts[0])
	if ticketID == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		s.handleModsTicketStatus(w, r, ticketID)
		return
	}
	switch parts[1] {
	case "cancel":
		s.handleModsCancel(w, r, ticketID)
	case "resume":
		s.handleModsResume(w, r, ticketID)
	case "logs":
		s.handleModsLogs(w, r, ticketID, parts[2:])
	case "events":
		s.handleModsEvents(w, r, ticketID)
	default:
		http.NotFound(w, r)
	}
}

func (s *controlPlaneServer) handleModsSubmit(w http.ResponseWriter, r *http.Request) {
	var req modsapi.TicketSubmitRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.TicketID) == "" {
		writeErrorMessage(w, http.StatusBadRequest, "ticket_id is required")
		return
	}
	if len(req.Stages) == 0 {
		writeErrorMessage(w, http.StatusBadRequest, "stages are required")
		return
	}
	spec := controlplanemods.TicketSpec{
		TicketID:   strings.TrimSpace(req.TicketID),
		Tenant:     strings.TrimSpace(req.Tenant),
		Submitter:  strings.TrimSpace(req.Submitter),
		Repository: strings.TrimSpace(req.Repository),
		Metadata:   cloneStringMap(req.Metadata),
		Stages:     make([]controlplanemods.StageDefinition, 0, len(req.Stages)),
	}
	for _, stage := range req.Stages {
		converted := controlplanemods.StageDefinition{
			ID:           strings.TrimSpace(stage.ID),
			Dependencies: cloneStringSlice(stage.Dependencies),
			Lane:         strings.TrimSpace(stage.Lane),
			Priority:     strings.TrimSpace(stage.Priority),
			MaxAttempts:  stage.MaxAttempts,
			Metadata:     cloneStringMap(stage.Metadata),
		}
		if converted.ID == "" {
			writeErrorMessage(w, http.StatusBadRequest, "stage id is required")
			return
		}
		spec.Stages = append(spec.Stages, converted)
	}
	status, err := s.mods.Submit(r.Context(), spec)
	if err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}
	resp := modsapi.TicketSubmitResponse{Ticket: toAPITicketSummary(status)}
	writeJSON(w, http.StatusAccepted, resp)
}

func (s *controlPlaneServer) handleModsTicketStatus(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.mods.TicketStatus(r.Context(), ticketID)
	if err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}
	writeJSON(w, http.StatusOK, modsapi.TicketStatusResponse{Ticket: toAPITicketSummary(status)})
}

func (s *controlPlaneServer) handleModsCancel(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.mods.Cancel(r.Context(), ticketID); err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *controlPlaneServer) handleModsResume(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.mods.Resume(r.Context(), ticketID)
	if err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}
	writeJSON(w, http.StatusOK, modsapi.TicketStatusResponse{Ticket: toAPITicketSummary(status)})
}

func (s *controlPlaneServer) handleModsLogs(w http.ResponseWriter, r *http.Request, ticketID string, parts []string) {
	if len(parts) == 0 {
		s.handleModsLogsSnapshot(w, r, ticketID)
		return
	}
	if len(parts) == 1 && parts[0] == "stream" {
		s.handleModsLogsStream(w, r, ticketID)
		return
	}
	http.NotFound(w, r)
}

func (s *controlPlaneServer) handleModsLogsSnapshot(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	events, err := s.snapshotLogStream(r.Context(), modsLogStreamID(ticketID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	payload := map[string]any{
		"events": buildLogEventDTOs(events),
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *controlPlaneServer) handleModsLogsStream(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	since, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}
	if err := logstream.Serve(w, r, s.streams, modsLogStreamID(ticketID), since); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
}

func (s *controlPlaneServer) handleModsEvents(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mods == nil {
		http.Error(w, "mods service unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	since, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}

	status, rev, err := s.mods.TicketStatusWithRevision(r.Context(), ticketID)
	if err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if _, err := io.WriteString(w, ":ok\n\n"); err != nil {
		return
	}
	flusher.Flush()

	if rev > 0 && (since <= 0 || rev > since) {
		if err := writeSSEJSON(w, rev, "ticket", toAPITicketSummary(status)); err != nil {
			return
		}
		flusher.Flush()
		since = rev
	}

	events, err := s.mods.WatchTicket(r.Context(), ticketID, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			switch evt.Kind {
			case controlplanemods.EventTicket:
				if evt.Ticket == nil {
					continue
				}
				if err := writeSSEJSON(w, evt.Revision, "ticket", toAPITicketSummary(evt.Ticket)); err != nil {
					return
				}
				flusher.Flush()
			case controlplanemods.EventStage:
				if evt.Stage == nil {
					continue
				}
				payload := modsStageEvent{
					TicketID: ticketID,
					Stage:    toAPIStageStatus(evt.Stage),
				}
				if err := writeSSEJSON(w, evt.Revision, "stage", payload); err != nil {
					return
				}
				flusher.Flush()
			}
		case <-ticker.C:
			if _, err := io.WriteString(w, ":ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *controlPlaneServer) handleNodeJoin(w http.ResponseWriter, r *http.Request) {
	var req nodeJoinRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	clusterID := strings.TrimSpace(req.ClusterID)
	if clusterID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "cluster_id is required")
		return
	}
	address := strings.TrimSpace(req.Address)
	if address == "" {
		writeErrorMessage(w, http.StatusBadRequest, "address is required")
		return
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		generated, err := gonanoid.Generate("abcdefghijklmnopqrstuvwxyz0123456789", 12)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		workerID = generated
	}

	created, err := deploy.EnsureClusterPKI(r.Context(), s.etcd, clusterID, deploy.EnsurePKIOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if created {
		log.Printf("auto-bootstrapped cluster PKI for %s", clusterID)
	}

	probes := make([]deploy.WorkerHealthProbe, 0, len(req.Probes))
	for _, probe := range req.Probes {
		probes = append(probes, deploy.WorkerHealthProbe{
			Name:         strings.TrimSpace(probe.Name),
			Endpoint:     strings.TrimSpace(probe.Endpoint),
			ExpectStatus: probe.ExpectStatus,
		})
	}

	opts := deploy.WorkerJoinOptions{
		ClusterID:    clusterID,
		WorkerID:     workerID,
		Address:      address,
		Labels:       req.Labels,
		HealthProbes: probes,
		DryRun:       req.DryRun,
		Clock:        func() time.Time { return time.Now().UTC() },
	}

	result, err := deploy.RunWorkerJoin(r.Context(), s.etcd, opts)
	if err != nil {
		switch {
		case errors.Is(err, registry.ErrWorkerExists), errors.Is(err, deploy.ErrWorkerExists):
			writeErrorMessage(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}

	resp := map[string]any{
		"worker_id":   result.Descriptor.ID,
		"descriptor":  descriptorDTOFrom(result.Descriptor),
		"certificate": result.Certificate,
		"health":      result.Health,
		"dry_run":     result.DryRun,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *controlPlaneServer) handleNodeList(w http.ResponseWriter, r *http.Request) {
	clusterID := strings.TrimSpace(r.URL.Query().Get("cluster_id"))
	if clusterID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "cluster_id query parameter required")
		return
	}
	reg, err := registry.NewWorkerRegistry(s.etcd, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	descriptors, err := reg.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	nodes := make([]workerDescriptorDTO, 0, len(descriptors))
	for _, descriptor := range descriptors {
		nodes = append(nodes, descriptorDTOFrom(descriptor))
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
}

func (s *controlPlaneServer) handleNodeDelete(w http.ResponseWriter, r *http.Request) {
	if !s.ensureScheduler(w) {
		return
	}
	var req nodeDeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	clusterID := strings.TrimSpace(req.ClusterID)
	workerID := strings.TrimSpace(req.WorkerID)
	if clusterID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "cluster_id is required")
		return
	}
	if workerID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "worker_id is required")
		return
	}
	if strings.TrimSpace(req.Confirm) != workerID {
		writeErrorMessage(w, http.StatusBadRequest, "confirm must match worker_id")
		return
	}
	if req.DrainTimeoutSeconds < 0 {
		writeErrorMessage(w, http.StatusBadRequest, "drain_timeout_seconds must be non-negative")
		return
	}
	timeout := time.Duration(req.DrainTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	if err := s.waitForNodeDrain(r.Context(), workerID, timeout); err != nil {
		var drainErr nodeDrainError
		switch {
		case errors.As(err, &drainErr):
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":        fmt.Sprintf("node %s still has running jobs", workerID),
				"running_jobs": drainErr.Remaining,
			})
		case errors.Is(err, context.DeadlineExceeded):
			writeError(w, http.StatusRequestTimeout, err)
		case errors.Is(err, context.Canceled):
			writeError(w, http.StatusRequestTimeout, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	manager, err := deploy.NewCARotationManager(s.etcd, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reg, err := registry.NewWorkerRegistry(s.etcd, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := reg.Get(r.Context(), workerID); err != nil {
		if errors.Is(err, registry.ErrWorkerNotFound) {
			writeErrorMessage(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := manager.RemoveWorker(r.Context(), workerID); err != nil {
		switch {
		case errors.Is(err, deploy.ErrWorkerNotFound):
			writeErrorMessage(w, http.StatusNotFound, err.Error())
		case errors.Is(err, deploy.ErrConcurrentWorkerUpdate):
			writeErrorMessage(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	if err := reg.Delete(r.Context(), workerID); err != nil {
		if errors.Is(err, registry.ErrWorkerNotFound) {
			writeErrorMessage(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *controlPlaneServer) handleGitLabConfigGet(w http.ResponseWriter, r *http.Request) {
	store := gitlab.NewStore(gitlab.NewEtcdKV(s.etcd))
	cfg, revision, err := store.Load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if revision == 0 {
		writeErrorMessage(w, http.StatusNotFound, "gitlab configuration not set")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"config":   cfg.Sanitize(),
		"revision": revision,
	})
}

func (s *controlPlaneServer) handleGitLabConfigPut(w http.ResponseWriter, r *http.Request) {
	var req gitlabConfigRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Revision < 0 {
		writeErrorMessage(w, http.StatusBadRequest, "revision must be non-negative")
		return
	}

	store := gitlab.NewStore(gitlab.NewEtcdKV(s.etcd))
	_, currentRevision, err := store.Load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	revisionMismatch := func() {
		writeErrorMessage(w, http.StatusConflict, "revision mismatch")
	}

	if currentRevision == 0 {
		if req.Revision != 0 {
			revisionMismatch()
			return
		}
	} else if req.Revision != currentRevision {
		revisionMismatch()
		return
	}

	normalized, err := gitlab.Normalize(req.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	newRevision, err := store.Save(r.Context(), normalized)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"revision": newRevision,
		"config":   normalized.Sanitize(),
	})
}

func (s *controlPlaneServer) handleJobs(w http.ResponseWriter, r *http.Request) {
	if !s.ensureScheduler(w) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleJobSubmit(w, r)
	case http.MethodGet:
		s.handleJobList(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *controlPlaneServer) handleJobSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.ensureScheduler(w) {
		return
	}
	var req struct {
		Ticket      string            `json:"ticket"`
		StepID      string            `json:"step_id"`
		Priority    string            `json:"priority"`
		MaxAttempts int               `json:"max_attempts"`
		Metadata    map[string]string `json:"metadata"`
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	job, err := s.scheduler.SubmitJob(r.Context(), scheduler.JobSpec{
		Ticket:      req.Ticket,
		StepID:      req.StepID,
		Priority:    req.Priority,
		MaxAttempts: req.MaxAttempts,
		Metadata:    req.Metadata,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, jobDTOFrom(job))
}

func (s *controlPlaneServer) handleJobList(w http.ResponseWriter, r *http.Request) {
	if !s.ensureScheduler(w) {
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		http.Error(w, "ticket query parameter required", http.StatusBadRequest)
		return
	}
	jobs, err := s.scheduler.ListJobs(r.Context(), ticket)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]jobDTO, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, jobDTOFrom(job))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": items})
}

func (s *controlPlaneServer) handleClaim(w http.ResponseWriter, r *http.Request) {
	if !s.ensureScheduler(w) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.scheduler.ClaimNext(r.Context(), scheduler.ClaimRequest{NodeID: req.NodeID})
	if err != nil {
		if errors.Is(err, scheduler.ErrNoJobs) {
			writeJSON(w, http.StatusOK, map[string]any{"status": "empty"})
			return
		}
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "claimed",
		"node_id": res.NodeID,
		"job":     jobDTOFrom(res.Job),
	})
}

func (s *controlPlaneServer) handleJobSubpath(w http.ResponseWriter, r *http.Request) {
	if !s.ensureScheduler(w) {
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	parts := strings.Split(strings.Trim(rel, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	jobID := parts[0]
	if len(parts) == 1 {
		s.handleJobGet(w, r, jobID)
		return
	}
	switch parts[1] {
	case "heartbeat":
		s.handleJobHeartbeat(w, r, jobID)
	case "complete":
		s.handleJobComplete(w, r, jobID)
	case "logs":
		s.handleJobLogs(w, r, jobID, parts[2:])
	case "events":
		s.handleJobEvents(w, r, jobID)
	default:
		http.NotFound(w, r)
	}
}

func (s *controlPlaneServer) handleJobGet(w http.ResponseWriter, r *http.Request, jobID string) {
	if !s.ensureScheduler(w) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		http.Error(w, "ticket query parameter required", http.StatusBadRequest)
		return
	}
	job, err := s.scheduler.GetJob(r.Context(), ticket, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, jobDTOFrom(job))
}

func (s *controlPlaneServer) handleJobHeartbeat(w http.ResponseWriter, r *http.Request, jobID string) {
	if !s.ensureScheduler(w) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Ticket string `json:"ticket"`
		NodeID string `json:"node_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.scheduler.Heartbeat(r.Context(), scheduler.HeartbeatRequest{
		JobID:  jobID,
		Ticket: req.Ticket,
		NodeID: req.NodeID,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *controlPlaneServer) handleJobComplete(w http.ResponseWriter, r *http.Request, jobID string) {
	if !s.ensureScheduler(w) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Ticket     string                            `json:"ticket"`
		NodeID     string                            `json:"node_id"`
		State      string                            `json:"state"`
		Artifacts  map[string]string                 `json:"artifacts"`
		Bundles    map[string]scheduler.BundleRecord `json:"bundles"`
		Error      *scheduler.JobError               `json:"error"`
		Inspection bool                              `json:"inspection"`
		Shift      *struct {
			Result          string  `json:"result"`
			DurationSeconds float64 `json:"duration_seconds"`
		} `json:"shift"`
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var shiftMetrics *scheduler.ShiftMetrics
	if req.Shift != nil {
		d := time.Duration(req.Shift.DurationSeconds * float64(time.Second))
		if d < 0 {
			d = 0
		}
		shiftMetrics = &scheduler.ShiftMetrics{
			Result:   req.Shift.Result,
			Duration: d,
		}
	}
	job, err := s.scheduler.CompleteJob(r.Context(), scheduler.CompleteRequest{
		JobID:      jobID,
		Ticket:     req.Ticket,
		NodeID:     req.NodeID,
		State:      scheduler.JobState(req.State),
		Artifacts:  req.Artifacts,
		Bundles:    req.Bundles,
		Error:      req.Error,
		Inspection: req.Inspection,
		Shift:      shiftMetrics,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, jobDTOFrom(job))
}

func (s *controlPlaneServer) handleJobLogs(w http.ResponseWriter, r *http.Request, jobID string, parts []string) {
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	switch parts[0] {
	case "stream":
		s.handleJobLogsStream(w, r, jobID)
	default:
		http.NotFound(w, r)
	}
}

func (s *controlPlaneServer) handleJobLogsStream(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}

	lastID, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}
	if err := logstream.Serve(w, r, s.streams, jobID, lastID); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
}

func (s *controlPlaneServer) handleJobEvents(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.etcd == nil {
		http.Error(w, "etcd unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	lastID, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}
	ticket, key, rev, err := s.lookupJobKey(r.Context(), jobID)
	if err != nil {
		writeErrorMessage(w, http.StatusNotFound, "job not found")
		return
	}
	job, err := s.scheduler.GetJob(r.Context(), ticket, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if _, err := io.WriteString(w, ":ok\n\n"); err != nil {
		return
	}
	flusher.Flush()

	if rev > 0 && (lastID <= 0 || rev > lastID) {
		if err := writeSSEJSON(w, rev, "job", jobDTOFrom(job)); err != nil {
			return
		}
		flusher.Flush()
	}

	watchRev := rev + 1
	if lastID >= watchRev {
		watchRev = lastID + 1
	}

	opts := []clientv3.OpOption{}
	if watchRev > 0 {
		opts = append(opts, clientv3.WithRev(watchRev))
	}
	watch := s.etcd.Watch(r.Context(), key, opts...)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case resp, ok := <-watch:
			if !ok {
				return
			}
			if err := resp.Err(); err != nil {
				continue
			}
			for _, ev := range resp.Events {
				if ev == nil || ev.Type != clientv3.EventTypePut || ev.Kv == nil {
					continue
				}
				current, err := s.scheduler.GetJob(r.Context(), ticket, jobID)
				if err != nil {
					continue
				}
				if err := writeSSEJSON(w, ev.Kv.ModRevision, "job", jobDTOFrom(current)); err != nil {
					return
				}
				flusher.Flush()
			}
		case <-ticker.C:
			if _, err := io.WriteString(w, ":ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *controlPlaneServer) handleSignerSecrets(w http.ResponseWriter, r *http.Request) {
	if s.signer == nil {
		http.Error(w, "gitlab signer unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Secret string   `json:"secret"`
		APIKey string   `json:"api_key"`
		Scopes []string `json:"scopes"`
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := s.signer.RotateSecret(r.Context(), gitlab.RotateSecretRequest{
		SecretName: req.Secret,
		APIKey:     req.APIKey,
		Scopes:     req.Scopes,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload := map[string]any{
		"secret":     strings.TrimSpace(req.Secret),
		"revision":   result.Revision,
		"updated_at": result.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *controlPlaneServer) handleSignerTokens(w http.ResponseWriter, r *http.Request) {
	if s.signer == nil {
		http.Error(w, "gitlab signer unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Secret     string   `json:"secret"`
		Scopes     []string `json:"scopes"`
		TTLSeconds int64    `json:"ttl_seconds"`
		NodeID     string   `json:"node_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.NodeID) == "" {
		http.Error(w, "node_id required", http.StatusBadRequest)
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	token, err := s.signer.IssueToken(r.Context(), gitlab.IssueTokenRequest{
		SecretName: req.Secret,
		Scopes:     req.Scopes,
		TTL:        ttl,
		NodeID:     req.NodeID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload := map[string]any{
		"secret":      token.SecretName,
		"token":       token.Value,
		"scopes":      token.Scopes,
		"issued_at":   token.IssuedAt.UTC().Format(time.RFC3339Nano),
		"expires_at":  token.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"ttl_seconds": int64(token.ExpiresAt.Sub(token.IssuedAt).Seconds()),
		"token_id":    token.TokenID,
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *controlPlaneServer) handleSignerRotations(w http.ResponseWriter, r *http.Request) {
	if s.rotations == nil {
		http.Error(w, "gitlab rotations unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	timeout := 30 * time.Second
	if raw := strings.TrimSpace(r.URL.Query().Get("timeout")); raw != "" {
		dur, err := time.ParseDuration(raw)
		if err != nil {
			http.Error(w, "invalid timeout duration", http.StatusBadRequest)
			return
		}
		if dur > 0 {
			timeout = dur
		} else {
			timeout = 0
		}
	}

	var since int64
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		revision, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "invalid since revision", http.StatusBadRequest)
			return
		}
		since = revision
	}

	ctx := r.Context()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	secret := strings.TrimSpace(r.URL.Query().Get("secret"))
	evt, ok := s.rotations.Wait(ctx, secret, since)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	payload := map[string]any{
		"secret":   evt.SecretName,
		"revision": evt.Revision,
	}
	if !evt.UpdatedAt.IsZero() {
		payload["updated_at"] = evt.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	writeJSON(w, http.StatusOK, payload)
}

type nodeJoinRequest struct {
	ClusterID string             `json:"cluster_id"`
	WorkerID  string             `json:"worker_id"`
	Address   string             `json:"address"`
	Labels    map[string]string  `json:"labels"`
	Probes    []nodeProbeRequest `json:"probes"`
	DryRun    bool               `json:"dry_run"`
}

type nodeProbeRequest struct {
	Name         string `json:"name"`
	Endpoint     string `json:"endpoint"`
	ExpectStatus int    `json:"expect_status"`
}

type nodeDeleteRequest struct {
	ClusterID           string `json:"cluster_id"`
	WorkerID            string `json:"worker_id"`
	Confirm             string `json:"confirm"`
	DrainTimeoutSeconds int    `json:"drain_timeout_seconds"`
}

type gitlabConfigRequest struct {
	Revision int64         `json:"revision"`
	Config   gitlab.Config `json:"config"`
}

type workerDescriptorDTO struct {
	ID                 string            `json:"id"`
	Address            string            `json:"address"`
	Labels             map[string]string `json:"labels,omitempty"`
	RegisteredAt       string            `json:"registered_at"`
	CertificateVersion string            `json:"certificate_version,omitempty"`
	Status             workerStatusDTO   `json:"status"`
}

type workerStatusDTO struct {
	Phase     string                       `json:"phase"`
	CheckedAt string                       `json:"checked_at"`
	Message   string                       `json:"message,omitempty"`
	Probes    []registry.WorkerProbeResult `json:"probes,omitempty"`
}

type nodeDrainError struct {
	Remaining int
}

func (e nodeDrainError) Error() string {
	if e.Remaining <= 0 {
		return "no running jobs"
	}
	return fmt.Sprintf("%d jobs still running", e.Remaining)
}

func descriptorDTOFrom(desc registry.WorkerDescriptor) workerDescriptorDTO {
	labels := copyMap(desc.Labels)
	if len(labels) == 0 {
		labels = nil
	}
	statusProbes := make([]registry.WorkerProbeResult, 0, len(desc.Status.Probes))
	if len(desc.Status.Probes) > 0 {
		statusProbes = append(statusProbes, desc.Status.Probes...)
	}
	dto := workerDescriptorDTO{
		ID:                 desc.ID,
		Address:            desc.Address,
		Labels:             labels,
		RegisteredAt:       formatTime(desc.RegisteredAt),
		CertificateVersion: desc.CertificateVersion,
		Status: workerStatusDTO{
			Phase:     desc.Status.Phase,
			CheckedAt: formatTime(desc.Status.CheckedAt),
			Message:   desc.Status.Message,
			Probes:    statusProbes,
		},
	}
	if strings.TrimSpace(dto.Status.Message) == "" {
		dto.Status.Message = ""
	}
	if len(dto.Status.Probes) == 0 {
		dto.Status.Probes = nil
	}
	return dto
}

func (s *controlPlaneServer) waitForNodeDrain(ctx context.Context, nodeID string, timeout time.Duration) error {
	if s.scheduler == nil {
		return errors.New("scheduler unavailable")
	}
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		jobs, err := s.scheduler.RunningJobsForNode(ctx, nodeID)
		if err != nil {
			return err
		}
		remaining := len(jobs)
		if remaining == 0 {
			return nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nodeDrainError{Remaining: remaining}
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nodeDrainError{Remaining: remaining}
			}
			return ctx.Err()
		case <-ticker.C:
		}
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

func recordConfigRequest(method string, status int) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	configRequestsTotal.WithLabelValues(method, strconv.Itoa(status)).Inc()
}

func recordBeaconRequest(resource, method string, status int) {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		resource = "unknown"
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	beaconRequestsTotal.WithLabelValues(resource, method, strconv.Itoa(status)).Inc()
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func parseRevisionHeader(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, errors.New("config: If-Match header required")
	}
	if trimmed == "*" {
		return -1, nil
	}
	if strings.HasPrefix(trimmed, "W/") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "W/"))
	}
	trimmed = strings.Trim(trimmed, `"`)
	if trimmed == "" {
		return 0, errors.New("config: invalid If-Match header")
	}
	revision, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: parse If-Match revision: %w", err)
	}
	if revision < 0 {
		return 0, errors.New("config: revision must be non-negative")
	}
	return revision, nil
}

func (s *controlPlaneServer) collectQueueDepths() map[string]float64 {
	depths := make(map[string]float64)
	if s.gatherer == nil {
		return depths
	}
	families, err := s.gatherer.Gather()
	if err != nil {
		return depths
	}
	for _, fam := range families {
		if fam == nil || fam.GetName() != "ploy_controlplane_queue_depth" {
			continue
		}
		for _, metric := range fam.GetMetric() {
			if metric == nil || metric.GetGauge() == nil {
				continue
			}
			priority := labelValue(metric, "priority")
			if priority == "" {
				priority = "default"
			}
			depths[priority] = metric.GetGauge().GetValue()
		}
	}
	return depths
}

func labelValue(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label == nil {
			continue
		}
		if strings.EqualFold(label.GetName(), name) {
			return label.GetValue()
		}
	}
	return ""
}

func sanitizeLeafCertificates(ids []string, certificates map[string]deploy.LeafCertificate) []map[string]any {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		entry := map[string]any{
			"node_id": id,
		}
		if cert, ok := certificates[id]; ok {
			entry["usage"] = cert.Usage
			entry["version"] = cert.Version
			entry["parent_version"] = cert.ParentVersion
			entry["serial_number"] = cert.SerialNumber
			entry["certificate_pem"] = cert.CertificatePEM
			if !cert.IssuedAt.IsZero() {
				entry["issued_at"] = cert.IssuedAt.UTC().Format(time.RFC3339Nano)
			}
			if !cert.ExpiresAt.IsZero() {
				entry["expires_at"] = cert.ExpiresAt.UTC().Format(time.RFC3339Nano)
			}
		} else {
			entry["missing"] = true
		}
		out = append(out, entry)
	}
	return out
}

type signedEnvelope struct {
	Payload   json.RawMessage `json:"payload"`
	Signature signatureDTO    `json:"signature"`
}

type signatureDTO struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Value     string `json:"value"`
}

func (s *controlPlaneServer) sendSignedJSON(w http.ResponseWriter, status int, payload any, bundle deploy.CABundle) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode signed payload: %w", err)
	}
	signature, err := signPayload(bundle, data)
	if err != nil {
		return err
	}
	env := signedEnvelope{
		Payload:   data,
		Signature: signature,
	}
	writeJSON(w, status, env)
	return nil
}

func signPayload(bundle deploy.CABundle, payload []byte) (signatureDTO, error) {
	key, err := parsePrivateKey(bundle.KeyPEM)
	if err != nil {
		return signatureDTO{}, err
	}
	digest := sha256.Sum256(payload)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		return signatureDTO{}, fmt.Errorf("sign payload: %w", err)
	}
	return signatureDTO{
		Algorithm: "ES256",
		KeyID:     strings.TrimSpace(bundle.Version),
		Value:     base64.StdEncoding.EncodeToString(sig),
	}, nil
}

func parsePrivateKey(pemData string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("beacon: decode private key")
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("beacon: parse private key: %w", err)
	}
	ecdsaKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("beacon: private key is not ECDSA")
	}
	return ecdsaKey, nil
}

func (s *controlPlaneServer) persistBeaconCanonical(ctx context.Context, clusterID string, record map[string]any) error {
	if s.etcd == nil {
		return errors.New("beacon: etcd unavailable")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("beacon: encode canonical record: %w", err)
	}
	if _, err := s.etcd.Put(ctx, beaconCanonicalKey(clusterID), string(data)); err != nil {
		return fmt.Errorf("beacon: persist canonical record: %w", err)
	}
	return nil
}

func beaconCanonicalKey(clusterID string) string {
	return fmt.Sprintf("/ploy/clusters/%s/beacon/canonical", strings.TrimSpace(clusterID))
}

func writeError(w http.ResponseWriter, status int, err error) {
	if err == nil {
		writeErrorMessage(w, status, http.StatusText(status))
		return
	}
	writeErrorMessage(w, status, err.Error())
}

func mapModsError(err error) (int, string) {
	switch {
	case errors.Is(err, controlplanemods.ErrTicketNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, controlplanemods.ErrStageNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, controlplanemods.ErrStageAlreadyClaimed):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}

func toAPITicketSummary(status *controlplanemods.TicketStatus) modsapi.TicketSummary {
	if status == nil {
		return modsapi.TicketSummary{}
	}
	stages := make(map[string]modsapi.StageStatus, len(status.Stages))
	for id, stage := range status.Stages {
		stageCopy := stage
		stages[id] = toAPIStageStatus(&stageCopy)
	}
	return modsapi.TicketSummary{
		TicketID:   status.TicketID,
		State:      modsapi.TicketState(status.State),
		Tenant:     status.Tenant,
		Submitter:  status.Submitter,
		Repository: status.Repository,
		Metadata:   cloneStringMap(status.Metadata),
		CreatedAt:  status.CreatedAt.UTC(),
		UpdatedAt:  status.UpdatedAt.UTC(),
		Stages:     stages,
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

type modsStageEvent struct {
	TicketID string              `json:"ticket_id"`
	Stage    modsapi.StageStatus `json:"stage"`
}

func toAPIStageStatus(stage *controlplanemods.StageStatus) modsapi.StageStatus {
	if stage == nil {
		return modsapi.StageStatus{}
	}
	return modsapi.StageStatus{
		StageID:      stage.StageID,
		State:        modsapi.StageState(stage.State),
		Attempts:     stage.Attempts,
		MaxAttempts:  stage.MaxAttempts,
		CurrentJobID: stage.CurrentJobID,
		Artifacts:    cloneStringMap(stage.Artifacts),
		LastError:    stage.LastError,
	}
}

func writeErrorMessage(w http.ResponseWriter, status int, message string) {
	writeErrorWithCode(w, status, "", message)
}

func writeErrorWithCode(w http.ResponseWriter, status int, code, message string) {
	if strings.TrimSpace(message) == "" {
		message = http.StatusText(status)
	}
	payload := map[string]any{"error": message}
	code = strings.TrimSpace(code)
	if code != "" {
		payload["error_code"] = code
	}
	writeJSON(w, status, payload)
}

// jobDTO is the API representation for a job.
type jobDTO struct {
	ID             string                            `json:"id"`
	Ticket         string                            `json:"ticket"`
	StepID         string                            `json:"step_id"`
	Priority       string                            `json:"priority"`
	State          string                            `json:"state"`
	CreatedAt      string                            `json:"created_at"`
	EnqueuedAt     string                            `json:"enqueued_at"`
	ClaimedAt      string                            `json:"claimed_at,omitempty"`
	CompletedAt    string                            `json:"completed_at,omitempty"`
	ExpiresAt      string                            `json:"expires_at,omitempty"`
	LeaseID        int64                             `json:"lease_id,omitempty"`
	LeaseExpiresAt string                            `json:"lease_expires_at,omitempty"`
	ClaimedBy      string                            `json:"claimed_by,omitempty"`
	RetryAttempt   int                               `json:"retry_attempt"`
	MaxAttempts    int                               `json:"max_attempts"`
	Metadata       map[string]string                 `json:"metadata,omitempty"`
	Artifacts      map[string]string                 `json:"artifacts,omitempty"`
	Bundles        map[string]scheduler.BundleRecord `json:"bundles,omitempty"`
	Shift          *shiftDTO                         `json:"shift,omitempty"`
	Retention      *scheduler.JobRetention           `json:"retention,omitempty"`
	NodeSnapshot   *nodeSnapshotDTO                  `json:"node_snapshot,omitempty"`
	Error          *scheduler.JobError               `json:"error,omitempty"`
}

func jobDTOFrom(job *scheduler.Job) jobDTO {
	return jobDTO{
		ID:             job.ID,
		Ticket:         job.Ticket,
		StepID:         job.StepID,
		Priority:       job.Priority,
		State:          string(job.State),
		CreatedAt:      job.CreatedAt.UTC().Format(time.RFC3339Nano),
		EnqueuedAt:     job.EnqueuedAt.UTC().Format(time.RFC3339Nano),
		ClaimedAt:      formatTime(job.ClaimedAt),
		CompletedAt:    formatTime(job.CompletedAt),
		ExpiresAt:      formatTime(job.ExpiresAt),
		LeaseID:        int64(job.LeaseID),
		LeaseExpiresAt: formatTime(job.LeaseExpiresAt),
		ClaimedBy:      job.ClaimedBy,
		RetryAttempt:   job.RetryAttempt,
		MaxAttempts:    job.MaxAttempts,
		Metadata:       copyMap(job.Metadata),
		Artifacts:      copyMap(job.Artifacts),
		Bundles:        copyBundles(job.Bundles),
		Shift:          copyShift(job.Shift),
		Retention:      copyRetention(job.Retention),
		NodeSnapshot:   copyNodeSnapshot(job.NodeSnapshot),
		Error:          job.Error,
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func copyMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func copyBundles(src map[string]scheduler.BundleRecord) map[string]scheduler.BundleRecord {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]scheduler.BundleRecord, len(src))
	for k, v := range src {
		out[k] = scheduler.BundleRecord{
			CID:       v.CID,
			Digest:    v.Digest,
			Size:      v.Size,
			Retained:  v.Retained,
			TTL:       v.TTL,
			ExpiresAt: v.ExpiresAt,
		}
	}
	return out
}

type shiftDTO struct {
	Result          string  `json:"result"`
	DurationSeconds float64 `json:"duration_seconds"`
}

func copyShift(src *scheduler.ShiftSummary) *shiftDTO {
	if src == nil {
		return nil
	}
	dst := &shiftDTO{Result: src.Result}
	if src.Duration > 0 {
		dst.DurationSeconds = src.Duration.Seconds()
	}
	return dst
}

func copyRetention(src *scheduler.JobRetention) *scheduler.JobRetention {
	if src == nil {
		return nil
	}
	return &scheduler.JobRetention{
		Retained:   src.Retained,
		TTL:        src.TTL,
		ExpiresAt:  src.ExpiresAt,
		Bundle:     src.Bundle,
		BundleCID:  src.BundleCID,
		Inspection: src.Inspection,
	}
}

type nodeSnapshotDTO struct {
	NodeID     string         `json:"node_id"`
	Capacity   map[string]any `json:"capacity,omitempty"`
	CapacityAt string         `json:"capacity_at,omitempty"`
	Status     map[string]any `json:"status,omitempty"`
	StatusAt   string         `json:"status_at,omitempty"`
}

func copyNodeSnapshot(src *scheduler.JobNodeSnapshot) *nodeSnapshotDTO {
	if src == nil {
		return nil
	}
	dto := &nodeSnapshotDTO{NodeID: src.NodeID}
	if len(src.Capacity) > 0 {
		dto.Capacity = copyAnyMap(src.Capacity)
	}
	if !src.CapacityAt.IsZero() {
		dto.CapacityAt = src.CapacityAt.UTC().Format(time.RFC3339Nano)
	}
	if len(src.Status) > 0 {
		dto.Status = copyAnyMap(src.Status)
	}
	if !src.StatusAt.IsZero() {
		dto.StatusAt = src.StatusAt.UTC().Format(time.RFC3339Nano)
	}
	return dto
}

func copyAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func modsLogStreamID(ticketID string) string {
	return strings.TrimSpace(ticketID)
}

func (s *controlPlaneServer) snapshotLogStream(ctx context.Context, streamID string) ([]logstream.Event, error) {
	sub, err := s.streams.Subscribe(ctx, streamID, 0)
	if err != nil {
		return nil, err
	}
	defer sub.Cancel()
	events := make([]logstream.Event, 0, 8)
	for {
		select {
		case evt, ok := <-sub.Events:
			if !ok {
				return events, nil
			}
			events = append(events, evt)
			if strings.EqualFold(evt.Type, "done") {
				return events, nil
			}
		default:
			return events, nil
		}
	}
}

func buildLogEventDTOs(events []logstream.Event) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, evt := range events {
		dto := map[string]any{
			"id":   evt.ID,
			"type": evt.Type,
		}
		if len(evt.Data) > 0 {
			var payload any
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				dto["data"] = strings.TrimSpace(string(evt.Data))
			} else {
				dto["data"] = payload
			}
		}
		out = append(out, dto)
	}
	return out
}

func writeSSEJSON(w io.Writer, id int64, event string, payload any) error {
	if id > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", id); err != nil {
			return err
		}
	}
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(w, "\n")
	return err
}

func parseLastEventID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	if id < 0 {
		return 0, nil
	}
	return id, nil
}

func (s *controlPlaneServer) lookupJobKey(ctx context.Context, jobID string) (string, string, int64, error) {
	if s.etcd == nil {
		return "", "", 0, fmt.Errorf("etcd unavailable")
	}
	prefix := s.modsPrefix()
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	resp, err := s.etcd.Get(ctx, prefix, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		return "", "", 0, err
	}
	suffix := "/jobs/" + jobID
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		if !strings.HasSuffix(key, suffix) {
			continue
		}
		trimmed := strings.TrimPrefix(key, prefix)
		parts := strings.Split(trimmed, "/")
		if len(parts) < 3 {
			continue
		}
		ticket := strings.TrimSpace(parts[0])
		if ticket == "" {
			continue
		}
		detail, err := s.etcd.Get(ctx, key)
		if err != nil {
			return "", "", 0, err
		}
		if len(detail.Kvs) == 0 {
			continue
		}
		return ticket, key, detail.Kvs[0].ModRevision, nil
	}
	return "", "", 0, fmt.Errorf("job %s not found", jobID)
}

func (s *controlPlaneServer) modsPrefix() string {
	if s.mods != nil {
		if prefix := strings.TrimSpace(s.mods.Prefix()); prefix != "" {
			return prefix
		}
	}
	return "mods/"
}

func decodeJSON(r *http.Request, dst any) error {
	defer func() {
		_ = r.Body.Close()
	}()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(new(struct{})); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing json data")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}

func (s *controlPlaneServer) ensureScheduler(w http.ResponseWriter) bool {
	if s.scheduler == nil {
		http.Error(w, "scheduler unavailable", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func recordArtifactRequest(method string, status int) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	artifactRequestsTotal.WithLabelValues(method, strconv.Itoa(status)).Inc()
}

func recordArtifactPayload(operation string, bytesCopied int64) {
	if bytesCopied <= 0 {
		return
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "unknown"
	}
	artifactPayloadBytes.WithLabelValues(operation).Add(float64(bytesCopied))
}

func recordRegistryRequest(resource, method string, status int) {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		resource = "unknown"
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	registryRequestsTotal.WithLabelValues(resource, method, strconv.Itoa(status)).Inc()
}

func recordRegistryPayload(resource, operation string, bytesCopied int64) {
	if bytesCopied <= 0 {
		return
	}
	resource = strings.TrimSpace(resource)
	if resource == "" {
		resource = "unknown"
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "unknown"
	}
	registryPayloadBytes.WithLabelValues(resource, operation).Add(float64(bytesCopied))
}
