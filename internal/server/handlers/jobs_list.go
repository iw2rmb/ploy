package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// listJobsHandler returns a paginated, optionally filtered list of jobs with mig context.
// GET /v1/jobs
// Query params: ?limit=N&offset=N&run_id=<id>&node_id=<id>&status=<status> (all optional)
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
		nodeID, err := optionalQuery[domaintypes.NodeID](r, "node_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		var status *domaintypes.JobStatus
		if rawStatus := strings.TrimSpace(r.URL.Query().Get("status")); rawStatus != "" {
			parsed, err := domaintypes.ParseJobStatus(rawStatus)
			if err != nil {
				writeHTTPError(w, http.StatusBadRequest, "status: %s", err)
				return
			}
			status = &parsed
		}

		var runIDStr, nodeIDStr, statusStr *string
		if runID != nil {
			s := runID.String()
			runIDStr = &s
		}
		if nodeID != nil {
			s := nodeID.String()
			nodeIDStr = &s
		}
		if status != nil {
			s := status.String()
			statusStr = &s
		}

		filters := store.CountJobsPageParams{
			RunID:  runIDStr,
			NodeID: nodeIDStr,
			Status: statusStr,
		}
		jobs, err := st.ListJobsPage(r.Context(), store.ListJobsPageParams{
			Limit:  limit,
			Offset: offset,
			RunID:  filters.RunID,
			NodeID: filters.NodeID,
			Status: filters.Status,
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list jobs: %v", err)
			slog.Error("list jobs: fetch failed", "err", err)
			return
		}

		total, err := st.CountJobsPage(r.Context(), filters)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to count jobs: %v", err)
			slog.Error("list jobs: count failed", "err", err)
			return
		}

		items := make([]domainapi.JobListItem, 0, len(jobs))
		for _, j := range jobs {
			items = append(items, domainapi.JobListItem{
				JobID:      j.JobID,
				Name:       j.Name,
				JobType:    j.JobType,
				Status:     j.Status,
				DurationMs: j.DurationMs,
				JobImage:   j.JobImage,
				NodeID:     j.NodeID,
				MigName:    j.MigName,
				RunID:      j.RunID,
				RepoID:     j.RepoID,
			})
		}

		resp := domainapi.JobListResponse{
			Jobs:  items,
			Total: total,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
