package handlers

import (
	"context"
	"testing"

	"github.com/google/uuid"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestResolveJavaClasspathClaimContext(t *testing.T) {
	t.Parallel()

	testBundleIDA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	testBundleIDB := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	buildMetaWithMirror := func(sourceJobID domaintypes.JobID) []byte {
		meta, err := contracts.MarshalJobMeta(&contracts.JobMeta{
			Kind: contracts.JobKindMig,
			CacheMirror: &contracts.CacheMirrorMetadata{
				SourceJobID: sourceJobID,
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
		assertion func(t *testing.T, got *contracts.JavaClasspathClaimContext, st *jobStore)
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
			assertion: func(t *testing.T, got *contracts.JavaClasspathClaimContext, st *jobStore) {
				t.Helper()
				if got != nil {
					t.Fatalf("context=%+v, want nil", *got)
				}
			},
		},
		{
			name: "non-sbom job without sbom ancestor has no java classpath context",
			job: func() store.Job {
				runID := domaintypes.NewRunID()
				repoID := domaintypes.NewRepoID()
				jobID := domaintypes.NewJobID()
				return store.Job{
					ID:      jobID,
					RunID:   runID,
					RepoID:  repoID,
					Attempt: 1,
					JobType: domaintypes.JobTypeMig,
				}
			}(),
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
			assertion: func(t *testing.T, got *contracts.JavaClasspathClaimContext, st *jobStore) {
				t.Helper()
				if got != nil {
					t.Fatalf("context=%+v, want nil", *got)
				}
			},
		},
		{
			name: "non-sbom job with sbom ancestor receives source artifact context",
			job: func() store.Job {
				runID := domaintypes.NewRunID()
				repoID := domaintypes.NewRepoID()
				jobID := domaintypes.NewJobID()
				return store.Job{
					ID:      jobID,
					RunID:   runID,
					RepoID:  repoID,
					Attempt: 1,
					JobType: domaintypes.JobTypePostGate,
				}
			}(),
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
				st.getJobResults = map[domaintypes.JobID]store.Job{
					migID: {
						ID:      migID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						JobType: domaintypes.JobTypeMig,
					},
				}
				name := "mig-out"
				st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
					{
						ID:   pgtype.UUID{Bytes: testBundleIDA, Valid: true},
						Name: &name,
					},
				}
			},
			assertion: func(t *testing.T, got *contracts.JavaClasspathClaimContext, st *jobStore) {
				t.Helper()
				if got == nil {
					t.Fatal("context=nil, want non-nil")
				}
				if !got.Required {
					t.Fatal("context.Required=false, want true")
				}
				if got.SourceArtifactID != testBundleIDA.String() {
					t.Fatalf("context.SourceArtifactID=%q, want %q", got.SourceArtifactID, testBundleIDA.String())
				}
				if st.listArtifactBundlesByRunAndJob.params.JobID == nil {
					t.Fatal("ListArtifactBundlesByRunAndJob JobID=nil, want non-nil")
				}
			},
		},
		{
			name: "mirrored predecessor resolves source artifact from effective source job",
			job: func() store.Job {
				runID := domaintypes.NewRunID()
				repoID := domaintypes.NewRepoID()
				jobID := domaintypes.NewJobID()
				return store.Job{
					ID:      jobID,
					RunID:   runID,
					RepoID:  repoID,
					Attempt: 1,
					JobType: domaintypes.JobTypeMig,
				}
			}(),
			setup: func(t *testing.T, st *jobStore, job store.Job) {
				t.Helper()
				sbomID := domaintypes.NewJobID()
				mirroredPreGateID := domaintypes.NewJobID()
				sourcePreGateID := domaintypes.NewJobID()
				st.listJobsByRunRepoAttempt.val = []store.Job{
					{
						ID:      sbomID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &mirroredPreGateID,
						JobType: domaintypes.JobTypeSBOM,
						Status:  domaintypes.JobStatusSuccess,
					},
					{
						ID:      mirroredPreGateID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &job.ID,
						JobType: domaintypes.JobTypePreGate,
						Status:  domaintypes.JobStatusSuccess,
						Meta:    buildMetaWithMirror(sourcePreGateID),
					},
					job,
				}
				st.getJobResults = map[domaintypes.JobID]store.Job{
					mirroredPreGateID: {
						ID:      mirroredPreGateID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						JobType: domaintypes.JobTypePreGate,
						Status:  domaintypes.JobStatusSuccess,
						Meta:    buildMetaWithMirror(sourcePreGateID),
					},
					sourcePreGateID: {
						ID:      sourcePreGateID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						JobType: domaintypes.JobTypePreGate,
						Status:  domaintypes.JobStatusSuccess,
					},
				}
				name := "build-gate-out"
				st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
					{
						ID:   pgtype.UUID{Bytes: testBundleIDB, Valid: true},
						Name: &name,
					},
				}
			},
			assertion: func(t *testing.T, got *contracts.JavaClasspathClaimContext, st *jobStore) {
				t.Helper()
				if got == nil {
					t.Fatal("context=nil, want non-nil")
				}
				if got.SourceArtifactID != testBundleIDB.String() {
					t.Fatalf("context.SourceArtifactID=%q, want %q", got.SourceArtifactID, testBundleIDB.String())
				}
				params := st.listArtifactBundlesByRunAndJob.params
				if params.JobID == nil {
					t.Fatal("ListArtifactBundlesByRunAndJob JobID=nil, want source job id")
				}
				if got.SourceJobID.IsZero() {
					t.Fatal("context.SourceJobID is zero, want source job id")
				}
				if got.SourceJobType != domaintypes.JobTypePreGate {
					t.Fatalf("context.SourceJobType=%q, want %q", got.SourceJobType, domaintypes.JobTypePreGate)
				}
				if *params.JobID != got.SourceJobID {
					t.Fatalf("artifact lookup job_id=%s, want context source job id=%s", *params.JobID, got.SourceJobID)
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
			tc.assertion(t, got, st)
		})
	}
}
