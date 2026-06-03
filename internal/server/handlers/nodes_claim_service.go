package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// claimResult is the domain output from claim orchestration.
type claimResult struct {
	Payload  workClaimPayload
	Response any
}

// claimService orchestrates the claim pipeline.
type claimService struct {
	store         store.Store
	blobStore     blobstore.Store
	configHolder  *ConfigHolder
	eventsService *events.Service
}

func newClaimer(st store.Store, bs blobstore.Store, configHolder *ConfigHolder, eventsService ...*events.Service) *claimService {
	var evtSvc *events.Service
	if len(eventsService) > 0 {
		evtSvc = eventsService[0]
	}
	svc := &claimService{
		store:         st,
		blobStore:     bs,
		configHolder:  configHolder,
		eventsService: evtSvc,
	}
	return svc
}

// claimBadRequest maps to HTTP 400.
type claimBadRequest struct{ Message string }

func (e *claimBadRequest) Error() string { return e.Message }

// claimNotFound maps to HTTP 404.
type claimNotFound struct{ Message string }

func (e *claimNotFound) Error() string { return e.Message }

// claimNoWork maps to HTTP 204.
type claimNoWork struct{}

func (e *claimNoWork) Error() string { return "no work available" }

// claimInternalError maps to HTTP 500.
type claimInternalError struct {
	Message string
	Detail  string
	Err     error
}

func (e *claimInternalError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Detail)
	}
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *claimInternalError) Unwrap() error { return e.Err }

// claimTerminalError marks claim-time payload errors that are deterministic
// for the claimed job payload and must fail the job instead of requeueing it.
type claimTerminalError struct {
	Message string
	Err     error
}

func (e *claimTerminalError) Error() string {
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

func (e *claimTerminalError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func claimInternal(message string, err error) error {
	return &claimInternalError{
		Message: message,
		Detail:  safeErrorString(err),
		Err:     err,
	}
}

func (s *claimService) Claim(ctx context.Context, nodeID domaintypes.NodeID) (claimResult, error) {
	_, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		if isNoRowsError(err) {
			return claimResult{}, &claimNotFound{Message: "node not found"}
		}
		slog.Error("claim: node check failed", "node_id", nodeID, "err_type", fmt.Sprintf("%T", err), "err", safeErrorString(err))
		return claimResult{}, claimInternal("failed to check node", err)
	}

	job, err := s.store.ClaimJob(ctx, nodeID)
	if err != nil {
		if isNoRowsError(err) {
			action, actionErr := s.store.ClaimRunAction(ctx, nodeID)
			if actionErr != nil {
				if isNoRowsError(actionErr) {
					slog.Debug("claim: no work available", "node_id", nodeID)
					return claimResult{}, &claimNoWork{}
				}
				slog.Error("claim: database action-claim error", "node_id", nodeID, "err_type", fmt.Sprintf("%T", actionErr), "err", safeErrorString(actionErr))
				return claimResult{}, claimInternal("failed to claim action", actionErr)
			}

			run, getRunErr := s.store.GetRun(ctx, action.RunID)
			if getRunErr != nil {
				slog.Error("claim: get run failed for action", "node_id", nodeID, "action_id", action.ID, "err", getRunErr)
				return claimResult{}, claimInternal("failed to get run for claimed action", getRunErr)
			}
			repoURL, repoErr := repoURLForID(ctx, s.store, run.RepoID)
			if repoErr != nil {
				slog.Error("claim: get repo failed for action", "node_id", nodeID, "action_id", action.ID, "repo_id", run.RepoID, "err", repoErr)
				return claimResult{}, claimInternal("failed to get repo for claimed action", repoErr)
			}
			spec, specErr := s.store.GetSpec(ctx, run.SpecID)
			if specErr != nil {
				slog.Error("claim: get spec failed for action", "node_id", nodeID, "action_id", action.ID, "spec_id", run.SpecID, "err", specErr)
				return claimResult{}, claimInternal("failed to get spec for claimed action", specErr)
			}

			payload := buildRunActionClaimPayload(spec.Spec, run, repoURL, action)
			slog.Info("action claimed",
				"action_id", action.ID,
				"action_type", action.ActionType,
				"run_id", run.ID,
				"node_id", nodeID,
			)
			return claimResult{Payload: payload}, nil
		}
		slog.Error("claim: database error", "node_id", nodeID, "err_type", fmt.Sprintf("%T", err), "err", safeErrorString(err))
		return claimResult{}, claimInternal("failed to claim job", err)
	}

	run, err := s.store.GetRun(ctx, job.RunID)
	if err != nil {
		slog.Error("claim: get run failed for job", "node_id", nodeID, "job_id", job.ID, "err", err)
		return claimResult{}, claimInternal("failed to get run for claimed job", err)
	}

	claimDecision := lifecycle.EvaluateClaimDecision(domaintypes.JobType(job.JobType), run.Status)

	repoURL, err := repoURLForID(ctx, s.store, job.RepoID)
	if err != nil {
		slog.Error("claim: get repo failed for job", "node_id", nodeID, "job_id", job.ID, "repo_id", job.RepoID, "err", err)
		return claimResult{}, claimInternal("failed to get repo for claimed job", err)
	}

	spec, err := s.store.GetSpec(ctx, run.SpecID)
	if err != nil {
		slog.Error("claim: get spec failed for job", "node_id", nodeID, "job_id", job.ID, "spec_id", run.SpecID, "err", err)
		return claimResult{}, claimInternal("failed to get spec for claimed job", err)
	}

	payload, err := buildJobClaimPayload(ctx, s.store, s.blobStore, s.configHolder, run, spec.Spec, repoURL, job)
	if err != nil {
		slog.Error("claim: failed to build response", "job_id", job.ID, "run_id", run.ID, "err", err)
		var terminalErr *claimTerminalError
		if errors.As(err, &terminalErr) {
			completeSvc := newCompletionService(s.store, nil, nil)
			_, completeErr := completeSvc.Complete(ctx, completionInput{
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
				return claimResult{}, &claimNoWork{}
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
		return claimResult{}, claimInternal("failed to build claim response", err)
	}
	if claimDecision.AdvanceRunToRunning {
		if err := s.store.UpdateRunStatus(ctx, store.UpdateRunStatusParams{
			ID:     job.RunID,
			Status: domaintypes.RunStatusRunning,
		}); err != nil {
			slog.Error("claim: failed to transition run to Running", "node_id", nodeID, "job_id", job.ID, "run_id", job.RunID, "repo_id", job.RepoID, "err", err)
		}
	}
	slog.Info("job claimed",
		"job_id", job.ID,
		"run_id", run.ID,
		"next_id", job.NextID,
		"node_id", nodeID,
	)
	return claimResult{Payload: payload}, nil
}
