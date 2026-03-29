package handlers

import (
	"context"
	"errors"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestCompleteJobService_Complete_ReturnsConflictForNonRunningJob(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	jobID := domaintypes.NewJobID()
	st := &jobStore{
		getJobResult: store.Job{
			ID:        jobID,
			RunID:     domaintypes.NewRunID(),
			RepoID:    domaintypes.NewRepoID(),
			NodeID:    &nodeID,
			Status:    domaintypes.JobStatusQueued,
			JobType:   domaintypes.JobTypeMod,
			RepoShaIn: "0123456789abcdef0123456789abcdef01234567",
		},
	}

	svc := NewCompleteJobService(st, nil, nil, nil)
	_, err := svc.Complete(context.Background(), CompleteJobInput{
		JobID:      jobID,
		NodeID:     nodeID,
		Status:     domaintypes.JobStatusSuccess,
		StatsBytes: []byte("{}"),
	})
	var conflict *CompleteJobConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("expected CompleteJobConflict, got %T (%v)", err, err)
	}
}

func TestCompleteJobService_Complete_SuccessPromotesNextJob(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	jobID := domaintypes.NewJobID()
	nextID := domaintypes.NewJobID()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	st := &jobStore{
		getJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Status:      domaintypes.JobStatusRunning,
			JobType:     domaintypes.JobTypeMod,
			RepoShaIn:   "0123456789abcdef0123456789abcdef01234567",
			NextID:      &nextID,
		},
	}

	svc := NewCompleteJobService(st, nil, nil, nil)
	_, err := svc.Complete(context.Background(), CompleteJobInput{
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
