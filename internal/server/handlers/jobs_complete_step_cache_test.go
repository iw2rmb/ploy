package handlers

import (
	"context"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestMaybeCloneSkippedStepDiffBeforeCompletion_UsesCacheMirrorSourceJobForChangingJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		jobType domaintypes.JobType
	}{
		{name: "mig", jobType: domaintypes.JobTypeMig},
		{name: "hook", jobType: domaintypes.JobTypeHook},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runID := domaintypes.NewRunID()
			sourceJobID := domaintypes.NewJobID()
			targetJobID := domaintypes.NewJobID()

			targetMeta := contracts.NewMigJobMeta()
			targetMeta.CacheMirror = &contracts.CacheMirrorMetadata{SourceJobID: sourceJobID}
			targetMetaJSON, err := contracts.MarshalJobMeta(targetMeta)
			if err != nil {
				t.Fatalf("marshal target meta: %v", err)
			}
			sourceMetaJSON, err := contracts.MarshalJobMeta(contracts.NewMigJobMeta())
			if err != nil {
				t.Fatalf("marshal source meta: %v", err)
			}

			sourceJob := store.Job{
				ID:      sourceJobID,
				RunID:   runID,
				RepoID:  domaintypes.NewRepoID(),
				JobType: tc.jobType,
				Meta:    sourceMetaJSON,
			}
			targetJob := store.Job{
				ID:      targetJobID,
				RunID:   runID,
				RepoID:  sourceJob.RepoID,
				JobType: tc.jobType,
				Meta:    targetMetaJSON,
			}

			sourceDiffKey := "diffs/run/" + runID.String() + "/source.patch.gz"
			sourceDiff := store.Diff{
				ID:        pgtype.UUID{Valid: true},
				RunID:     runID,
				JobID:     &sourceJobID,
				ObjectKey: ptr(sourceDiffKey),
			}

			st := &jobStore{
				getJobResults: map[domaintypes.JobID]store.Job{
					sourceJobID: sourceJob,
					targetJobID: targetJob,
				},
				getLatestDiffByJobByID: map[domaintypes.JobID]store.Diff{
					sourceJobID: sourceDiff,
				},
			}
			st.getLatestDiffByJob.err = pgx.ErrNoRows
			targetDiffKey := "diffs/run/" + runID.String() + "/target.patch.gz"
			st.createDiff.val = store.Diff{
				ID:        pgtype.UUID{Valid: true},
				RunID:     runID,
				JobID:     &targetJobID,
				ObjectKey: ptr(targetDiffKey),
			}

			bs := bsmock.New()
			sourcePatch := []byte("diff --git a/app b/app\n+patched")
			if _, err := bs.Put(context.Background(), sourceDiffKey, "application/gzip", sourcePatch); err != nil {
				t.Fatalf("seed source diff blob: %v", err)
			}
			bp := blobpersist.New(st, bs)

			if err := maybeCloneSkippedStepDiffBeforeCompletion(context.Background(), st, bp, targetJob); err != nil {
				t.Fatalf("maybeCloneSkippedStepDiffBeforeCompletion() error = %v", err)
			}

			if !st.createDiff.called {
				t.Fatal("expected CreateDiff to be called")
			}
			if got, want := st.createDiff.params.RunID, runID; got != want {
				t.Fatalf("CreateDiff run_id = %s, want %s", got, want)
			}
			if st.createDiff.params.JobID == nil || *st.createDiff.params.JobID != targetJobID {
				t.Fatalf("CreateDiff job_id = %v, want %s", st.createDiff.params.JobID, targetJobID)
			}
			clonedPatch, ok := bs.GetData(targetDiffKey)
			if !ok {
				t.Fatalf("expected cloned patch at key %q", targetDiffKey)
			}
			if string(clonedPatch) != string(sourcePatch) {
				t.Fatalf("cloned patch mismatch: got %q want %q", string(clonedPatch), string(sourcePatch))
			}
		})
	}
}

func TestMaybeCloneSkippedStepDiffBeforeCompletion_ErrorsWhenSourceDiffMissing(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	sourceJobID := domaintypes.NewJobID()
	targetJobID := domaintypes.NewJobID()

	targetMeta := contracts.NewMigJobMeta()
	targetMeta.CacheMirror = &contracts.CacheMirrorMetadata{SourceJobID: sourceJobID}
	targetMetaJSON, err := contracts.MarshalJobMeta(targetMeta)
	if err != nil {
		t.Fatalf("marshal target meta: %v", err)
	}
	sourceMetaJSON, err := contracts.MarshalJobMeta(contracts.NewMigJobMeta())
	if err != nil {
		t.Fatalf("marshal source meta: %v", err)
	}

	sourceJob := store.Job{
		ID:         sourceJobID,
		RunID:      runID,
		RepoID:     domaintypes.NewRepoID(),
		JobType:    domaintypes.JobTypeMig,
		RepoShaIn:  "0123456789abcdef0123456789abcdef01234567",
		RepoShaOut: "89abcdef0123456789abcdef0123456789abcdef",
		Meta:       sourceMetaJSON,
	}
	targetJob := store.Job{
		ID:      targetJobID,
		RunID:   runID,
		RepoID:  sourceJob.RepoID,
		JobType: domaintypes.JobTypeMig,
		Meta:    targetMetaJSON,
	}

	st := &jobStore{
		getJobResults: map[domaintypes.JobID]store.Job{
			sourceJobID: sourceJob,
			targetJobID: targetJob,
		},
	}
	st.getLatestDiffByJob.err = pgx.ErrNoRows
	bp := blobpersist.New(st, bsmock.New())

	err = maybeCloneSkippedStepDiffBeforeCompletion(context.Background(), st, bp, targetJob)
	if err == nil {
		t.Fatal("expected error when source mirrored job has no diff")
	}
}
