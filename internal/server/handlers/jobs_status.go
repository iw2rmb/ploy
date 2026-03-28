package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// getJobStatusHandler returns canonical job status for worker-side cancellation polling.
// Ownership is enforced: only the node assigned to the job can read status.
func getJobStatusHandler(st store.Store) http.HandlerFunc {
	type response struct {
		JobID  domaintypes.JobID `json:"job_id"`
		Status string            `json:"status"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := parseRequiredPathID[domaintypes.JobID](r, "job_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		nodeIDHeaderStr := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
		if nodeIDHeaderStr == "" {
			writeHTTPError(w, http.StatusBadRequest, "PLOY_NODE_UUID header is required")
			return
		}
		var nodeIDHeader domaintypes.NodeID
		if err := nodeIDHeader.UnmarshalText([]byte(nodeIDHeaderStr)); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "invalid PLOY_NODE_UUID header")
			return
		}

		job, err := st.GetJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "job not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get job: %v", err)
			return
		}

		if job.NodeID == nil || *job.NodeID != nodeIDHeader {
			writeHTTPError(w, http.StatusForbidden, "job not assigned to this node")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response{
			JobID:  job.ID,
			Status: string(job.Status),
		})
	}
}
