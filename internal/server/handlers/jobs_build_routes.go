package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

const buildKindReGate = "re_gate"

type createJobBuildRequest struct {
	BuildKind string `json:"build_kind"`
	Reason    string `json:"reason"`
}

type createJobBuildResponse struct {
	ChildJobID domaintypes.JobID `json:"child_job_id"`
	StatusURL  string            `json:"status_url"`
	Status     string            `json:"status"`
}

type getJobBuildResponse struct {
	JobID    domaintypes.JobID `json:"job_id"`
	Status   string            `json:"status"`
	Terminal bool              `json:"terminal"`
	Success  bool              `json:"success"`
}

// createJobBuildHandler is a worker-facing contract endpoint for creating child
// build jobs under a parent mig/heal job.
//
// Endpoint: POST /v1/jobs/{parent_job_id}/builds
func createJobBuildHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parentJobID, err := parseRequiredPathID[domaintypes.JobID](r, "parent_job_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		var req createJobBuildRequest
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}
		if strings.TrimSpace(req.BuildKind) != buildKindReGate {
			writeHTTPError(w, http.StatusBadRequest, "build_kind must be %q", buildKindReGate)
			return
		}
		if strings.TrimSpace(req.Reason) == "" {
			writeHTTPError(w, http.StatusBadRequest, "reason is required")
			return
		}

		parentJob, ok := authorizeParentBuildJob(w, r, st, parentJobID)
		if !ok {
			return
		}
		childJob, err := createJobBuildReGateChild(r.Context(), st, parentJob, req.BuildKind)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to create child build job: %v", err)
			return
		}

		status := projectJobBuildStatus(childJob.Status)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createJobBuildResponse{
			ChildJobID: childJob.ID,
			StatusURL:  childBuildStatusURL(r, parentJobID, childJob.ID),
			Status:     status.Status,
		})
	}
}

// getJobBuildStatusHandler is a worker-facing status polling endpoint for child
// build jobs under a parent mig/heal job.
//
// Endpoint: GET /v1/jobs/{parent_job_id}/builds/{child_job_id}
func getJobBuildStatusHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parentJobID, err := parseRequiredPathID[domaintypes.JobID](r, "parent_job_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		childJobID, err := parseRequiredPathID[domaintypes.JobID](r, "child_job_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		parentJob, ok := authorizeParentBuildJob(w, r, st, parentJobID)
		if !ok {
			return
		}

		childJob, err := getLinkedJobBuildChild(r.Context(), st, parentJob, childJobID)
		if err != nil {
			writeJobBuildStatusLookupError(w, err)
			return
		}

		status := projectJobBuildStatus(childJob.Status)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(getJobBuildResponse{
			JobID:    childJob.ID,
			Status:   status.Status,
			Terminal: status.Terminal,
			Success:  status.Success,
		})
	}
}

func childBuildStatusURL(r *http.Request, parentJobID, childJobID domaintypes.JobID) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/v1/jobs/%s/builds/%s", scheme, r.Host, parentJobID, childJobID)
}
