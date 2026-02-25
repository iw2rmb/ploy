package recovery

import (
	"context"
	"errors"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

type staleKey struct {
	runID   domaintypes.RunID
	repoID  domaintypes.MigRepoID
	attempt int32
}

type mockStore struct {
	store.Store

	listStaleRunningJobsCalled bool
	listStaleRunningJobsParam  pgtype.Timestamptz
	listStaleRunningJobsResult []store.ListStaleRunningJobsRow
	listStaleRunningJobsErr    error

	cancelCalls      []store.CancelActiveJobsByRunRepoAttemptParams
	cancelRowsResult int64
	cancelRowsErr    error

	jobsByAttempt map[staleKey][]store.Job

	updateRunRepoStatusCalled bool
	updateRunRepoStatusParams []store.UpdateRunRepoStatusParams
	updateRunRepoStatusErr    error

	countRunReposByStatusCalled bool
	countRunReposByStatusResult map[domaintypes.RunID][]store.CountRunReposByStatusRow
	countRunReposByStatusErr    error

	getRunCalled bool
	runsByID     map[domaintypes.RunID]store.Run
	getRunErr    error

	updateRunStatusCalled bool
	updateRunStatusParams []store.UpdateRunStatusParams
	updateRunStatusErr    error
}

func (m *mockStore) ListStaleRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	m.listStaleRunningJobsCalled = true
	m.listStaleRunningJobsParam = lastHeartbeat
	return m.listStaleRunningJobsResult, m.listStaleRunningJobsErr
}

func (m *mockStore) CancelActiveJobsByRunRepoAttempt(ctx context.Context, arg store.CancelActiveJobsByRunRepoAttemptParams) (int64, error) {
	m.cancelCalls = append(m.cancelCalls, arg)
	return m.cancelRowsResult, m.cancelRowsErr
}

func (m *mockStore) ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	if m.jobsByAttempt == nil {
		return nil, nil
	}
	return m.jobsByAttempt[staleKey{runID: arg.RunID, repoID: arg.RepoID, attempt: arg.Attempt}], nil
}

func (m *mockStore) UpdateRunRepoStatus(ctx context.Context, arg store.UpdateRunRepoStatusParams) error {
	m.updateRunRepoStatusCalled = true
	m.updateRunRepoStatusParams = append(m.updateRunRepoStatusParams, arg)
	return m.updateRunRepoStatusErr
}

func (m *mockStore) CountRunReposByStatus(ctx context.Context, runID domaintypes.RunID) ([]store.CountRunReposByStatusRow, error) {
	m.countRunReposByStatusCalled = true
	if m.countRunReposByStatusErr != nil {
		return nil, m.countRunReposByStatusErr
	}
	if m.countRunReposByStatusResult == nil {
		return nil, nil
	}
	return m.countRunReposByStatusResult[runID], nil
}

func (m *mockStore) GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error) {
	m.getRunCalled = true
	if m.getRunErr != nil {
		return store.Run{}, m.getRunErr
	}
	if m.runsByID == nil {
		return store.Run{}, nil
	}
	return m.runsByID[id], nil
}

func (m *mockStore) UpdateRunStatus(ctx context.Context, arg store.UpdateRunStatusParams) error {
	m.updateRunStatusCalled = true
	m.updateRunStatusParams = append(m.updateRunStatusParams, arg)
	return m.updateRunStatusErr
}

func TestNewStaleJobRecoveryTask(t *testing.T) {
	t.Parallel()

	t.Run("requires store", func(t *testing.T) {
		t.Parallel()
		task, err := NewStaleJobRecoveryTask(Options{})
		if !errors.Is(err, ErrNilStore) {
			t.Fatalf("error = %v, want %v", err, ErrNilStore)
		}
		if task != nil {
			t.Fatalf("expected nil task, got %#v", task)
		}
	})

	t.Run("applies defaults", func(t *testing.T) {
		t.Parallel()
		task, err := NewStaleJobRecoveryTask(Options{Store: &mockStore{}})
		if err != nil {
			t.Fatalf("NewStaleJobRecoveryTask() error = %v", err)
		}
		if task.Interval() != defaultStaleJobRecoveryInterval {
			t.Fatalf("interval = %v, want %v", task.Interval(), defaultStaleJobRecoveryInterval)
		}
		if task.nodeStaleAfter != defaultNodeStaleAfter {
			t.Fatalf("nodeStaleAfter = %v, want %v", task.nodeStaleAfter, defaultNodeStaleAfter)
		}
	})
}

func TestStaleJobRecoveryTask_Run_CompletesRunWhenReposTerminal(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	st := &mockStore{
		listStaleRunningJobsResult: []store.ListStaleRunningJobsRow{
			{RunID: runID, RepoID: repoID, Attempt: 2, RunningJobs: 3},
		},
		cancelRowsResult: 3,
		jobsByAttempt: map[staleKey][]store.Job{
			{runID: runID, repoID: repoID, attempt: 2}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     2,
					Name:        "mig-0",
					Status:      store.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMod.String(),
					Meta:        []byte(`{"next_id":2000}`),
				},
			},
		},
		countRunReposByStatusResult: map[domaintypes.RunID][]store.CountRunReposByStatusRow{
			runID: {
				{Status: store.RunRepoStatusCancelled, Count: 1},
			},
		},
		runsByID: map[domaintypes.RunID]store.Run{
			runID: {ID: runID, Status: store.RunStatusStarted},
		},
	}

	task, err := NewStaleJobRecoveryTask(Options{
		Store:          st,
		Interval:       25 * time.Millisecond,
		NodeStaleAfter: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewStaleJobRecoveryTask() error = %v", err)
	}

	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !st.listStaleRunningJobsCalled {
		t.Fatal("expected ListStaleRunningJobs call")
	}
	if !st.listStaleRunningJobsParam.Valid {
		t.Fatal("expected cutoff timestamp to be valid")
	}
	if len(st.cancelCalls) != 1 {
		t.Fatalf("cancel calls = %d, want 1", len(st.cancelCalls))
	}
	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus call")
	}
	if len(st.updateRunRepoStatusParams) != 1 {
		t.Fatalf("repo status updates = %d, want 1", len(st.updateRunRepoStatusParams))
	}
	if st.updateRunRepoStatusParams[0].Status != store.RunRepoStatusCancelled {
		t.Fatalf("repo status = %q, want %q", st.updateRunRepoStatusParams[0].Status, store.RunRepoStatusCancelled)
	}
	if !st.getRunCalled {
		t.Fatal("expected GetRun call")
	}
	if !st.countRunReposByStatusCalled {
		t.Fatal("expected CountRunReposByStatus call")
	}
	if !st.updateRunStatusCalled {
		t.Fatal("expected UpdateRunStatus call")
	}
	if len(st.updateRunStatusParams) != 1 {
		t.Fatalf("run status updates = %d, want 1", len(st.updateRunStatusParams))
	}
	if st.updateRunStatusParams[0].Status != store.RunStatusFinished {
		t.Fatalf("run status = %q, want %q", st.updateRunStatusParams[0].Status, store.RunStatusFinished)
	}
}

func TestStaleJobRecoveryTask_Run_DoesNotCompleteRunWhenOtherReposNonTerminal(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	st := &mockStore{
		listStaleRunningJobsResult: []store.ListStaleRunningJobsRow{{RunID: runID, RepoID: repoID, Attempt: 1, RunningJobs: 1}},
		cancelRowsResult:           1,
		jobsByAttempt: map[staleKey][]store.Job{
			{runID: runID, repoID: repoID, attempt: 1}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     1,
					Name:        "mig-0",
					Status:      store.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMod.String(),
					Meta:        []byte(`{"next_id":1000}`),
				},
			},
		},
		countRunReposByStatusResult: map[domaintypes.RunID][]store.CountRunReposByStatusRow{
			runID: {
				{Status: store.RunRepoStatusCancelled, Count: 1},
				{Status: store.RunRepoStatusRunning, Count: 1},
			},
		},
		runsByID: map[domaintypes.RunID]store.Run{
			runID: {ID: runID, Status: store.RunStatusStarted},
		},
	}

	task, err := NewStaleJobRecoveryTask(Options{Store: st, Interval: time.Second, NodeStaleAfter: time.Minute})
	if err != nil {
		t.Fatalf("NewStaleJobRecoveryTask() error = %v", err)
	}

	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus call")
	}
	if st.updateRunStatusCalled {
		t.Fatal("did not expect UpdateRunStatus while run has non-terminal repos")
	}
}
