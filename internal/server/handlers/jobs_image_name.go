package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

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
// - Only allowed for mig/heal/gate jobs.
func saveJobImageNameHandler(st store.Store) http.HandlerFunc {
	type request struct {
		Image string `json:"image"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		jobID, err := parseParam[domaintypes.JobID](r, "job_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		nodeIDHeaderStr := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
		if nodeIDHeaderStr == "" {
			httpErr(w, http.StatusBadRequest, "PLOY_NODE_UUID header is required")
			return
		}
		var nodeIDHeader domaintypes.NodeID
		if err := nodeIDHeader.UnmarshalText([]byte(nodeIDHeaderStr)); err != nil {
			httpErr(w, http.StatusBadRequest, "invalid PLOY_NODE_UUID header")
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpErr(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		image := strings.TrimSpace(req.Image)
		if image == "" {
			httpErr(w, http.StatusBadRequest, "image is required")
			return
		}

		job, err := st.GetJob(ctx, jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "job not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get job: %v", err)
			slog.Error("save job image name: get job failed", "job_id", jobID, "err", err)
			return
		}

		// Verify ownership: only the assigned node may persist the runtime image name.
		if job.NodeID == nil || *job.NodeID != nodeIDHeader {
			httpErr(w, http.StatusForbidden, "job not assigned to this node")
			return
		}

		// Enforce "before execution starts" semantics: only allow while Running.
		if job.Status != domaintypes.JobStatusRunning {
			httpErr(w, http.StatusConflict, "job status is %s, expected Running", job.Status)
			return
		}

		jobType := domaintypes.JobType(job.JobType)
		switch jobType {
		case domaintypes.JobTypeMod, domaintypes.JobTypeHeal,
			domaintypes.JobTypePreGate, domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
			// allowed
		default:
			httpErr(w, http.StatusConflict, "job type is %s, expected mig/heal/gate", job.JobType)
			return
		}

		if err := st.UpdateJobImageName(ctx, store.UpdateJobImageNameParams{ID: jobID, JobImage: image}); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to save job image: %v", err)
			slog.Error("save job image name: update failed", "job_id", jobID, "node_id", nodeIDHeader, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
