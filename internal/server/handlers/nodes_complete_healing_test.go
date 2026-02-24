package handlers

import (
	"context"
	"encoding/json"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestMaybeCreateHealingJobs_FirstAttemptCreatesJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	specID := domaintypes.NewSpecID()

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "migs-orw:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"retries": float64(2),
				"image":   "migs-codex:latest",
			},
			"router": map[string]any{
				"image": "migs-router:latest",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	st := &mockStore{
		getSpecResult: store.Spec{ID: specID, Spec: specBytes},
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "pre-gate",
				Status:      store.JobStatusFail,
				JobType:     domaintypes.JobTypePreGate.String(),
				Meta:        []byte(`{}`),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      store.JobStatusCreated,
				JobType:     domaintypes.JobTypeMod.String(),
				Meta:        []byte(`{}`),
			},
		},
	}
	successorID := st.listJobsByRunRepoAttemptResult[1].ID
	st.listJobsByRunRepoAttemptResult[0].NextID = &successorID

	run := store.Run{ID: runID, SpecID: specID, Status: store.RunStatusStarted}
	failed := st.listJobsByRunRepoAttemptResult[0]

	if err := maybeCreateHealingJobs(ctx, st, run, failed); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if st.createJobCallCount != 2 {
		t.Fatalf("expected 2 CreateJob calls (heal + re-gate), got %d", st.createJobCallCount)
	}

	jobsByName := make(map[string]store.CreateJobParams)
	for _, p := range st.createJobParams {
		jobsByName[p.Name] = p
	}

	healJob, ok := jobsByName["heal-1-0"]
	if !ok {
		t.Fatalf("expected heal-1-0 job to be created")
	}
	if healJob.Status != store.JobStatusQueued {
		t.Fatalf("expected heal-1-0 status=Queued, got %s", healJob.Status)
	}
	if healJob.JobImage != "migs-codex:latest" {
		t.Fatalf("expected heal-1-0 image=migs-codex:latest, got %q", healJob.JobImage)
	}
	if healJob.NextID == nil {
		t.Fatalf("expected heal-1-0 next_id to be set")
	}

	reGateJob, ok := jobsByName["re-gate-1"]
	if !ok {
		t.Fatalf("expected re-gate-1 job to be created")
	}
	if reGateJob.Status != store.JobStatusCreated {
		t.Fatalf("expected re-gate-1 status=Created, got %s", reGateJob.Status)
	}
	if healJob.NextID == nil || *healJob.NextID != reGateJob.ID {
		t.Fatalf("expected heal to point to re-gate")
	}
	if reGateJob.NextID == nil || *reGateJob.NextID != successorID {
		t.Fatalf("expected re-gate to preserve original successor %s", successorID)
	}
	if len(st.updateJobNextIDParams) < 2 {
		t.Fatalf("expected next_id rewiring updates, got %d", len(st.updateJobNextIDParams))
	}
}

func TestMaybeCreateHealingJobs_SecondAttemptUsesExistingHealJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	specID := domaintypes.NewSpecID()

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "migs-orw:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"retries": float64(3),
				"image":   "heal:latest",
			},
			"router": map[string]any{
				"image": "router:latest",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	st := &mockStore{
		getSpecResult: store.Spec{ID: specID, Spec: specBytes},
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "pre-gate",
				Status:      store.JobStatusFail,
				JobType:     domaintypes.JobTypePreGate.String(),
				Meta:        []byte(`{}`),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "heal-1-0",
				Status:      store.JobStatusSuccess,
				JobType:     domaintypes.JobTypeHeal.String(),
				Meta:        []byte(`{}`),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "re-gate-1",
				Status:      store.JobStatusFail,
				JobType:     domaintypes.JobTypeReGate.String(),
				Meta:        []byte(`{}`),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      store.JobStatusCreated,
				JobType:     domaintypes.JobTypeMod.String(),
				Meta:        []byte(`{}`),
			},
		},
	}
	heal1ID := st.listJobsByRunRepoAttemptResult[1].ID
	reGate1ID := st.listJobsByRunRepoAttemptResult[2].ID
	mod0ID := st.listJobsByRunRepoAttemptResult[3].ID
	st.listJobsByRunRepoAttemptResult[0].NextID = &heal1ID
	st.listJobsByRunRepoAttemptResult[1].NextID = &reGate1ID
	st.listJobsByRunRepoAttemptResult[2].NextID = &mod0ID

	run := store.Run{ID: runID, SpecID: specID, Status: store.RunStatusStarted}
	failed := st.listJobsByRunRepoAttemptResult[2]

	if err := maybeCreateHealingJobs(ctx, st, run, failed); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if st.createJobCallCount != 2 {
		t.Fatalf("expected 2 CreateJob calls (heal + re-gate), got %d", st.createJobCallCount)
	}
	if st.createJobParams[0].Name != "heal-2-0" {
		t.Fatalf("expected first healing job name heal-2-0, got %q", st.createJobParams[0].Name)
	}
	if st.createJobParams[0].JobType != domaintypes.JobTypeHeal.String() {
		t.Fatalf("expected heal job JobType=heal, got %q", st.createJobParams[0].JobType)
	}
	if st.createJobParams[1].Name != "re-gate-2" {
		t.Fatalf("expected re-gate job name re-gate-2, got %q", st.createJobParams[1].Name)
	}
	if st.createJobParams[1].JobType != domaintypes.JobTypeReGate.String() {
		t.Fatalf("expected re-gate job JobType=re_gate, got %q", st.createJobParams[1].JobType)
	}
	if st.createJobParams[1].NextID == nil || *st.createJobParams[1].NextID != mod0ID {
		t.Fatalf("expected re-gate-2 to link back to original successor %s", mod0ID)
	}
}

func TestMaybeCompleteMultiStepRun_FinishesWhenAllReposTerminal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()

	st := &mockStore{
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusSuccess, Count: 1},
			{Status: store.RunRepoStatusFail, Count: 1},
		},
	}

	run := store.Run{ID: runID, Status: store.RunStatusStarted}
	if err := maybeCompleteRunIfAllReposTerminal(ctx, st, nil, run, runID); err != nil {
		t.Fatalf("maybeCompleteRunIfAllReposTerminal returned error: %v", err)
	}

	if !st.updateRunStatusCalled {
		t.Fatalf("expected UpdateRunStatus to be called")
	}
	if st.updateRunStatusParams.ID != runID || st.updateRunStatusParams.Status != store.RunStatusFinished {
		t.Fatalf("unexpected UpdateRunStatus params: %+v", st.updateRunStatusParams)
	}
}
