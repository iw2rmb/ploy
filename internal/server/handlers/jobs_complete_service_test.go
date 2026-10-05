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
