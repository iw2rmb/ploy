package handlers

import (
	"context"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestResolveJavaClasspathClaimContext(t *testing.T) {
	t.Parallel()

	preGateSBOMMeta := func() []byte {
		meta, err := contracts.MarshalJobMeta(&contracts.JobMeta{
			Kind: contracts.JobKindMig,
			SBOM: &contracts.SBOMJobMetadata{
				Phase:     contracts.SBOMPhasePre,
				CycleName: "pre-gate",
				Role:      contracts.SBOMRoleInitial,
			},
		})
		if err != nil {
			panic(err)
		}
		return meta
	}

	tests := []struct {
		name      string
		job       store.Job
		setup     func(t *testing.T, st *jobStore, job store.Job)
		assertion func(t *testing.T, got *contracts.JavaClasspathClaimContext)
	}{
		{
			name: "sbom job has no java classpath context",
			job: store.Job{
				ID:      domaintypes.NewJobID(),
				RunID:   domaintypes.NewRunID(),
				RepoID:  domaintypes.NewRepoID(),
				Attempt: 1,
				JobType: domaintypes.JobTypeSBOM,
			},
			setup: func(t *testing.T, st *jobStore, job store.Job) {},
			assertion: func(t *testing.T, got *contracts.JavaClasspathClaimContext) {
				t.Helper()
				if got != nil {
					t.Fatalf("context=%+v, want nil", *got)
				}
			},
		},
		{
			name: "non-sbom job without sbom ancestor has no java classpath context",
			job: store.Job{
				ID:      domaintypes.NewJobID(),
				RunID:   domaintypes.NewRunID(),
				RepoID:  domaintypes.NewRepoID(),
				Attempt: 1,
				JobType: domaintypes.JobTypeMig,
			},
			setup: func(t *testing.T, st *jobStore, job store.Job) {
				t.Helper()
				predecessorID := domaintypes.NewJobID()
				st.listJobsByRunRepoAttempt.val = []store.Job{
					{
						ID:      predecessorID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &job.ID,
						JobType: domaintypes.JobTypePreGate,
						Status:  domaintypes.JobStatusSuccess,
					},
					job,
				}
			},
			assertion: func(t *testing.T, got *contracts.JavaClasspathClaimContext) {
				t.Helper()
				if got != nil {
					t.Fatalf("context=%+v, want nil", *got)
				}
			},
		},
		{
			name: "heal job after sbom predecessor does not request java classpath",
			job: store.Job{
				ID:      domaintypes.NewJobID(),
				RunID:   domaintypes.NewRunID(),
				RepoID:  domaintypes.NewRepoID(),
				Attempt: 1,
				JobType: domaintypes.JobTypeHeal,
			},
			setup: func(t *testing.T, st *jobStore, job store.Job) {
				t.Helper()
				sbomSuccessID := domaintypes.NewJobID()
				migID := domaintypes.NewJobID()
				sbomFailID := domaintypes.NewJobID()
				st.listJobsByRunRepoAttempt.val = []store.Job{
					{
						ID:      sbomSuccessID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &migID,
						JobType: domaintypes.JobTypeSBOM,
						Status:  domaintypes.JobStatusSuccess,
						Meta:    preGateSBOMMeta(),
					},
					{
						ID:      migID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &sbomFailID,
						JobType: domaintypes.JobTypeMig,
						Status:  domaintypes.JobStatusSuccess,
					},
					{
						ID:      sbomFailID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &job.ID,
						JobType: domaintypes.JobTypeSBOM,
						Status:  domaintypes.JobStatusFail,
					},
					job,
				}
			},
			assertion: func(t *testing.T, got *contracts.JavaClasspathClaimContext) {
				t.Helper()
				if got != nil {
					t.Fatalf("context=%+v, want nil", *got)
				}
			},
		},
		{
			name: "non-sbom job with sbom ancestor is marked as requiring java classpath",
			job: store.Job{
				ID:      domaintypes.NewJobID(),
				RunID:   domaintypes.NewRunID(),
				RepoID:  domaintypes.NewRepoID(),
				Attempt: 1,
				JobType: domaintypes.JobTypePostGate,
			},
			setup: func(t *testing.T, st *jobStore, job store.Job) {
				t.Helper()
				sbomID := domaintypes.NewJobID()
				preGateID := domaintypes.NewJobID()
				migID := domaintypes.NewJobID()
				st.listJobsByRunRepoAttempt.val = []store.Job{
					{
						ID:      sbomID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &preGateID,
						JobType: domaintypes.JobTypeSBOM,
						Status:  domaintypes.JobStatusSuccess,
						Meta:    preGateSBOMMeta(),
					},
					{
						ID:      preGateID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &migID,
						JobType: domaintypes.JobTypePreGate,
						Status:  domaintypes.JobStatusSuccess,
					},
					{
						ID:      migID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &job.ID,
						JobType: domaintypes.JobTypeMig,
						Status:  domaintypes.JobStatusSuccess,
					},
					job,
				}
			},
			assertion: func(t *testing.T, got *contracts.JavaClasspathClaimContext) {
				t.Helper()
				if got == nil {
					t.Fatal("context=nil, want non-nil")
				}
				if !got.Required {
					t.Fatal("context.Required=false, want true")
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st := &jobStore{}
			tc.setup(t, st, tc.job)
			got, err := resolveJavaClasspathClaimContext(context.Background(), st, tc.job)
			if err != nil {
				t.Fatalf("resolveJavaClasspathClaimContext() error = %v", err)
			}
			tc.assertion(t, got)
		})
	}
}
