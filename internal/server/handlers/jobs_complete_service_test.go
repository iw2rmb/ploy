package handlers

import (
	"context"
	"errors"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestCompletionService_Complete_ReturnsConflictForNonRunningJob(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	jobID := domaintypes.NewJobID()
	st := &handlerStore{}
	st.getJob.val = store.Job{
		ID:        jobID,
		RunID:     domaintypes.NewRunID(),
		RepoID:    domaintypes.NewRepoID(),
		NodeID:    &nodeID,
		Status:    domaintypes.JobStatusQueued,
		JobType:   domaintypes.JobTypeMig,
		RepoShaIn: "0123456789abcdef0123456789abcdef01234567",
	}

	svc := newCompletionService(st, nil, nil)
	err := svc.Complete(context.Background(), completionInput{
		JobID:      jobID,
		NodeID:     nodeID,
		Status:     domaintypes.JobStatusSuccess,
		StatsBytes: []byte("{}"),
	})
	var completionErr *completionError
	if !errors.As(err, &completionErr) {
		t.Fatalf("expected completionError, got %T (%v)", err, err)
	}
	if completionErr.status != 409 {
		t.Fatalf("completionError status = %d, want 409", completionErr.status)
	}
}

func TestCompletionService_Complete_SuccessPromotesNextJob(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	jobID := domaintypes.NewJobID()
	nextID := domaintypes.NewJobID()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	st := &handlerStore{}
	st.getJob.val = store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeID,
		Status:      domaintypes.JobStatusRunning,
		JobType:     domaintypes.JobTypeMig,
		RepoShaIn:   "0123456789abcdef0123456789abcdef01234567",
		NextID:      &nextID,
	}

	svc := newCompletionService(st, nil, nil)
	err := svc.Complete(context.Background(), completionInput{
		JobID:        jobID,
		NodeID:       nodeID,
		Status:       domaintypes.JobStatusSuccess,
		RepoSHAOut:   "0123456789abcdef0123456789abcdef01234567",
		StatsBytes:   []byte("{}"),
		StatsPayload: JobStatsPayload{},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	assertCalled(t, "UpdateJobCompletion", st.updateJobCompletion.called)
	assertCalled(t, "PromoteJobByIDIfUnblocked", st.promoteJobByIDIfUnblocked.called)
	if st.promoteJobByIDIfUnblocked.params != nextID {
		t.Fatalf("promoted next_id = %s, want %s", st.promoteJobByIDIfUnblocked.params, nextID)
	}
}

// A delayed completion cannot change the retried job or advance its successor.
func TestCompletionService_RejectsEarlierResumeGeneration(t *testing.T) {
	st := &handlerStore{}
	node := domaintypes.NodeID("node")
	st.getRun.val.Stats = []byte(`{"resume_count":1}`)
	st.getJob.val = store.Job{ID: domaintypes.NewJobID(), NodeID: &node, Status: domaintypes.JobStatusRunning}
	err := newCompletionService(st, nil, nil).Complete(context.Background(), completionInput{JobID: st.getJob.val.ID, NodeID: node, Status: domaintypes.JobStatusSuccess})
	var completionErr *completionError
	if !errors.As(err, &completionErr) || completionErr.status != 409 {
		t.Fatalf("expected conflict: %v", err)
	}
	if st.updateJobCompletion.called || st.promoteJobByIDIfUnblocked.called {
		t.Fatal("stale completion changed execution")
	}
}

// Wave aggregation must see committed statuses from concurrent run completions.
func TestCompletionService_ReconcilesWaveAfterCommit(t *testing.T) {
	for _, failCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "rollback"}[failCommit], func(t *testing.T) {
			f := newRepoScopedFixture("mig")
			terminal := f.Job
			terminal.Status = domaintypes.JobStatusSuccess
			base := newJobStoreForFixture(f,
				withRepoAttemptJobs([]store.Job{terminal}),
				withRunStatusCounts([]store.CountRunsByWaveStatusRow{{Status: domaintypes.RunStatusSuccess, Count: 2}}),
			)
			st := &completionCommitStore{handlerStore: base, failCommit: failCommit}
			err := newCompletionService(st, nil, nil).Complete(context.Background(), completionInput{
				JobID: f.JobID, NodeID: *f.Job.NodeID, Status: domaintypes.JobStatusSuccess,
				RepoSHAOut: "0123456789abcdef0123456789abcdef01234567",
			})
			if (err != nil) != failCommit {
				t.Fatalf("Complete error = %v, failCommit = %v", err, failCommit)
			}
			if st.countedBeforeCommit {
				t.Fatal("wave aggregation used uncommitted run statuses")
			}
			if base.updateWaveStatus.called == failCommit {
				t.Fatalf("wave completion called = %v, failCommit = %v", base.updateWaveStatus.called, failCommit)
			}
		})
	}
}

type completionCommitStore struct {
	*handlerStore
	committed           bool
	failCommit          bool
	countedBeforeCommit bool
}

func (s *completionCommitStore) WithJobExecution(ctx context.Context, id domaintypes.JobID, count int, complete func(store.Store, store.Job) error) error {
	if err := s.handlerStore.WithJobExecution(ctx, id, count, func(_ store.Store, job store.Job) error {
		return complete(s, job)
	}); err != nil {
		return err
	}
	if s.failCommit {
		return errors.New("commit failed")
	}
	s.committed = true
	return nil
}

func (s *completionCommitStore) CountRunsByWaveStatus(ctx context.Context, id domaintypes.WaveID) ([]store.CountRunsByWaveStatusRow, error) {
	s.countedBeforeCommit = !s.committed
	return s.handlerStore.CountRunsByWaveStatus(ctx, id)
}
