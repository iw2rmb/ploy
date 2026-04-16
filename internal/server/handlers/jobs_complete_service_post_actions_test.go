package handlers

import (
	"context"
	"errors"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestOnSuccess_SBOMPersistenceFailureStopsChainAdvancement(t *testing.T) {
	t.Parallel()

	nextID := domaintypes.NewJobID()
	job := store.Job{
		ID:     domaintypes.NewJobID(),
		RunID:  domaintypes.NewRunID(),
		RepoID: domaintypes.NewRepoID(),
		NextID: &nextID,
	}
	st := &jobStore{}
	st.listArtifactBundlesByRunAndJob.err = errors.New("list bundle failed")
	svc := &CompleteJobService{
		store:       st,
		blobpersist: blobpersist.New(st, bsmock.New()),
	}
	state := &completeJobState{
		input: CompleteJobInput{
			Status:     domaintypes.JobStatusSuccess,
			RepoSHAOut: "0123456789abcdef0123456789abcdef01234567",
		},
		job:           job,
		jobType:       domaintypes.JobTypeSBOM,
		serviceType:   completeJobServiceTypeSBOM,
		serviceTypeOK: true,
	}

	svc.onSuccess(context.Background(), state)

	if st.promoteJobByIDIfUnblocked.called {
		t.Fatal("did not expect PromoteJobByIDIfUnblocked when sbom persistence fails")
	}
	if st.createJob.called {
		t.Fatal("did not expect job creation during sbom completion")
	}
}

func TestOnSuccess_SBOMSuccessPromotesLinkedNext(t *testing.T) {
	t.Parallel()

	nextID := domaintypes.NewJobID()
	job := store.Job{
		ID:     domaintypes.NewJobID(),
		RunID:  domaintypes.NewRunID(),
		RepoID: domaintypes.NewRepoID(),
		NextID: &nextID,
	}
	next := store.Job{
		ID:     nextID,
		RunID:  job.RunID,
		RepoID: job.RepoID,
		Status: domaintypes.JobStatusCreated,
	}

	st := &jobStore{}
	st.promoteJobByIDIfUnblocked.val = next
	svc := &CompleteJobService{
		store:       st,
		blobpersist: blobpersist.New(st, bsmock.New()),
	}
	state := &completeJobState{
		input: CompleteJobInput{
			Status:     domaintypes.JobStatusSuccess,
			RepoSHAOut: "0123456789abcdef0123456789abcdef01234567",
		},
		job:           job,
		jobType:       domaintypes.JobTypeSBOM,
		serviceType:   completeJobServiceTypeSBOM,
		serviceTypeOK: true,
	}

	svc.onSuccess(context.Background(), state)

	if st.createJob.called {
		t.Fatal("did not expect sbom completion to create additional jobs")
	}
	if !st.promoteJobByIDIfUnblocked.called {
		t.Fatal("expected successor promotion for sbom success")
	}
}
