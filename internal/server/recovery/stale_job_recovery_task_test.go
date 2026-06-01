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

func TestStaleJobRecoveryTask_Run_CompletesWaveWhenRunsTerminal(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	waveID := domaintypes.NewWaveID()
	repoID := domaintypes.NewRepoID()
	st := &staleTaskStore{
		StaleRows:         []store.ListStaleRunningJobsRow{{RunID: runID, Attempt: 2, RunningJobs: 3}},
		StaleNodesCount:   1,
		CancelRowsResult:  3,
		JobsByAttempt:     terminalJobs(runID, repoID, 2, domaintypes.JobStatusCancelled),
		RunsByID:          map[domaintypes.RunID]store.Run{runID: {ID: runID, WaveID: waveID, RepoID: repoID, Status: domaintypes.RunStatusRunning}},
		WavesByID:         map[domaintypes.WaveID]store.Wave{waveID: {ID: waveID, Status: domaintypes.WaveStatusStarted}},
		CountRunsByWave:   map[domaintypes.WaveID][]store.CountRunsByWaveStatusRow{waveID: {{Status: domaintypes.RunStatusCancelled, Count: 1}}},
		ReposByID:         map[domaintypes.RepoID]store.Repo{repoID: {ID: repoID, Url: "https://github.com/user/repo.git"}},
		UpdateRunStatusOK: true,
	}

	task, err := NewStaleJobRecoveryTask(Options{Store: st, Interval: time.Second, NodeStaleAfter: time.Minute})
	if err != nil {
		t.Fatalf("NewStaleJobRecoveryTask() error = %v", err)
	}

	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(st.CancelCalls) != 1 {
		t.Fatalf("cancel calls = %d, want 1", len(st.CancelCalls))
	}
	if len(st.UpdateRunCalls) != 1 || st.UpdateRunCalls[0].Status != domaintypes.RunStatusCancelled {
		t.Fatalf("run updates = %+v, want one Cancelled update", st.UpdateRunCalls)
	}
	if !st.CountRunsCalled {
		t.Fatal("expected CountRunsByWaveStatus call")
	}
	if len(st.UpdateWaveCalls) != 1 || st.UpdateWaveCalls[0].Status != domaintypes.WaveStatusFinished {
		t.Fatalf("wave updates = %+v, want one Finished update", st.UpdateWaveCalls)
	}
}

func TestStaleJobRecoveryTask_Run_DoesNotCompleteWaveWhenOtherRunsNonTerminal(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	waveID := domaintypes.NewWaveID()
	repoID := domaintypes.NewRepoID()
	st := &staleTaskStore{
		StaleRows:        []store.ListStaleRunningJobsRow{{RunID: runID, Attempt: 1, RunningJobs: 1}},
		StaleNodesCount:  1,
		CancelRowsResult: 1,
		JobsByAttempt:    terminalJobs(runID, repoID, 1, domaintypes.JobStatusCancelled),
		RunsByID:         map[domaintypes.RunID]store.Run{runID: {ID: runID, WaveID: waveID, RepoID: repoID, Status: domaintypes.RunStatusRunning}},
		WavesByID:        map[domaintypes.WaveID]store.Wave{waveID: {ID: waveID, Status: domaintypes.WaveStatusStarted}},
		CountRunsByWave: map[domaintypes.WaveID][]store.CountRunsByWaveStatusRow{
			waveID: {
				{Status: domaintypes.RunStatusCancelled, Count: 1},
				{Status: domaintypes.RunStatusRunning, Count: 1},
			},
		},
	}

	task, err := NewStaleJobRecoveryTask(Options{Store: st, Interval: time.Second, NodeStaleAfter: time.Minute})
	if err != nil {
		t.Fatalf("NewStaleJobRecoveryTask() error = %v", err)
	}

	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(st.UpdateRunCalls) != 1 {
		t.Fatalf("run updates = %d, want 1", len(st.UpdateRunCalls))
	}
	if len(st.UpdateWaveCalls) != 0 {
		t.Fatalf("wave updates = %d, want 0", len(st.UpdateWaveCalls))
	}
}

func TestStaleJobRecoveryTask_Run_LogsCycleCounters(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	waveID := domaintypes.NewWaveID()
	repoID := domaintypes.NewRepoID()
	st := &staleTaskStore{
		StaleRows:        []store.ListStaleRunningJobsRow{{RunID: runID, Attempt: 1, RunningJobs: 2}},
		StaleNodesCount:  2,
		CancelRowsResult: 2,
		JobsByAttempt:    terminalJobs(runID, repoID, 1, domaintypes.JobStatusCancelled),
		RunsByID:         map[domaintypes.RunID]store.Run{runID: {ID: runID, WaveID: waveID, RepoID: repoID, Status: domaintypes.RunStatusRunning}},
		WavesByID:        map[domaintypes.WaveID]store.Wave{waveID: {ID: waveID, Status: domaintypes.WaveStatusStarted}},
		CountRunsByWave:  map[domaintypes.WaveID][]store.CountRunsByWaveStatusRow{waveID: {{Status: domaintypes.RunStatusCancelled, Count: 1}}},
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
		if got := int64(payload["runs_updated"].(float64)); got != 1 {
			t.Fatalf("runs_updated=%d, want 1", got)
		}
	}
	if !foundCycleLog {
		t.Fatal("expected stale-job-recovery cycle log")
	}
}

func terminalJobs(runID domaintypes.RunID, repoID domaintypes.RepoID, attempt int32, status domaintypes.JobStatus) map[ids.AttemptKey][]store.Job {
	return map[ids.AttemptKey][]store.Job{
		{RunID: runID, Attempt: attempt}: {{
			ID:          domaintypes.NewJobID(),
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     attempt,
			Name:        "mig-0",
			Status:      status,
			JobType:     domaintypes.JobTypeMig,
			Meta:        []byte(`{"next_id":1000}`),
		}},
	}
}

type staleTaskStore struct {
	store.Store

	StaleRows        []store.ListStaleRunningJobsRow
	StaleNodesCount  int64
	StaleNodesErr    error
	CancelRowsResult int64
	CancelErr        error
	JobsByAttempt    map[ids.AttemptKey][]store.Job
	RunsByID         map[domaintypes.RunID]store.Run
	WavesByID        map[domaintypes.WaveID]store.Wave
	ReposByID        map[domaintypes.RepoID]store.Repo
	CountRunsByWave  map[domaintypes.WaveID][]store.CountRunsByWaveStatusRow

	StaleJobsParam    pgtype.Timestamptz
	StaleNodeParam    pgtype.Timestamptz
	CountRunsCalled   bool
	UpdateRunStatusOK bool
	CancelCalls       []store.CancelActiveJobsByRunAttemptParams
	UpdateRunCalls    []store.UpdateRunStatusParams
	UpdateWaveCalls   []store.UpdateWaveStatusParams
}

func (s *staleTaskStore) ListStaleRunningJobs(_ context.Context, lastHeartbeat pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	s.StaleJobsParam = lastHeartbeat
	return s.StaleRows, nil
}

func (s *staleTaskStore) CountStaleNodesWithRunningJobs(_ context.Context, lastHeartbeat pgtype.Timestamptz) (int64, error) {
	s.StaleNodeParam = lastHeartbeat
	return s.StaleNodesCount, s.StaleNodesErr
}

func (s *staleTaskStore) CancelActiveJobsByRunAttempt(_ context.Context, arg store.CancelActiveJobsByRunAttemptParams) (int64, error) {
	s.CancelCalls = append(s.CancelCalls, arg)
	return s.CancelRowsResult, s.CancelErr
}

func (s *staleTaskStore) ListJobsByRunAttempt(_ context.Context, arg store.ListJobsByRunAttemptParams) ([]store.Job, error) {
	return s.JobsByAttempt[ids.AttemptKey{RunID: arg.RunID, Attempt: arg.Attempt}], nil
}

func (s *staleTaskStore) UpdateRunStatus(_ context.Context, arg store.UpdateRunStatusParams) error {
	s.UpdateRunCalls = append(s.UpdateRunCalls, arg)
	if s.RunsByID != nil {
		run := s.RunsByID[arg.ID]
		run.ID = arg.ID
		run.Status = arg.Status
		s.RunsByID[arg.ID] = run
	}
	return nil
}

func (s *staleTaskStore) GetRun(_ context.Context, id domaintypes.RunID) (store.Run, error) {
	return s.RunsByID[id], nil
}

func (s *staleTaskStore) GetWave(_ context.Context, id domaintypes.WaveID) (store.Wave, error) {
	return s.WavesByID[id], nil
}

func (s *staleTaskStore) CountRunsByWaveStatus(_ context.Context, waveID domaintypes.WaveID) ([]store.CountRunsByWaveStatusRow, error) {
	s.CountRunsCalled = true
	return s.CountRunsByWave[waveID], nil
}

func (s *staleTaskStore) UpdateWaveStatus(_ context.Context, arg store.UpdateWaveStatusParams) error {
	s.UpdateWaveCalls = append(s.UpdateWaveCalls, arg)
	return nil
}

func (s *staleTaskStore) GetRepo(_ context.Context, repoID domaintypes.RepoID) (store.Repo, error) {
	if s.ReposByID == nil {
		return store.Repo{ID: repoID, Url: "https://github.com/user/repo.git"}, nil
	}
	return s.ReposByID[repoID], nil
}
