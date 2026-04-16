package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

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
	if parentJob.Status != domaintypes.JobStatusRunning {
		writeHTTPError(w, http.StatusConflict, "parent job status is %s, expected Running", parentJob.Status)
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
