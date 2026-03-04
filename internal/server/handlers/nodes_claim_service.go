package handlers

import (
	"context"
	"fmt"
	"log/slog"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// ClaimResult is the domain output from claim orchestration.
type ClaimResult struct {
	Payload claimResponsePayload
}

// ClaimService orchestrates the claim pipeline.
type ClaimService struct {
	store        store.Store
	configHolder *ConfigHolder
	gateResolver GateProfileResolver
}

func NewClaimService(st store.Store, configHolder *ConfigHolder, resolver GateProfileResolver) *ClaimService {
	return &ClaimService{
		store:        st,
		configHolder: configHolder,
		gateResolver: resolver,
	}
}

// ClaimBadRequest maps to HTTP 400.
type ClaimBadRequest struct{ Message string }

func (e *ClaimBadRequest) Error() string { return e.Message }

// ClaimNotFound maps to HTTP 404.
type ClaimNotFound struct{ Message string }

func (e *ClaimNotFound) Error() string { return e.Message }

// ClaimNoWork maps to HTTP 204.
type ClaimNoWork struct{}

func (e *ClaimNoWork) Error() string { return "no work available" }

// ClaimInternal maps to HTTP 500.
type ClaimInternal struct {
	Message string
	Detail  string
	Err     error
}

func (e *ClaimInternal) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Detail)
	}
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *ClaimInternal) Unwrap() error { return e.Err }

func claimInternal(message string, err error) error {
	return &ClaimInternal{
		Message: message,
		Detail:  safeErrorString(err),
		Err:     err,
	}
}

func (s *ClaimService) Claim(ctx context.Context, nodeID domaintypes.NodeID) (ClaimResult, error) {
	_, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		if isNoRowsError(err) {
			return ClaimResult{}, &ClaimNotFound{Message: "node not found"}
		}
		slog.Error("claim: node check failed", "node_id", nodeID, "err_type", fmt.Sprintf("%T", err), "err", safeErrorString(err))
		return ClaimResult{}, claimInternal("failed to check node", err)
	}

	job, err := s.store.ClaimJob(ctx, nodeID)
	if err != nil {
		if isNoRowsError(err) {
			slog.Debug("claim: no work available", "node_id", nodeID)
			return ClaimResult{}, &ClaimNoWork{}
		}
		slog.Error("claim: database error", "node_id", nodeID, "err_type", fmt.Sprintf("%T", err), "err", safeErrorString(err))
		return ClaimResult{}, claimInternal("failed to claim job", err)
	}

	run, err := s.store.GetRun(ctx, job.RunID)
	if err != nil {
		slog.Error("claim: get run failed for job", "node_id", nodeID, "job_id", job.ID, "err", err)
		return ClaimResult{}, claimInternal("failed to get run for claimed job", err)
	}

	rr, err := s.store.GetRunRepo(ctx, store.GetRunRepoParams{RunID: job.RunID, RepoID: job.RepoID})
	if err != nil {
		slog.Error("claim: get run repo failed for job", "node_id", nodeID, "job_id", job.ID, "err", err)
		return ClaimResult{}, claimInternal("failed to get run repo for claimed job", err)
	}

	isMRJob := job.JobType == domaintypes.JobTypeMR
	if !isMRJob && rr.Status == domaintypes.RunRepoStatusQueued {
		if err := s.store.UpdateRunRepoStatus(ctx, store.UpdateRunRepoStatusParams{
			RunID:  job.RunID,
			RepoID: job.RepoID,
			Status: domaintypes.RunRepoStatusRunning,
		}); err != nil {
			slog.Error("claim: failed to transition run repo to Running", "node_id", nodeID, "job_id", job.ID, "run_id", job.RunID, "repo_id", job.RepoID, "err", err)
		}
	}

	repoURL, err := repoURLForID(ctx, s.store, job.RepoID)
	if err != nil {
		slog.Error("claim: get repo failed for job", "node_id", nodeID, "job_id", job.ID, "repo_id", job.RepoID, "err", err)
		return ClaimResult{}, claimInternal("failed to get repo for claimed job", err)
	}

	spec, err := s.store.GetSpec(ctx, run.SpecID)
	if err != nil {
		slog.Error("claim: get spec failed for job", "node_id", nodeID, "job_id", job.ID, "spec_id", run.SpecID, "err", err)
		return ClaimResult{}, claimInternal("failed to get spec for claimed job", err)
	}

	payload, err := buildClaimResponsePayload(ctx, s.store, s.configHolder, run, spec.Spec, rr, repoURL, job, s.gateResolver)
	if err != nil {
		slog.Error("claim: failed to build response", "job_id", job.ID, "run_id", run.ID, "err", err)
		return ClaimResult{}, claimInternal("failed to build claim response", err)
	}

	slog.Info("job claimed",
		"job_id", job.ID,
		"job_name", job.Name,
		"run_id", run.ID,
		"next_id", job.NextID,
		"node_id", nodeID,
	)
	return ClaimResult{Payload: payload}, nil
}
