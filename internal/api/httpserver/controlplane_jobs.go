package httpserver

import (
	"context"
	"errors"
	"fmt"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	clientv3 "go.etcd.io/etcd/client/v3"
	"io"
	"net/http"
	"strings"
	"time"
)

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
