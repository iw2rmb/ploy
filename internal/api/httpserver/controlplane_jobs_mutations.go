package httpserver

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
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
		Gate       *struct {
			Result          string  `json:"result"`
			DurationSeconds float64 `json:"duration_seconds"`
		} `json:"gate"`
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var gateMetrics *scheduler.GateMetrics
	if req.Gate != nil {
		d := time.Duration(req.Gate.DurationSeconds * float64(time.Second))
		if d < 0 {
			d = 0
		}
		gateMetrics = &scheduler.GateMetrics{
			Result:   req.Gate.Result,
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
		Gate:       gateMetrics,
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

	// Notify Mods orchestrator about stage completion to drive dependent
	// scheduling. This supplements background watchers and ensures timely
	// transitions even when watchers are disabled.
	if s.mods != nil {
		state := controlplanemods.JobCompletionState(strings.ToLower(string(job.State)))
		// Map completion state to orchestrator variants; ignore inspection_ready here.
		switch job.State {
		case scheduler.JobStateSucceeded:
			state = controlplanemods.JobCompletionSucceeded
		case scheduler.JobStateFailed:
			state = controlplanemods.JobCompletionFailed
		default:
			// Treat unsupported states as failed to trigger retry/cancel paths if any.
			if state == "" {
				state = controlplanemods.JobCompletionFailed
			}
		}
		var errMsg string
		if req.Error != nil && strings.TrimSpace(req.Error.Message) != "" {
			errMsg = req.Error.Message
		}
		_ = s.mods.ProcessJobCompletion(r.Context(), controlplanemods.JobCompletion{
			TicketID:  job.Ticket,
			StageID:   job.StepID,
			JobID:     job.ID,
			State:     state,
			Error:     errMsg,
			Artifacts: job.Artifacts,
		})
	}
}
