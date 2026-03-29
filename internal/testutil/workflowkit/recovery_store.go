// Package workflowkit is the canonical owner for cross-module run/repo/job
// scenario builders used in recovery and orchestration tests.
package workflowkit

import (
	"context"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/testutil/workflowkit/ids"
	"github.com/jackc/pgx/v5/pgtype"
)

// AttemptKey is a cycle-safe alias for ids.AttemptKey, re-exported here so
// existing callers (server/recovery tests) need not change their imports.
type AttemptKey = ids.AttemptKey

// RecoveryStore implements store.Store for recovery scenario tests.
// It is the canonical fixture for testing claim/complete/recover/heal
// orchestration flows in server/recovery and cross-module tests.
//
// Populate the response fields before calling the code under test.
// After the call, inspect the tracked-call fields for assertions.
type RecoveryStore struct {
	store.Store

	// --- configurable responses ---

	StaleRows        []store.ListStaleRunningJobsRow
	StaleNodesCount  int64
	StaleNodesErr    error
	CancelRowsResult int64
	CancelErr        error
	JobsByAttempt    map[AttemptKey][]store.Job
	RunsByID         map[domaintypes.RunID]store.Run
	GetRunErr        error
	CountByStatus    map[domaintypes.RunID][]store.CountRunReposByStatusRow
	CountByStatusErr error
	UpdateRepoErr    error
	UpdateRunErr     error
	RunReposResult   []store.RunRepo
	RunReposErr      error
	RunReposWithURL    []store.ListRunReposWithURLByRunRow
	RunReposWithURLErr error
	MigRepoResult    store.MigRepo
	MigRepoErr       error

	// --- tracked calls ---

	// StaleJobsParam is set when ListStaleRunningJobs is called.
	// Its Valid field indicates whether the method was called.
	StaleJobsParam pgtype.Timestamptz
	// StaleNodeParam is set when CountStaleNodesWithRunningJobs is called.
	StaleNodeParam    pgtype.Timestamptz
	GetRunCalled      bool
	CountStatusCalled bool
	CancelCalls       []store.CancelActiveJobsByRunRepoAttemptParams
	UpdateRepoCalls   []store.UpdateRunRepoStatusParams
	UpdateRunCalls    []store.UpdateRunStatusParams
}

func (s *RecoveryStore) ListStaleRunningJobs(_ context.Context, lastHeartbeat pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	s.StaleJobsParam = lastHeartbeat
	return s.StaleRows, nil
}

func (s *RecoveryStore) CountStaleNodesWithRunningJobs(_ context.Context, lastHeartbeat pgtype.Timestamptz) (int64, error) {
	s.StaleNodeParam = lastHeartbeat
	return s.StaleNodesCount, s.StaleNodesErr
}

func (s *RecoveryStore) CancelActiveJobsByRunRepoAttempt(_ context.Context, arg store.CancelActiveJobsByRunRepoAttemptParams) (int64, error) {
	s.CancelCalls = append(s.CancelCalls, arg)
	return s.CancelRowsResult, s.CancelErr
}

func (s *RecoveryStore) ListJobsByRunRepoAttempt(_ context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	if s.JobsByAttempt == nil {
		return nil, nil
	}
	return s.JobsByAttempt[AttemptKey{RunID: arg.RunID, RepoID: arg.RepoID, Attempt: arg.Attempt}], nil
}

func (s *RecoveryStore) UpdateRunRepoStatus(_ context.Context, arg store.UpdateRunRepoStatusParams) error {
	s.UpdateRepoCalls = append(s.UpdateRepoCalls, arg)
	return s.UpdateRepoErr
}

func (s *RecoveryStore) CountRunReposByStatus(_ context.Context, runID domaintypes.RunID) ([]store.CountRunReposByStatusRow, error) {
	s.CountStatusCalled = true
	if s.CountByStatusErr != nil {
		return nil, s.CountByStatusErr
	}
	if s.CountByStatus == nil {
		return nil, nil
	}
	return s.CountByStatus[runID], nil
}

func (s *RecoveryStore) GetRun(_ context.Context, id domaintypes.RunID) (store.Run, error) {
	s.GetRunCalled = true
	if s.GetRunErr != nil {
		return store.Run{}, s.GetRunErr
	}
	if s.RunsByID == nil {
		return store.Run{}, nil
	}
	return s.RunsByID[id], nil
}

func (s *RecoveryStore) UpdateRunStatus(_ context.Context, arg store.UpdateRunStatusParams) error {
	s.UpdateRunCalls = append(s.UpdateRunCalls, arg)
	if s.RunsByID != nil {
		run := s.RunsByID[arg.ID]
		run.ID = arg.ID
		run.Status = arg.Status
		s.RunsByID[arg.ID] = run
	}
	return s.UpdateRunErr
}

func (s *RecoveryStore) ListRunReposByRun(_ context.Context, _ domaintypes.RunID) ([]store.RunRepo, error) {
	return s.RunReposResult, s.RunReposErr
}

func (s *RecoveryStore) ListRunReposWithURLByRun(_ context.Context, runID domaintypes.RunID) ([]store.ListRunReposWithURLByRunRow, error) {
	if s.RunReposWithURLErr != nil {
		return nil, s.RunReposWithURLErr
	}
	if len(s.RunReposWithURL) > 0 {
		return s.RunReposWithURL, nil
	}
	if len(s.RunReposResult) > 0 {
		var rows []store.ListRunReposWithURLByRunRow
		for _, rr := range s.RunReposResult {
			if rr.RunID != runID {
				continue
			}
			rows = append(rows, store.ListRunReposWithURLByRunRow{
				RunID:         rr.RunID,
				RepoID:        rr.RepoID,
				RepoBaseRef:   rr.RepoBaseRef,
				RepoTargetRef: rr.RepoTargetRef,
				Status:        rr.Status,
				Attempt:       rr.Attempt,
				CreatedAt:     rr.CreatedAt,
				StartedAt:     rr.StartedAt,
				FinishedAt:    rr.FinishedAt,
				RepoUrl:       "https://github.com/user/repo.git",
			})
		}
		if len(rows) > 0 {
			return rows, nil
		}
	}
	for _, stale := range s.StaleRows {
		if stale.RunID == runID {
			return []store.ListRunReposWithURLByRunRow{
				{RunID: runID, RepoID: stale.RepoID, RepoUrl: "https://github.com/user/repo.git"},
			}, nil
		}
	}
	return nil, nil
}

func (s *RecoveryStore) GetMigRepo(_ context.Context, _ domaintypes.MigRepoID) (store.MigRepo, error) {
	return s.MigRepoResult, s.MigRepoErr
}
