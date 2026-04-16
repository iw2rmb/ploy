package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

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
	ChildJobID domaintypes.JobID `json:"child_job_id"`
	Status     string            `json:"status"`
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

		if _, ok := authorizeParentBuildJob(w, r, st, parentJobID); !ok {
			return
		}

		// The route contract is now worker-visible; persistence and status
		// projection are implemented in the follow-up step.
		writeHTTPError(w, http.StatusNotImplemented, "child-build creation is not implemented yet")
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
		if _, err := parseRequiredPathID[domaintypes.JobID](r, "child_job_id"); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		if _, ok := authorizeParentBuildJob(w, r, st, parentJobID); !ok {
			return
		}

		writeHTTPError(w, http.StatusNotImplemented, "child-build status polling is not implemented yet")
	}
}

func authorizeParentBuildJob(w http.ResponseWriter, r *http.Request, st store.Store, parentJobID domaintypes.JobID) (store.Job, bool) {
	nodeIDHeaderStr := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
	if nodeIDHeaderStr == "" {
		writeHTTPError(w, http.StatusBadRequest, "PLOY_NODE_UUID header is required")
		return store.Job{}, false
	}
	var nodeIDHeader domaintypes.NodeID
	if err := nodeIDHeader.UnmarshalText([]byte(nodeIDHeaderStr)); err != nil {
		writeHTTPError(w, http.StatusBadRequest, "invalid PLOY_NODE_UUID header")
		return store.Job{}, false
	}

	parentJob, err := st.GetJob(r.Context(), parentJobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeHTTPError(w, http.StatusNotFound, "parent job not found")
			return store.Job{}, false
		}
		writeHTTPError(w, http.StatusInternalServerError, "failed to get parent job: %v", err)
		return store.Job{}, false
	}
	if parentJob.NodeID == nil || *parentJob.NodeID != nodeIDHeader {
		writeHTTPError(w, http.StatusForbidden, "parent job not assigned to this node")
		return store.Job{}, false
	}

	parentJobType := domaintypes.JobType(parentJob.JobType)
	switch parentJobType {
	case domaintypes.JobTypeMig, domaintypes.JobTypeHeal:
		return parentJob, true
	default:
		writeHTTPError(w, http.StatusConflict, "parent job type is %s, expected mig/heal", parentJob.JobType)
		return store.Job{}, false
	}
}
