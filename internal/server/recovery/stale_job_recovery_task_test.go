package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/testutil/workflowkit/ids"
	"github.com/jackc/pgx/v5/pgtype"
)

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
		task, err := NewStaleJobRecoveryTask(Options{Store: &staleTaskStore{}})
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
	repoID := domaintypes.NewRepoID()
	st := &staleTaskStore{
		StaleRows: []store.ListStaleRunningJobsRow{
			{RunID: runID, RepoID: repoID, Attempt: 2, RunningJobs: 3},
		},
		StaleNodesCount:  1,
		CancelRowsResult: 3,
		JobsByAttempt: map[ids.AttemptKey][]store.Job{
			{RunID: runID, RepoID: repoID, Attempt: 2}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     2,
					Name:        "mig-0",
					Status:      domaintypes.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMig,
					Meta:        []byte(`{"next_id":2000}`),
				},
			},
		},
		CountByStatus: map[domaintypes.RunID][]store.CountRunReposByStatusRow{
			runID: {
				{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
			},
		},
		RunsByID: map[domaintypes.RunID]store.Run{
			runID: {ID: runID, Status: domaintypes.RunStatusStarted},
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

	if !st.StaleJobsParam.Valid {
		t.Fatal("expected ListStaleRunningJobs call")
	}
	if !st.StaleNodeParam.Valid {
		t.Fatal("expected CountStaleNodesWithRunningJobs call")
	}
	if len(st.CancelCalls) != 1 {
		t.Fatalf("cancel calls = %d, want 1", len(st.CancelCalls))
	}
	if len(st.UpdateRepoCalls) == 0 {
		t.Fatal("expected UpdateRunRepoStatus call")
	}
	if len(st.UpdateRepoCalls) != 1 {
		t.Fatalf("repo status updates = %d, want 1", len(st.UpdateRepoCalls))
	}
	if st.UpdateRepoCalls[0].Status != domaintypes.RunRepoStatusCancelled {
		t.Fatalf("repo status = %q, want %q", st.UpdateRepoCalls[0].Status, domaintypes.RunRepoStatusCancelled)
	}
	if !st.GetRunCalled {
		t.Fatal("expected GetRun call")
	}
	if !st.CountStatusCalled {
		t.Fatal("expected CountRunReposByStatus call")
	}
	if len(st.UpdateRunCalls) == 0 {
		t.Fatal("expected UpdateRunStatus call")
	}
	if len(st.UpdateRunCalls) != 1 {
		t.Fatalf("run status updates = %d, want 1", len(st.UpdateRunCalls))
	}
	if st.UpdateRunCalls[0].Status != domaintypes.RunStatusFinished {
		t.Fatalf("run status = %q, want %q", st.UpdateRunCalls[0].Status, domaintypes.RunStatusFinished)
	}
}

func TestStaleJobRecoveryTask_Run_DoesNotCompleteRunWhenOtherReposNonTerminal(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	st := &staleTaskStore{
		StaleRows: []store.ListStaleRunningJobsRow{
			{RunID: runID, RepoID: repoID, Attempt: 1, RunningJobs: 1},
		},
		StaleNodesCount:  1,
		CancelRowsResult: 1,
		JobsByAttempt: map[ids.AttemptKey][]store.Job{
			{RunID: runID, RepoID: repoID, Attempt: 1}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     1,
					Name:        "mig-0",
					Status:      domaintypes.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMig,
					Meta:        []byte(`{"next_id":1000}`),
				},
			},
		},
		CountByStatus: map[domaintypes.RunID][]store.CountRunReposByStatusRow{
			runID: {
				{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
				{Status: domaintypes.RunRepoStatusRunning, Count: 1},
			},
		},
		RunsByID: map[domaintypes.RunID]store.Run{
			runID: {ID: runID, Status: domaintypes.RunStatusStarted},
		},
	}

	task, err := NewStaleJobRecoveryTask(Options{Store: st, Interval: time.Second, NodeStaleAfter: time.Minute})
	if err != nil {
		t.Fatalf("NewStaleJobRecoveryTask() error = %v", err)
	}

	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(st.UpdateRepoCalls) == 0 {
		t.Fatal("expected UpdateRunRepoStatus call")
	}
	if len(st.UpdateRunCalls) > 0 {
		t.Fatal("did not expect UpdateRunStatus while run has non-terminal repos")
	}
}

func TestStaleJobRecoveryTask_Run_LogsCycleCounters(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	st := &staleTaskStore{
		StaleRows: []store.ListStaleRunningJobsRow{
			{RunID: runID, RepoID: repoID, Attempt: 1, RunningJobs: 2},
		},
		StaleNodesCount:  2,
		CancelRowsResult: 2,
		JobsByAttempt: map[ids.AttemptKey][]store.Job{
			{RunID: runID, RepoID: repoID, Attempt: 1}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     1,
					Name:        "mig-0",
					Status:      domaintypes.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMig,
					Meta:        []byte(`{"next_id":1000}`),
				},
			},
		},
		CountByStatus: map[domaintypes.RunID][]store.CountRunReposByStatusRow{
			runID: {
				{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
			},
		},
		RunsByID: map[domaintypes.RunID]store.Run{
			runID: {ID: runID, Status: domaintypes.RunStatusStarted},
		},
	}

	var logBuf bytes.Buffer
	task, err := NewStaleJobRecoveryTask(Options{
		Store:          st,
		Interval:       10 * time.Millisecond,
		NodeStaleAfter: time.Minute,
		Logger:         slog.New(slog.NewJSONHandler(&logBuf, nil)),
	})
	if err != nil {
		t.Fatalf("NewStaleJobRecoveryTask() error = %v", err)
	}

	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
	if len(lines) == 0 {
		t.Fatal("expected recovery logs, got none")
	}

	foundCycleLog := false
	for _, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("unmarshal log payload: %v", err)
		}
		if payload["msg"] != "stale-job-recovery: cycle completed" {
			continue
		}
		foundCycleLog = true
		if got := int64(payload["stale_nodes"].(float64)); got != 2 {
			t.Fatalf("stale_nodes=%d, want 2", got)
		}
		if got := int64(payload["stale_attempts"].(float64)); got != 1 {
			t.Fatalf("stale_attempts=%d, want 1", got)
		}
		if got := int64(payload["jobs_cancelled"].(float64)); got != 2 {
			t.Fatalf("jobs_cancelled=%d, want 2", got)
		}
		if got := int64(payload["repos_updated"].(float64)); got != 1 {
			t.Fatalf("repos_updated=%d, want 1", got)
		}
		if got := int64(payload["runs_finalized"].(float64)); got != 1 {
			t.Fatalf("runs_finalized=%d, want 1", got)
		}
	}

	if !foundCycleLog {
		t.Fatal("expected stale-job-recovery cycle log")
	}
}

func TestStaleJobRecoveryTask_Run_EmitsTerminalSSEOnlyOncePerRun(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoA := domaintypes.NewRepoID()
	repoB := domaintypes.NewRepoID()
	st := &staleTaskStore{
		StaleRows: []store.ListStaleRunningJobsRow{
			{RunID: runID, RepoID: repoA, Attempt: 1, RunningJobs: 1},
			{RunID: runID, RepoID: repoB, Attempt: 1, RunningJobs: 1},
		},
		StaleNodesCount:  1,
		CancelRowsResult: 1,
		JobsByAttempt: map[ids.AttemptKey][]store.Job{
			{RunID: runID, RepoID: repoA, Attempt: 1}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoA,
					RepoBaseRef: "main",
					Attempt:     1,
					Name:        "mig-a",
					Status:      domaintypes.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMig,
					Meta:        []byte(`{"next_id":1000}`),
				},
			},
			{RunID: runID, RepoID: repoB, Attempt: 1}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoB,
					RepoBaseRef: "main",
					Attempt:     1,
					Name:        "mig-b",
					Status:      domaintypes.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMig,
					Meta:        []byte(`{"next_id":2000}`),
				},
			},
		},
		CountByStatus: map[domaintypes.RunID][]store.CountRunReposByStatusRow{
			runID: {
				{Status: domaintypes.RunRepoStatusCancelled, Count: 2},
			},
		},
		RunsByID: map[domaintypes.RunID]store.Run{
			runID: {ID: runID, Status: domaintypes.RunStatusStarted},
		},
	}

	eventsService, err := server.NewEventsService(server.EventsOptions{
		BufferSize:  10,
		HistorySize: 20,
	})
	if err != nil {
		t.Fatalf("NewEventsService() error = %v", err)
	}

	task, err := NewStaleJobRecoveryTask(Options{
		Store:          st,
		EventsService:  eventsService,
		Interval:       10 * time.Millisecond,
		NodeStaleAfter: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewStaleJobRecoveryTask() error = %v", err)
	}

	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(st.UpdateRunCalls) != 1 {
		t.Fatalf("run status updates = %d, want 1", len(st.UpdateRunCalls))
	}

	events := eventsService.Hub().Snapshot(runID)
	var (
		runEvents  int
		doneEvents int
	)
	for _, evt := range events {
		if evt.Type == domaintypes.SSEEventRun {
			runEvents++
		}
		if evt.Type == domaintypes.SSEEventDone {
			doneEvents++
		}
	}
	if runEvents != 1 {
		t.Fatalf("run events=%d, want 1", runEvents)
	}
	if doneEvents != 1 {
		t.Fatalf("done events=%d, want 1", doneEvents)
	}
}

type staleTaskStore struct {
	store.Store

	StaleRows          []store.ListStaleRunningJobsRow
	StaleNodesCount    int64
	StaleNodesErr      error
	CancelRowsResult   int64
	CancelErr          error
	JobsByAttempt      map[ids.AttemptKey][]store.Job
	RunsByID           map[domaintypes.RunID]store.Run
	GetRunErr          error
	CountByStatus      map[domaintypes.RunID][]store.CountRunReposByStatusRow
	CountByStatusErr   error
	UpdateRepoErr      error
	UpdateRunErr       error
	RunReposResult     []store.RunRepo
	RunReposErr        error
	RunReposWithURL    []store.ListRunReposWithURLByRunRow
	RunReposWithURLErr error
	MigRepoResult      store.MigRepo
	MigRepoErr         error

	StaleJobsParam    pgtype.Timestamptz
	StaleNodeParam    pgtype.Timestamptz
	GetRunCalled      bool
	CountStatusCalled bool
	CancelCalls       []store.CancelActiveJobsByRunRepoAttemptParams
	UpdateRepoCalls   []store.UpdateRunRepoStatusParams
	UpdateRunCalls    []store.UpdateRunStatusParams
}

func (s *staleTaskStore) ListStaleRunningJobs(_ context.Context, lastHeartbeat pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	s.StaleJobsParam = lastHeartbeat
	return s.StaleRows, nil
}

func (s *staleTaskStore) CountStaleNodesWithRunningJobs(_ context.Context, lastHeartbeat pgtype.Timestamptz) (int64, error) {
	s.StaleNodeParam = lastHeartbeat
	return s.StaleNodesCount, s.StaleNodesErr
}

func (s *staleTaskStore) CancelActiveJobsByRunRepoAttempt(_ context.Context, arg store.CancelActiveJobsByRunRepoAttemptParams) (int64, error) {
	s.CancelCalls = append(s.CancelCalls, arg)
	return s.CancelRowsResult, s.CancelErr
}

func (s *staleTaskStore) ListJobsByRunRepoAttempt(_ context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	if s.JobsByAttempt == nil {
		return nil, nil
	}
	return s.JobsByAttempt[ids.AttemptKey{RunID: arg.RunID, RepoID: arg.RepoID, Attempt: arg.Attempt}], nil
}

func (s *staleTaskStore) UpdateRunRepoStatus(_ context.Context, arg store.UpdateRunRepoStatusParams) error {
	s.UpdateRepoCalls = append(s.UpdateRepoCalls, arg)
	return s.UpdateRepoErr
}

func (s *staleTaskStore) CountRunReposByStatus(_ context.Context, runID domaintypes.RunID) ([]store.CountRunReposByStatusRow, error) {
	s.CountStatusCalled = true
	if s.CountByStatusErr != nil {
		return nil, s.CountByStatusErr
	}
	if s.CountByStatus == nil {
		return nil, nil
	}
	return s.CountByStatus[runID], nil
}

func (s *staleTaskStore) GetRun(_ context.Context, id domaintypes.RunID) (store.Run, error) {
	s.GetRunCalled = true
	if s.GetRunErr != nil {
		return store.Run{}, s.GetRunErr
	}
	if s.RunsByID == nil {
		return store.Run{}, nil
	}
	return s.RunsByID[id], nil
}

func (s *staleTaskStore) UpdateRunStatus(_ context.Context, arg store.UpdateRunStatusParams) error {
	s.UpdateRunCalls = append(s.UpdateRunCalls, arg)
	if s.RunsByID != nil {
		run := s.RunsByID[arg.ID]
		run.ID = arg.ID
		run.Status = arg.Status
		s.RunsByID[arg.ID] = run
	}
	return s.UpdateRunErr
}

func (s *staleTaskStore) ListRunReposByRun(_ context.Context, _ domaintypes.RunID) ([]store.RunRepo, error) {
	return s.RunReposResult, s.RunReposErr
}

func (s *staleTaskStore) ListRunReposWithURLByRun(_ context.Context, runID domaintypes.RunID) ([]store.ListRunReposWithURLByRunRow, error) {
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

func (s *staleTaskStore) GetMigRepo(_ context.Context, _ domaintypes.MigRepoID) (store.MigRepo, error) {
	return s.MigRepoResult, s.MigRepoErr
}
