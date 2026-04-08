package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// ClaimResult is the domain output from claim orchestration.
type ClaimResult struct {
	Payload claimResponsePayload
}

// ClaimService orchestrates the claim pipeline.
type ClaimService struct {
	store        store.Store
	blobStore    blobstore.Store
	configHolder *ConfigHolder
	gateResolver GateProfileResolver
}

func NewClaimService(st store.Store, bs blobstore.Store, configHolder *ConfigHolder, resolver GateProfileResolver) *ClaimService {
	return &ClaimService{
		store:        st,
		blobStore:    bs,
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

// ClaimJobTerminalError marks claim-time payload errors that are deterministic
// for the claimed job payload and must fail the job instead of requeueing it.
type ClaimJobTerminalError struct {
	Message string
	Err     error
}

func (e *ClaimJobTerminalError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Message
	}
	if e.Message == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *ClaimJobTerminalError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

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
			action, actionErr := s.store.ClaimRunRepoAction(ctx, nodeID)
			if actionErr != nil {
				if isNoRowsError(actionErr) {
					slog.Debug("claim: no work available", "node_id", nodeID)
					return ClaimResult{}, &ClaimNoWork{}
				}
				slog.Error("claim: database action-claim error", "node_id", nodeID, "err_type", fmt.Sprintf("%T", actionErr), "err", safeErrorString(actionErr))
				return ClaimResult{}, claimInternal("failed to claim action", actionErr)
			}

			run, getRunErr := s.store.GetRun(ctx, action.RunID)
			if getRunErr != nil {
				slog.Error("claim: get run failed for action", "node_id", nodeID, "action_id", action.ID, "err", getRunErr)
				return ClaimResult{}, claimInternal("failed to get run for claimed action", getRunErr)
			}
			rr, getRunRepoErr := s.store.GetRunRepo(ctx, store.GetRunRepoParams{RunID: action.RunID, RepoID: action.RepoID})
			if getRunRepoErr != nil {
				slog.Error("claim: get run repo failed for action", "node_id", nodeID, "action_id", action.ID, "err", getRunRepoErr)
				return ClaimResult{}, claimInternal("failed to get run repo for claimed action", getRunRepoErr)
			}
			repoURL, repoErr := repoURLForID(ctx, s.store, action.RepoID)
			if repoErr != nil {
				slog.Error("claim: get repo failed for action", "node_id", nodeID, "action_id", action.ID, "repo_id", action.RepoID, "err", repoErr)
				return ClaimResult{}, claimInternal("failed to get repo for claimed action", repoErr)
			}
			spec, specErr := s.store.GetSpec(ctx, run.SpecID)
			if specErr != nil {
				slog.Error("claim: get spec failed for action", "node_id", nodeID, "action_id", action.ID, "spec_id", run.SpecID, "err", specErr)
				return ClaimResult{}, claimInternal("failed to get spec for claimed action", specErr)
			}

			payload := buildActionClaimResponsePayload(run, spec.Spec, rr, repoURL, action)
			slog.Info("action claimed",
				"action_id", action.ID,
				"action_type", action.ActionType,
				"run_id", run.ID,
				"node_id", nodeID,
			)
			return ClaimResult{Payload: payload}, nil
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

	claimDecision := lifecycle.EvaluateClaimDecision(domaintypes.JobType(job.JobType), rr.Status)

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

	payload, err := buildClaimResponsePayload(ctx, s.store, s.blobStore, s.configHolder, run, spec.Spec, rr, repoURL, job, s.gateResolver)
	if err != nil {
		slog.Error("claim: failed to build response", "job_id", job.ID, "run_id", run.ID, "err", err)
		var terminalErr *ClaimJobTerminalError
		if errors.As(err, &terminalErr) {
			completeSvc := NewCompleteJobService(s.store, nil, nil, nil)
			_, completeErr := completeSvc.Complete(ctx, CompleteJobInput{
				JobID:        job.ID,
				NodeID:       nodeID,
				Status:       domaintypes.JobStatusError,
				StatsPayload: JobStatsPayload{Error: terminalErr.Error()},
			})
			if completeErr == nil {
				slog.Error("claim: marked claimed job as Error due terminal claim payload error",
					"job_id", job.ID,
					"run_id", run.ID,
					"node_id", nodeID,
					"error", terminalErr.Error(),
				)
				return ClaimResult{}, &ClaimNoWork{}
			}
			slog.Error("claim: failed to mark claimed job as Error after terminal payload error",
				"job_id", job.ID,
				"run_id", run.ID,
				"node_id", nodeID,
				"claim_err", terminalErr.Error(),
				"complete_err", completeErr,
			)
		}
		if unclaimErr := s.store.UnclaimJob(ctx, store.UnclaimJobParams{
			ID:     job.ID,
			NodeID: nodeID,
		}); unclaimErr != nil {
			slog.Error("claim: failed to unclaim job after payload build error", "job_id", job.ID, "run_id", run.ID, "node_id", nodeID, "err", unclaimErr)
		}
		return ClaimResult{}, claimInternal("failed to build claim response", err)
	}
	if claimDecision.AdvanceRunRepoToRunning {
		if err := s.store.UpdateRunRepoStatus(ctx, store.UpdateRunRepoStatusParams{
			RunID:  job.RunID,
			RepoID: job.RepoID,
			Status: domaintypes.RunRepoStatusRunning,
		}); err != nil {
			slog.Error("claim: failed to transition run repo to Running", "node_id", nodeID, "job_id", job.ID, "run_id", job.RunID, "repo_id", job.RepoID, "err", err)
		}
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
