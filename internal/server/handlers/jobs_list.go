package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// listJobsHandler returns a paginated, optionally run-filtered list of jobs with mig context.
// GET /v1/jobs
// Query params: ?limit=N&offset=N&run_id=<id> (all optional)
func listJobsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset, err := parsePagination(r)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		runID, err := optionalQuery[domaintypes.RunID](r, "run_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Convert typed RunID pointer to plain string pointer for store query.
		var runIDStr *string
		if runID != nil {
			s := runID.String()
			runIDStr = &s
		}

		jobs, err := st.ListJobsForTUI(r.Context(), store.ListJobsForTUIParams{
			Limit:  limit,
			Offset: offset,
			RunID:  runIDStr,
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list jobs: %v", err)
			slog.Error("list jobs: fetch failed", "err", err)
			return
		}

		total, err := st.CountJobsForTUI(r.Context(), runIDStr)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to count jobs: %v", err)
			slog.Error("list jobs: count failed", "err", err)
			return
		}

		type jobItem struct {
			JobID      domaintypes.JobID     `json:"job_id"`
			Name       string                `json:"name"`
			Status     domaintypes.JobStatus `json:"status"`
			DurationMs int64                 `json:"duration_ms"`
			JobImage   string                `json:"job_image"`
			NodeID     *domaintypes.NodeID   `json:"node_id"`
			MigName    string                `json:"mig_name"`
			RunID      domaintypes.RunID     `json:"run_id"`
			RepoID     domaintypes.RepoID    `json:"repo_id"`
		}

		items := make([]jobItem, 0, len(jobs))
		for _, j := range jobs {
			items = append(items, jobItem{
				JobID:      j.JobID,
				Name:       j.Name,
				Status:     j.Status,
				DurationMs: j.DurationMs,
				JobImage:   j.JobImage,
				NodeID:     j.NodeID,
				MigName:    j.MigName,
				RunID:      j.RunID,
				RepoID:     j.RepoID,
			})
		}

		resp := struct {
			Jobs  []jobItem `json:"jobs"`
			Total int64     `json:"total"`
		}{
			Jobs:  items,
			Total: total,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
