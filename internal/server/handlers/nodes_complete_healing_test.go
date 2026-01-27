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
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "mods-orw:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"retries": float64(2),
				"image":   "mods-codex:latest",
			},
			"router": map[string]any{
				"image": "mods-router:latest",
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
				ModType:     domaintypes.ModTypePreGate.String(),
				StepIndex:   1000,
				Meta:        []byte(`{}`),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mod-0",
				Status:      store.JobStatusCreated,
				ModType:     domaintypes.ModTypeMod.String(),
				StepIndex:   2000,
				Meta:        []byte(`{}`),
			},
		},
	}

	run := store.Run{ID: runID, SpecID: specID, Status: store.RunStatusStarted}

	if err := maybeCreateHealingJobs(ctx, st, run, runID, repoID, 1, domaintypes.StepIndex(1000)); err != nil {
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
	if healJob.ModImage != "mods-codex:latest" {
		t.Fatalf("expected heal-1-0 image=mods-codex:latest, got %q", healJob.ModImage)
	}
	if healJob.StepIndex <= 1000 || healJob.StepIndex >= 2000 {
		t.Fatalf("heal step_index=%f should be between 1000 and 2000", healJob.StepIndex)
	}

	reGateJob, ok := jobsByName["re-gate-1"]
	if !ok {
		t.Fatalf("expected re-gate-1 job to be created")
	}
	if reGateJob.Status != store.JobStatusCreated {
		t.Fatalf("expected re-gate-1 status=Created, got %s", reGateJob.Status)
	}
	if reGateJob.StepIndex <= healJob.StepIndex || reGateJob.StepIndex >= 2000 {
		t.Fatalf("re-gate step_index=%f should be between heal (%f) and 2000", reGateJob.StepIndex, healJob.StepIndex)
	}
}

func TestMaybeCreateHealingJobs_SecondAttemptUsesExistingHealJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "mods-orw:latest"},
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

	reGateStepIndex := 1666.6666666666665
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
				ModType:     domaintypes.ModTypePreGate.String(),
				StepIndex:   1000,
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
				ModType:     domaintypes.ModTypeHeal.String(),
				StepIndex:   1333.3333333333333,
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
				ModType:     domaintypes.ModTypeReGate.String(),
				StepIndex:   domaintypes.StepIndex(reGateStepIndex),
				Meta:        []byte(`{}`),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mod-0",
				Status:      store.JobStatusCreated,
				ModType:     domaintypes.ModTypeMod.String(),
				StepIndex:   2000,
				Meta:        []byte(`{}`),
			},
		},
	}

	run := store.Run{ID: runID, SpecID: specID, Status: store.RunStatusStarted}

	if err := maybeCreateHealingJobs(ctx, st, run, runID, repoID, 1, domaintypes.StepIndex(reGateStepIndex)); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if st.createJobCallCount != 2 {
		t.Fatalf("expected 2 CreateJob calls (heal + re-gate), got %d", st.createJobCallCount)
	}
	if st.createJobParams[0].Name != "heal-2-0" {
		t.Fatalf("expected first healing job name heal-2-0, got %q", st.createJobParams[0].Name)
	}
	if st.createJobParams[0].ModType != domaintypes.ModTypeHeal.String() {
		t.Fatalf("expected heal job ModType=heal, got %q", st.createJobParams[0].ModType)
	}
	if st.createJobParams[1].Name != "re-gate-2" {
		t.Fatalf("expected re-gate job name re-gate-2, got %q", st.createJobParams[1].Name)
	}
	if st.createJobParams[1].ModType != domaintypes.ModTypeReGate.String() {
		t.Fatalf("expected re-gate job ModType=re_gate, got %q", st.createJobParams[1].ModType)
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
