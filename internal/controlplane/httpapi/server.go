package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/config/gitlab"
	"github.com/iw2rmb/ploy/internal/controlplane/events"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/node/logstream"
)

// Server exposes the control-plane scheduler over HTTP.
type Server struct {
	scheduler *scheduler.Scheduler
	signer    *gitlab.Signer
	rotations *events.RotationHub
	streams   *logstream.Hub
}

// New returns an HTTP handler rooted at /v2.
func New(s *scheduler.Scheduler, signer *gitlab.Signer, streams *logstream.Hub) http.Handler {
	mux := http.NewServeMux()
	h := &Server{scheduler: s, signer: signer, streams: streams}
	if signer != nil {
		h.rotations = events.NewRotationHub(context.Background(), signer)
	}
	mux.HandleFunc("/v2/jobs", h.handleJobs)
	mux.HandleFunc("/v2/jobs/claim", h.handleClaim)
	mux.HandleFunc("/v2/jobs/", h.handleJobSubpath)
	mux.HandleFunc("/v2/health", h.handleHealth)
	mux.HandleFunc("/v2/gitlab/signer/secrets", h.handleSignerSecrets)
	mux.HandleFunc("/v2/gitlab/signer/tokens", h.handleSignerTokens)
	mux.HandleFunc("/v2/gitlab/signer/rotations", h.handleSignerRotations)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleJobSubmit(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleJobList(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleJobSubpath(w http.ResponseWriter, r *http.Request) {
	if !s.ensureScheduler(w) {
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/v2/jobs/")
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

func (s *Server) handleJobGet(w http.ResponseWriter, r *http.Request, jobID string) {
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

func (s *Server) handleJobHeartbeat(w http.ResponseWriter, r *http.Request, jobID string) {
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

func (s *Server) handleJobComplete(w http.ResponseWriter, r *http.Request, jobID string) {
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
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
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
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, jobDTOFrom(job))
}

func (s *Server) handleJobLogs(w http.ResponseWriter, r *http.Request, jobID string, parts []string) {
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

func (s *Server) handleJobLogsStream(w http.ResponseWriter, r *http.Request, jobID string) {
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

func (s *Server) handleSignerSecrets(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleSignerTokens(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleSignerRotations(w http.ResponseWriter, r *http.Request) {
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
	LeaseID        int64                             `json:"lease_id,omitempty"`
	LeaseExpiresAt string                            `json:"lease_expires_at,omitempty"`
	ClaimedBy      string                            `json:"claimed_by,omitempty"`
	RetryAttempt   int                               `json:"retry_attempt"`
	MaxAttempts    int                               `json:"max_attempts"`
	Metadata       map[string]string                 `json:"metadata,omitempty"`
	Artifacts      map[string]string                 `json:"artifacts,omitempty"`
	Bundles        map[string]scheduler.BundleRecord `json:"bundles,omitempty"`
	Retention      *scheduler.JobRetention           `json:"retention,omitempty"`
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
		LeaseID:        int64(job.LeaseID),
		LeaseExpiresAt: formatTime(job.LeaseExpiresAt),
		ClaimedBy:      job.ClaimedBy,
		RetryAttempt:   job.RetryAttempt,
		MaxAttempts:    job.MaxAttempts,
		Metadata:       copyMap(job.Metadata),
		Artifacts:      copyMap(job.Artifacts),
		Bundles:        copyBundles(job.Bundles),
		Retention:      copyRetention(job.Retention),
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

func (s *Server) ensureScheduler(w http.ResponseWriter) bool {
	if s.scheduler == nil {
		http.Error(w, "scheduler unavailable", http.StatusServiceUnavailable)
		return false
	}
	return true
}
