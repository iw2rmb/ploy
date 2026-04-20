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
			name: "heal job after sbom predecessor does not request java classpath",
			job: func() store.Job {
				runID := domaintypes.NewRunID()
				repoID := domaintypes.NewRepoID()
				jobID := domaintypes.NewJobID()
				return store.Job{
					ID:      jobID,
					RunID:   runID,
					RepoID:  repoID,
					Attempt: 1,
					JobType: domaintypes.JobTypeHeal,
				}
			}(),
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
						Name:    "pre-gate-sbom-000",
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
			assertion: func(t *testing.T, got *contracts.JavaClasspathClaimContext, st *jobStore) {
				t.Helper()
				if got != nil {
					t.Fatalf("context=%+v, want nil", *got)
				}
				if st.listArtifactBundlesByRunAndJob.called {
					t.Fatal("ListArtifactBundlesByRunAndJob called, want not called for heal after sbom predecessor")
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
						Name:    "pre-gate-sbom-000",
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
				st.getJobResults = map[domaintypes.JobID]store.Job{
					sbomID: {
						ID:      sbomID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						Name:    "pre-gate-sbom-000",
						JobType: domaintypes.JobTypeSBOM,
						Status:  domaintypes.JobStatusSuccess,
						Meta:    preGateSBOMMeta(),
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
				if got.SourceJobType != domaintypes.JobTypeSBOM {
					t.Fatalf("context.SourceJobType=%q, want %q", got.SourceJobType, domaintypes.JobTypeSBOM)
				}
			},
		},
		{
			name: "post-gate predecessor does not override pre-gate classpath source",
			job: func() store.Job {
				runID := domaintypes.NewRunID()
				repoID := domaintypes.NewRepoID()
				jobID := domaintypes.NewJobID()
				return store.Job{
					ID:      jobID,
					RunID:   runID,
					RepoID:  repoID,
					Attempt: 1,
					JobType: domaintypes.JobTypeHook,
				}
			}(),
			setup: func(t *testing.T, st *jobStore, job store.Job) {
				t.Helper()
				preSBOMID := domaintypes.NewJobID()
				preGateID := domaintypes.NewJobID()
				postSBOMID := domaintypes.NewJobID()
				st.listJobsByRunRepoAttempt.val = []store.Job{
					{
						ID:      preSBOMID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &preGateID,
						Name:    "pre-gate-sbom-000",
						JobType: domaintypes.JobTypeSBOM,
						Status:  domaintypes.JobStatusSuccess,
						Meta:    preGateSBOMMeta(),
					},
					{
						ID:      preGateID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &postSBOMID,
						JobType: domaintypes.JobTypePreGate,
						Status:  domaintypes.JobStatusSuccess,
					},
					{
						ID:      postSBOMID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &job.ID,
						Name:    "post-gate-sbom-000",
						JobType: domaintypes.JobTypeSBOM,
						Status:  domaintypes.JobStatusSuccess,
					},
					job,
				}
				st.getJobResults = map[domaintypes.JobID]store.Job{
					preSBOMID: {
						ID:      preSBOMID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						Name:    "pre-gate-sbom-000",
						JobType: domaintypes.JobTypeSBOM,
						Status:  domaintypes.JobStatusSuccess,
						Meta:    preGateSBOMMeta(),
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
				if got.SourceArtifactID != testBundleIDA.String() {
					t.Fatalf("context.SourceArtifactID=%q, want %q", got.SourceArtifactID, testBundleIDA.String())
				}
				if got.SourceJobType != domaintypes.JobTypeSBOM {
					t.Fatalf("context.SourceJobType=%q, want %q", got.SourceJobType, domaintypes.JobTypeSBOM)
				}
			},
		},
		{
			name: "java classpath source stays on pre-gate sbom when predecessor is gate",
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
				preGateID := domaintypes.NewJobID()
				st.listJobsByRunRepoAttempt.val = []store.Job{
					{
						ID:      sbomID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &preGateID,
						Name:    "pre-gate-sbom-000",
						JobType: domaintypes.JobTypeSBOM,
						Status:  domaintypes.JobStatusSuccess,
						Meta:    preGateSBOMMeta(),
					},
					{
						ID:      preGateID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						NextID:  &job.ID,
						JobType: domaintypes.JobTypePreGate,
						Status:  domaintypes.JobStatusSuccess,
						Meta:    []byte(`{"kind":"mig"}`),
					},
					job,
				}
				st.getJobResults = map[domaintypes.JobID]store.Job{
					sbomID: {
						ID:      sbomID,
						RunID:   job.RunID,
						RepoID:  job.RepoID,
						Attempt: job.Attempt,
						Name:    "pre-gate-sbom-000",
						JobType: domaintypes.JobTypeSBOM,
						Status:  domaintypes.JobStatusSuccess,
						Meta:    preGateSBOMMeta(),
					},
				}
				name := "mig-out"
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
				if got.SourceJobType != domaintypes.JobTypeSBOM {
					t.Fatalf("context.SourceJobType=%q, want %q", got.SourceJobType, domaintypes.JobTypeSBOM)
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
