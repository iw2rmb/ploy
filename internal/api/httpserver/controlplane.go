package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/iw2rmb/ploy/internal/config/gitlab"
	"github.com/iw2rmb/ploy/internal/controlplane/events"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/node/logstream"
)

// Server exposes the control-plane scheduler over HTTP.
type controlPlaneServer struct {
	scheduler *scheduler.Scheduler
	signer    *gitlab.Signer
	rotations *events.RotationHub
	streams   *logstream.Hub
	etcd      *clientv3.Client
}

// Options configure the HTTP server handlers.
type ControlPlaneOptions struct {
	Scheduler *scheduler.Scheduler
	Signer    *gitlab.Signer
	Streams   *logstream.Hub
	Gatherer  prometheus.Gatherer
	Etcd      *clientv3.Client
	Rotations *events.RotationHub
}

// New returns an HTTP handler rooted at /v1.
func NewControlPlaneHandler(opts ControlPlaneOptions) http.Handler {
	mux := http.NewServeMux()
	h := &controlPlaneServer{
		scheduler: opts.Scheduler,
		signer:    opts.Signer,
		streams:   opts.Streams,
		etcd:      opts.Etcd,
		rotations: opts.Rotations,
	}
	if h.rotations == nil && opts.Signer != nil {
		h.rotations = events.NewRotationHub(context.Background(), opts.Signer)
	}
	mux.HandleFunc("/v1/jobs", h.handleJobs)
	mux.HandleFunc("/v1/jobs/claim", h.handleClaim)
	mux.HandleFunc("/v1/jobs/", h.handleJobSubpath)
	mux.HandleFunc("/v1/health", h.handleHealth)
	mux.HandleFunc("/v1/gitlab/signer/secrets", h.handleSignerSecrets)
	mux.HandleFunc("/v1/gitlab/signer/tokens", h.handleSignerTokens)
	mux.HandleFunc("/v1/gitlab/signer/rotations", h.handleSignerRotations)
	mux.HandleFunc("/v1/nodes", h.handleNodes)
	mux.HandleFunc("/v1/beacon/rotate-ca", h.handleBeaconRotateCA)
	mux.HandleFunc("/v1/config/gitlab", h.handleGitLabConfig)
	gatherer := opts.Gatherer
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}
	mux.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	return mux
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

func (s *controlPlaneServer) handleBeaconRotateCA(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ClusterID string `json:"cluster_id"`
		DryRun    bool   `json:"dry_run"`
		Operator  string `json:"operator"`
		Reason    string `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	clusterID := strings.TrimSpace(req.ClusterID)
	if clusterID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "cluster_id is required")
		return
	}

	manager, err := deploy.NewCARotationManager(s.etcd, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	result, err := manager.Rotate(r.Context(), deploy.RotateOptions{
		DryRun:      req.DryRun,
		RequestedAt: time.Now().UTC(),
		Operator:    strings.TrimSpace(req.Operator),
		Reason:      strings.TrimSpace(req.Reason),
	})
	if err != nil {
		switch {
		case errors.Is(err, deploy.ErrConcurrentRotation):
			writeErrorMessage(w, http.StatusConflict, err.Error())
		case errors.Is(err, deploy.ErrPKINotBootstrapped):
			writeErrorMessage(w, http.StatusFailedDependency, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	payload := map[string]any{
		"dry_run":                     result.DryRun,
		"old_version":                 result.OldVersion,
		"new_version":                 result.NewVersion,
		"operator":                    strings.TrimSpace(result.Operator),
		"reason":                      strings.TrimSpace(result.Reason),
		"updated_beacon_certificates": result.UpdatedBeaconCertificates,
		"updated_worker_certificates": result.UpdatedWorkerCertificates,
		"revoked":                     result.Revoked,
	}
	writeJSON(w, http.StatusOK, payload)
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

	var sinceID int64
	if raw := strings.TrimSpace(r.Header.Get("Last-Event-ID")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
			return
		}
		if id > 0 {
			sinceID = id
		}
	}

	if err := logstream.Serve(w, r, s.streams, jobID, sinceID); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
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

func writeError(w http.ResponseWriter, status int, err error) {
	if err == nil {
		writeErrorMessage(w, status, http.StatusText(status))
		return
	}
	writeErrorMessage(w, status, err.Error())
}

func writeErrorMessage(w http.ResponseWriter, status int, message string) {
	if strings.TrimSpace(message) == "" {
		message = http.StatusText(status)
	}
	writeJSON(w, status, map[string]any{"error": message})
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
