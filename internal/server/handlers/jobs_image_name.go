package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// saveJobImageNameHandler persists the container image name that the node will use
// to execute a job. This allows the control plane to surface the exact runtime
// image (including stack-aware resolution) via jobs.job_image.
//
// Endpoint: POST /v1/jobs/{job_id}/image
//
// Security/validation:
// - Requires PLOY_NODE_UUID header and enforces that the job is assigned to that node.
// - Only allowed while the job is in Running status.
// - Only allowed for containerized runtime jobs (mig/gate).
func saveJobImageNameHandler(st store.Store) http.HandlerFunc {
	type request struct {
		Image string `json:"image"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		jobID, ok := parseRequiredPathIDOrWriteError[domaintypes.JobID](w, r, "job_id")
		if !ok {
			return
		}

		nodeIDHeader, ok := requireNodeUUIDHeader(w, r)
		if !ok {
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		image := strings.TrimSpace(req.Image)
		if image == "" {
			writeHTTPError(w, http.StatusBadRequest, "image is required")
			return
		}

		job, ok := getJobOrFail(w, r, st, jobID, "save job image name")
		if !ok {
			return
		}

		if !assertJobAssignedToNode(w, job, nodeIDHeader) {
			return
		}

		// Enforce "before execution starts" semantics: only allow while Running.
		if job.Status != domaintypes.JobStatusRunning {
			writeHTTPError(w, http.StatusConflict, "job status is %s, expected Running", job.Status)
			return
		}

		jobType := domaintypes.JobType(job.JobType)
		switch jobType {
		case domaintypes.JobTypeMig,
			domaintypes.JobTypePreGate, domaintypes.JobTypePostGate:
			// allowed
		default:
			writeHTTPError(w, http.StatusConflict, "job type is %s, expected mig/gate", job.JobType)
			return
		}

		if err := st.UpdateJobImageName(ctx, store.UpdateJobImageNameParams{ID: jobID, JobImage: image}); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to save job image: %v", err)
			slog.Error("save job image name: update failed", "job_id", jobID, "node_id", nodeIDHeader, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
