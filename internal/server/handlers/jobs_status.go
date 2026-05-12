package handlers

import (
	"net/http"

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
		jobID, ok := parseRequiredPathIDOrWriteError[domaintypes.JobID](w, r, "job_id")
		if !ok {
			return
		}

		nodeIDHeader, ok := requireNodeUUIDHeader(w, r)
		if !ok {
			return
		}

		job, ok := getJobOrFail(w, r, st, jobID, "get job status")
		if !ok {
			return
		}

		if !assertJobAssignedToNode(w, job, nodeIDHeader) {
			return
		}

		writeJSON(w, http.StatusOK, response{
			JobID:  job.ID,
			Status: string(job.Status),
		})
	}
}
