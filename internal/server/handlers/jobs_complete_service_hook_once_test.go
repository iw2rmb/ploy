package handlers

import (
	"context"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestRecordHookOnceLedger_UpsertsSuccessfulHookExecution(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
	svc := &CompleteJobService{store: st}
	job := store.Job{
		ID:     domaintypes.NewJobID(),
		RunID:  domaintypes.NewRunID(),
		RepoID: domaintypes.NewRepoID(),
	}
	state := &completeJobState{
		input: CompleteJobInput{
			Status: domaintypes.JobStatusSuccess,
			StatsPayload: JobStatsPayload{
				Metadata: map[string]string{
					"hook_hash": strings.Repeat("a", 64),
				},
			},
		},
		job:         job,
		serviceType: completeJobServiceTypeHook,
	}

	if err := svc.recordHookOnceLedger(context.Background(), state); err != nil {
		t.Fatalf("recordHookOnceLedger error: %v", err)
	}

	assertCalled(t, "UpsertHookOnceSuccess", st.upsertHookOnceSuccess.called)
	if st.markHookOnceSkipped.called {
		t.Fatal("did not expect MarkHookOnceSkipped to be called")
	}
	if st.upsertHookOnceSuccess.params.RunID != job.RunID ||
		st.upsertHookOnceSuccess.params.RepoID != job.RepoID ||
		st.upsertHookOnceSuccess.params.HookHash != strings.Repeat("a", 64) {
		t.Fatalf("unexpected UpsertHookOnceSuccess params: %+v", st.upsertHookOnceSuccess.params)
	}
	if st.upsertHookOnceSuccess.params.FirstSuccessJobID == nil || *st.upsertHookOnceSuccess.params.FirstSuccessJobID != job.ID {
		t.Fatalf("first_success_job_id = %v, want %s", st.upsertHookOnceSuccess.params.FirstSuccessJobID, job.ID)
	}
}

func TestRecordHookOnceLedger_SkippedHookDoesNotPersistLedger(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
	svc := &CompleteJobService{store: st}
	job := store.Job{
		ID:     domaintypes.NewJobID(),
		RunID:  domaintypes.NewRunID(),
		RepoID: domaintypes.NewRepoID(),
	}
	state := &completeJobState{
		input: CompleteJobInput{
			Status: domaintypes.JobStatusSuccess,
			StatsPayload: JobStatsPayload{
				Metadata: map[string]string{
					"hook_hash":             strings.Repeat("b", 64),
					"hook_should_run":       "false",
					"hook_once_skip_marked": "true",
				},
			},
		},
		job:         job,
		serviceType: completeJobServiceTypeHook,
	}

	if err := svc.recordHookOnceLedger(context.Background(), state); err != nil {
		t.Fatalf("recordHookOnceLedger error: %v", err)
	}

	if st.markHookOnceSkipped.called {
		t.Fatal("did not expect MarkHookOnceSkipped to be called")
	}
	if st.upsertHookOnceSuccess.called {
		t.Fatal("did not expect UpsertHookOnceSuccess to be called for skipped hook")
	}
}

func TestRecordHookOnceLedger_NoOpForNonHookJobs(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
	svc := &CompleteJobService{store: st}
	state := &completeJobState{
		input: CompleteJobInput{
			Status: domaintypes.JobStatusSuccess,
			StatsPayload: JobStatsPayload{
				Metadata: map[string]string{
					"hook_hash": strings.Repeat("c", 64),
				},
			},
		},
		job:         store.Job{ID: domaintypes.NewJobID(), RunID: domaintypes.NewRunID(), RepoID: domaintypes.NewRepoID()},
		serviceType: completeJobServiceTypeStep,
	}

	if err := svc.recordHookOnceLedger(context.Background(), state); err != nil {
		t.Fatalf("recordHookOnceLedger error: %v", err)
	}
	if st.upsertHookOnceSuccess.called || st.markHookOnceSkipped.called {
		t.Fatal("expected no hook-once store writes for non-hook service type")
	}
}

func TestRecordHookOnceLedger_InvalidHashReturnsError(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
	svc := &CompleteJobService{store: st}
	state := &completeJobState{
		input: CompleteJobInput{
			Status: domaintypes.JobStatusSuccess,
			StatsPayload: JobStatsPayload{
				Metadata: map[string]string{
					"hook_hash": "not-a-valid-hash",
				},
			},
		},
		job:         store.Job{ID: domaintypes.NewJobID(), RunID: domaintypes.NewRunID(), RepoID: domaintypes.NewRepoID()},
		serviceType: completeJobServiceTypeHook,
	}

	if err := svc.recordHookOnceLedger(context.Background(), state); err == nil {
		t.Fatal("expected error for invalid hook hash")
	}
	if st.upsertHookOnceSuccess.called || st.markHookOnceSkipped.called {
		t.Fatal("expected no hook-once store writes on parse error")
	}
}
