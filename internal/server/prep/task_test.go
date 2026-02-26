package prep

import (
	"context"
	"errors"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mockRunner struct {
	runFn func(ctx context.Context, req RunRequest) (RunResult, error)
}

func (m *mockRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if m.runFn == nil {
		return RunResult{}, nil
	}
	return m.runFn(ctx, req)
}

type mockTaskStore struct {
	store.Store

	claimedPending []store.MigRepo
	claimedRetry   []store.MigRepo

	createPrepRunCalls          []store.CreatePrepRunParams
	finishPrepRunCalls          []store.FinishPrepRunParams
	updateMigRepoPrepStateCalls []store.UpdateMigRepoPrepStateParams
	saveMigRepoPrepProfileCalls []store.SaveMigRepoPrepProfileParams
}

func (m *mockTaskStore) ClaimNextPrepRepo(ctx context.Context) (store.MigRepo, error) {
	if len(m.claimedPending) == 0 {
		return store.MigRepo{}, pgx.ErrNoRows
	}
	repo := m.claimedPending[0]
	m.claimedPending = m.claimedPending[1:]
	return repo, nil
}

func (m *mockTaskStore) ClaimNextPrepRetryRepo(ctx context.Context, cutoff pgtype.Timestamptz) (store.MigRepo, error) {
	if len(m.claimedRetry) == 0 {
		return store.MigRepo{}, pgx.ErrNoRows
	}
	repo := m.claimedRetry[0]
	m.claimedRetry = m.claimedRetry[1:]
	return repo, nil
}

func (m *mockTaskStore) CreatePrepRun(ctx context.Context, arg store.CreatePrepRunParams) (store.PrepRun, error) {
	m.createPrepRunCalls = append(m.createPrepRunCalls, arg)
	return store.PrepRun{RepoID: arg.RepoID, Attempt: arg.Attempt, Status: arg.Status}, nil
}

func (m *mockTaskStore) FinishPrepRun(ctx context.Context, arg store.FinishPrepRunParams) (store.PrepRun, error) {
	m.finishPrepRunCalls = append(m.finishPrepRunCalls, arg)
	return store.PrepRun{RepoID: arg.RepoID, Attempt: arg.Attempt, Status: arg.Status}, nil
}

func (m *mockTaskStore) UpdateMigRepoPrepState(ctx context.Context, arg store.UpdateMigRepoPrepStateParams) error {
	m.updateMigRepoPrepStateCalls = append(m.updateMigRepoPrepStateCalls, arg)
	return nil
}

func (m *mockTaskStore) SaveMigRepoPrepProfile(ctx context.Context, arg store.SaveMigRepoPrepProfileParams) error {
	m.saveMigRepoPrepProfileCalls = append(m.saveMigRepoPrepProfileCalls, arg)
	return nil
}

func (m *mockTaskStore) Pool() *pgxpool.Pool { return nil }

func TestNewTask(t *testing.T) {
	t.Parallel()

	repo := store.MigRepo{ID: domaintypes.NewMigRepoID()}
	st := &mockTaskStore{claimedPending: []store.MigRepo{repo}}
	runner := &mockRunner{}

	if _, err := NewTask(Options{Runner: runner}); !errors.Is(err, ErrNilStore) {
		t.Fatalf("NewTask() missing store error = %v, want %v", err, ErrNilStore)
	}
	if _, err := NewTask(Options{Store: st}); !errors.Is(err, ErrNilRunner) {
		t.Fatalf("NewTask() missing runner error = %v, want %v", err, ErrNilRunner)
	}

	task, err := NewTask(Options{Store: st, Runner: runner})
	if err != nil {
		t.Fatalf("NewTask() error = %v", err)
	}
	if got := task.Name(); got != taskNamePrepOrchestrator {
		t.Fatalf("Name() = %q, want %q", got, taskNamePrepOrchestrator)
	}
	if got := task.Interval(); got != defaultTaskInterval {
		t.Fatalf("Interval() = %v, want %v", got, defaultTaskInterval)
	}
}

func TestTaskRunCycleSuccess(t *testing.T) {
	t.Parallel()

	repo := store.MigRepo{
		ID:           domaintypes.NewMigRepoID(),
		PrepAttempts: 1,
		RepoUrl:      "https://example.com/repo.git",
		BaseRef:      "main",
		TargetRef:    "main",
	}
	st := &mockTaskStore{claimedPending: []store.MigRepo{repo}}
	runner := &mockRunner{runFn: func(ctx context.Context, req RunRequest) (RunResult, error) {
		return RunResult{
			ProfileJSON: []byte(validProfileJSON(req.Repo.ID.String())),
			ResultJSON:  []byte(`{"stage":"codex"}`),
			LogsRef:     strPtr("inline://prep/test"),
		}, nil
	}}

	task, err := NewTask(Options{Store: st, Runner: runner, Interval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewTask() error = %v", err)
	}

	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(st.createPrepRunCalls) != 1 {
		t.Fatalf("CreatePrepRun calls = %d, want 1", len(st.createPrepRunCalls))
	}
	if len(st.finishPrepRunCalls) != 1 {
		t.Fatalf("FinishPrepRun calls = %d, want 1", len(st.finishPrepRunCalls))
	}
	if st.finishPrepRunCalls[0].Status != store.PrepStatusReady {
		t.Fatalf("FinishPrepRun status = %q, want %q", st.finishPrepRunCalls[0].Status, store.PrepStatusReady)
	}
	if len(st.saveMigRepoPrepProfileCalls) != 1 {
		t.Fatalf("SaveMigRepoPrepProfile calls = %d, want 1", len(st.saveMigRepoPrepProfileCalls))
	}
	if len(st.updateMigRepoPrepStateCalls) != 0 {
		t.Fatalf("UpdateMigRepoPrepState calls = %d, want 0", len(st.updateMigRepoPrepStateCalls))
	}
}

func TestTaskRunCycleFailureSchedulesRetry(t *testing.T) {
	t.Parallel()

	repo := store.MigRepo{
		ID:           domaintypes.NewMigRepoID(),
		PrepAttempts: 1,
		RepoUrl:      "https://example.com/repo.git",
		BaseRef:      "main",
		TargetRef:    "main",
	}
	st := &mockTaskStore{claimedPending: []store.MigRepo{repo}}
	runner := &mockRunner{runFn: func(ctx context.Context, req RunRequest) (RunResult, error) {
		return RunResult{}, &RunError{Cause: errors.New("codex timeout"), FailureCode: FailureCodeTimeout}
	}}

	task, err := NewTask(Options{Store: st, Runner: runner, MaxAttempts: 3})
	if err != nil {
		t.Fatalf("NewTask() error = %v", err)
	}

	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(st.finishPrepRunCalls) != 1 {
		t.Fatalf("FinishPrepRun calls = %d, want 1", len(st.finishPrepRunCalls))
	}
	if st.finishPrepRunCalls[0].Status != store.PrepStatusFailed {
		t.Fatalf("FinishPrepRun status = %q, want %q", st.finishPrepRunCalls[0].Status, store.PrepStatusFailed)
	}
	if len(st.updateMigRepoPrepStateCalls) != 1 {
		t.Fatalf("UpdateMigRepoPrepState calls = %d, want 1", len(st.updateMigRepoPrepStateCalls))
	}
	if st.updateMigRepoPrepStateCalls[0].PrepStatus != store.PrepStatusRetryScheduled {
		t.Fatalf("PrepStatus = %q, want %q", st.updateMigRepoPrepStateCalls[0].PrepStatus, store.PrepStatusRetryScheduled)
	}
	if st.updateMigRepoPrepStateCalls[0].PrepFailureCode == nil || *st.updateMigRepoPrepStateCalls[0].PrepFailureCode != FailureCodeTimeout {
		t.Fatalf("failure code = %v, want %q", st.updateMigRepoPrepStateCalls[0].PrepFailureCode, FailureCodeTimeout)
	}
}

func TestTaskRunCycleClaimsRetryQueue(t *testing.T) {
	t.Parallel()

	repo := store.MigRepo{
		ID:           domaintypes.NewMigRepoID(),
		PrepAttempts: 2,
		RepoUrl:      "https://example.com/repo.git",
		BaseRef:      "main",
		TargetRef:    "main",
	}
	st := &mockTaskStore{claimedRetry: []store.MigRepo{repo}}
	runner := &mockRunner{runFn: func(ctx context.Context, req RunRequest) (RunResult, error) {
		return RunResult{
			ProfileJSON: []byte(validProfileJSON(req.Repo.ID.String())),
			ResultJSON:  []byte(`{"stage":"codex"}`),
		}, nil
	}}

	task, err := NewTask(Options{Store: st, Runner: runner})
	if err != nil {
		t.Fatalf("NewTask() error = %v", err)
	}
	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(st.createPrepRunCalls) != 1 {
		t.Fatalf("CreatePrepRun calls = %d, want 1", len(st.createPrepRunCalls))
	}
}

func validProfileJSON(repoID string) string {
	return `{
  "schema_version": 1,
  "repo_id": "` + repoID + `",
  "runner_mode": "simple",
  "targets": {
    "build": {"status": "passed", "command": "go test ./...", "env": {}, "failure_code": null},
    "unit": {"status": "not_attempted", "env": {}},
    "all_tests": {"status": "not_attempted", "env": {}}
  },
  "orchestration": {"pre": [], "post": []},
  "tactics_used": ["go_default"],
  "attempts": [],
  "evidence": {"log_refs": ["inline://prep/test"], "diagnostics": []},
  "repro_check": {"status": "passed", "details": "ok"},
  "prompt_delta_suggestion": {"status": "none", "summary": "", "candidate_lines": []}
}`
}
