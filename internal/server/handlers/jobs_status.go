package handlers

import (
	"net/http"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

// getJobStatusHandler returns canonical job status for worker cancellation
// polling and operator inspection. Worker callers must prove ownership through
// PLOY_NODE_UUID; control-plane callers can inspect by job ID.
func getJobStatusHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseRequiredPathIDOrWriteError[domaintypes.JobID](w, r, "job_id")
		if !ok {
			return
		}

		identity, hasIdentity := auth.IdentityFromContext(r.Context())
		workerScoped := !hasIdentity || identity.Role == auth.RoleWorker
		var nodeIDHeader domaintypes.NodeID
		if workerScoped {
			var ok bool
			nodeIDHeader, ok = requireNodeUUIDHeader(w, r)
			if !ok {
				return
			}
		}

		job, ok := getJobOrFail(w, r, st, jobID, "get job status")
		if !ok {
			return
		}

		if workerScoped && !assertJobAssignedToNode(w, job, nodeIDHeader) {
			return
		}

		writeJSON(w, http.StatusOK, jobStatusFromStore(job))
	}
}

func jobStatusFromStore(job store.Job) domainapi.JobStatusResponse {
	return domainapi.JobStatusResponse{
		JobID:       job.ID,
		RunID:       job.RunID,
		RepoID:      job.RepoID,
		Attempt:     job.Attempt,
		Name:        job.Name,
		JobType:     job.JobType,
		Status:      job.Status,
		JobImage:    job.JobImage,
		NodeID:      job.NodeID,
		ExitCode:    job.ExitCode,
		StartedAt:   timestamptzTime(job.StartedAt.Valid, job.StartedAt.Time),
		FinishedAt:  timestamptzTime(job.FinishedAt.Valid, job.FinishedAt.Time),
		DurationMs:  job.DurationMs,
		RepoShaIn:   job.RepoShaIn,
		RepoShaOut:  job.RepoShaOut,
		RepoShaIn8:  job.RepoShaIn8,
		RepoShaOut8: job.RepoShaOut8,
	}
}

func timestamptzTime(valid bool, value time.Time) *time.Time {
	if !valid {
		return nil
	}
	return &value
}
