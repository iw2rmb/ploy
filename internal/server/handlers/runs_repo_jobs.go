package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/jobchain"
)

// RunRepoJobResponse represents a job within a repo execution.
type RunRepoJobResponse struct {
	JobID         domaintypes.JobID   `json:"job_id"`
	Name          string              `json:"name"`
	JobType       string              `json:"job_type"`
	JobImage      string              `json:"job_image"`
	NextID        *domaintypes.JobID  `json:"next_id"`
	NodeID        *domaintypes.NodeID `json:"node_id"`
	Status        store.JobStatus     `json:"status"`
	ExitCode      *int32              `json:"exit_code,omitempty"`
	StartedAt     *time.Time          `json:"started_at,omitempty"`
	FinishedAt    *time.Time          `json:"finished_at,omitempty"`
	DurationMs    int64               `json:"duration_ms"`
	DisplayName   string              `json:"display_name,omitempty"`
	ActionSummary string              `json:"action_summary,omitempty"`
	BugSummary    string              `json:"bug_summary,omitempty"`
	Recovery      *RecoveryView       `json:"recovery,omitempty"`
}

// RecoveryView projects universal recovery loop metadata in repo job APIs.
type RecoveryView struct {
	LoopKind   string   `json:"loop_kind"`
	ErrorKind  string   `json:"error_kind"`
	StrategyID string   `json:"strategy_id,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// ListRunRepoJobsResponse is the response for GET /v1/runs/{run_id}/repos/{repo_id}/jobs.
type ListRunRepoJobsResponse struct {
	RunID   domaintypes.RunID     `json:"run_id"`
	RepoID  domaintypes.MigRepoID `json:"repo_id"`
	Attempt int32                 `json:"attempt"`
	Jobs    []RunRepoJobResponse  `json:"jobs"`
}

// listRunRepoJobsHandler returns jobs for a specific repo execution within a run.
// GET /v1/runs/{run_id}/repos/{repo_id}/jobs
// Query params: ?attempt=N (optional, defaults to current attempt)
func listRunRepoJobsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "run_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}
		repoID, err := parseParam[domaintypes.MigRepoID](r, "repo_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				httpErr(w, http.StatusNotFound, "repo not found")
			default:
				httpErr(w, http.StatusInternalServerError, "failed to get repo: %v", err)
				slog.Error("list run repo jobs: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			}
			return
		}

		// Use attempt from query param if provided, otherwise use current attempt.
		attempt := rr.Attempt
		if q := r.URL.Query().Get("attempt"); q != "" {
			parsed, err := strconv.ParseInt(q, 10, 32)
			if err != nil {
				httpErr(w, http.StatusBadRequest, "invalid attempt parameter")
				return
			}
			attempt = int32(parsed)
		}

		jobs, err := st.ListJobsByRunRepoAttempt(r.Context(), store.ListJobsByRunRepoAttemptParams{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: attempt,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list jobs: %v", err)
			slog.Error("list run repo jobs: list jobs failed", "run_id", runID.String(), "repo_id", repoID.String(), "attempt", attempt, "err", err)
			return
		}
		jobs = jobchain.Order(
			jobs,
			func(job store.Job) domaintypes.JobID { return job.ID },
			func(job store.Job) *domaintypes.JobID { return job.NextID },
		)

		resp := ListRunRepoJobsResponse{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: attempt,
			Jobs:    make([]RunRepoJobResponse, 0, len(jobs)),
		}

		for _, job := range jobs {
			jr := RunRepoJobResponse{
				JobID:      job.ID,
				Name:       job.Name,
				JobType:    job.JobType,
				JobImage:   job.JobImage,
				NextID:     job.NextID,
				NodeID:     job.NodeID,
				Status:     job.Status,
				ExitCode:   job.ExitCode,
				DurationMs: job.DurationMs,
			}

			// Extract projection fields from structured job metadata.
			if len(job.Meta) > 0 {
				meta, err := contracts.UnmarshalJobMeta(job.Meta)
				if err == nil {
					if meta.ModsStepName != "" {
						jr.DisplayName = meta.ModsStepName
					}
					if meta.ActionSummary != "" {
						jr.ActionSummary = meta.ActionSummary
					}
					if meta.Gate != nil && strings.TrimSpace(meta.Gate.BugSummary) != "" {
						jr.BugSummary = strings.TrimSpace(meta.Gate.BugSummary)
					}
					if meta.Recovery != nil {
						jr.Recovery = newRecoveryView(meta.Recovery)
					} else if meta.Gate != nil && meta.Gate.Recovery != nil {
						jr.Recovery = newRecoveryView(meta.Gate.Recovery)
					}
				}
			}

			// Set timestamps.
			if job.StartedAt.Valid {
				t := job.StartedAt.Time.UTC()
				jr.StartedAt = &t
			}
			if job.FinishedAt.Valid {
				t := job.FinishedAt.Time.UTC()
				jr.FinishedAt = &t
			}

			resp.Jobs = append(resp.Jobs, jr)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list run repo jobs: encode response failed", "err", err)
		}
	}
}

func newRecoveryView(meta *contracts.BuildGateRecoveryMetadata) *RecoveryView {
	if meta == nil {
		return nil
	}
	return &RecoveryView{
		LoopKind:   meta.LoopKind,
		ErrorKind:  meta.ErrorKind,
		StrategyID: meta.StrategyID,
		Confidence: meta.Confidence,
		Reason:     meta.Reason,
	}
}
