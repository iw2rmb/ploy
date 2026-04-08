package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestClaimService_Claim_ReturnsNoWorkWhenQueueEmpty(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	st := &jobStore{}
	st.claimJob.err = pgx.ErrNoRows
	st.getNode.val = store.Node{ID: nodeID}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	_, err := svc.Claim(context.Background(), nodeID)
	var noWork *ClaimNoWork
	if !errors.As(err, &noWork) {
		t.Fatalf("expected ClaimNoWork, got %T (%v)", err, err)
	}
}

func TestClaimService_Claim_SuccessBuildsPayloadAndTransitionsRepo(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &jobStore{
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
	}
	st.getNode.val = store.Node{ID: nodeID}
	st.getRun.val = store.Run{
		ID:        runID,
		SpecID:    specID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"img"}]}`)}
	st.claimJob.val = store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeID,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusRunning,
		JobType:     domaintypes.JobTypeMig,
		Meta:        []byte(`{}`),
	}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	result, err := svc.Claim(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if !st.updateRunRepoStatus.called {
		t.Fatal("expected UpdateRunRepoStatus to be called")
	}
	if st.unclaimJob.called {
		t.Fatal("expected UnclaimJob to not be called on successful claim")
	}
	if result.Payload.JobID != jobID {
		t.Fatalf("payload.job_id = %s, want %s", result.Payload.JobID, jobID)
	}
	if result.Payload.RunID != runID {
		t.Fatalf("payload.run_id = %s, want %s", result.Payload.RunID, runID)
	}
	if result.Payload.RepoURL == "" {
		t.Fatal("expected payload.repo_url to be populated")
	}
}

func TestClaimService_Claim_RequeuesClaimedJobWhenPayloadBuildFails(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &jobStore{
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
	}
	st.getNode.val = store.Node{ID: nodeID}
	st.getRun.val = store.Run{
		ID:        runID,
		SpecID:    specID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"img"}]}`)}
	st.claimJob.val = store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeID,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusRunning,
		JobType:     "invalid",
		Meta:        []byte(`{}`),
	}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	_, err := svc.Claim(context.Background(), nodeID)
	if err == nil {
		t.Fatal("expected Claim() to fail")
	}
	var internalErr *ClaimInternal
	if !errors.As(err, &internalErr) {
		t.Fatalf("expected ClaimInternal, got %T (%v)", err, err)
	}
	if !st.unclaimJob.called {
		t.Fatal("expected UnclaimJob to be called on payload build failure")
	}
	if got := st.unclaimJob.params.ID; got != jobID {
		t.Fatalf("unclaim job id = %s, want %s", got, jobID)
	}
	if got := st.unclaimJob.params.NodeID; got != nodeID {
		t.Fatalf("unclaim node id = %s, want %s", got, nodeID)
	}
	if st.updateRunRepoStatus.called {
		t.Fatal("expected UpdateRunRepoStatus to not be called when payload build fails")
	}
}
