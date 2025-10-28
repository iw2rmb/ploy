package httpserver

import (
    "errors"
    "log"
    "net/http"
    "strings"
    "time"

    "github.com/iw2rmb/ploy/internal/controlplane/scheduler"
    "github.com/iw2rmb/ploy/internal/node/logstream"
)

// handleJobSubmit enqueues a new job via the scheduler.
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

// handleClaim assigns the next runnable job to a node.
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
			log.Printf("control-plane: no jobs available for node %s", strings.TrimSpace(req.NodeID))
			writeJSON(w, http.StatusOK, map[string]any{"status": "empty"})
			return
		}
		log.Printf("control-plane: claim error for node %s: %v", strings.TrimSpace(req.NodeID), err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	log.Printf("control-plane: node %s claimed job %s (ticket=%s)", res.NodeID, res.Job.ID, res.Job.Ticket)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "claimed",
		"node_id": res.NodeID,
		"job":     jobDTOFrom(res.Job),
	})
}

// handleJobHeartbeat records liveness for a claimed job.
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

// handleJobComplete finalizes a job and updates scheduler state and artifacts.
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
    if s.streams != nil {
        status := "completed"
        switch job.State {
        case scheduler.JobStateFailed:
            status = "failed"
        case scheduler.JobStateInspectionReady:
            status = "inspection_ready"
        }
        _ = s.streams.PublishStatus(r.Context(), jobID, logstream.Status{Status: status})
    }
}
