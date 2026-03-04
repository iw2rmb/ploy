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
	"github.com/jackc/pgx/v5/pgtype"
)

type staleKey struct {
	runID   domaintypes.RunID
	repoID  domaintypes.RepoID
	attempt int32
}

type mockStore struct {
	store.Store

	listStaleRunningJobsCalled bool
	listStaleRunningJobsParam  pgtype.Timestamptz
	listStaleRunningJobsResult []store.ListStaleRunningJobsRow
	listStaleRunningJobsErr    error

	countStaleNodesWithRunningJobsCalled bool
	countStaleNodesWithRunningJobsParam  pgtype.Timestamptz
	countStaleNodesWithRunningJobsResult int64
	countStaleNodesWithRunningJobsErr    error

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

	listRunReposByRunCalled bool
	listRunReposByRunResult []store.RunRepo
	listRunReposByRunErr    error

	listRunReposWithURLByRunCalled bool
	listRunReposWithURLByRunResult []store.ListRunReposWithURLByRunRow
	listRunReposWithURLByRunErr    error

	getMigRepoCalled bool
	getMigRepoResult store.MigRepo
	getMigRepoErr    error
}

func (m *mockStore) ListStaleRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	m.listStaleRunningJobsCalled = true
	m.listStaleRunningJobsParam = lastHeartbeat
	return m.listStaleRunningJobsResult, m.listStaleRunningJobsErr
}

func (m *mockStore) CountStaleNodesWithRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) (int64, error) {
	m.countStaleNodesWithRunningJobsCalled = true
	m.countStaleNodesWithRunningJobsParam = lastHeartbeat
	return m.countStaleNodesWithRunningJobsResult, m.countStaleNodesWithRunningJobsErr
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
	if m.runsByID != nil {
		run := m.runsByID[arg.ID]
		run.ID = arg.ID
		run.Status = arg.Status
		m.runsByID[arg.ID] = run
	}
	return m.updateRunStatusErr
}

func (m *mockStore) ListRunReposByRun(ctx context.Context, runID domaintypes.RunID) ([]store.RunRepo, error) {
	m.listRunReposByRunCalled = true
	return m.listRunReposByRunResult, m.listRunReposByRunErr
}

func (m *mockStore) ListRunReposWithURLByRun(ctx context.Context, runID domaintypes.RunID) ([]store.ListRunReposWithURLByRunRow, error) {
	m.listRunReposWithURLByRunCalled = true
	if m.listRunReposWithURLByRunErr != nil {
		return nil, m.listRunReposWithURLByRunErr
	}
	if len(m.listRunReposWithURLByRunResult) > 0 {
		return m.listRunReposWithURLByRunResult, nil
	}
	if len(m.listRunReposByRunResult) > 0 {
		rows := make([]store.ListRunReposWithURLByRunRow, 0, len(m.listRunReposByRunResult))
		for _, rr := range m.listRunReposByRunResult {
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
	for _, stale := range m.listStaleRunningJobsResult {
		if stale.RunID == runID {
			return []store.ListRunReposWithURLByRunRow{
				{
					RunID:   runID,
					RepoID:  stale.RepoID,
					RepoUrl: "https://github.com/user/repo.git",
				},
			}, nil
		}
	}
	return nil, nil
}

func (m *mockStore) GetMigRepo(ctx context.Context, id domaintypes.MigRepoID) (store.MigRepo, error) {
	m.getMigRepoCalled = true
	return m.getMigRepoResult, m.getMigRepoErr
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
	repoID := domaintypes.NewRepoID()
	st := &mockStore{
		listStaleRunningJobsResult: []store.ListStaleRunningJobsRow{
			{RunID: runID, RepoID: repoID, Attempt: 2, RunningJobs: 3},
		},
		countStaleNodesWithRunningJobsResult: 1,
		cancelRowsResult:                     3,
		jobsByAttempt: map[staleKey][]store.Job{
			{runID: runID, repoID: repoID, attempt: 2}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     2,
					Name:        "mig-0",
					Status:      domaintypes.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMod,
					Meta:        []byte(`{"next_id":2000}`),
				},
			},
		},
		countRunReposByStatusResult: map[domaintypes.RunID][]store.CountRunReposByStatusRow{
			runID: {
				{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
			},
		},
		runsByID: map[domaintypes.RunID]store.Run{
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

	if !st.listStaleRunningJobsCalled {
		t.Fatal("expected ListStaleRunningJobs call")
	}
	if !st.listStaleRunningJobsParam.Valid {
		t.Fatal("expected cutoff timestamp to be valid")
	}
	if !st.countStaleNodesWithRunningJobsCalled {
		t.Fatal("expected CountStaleNodesWithRunningJobs call")
	}
	if !st.countStaleNodesWithRunningJobsParam.Valid {
		t.Fatal("expected stale node cutoff timestamp to be valid")
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
	if st.updateRunRepoStatusParams[0].Status != domaintypes.RunRepoStatusCancelled {
		t.Fatalf("repo status = %q, want %q", st.updateRunRepoStatusParams[0].Status, domaintypes.RunRepoStatusCancelled)
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
	if st.updateRunStatusParams[0].Status != domaintypes.RunStatusFinished {
		t.Fatalf("run status = %q, want %q", st.updateRunStatusParams[0].Status, domaintypes.RunStatusFinished)
	}
}

func TestStaleJobRecoveryTask_Run_DoesNotCompleteRunWhenOtherReposNonTerminal(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	st := &mockStore{
		listStaleRunningJobsResult:           []store.ListStaleRunningJobsRow{{RunID: runID, RepoID: repoID, Attempt: 1, RunningJobs: 1}},
		countStaleNodesWithRunningJobsResult: 1,
		cancelRowsResult:                     1,
		jobsByAttempt: map[staleKey][]store.Job{
			{runID: runID, repoID: repoID, attempt: 1}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     1,
					Name:        "mig-0",
					Status:      domaintypes.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMod,
					Meta:        []byte(`{"next_id":1000}`),
				},
			},
		},
		countRunReposByStatusResult: map[domaintypes.RunID][]store.CountRunReposByStatusRow{
			runID: {
				{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
				{Status: domaintypes.RunRepoStatusRunning, Count: 1},
			},
		},
		runsByID: map[domaintypes.RunID]store.Run{
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

	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus call")
	}
	if st.updateRunStatusCalled {
		t.Fatal("did not expect UpdateRunStatus while run has non-terminal repos")
	}
}

func TestStaleJobRecoveryTask_Run_LogsCycleCounters(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	st := &mockStore{
		listStaleRunningJobsResult: []store.ListStaleRunningJobsRow{
			{RunID: runID, RepoID: repoID, Attempt: 1, RunningJobs: 2},
		},
		countStaleNodesWithRunningJobsResult: 2,
		cancelRowsResult:                     2,
		jobsByAttempt: map[staleKey][]store.Job{
			{runID: runID, repoID: repoID, attempt: 1}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     1,
					Name:        "mig-0",
					Status:      domaintypes.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMod,
					Meta:        []byte(`{"next_id":1000}`),
				},
			},
		},
		countRunReposByStatusResult: map[domaintypes.RunID][]store.CountRunReposByStatusRow{
			runID: {
				{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
			},
		},
		runsByID: map[domaintypes.RunID]store.Run{
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
	st := &mockStore{
		listStaleRunningJobsResult: []store.ListStaleRunningJobsRow{
			{RunID: runID, RepoID: repoA, Attempt: 1, RunningJobs: 1},
			{RunID: runID, RepoID: repoB, Attempt: 1, RunningJobs: 1},
		},
		countStaleNodesWithRunningJobsResult: 1,
		cancelRowsResult:                     1,
		jobsByAttempt: map[staleKey][]store.Job{
			{runID: runID, repoID: repoA, attempt: 1}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoA,
					RepoBaseRef: "main",
					Attempt:     1,
					Name:        "mig-a",
					Status:      domaintypes.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMod,
					Meta:        []byte(`{"next_id":1000}`),
				},
			},
			{runID: runID, repoID: repoB, attempt: 1}: {
				{
					ID:          domaintypes.NewJobID(),
					RunID:       runID,
					RepoID:      repoB,
					RepoBaseRef: "main",
					Attempt:     1,
					Name:        "mig-b",
					Status:      domaintypes.JobStatusCancelled,
					JobType:     domaintypes.JobTypeMod,
					Meta:        []byte(`{"next_id":2000}`),
				},
			},
		},
		countRunReposByStatusResult: map[domaintypes.RunID][]store.CountRunReposByStatusRow{
			runID: {
				{Status: domaintypes.RunRepoStatusCancelled, Count: 2},
			},
		},
		runsByID: map[domaintypes.RunID]store.Run{
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

	if len(st.updateRunStatusParams) != 1 {
		t.Fatalf("run status updates = %d, want 1", len(st.updateRunStatusParams))
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
