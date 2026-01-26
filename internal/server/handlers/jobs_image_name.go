package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// saveJobImageNameHandler persists the container image name that the node will use
// to execute a job. This allows the control plane to surface the exact runtime
// image (including stack-aware resolution) via jobs.mod_image.
//
// Endpoint: POST /v1/jobs/{job_id}/image
//
// Security/validation:
// - Requires PLOY_NODE_UUID header and enforces that the job is assigned to that node.
// - Only allowed while the job is in Running status.
// - Only allowed for mod/heal/gate jobs.
func saveJobImageNameHandler(st store.Store) http.HandlerFunc {
	type request struct {
		Image string `json:"image"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		jobID, err := domaintypes.ParseJobIDParam(r, "job_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		nodeIDHeaderStr := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
		if nodeIDHeaderStr == "" {
			http.Error(w, "PLOY_NODE_UUID header is required", http.StatusBadRequest)
			return
		}
		var nodeIDHeader domaintypes.NodeID
		if err := nodeIDHeader.UnmarshalText([]byte(nodeIDHeaderStr)); err != nil {
			http.Error(w, "invalid PLOY_NODE_UUID header", http.StatusBadRequest)
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		image := strings.TrimSpace(req.Image)
		if image == "" {
			http.Error(w, "image is required", http.StatusBadRequest)
			return
		}

		job, err := st.GetJob(ctx, jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
			slog.Error("save job image name: get job failed", "job_id", jobID, "err", err)
			return
		}

		// Verify ownership: only the assigned node may persist the runtime image name.
		if job.NodeID == nil || *job.NodeID != nodeIDHeader {
			http.Error(w, "job not assigned to this node", http.StatusForbidden)
			return
		}

		// Enforce "before execution starts" semantics: only allow while Running.
		if job.Status != store.JobStatusRunning {
			http.Error(w, fmt.Sprintf("job status is %s, expected Running", job.Status), http.StatusConflict)
			return
		}

		modType := domaintypes.ModType(job.ModType)
		switch modType {
		case domaintypes.ModTypeMod, domaintypes.ModTypeHeal,
			domaintypes.ModTypePreGate, domaintypes.ModTypePostGate, domaintypes.ModTypeReGate:
			// allowed
		default:
			http.Error(w, fmt.Sprintf("job mod_type is %s, expected mod/heal/gate", job.ModType), http.StatusConflict)
			return
		}

		if err := st.UpdateJobImageName(ctx, store.UpdateJobImageNameParams{ID: jobID, ModImage: image}); err != nil {
			http.Error(w, fmt.Sprintf("failed to save job image: %v", err), http.StatusInternalServerError)
			slog.Error("save job image name: update failed", "job_id", jobID, "node_id", nodeIDHeader, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
