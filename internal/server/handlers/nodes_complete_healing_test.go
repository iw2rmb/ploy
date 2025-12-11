package handlers

import (
	"context"
	"encoding/json"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestMaybeCreateHealingJobs_SecondAttemptUsesModType verifies that healing
// retries are counted using the jobs.mod_type column so that subsequent
// attempts receive incremented attempt numbers (heal-branch-0-2-0, re-gate-branch-0-2, ...),
// avoiding duplicate job names on re-gate failure.
func TestMaybeCreateHealingJobs_SecondAttemptUsesModType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runID := domaintypes.NewRunID()

	// build_gate_healing with a single healing mod and retries=3.
	// Uses canonical single-mod schema (build_gate_healing.mod).
	spec := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(3),
			"mod": map[string]any{
				"image": "heal:latest",
			},
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	// Jobs for a run where:
	// - pre-gate (1000) failed previously
	// - heal-1-0 (1333.33) succeeded (first attempt)
	// - re-gate-1 (1666.66) has just failed (failedStepIndex)
	// - mod-0/post-gate are still created
	reGateStepIndex := 1666.6666666666665
	jobs := []store.Job{
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
			Meta:      []byte(`{}`),
		},
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "heal-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1333.3333333333333,
			Meta:      []byte(`{}`),
		},
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "re-gate-1",
			Status:    store.JobStatusFailed,
			ModType:   "re_gate",
			StepIndex: reGateStepIndex,
			Meta:      []byte(`{}`),
		},
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCreated,
			ModType:   "mod",
			StepIndex: 2000,
			Meta:      []byte(`{}`),
		},
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "post-gate",
			Status:    store.JobStatusCreated,
			ModType:   "post_gate",
			StepIndex: 3000,
			Meta:      []byte(`{}`),
		},
	}

	run := store.Run{
		ID:   runID.String(),
		Spec: specBytes,
	}

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	if err := maybeCreateHealingJobs(ctx, st, run, domaintypes.RunID(runID), domaintypes.StepIndex(reGateStepIndex), jobs); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	// With one existing "heal" job and one healing mod configured,
	// the second invocation should use attempt=2 and create:
	//   - heal-2-0 (pending)
	//   - re-gate-2 (created)
	if st.createJobCallCount != 2 {
		t.Fatalf("expected 2 CreateJob calls (heal + re-gate), got %d", st.createJobCallCount)
	}
	if len(st.createJobParams) != 2 {
		t.Fatalf("expected 2 CreateJobParams entries, got %d", len(st.createJobParams))
	}

	healJob := st.createJobParams[0]
	if healJob.Name != "heal-2-0" {
		t.Fatalf("expected first healing job name heal-2-0, got %q", healJob.Name)
	}
	if healJob.ModType != "heal" {
		t.Fatalf("expected heal job ModType=heal, got %q", healJob.ModType)
	}

	reGateJob := st.createJobParams[1]
	if reGateJob.Name != "re-gate-2" {
		t.Fatalf("expected re-gate job name re-gate-2, got %q", reGateJob.Name)
	}
	if reGateJob.ModType != "re_gate" {
		t.Fatalf("expected re-gate job ModType=re_gate, got %q", reGateJob.ModType)
	}
}

// TestMaybeCreateHealingJobs_FirstAttemptCreatesJobs verifies that when a gate
// fails and build_gate_healing.mod is configured, the first healing attempt
// creates a heal job and a re-gate job with the expected names and images.
func TestMaybeCreateHealingJobs_FirstAttemptCreatesJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runID := domaintypes.NewRunID()

	// Healing spec with a single mod and retries=2.
	spec := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(2),
			"mod": map[string]any{
				"image": "mods-codex:latest",
			},
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	// Jobs for a fresh run where pre-gate just failed.
	jobs := []store.Job{
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
			Meta:      []byte(`{}`),
		},
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCreated,
			ModType:   "mod",
			StepIndex: 2000,
			Meta:      []byte(`{}`),
		},
	}

	run := store.Run{
		ID:   runID.String(),
		Spec: specBytes,
	}

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	if err := maybeCreateHealingJobs(ctx, st, run, domaintypes.RunID(runID), domaintypes.StepIndex(1000), jobs); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	// Linear healing: exactly one heal job + one re-gate job.
	if st.createJobCallCount != 2 {
		t.Fatalf("expected 2 CreateJob calls (1 heal + 1 re-gate), got %d", st.createJobCallCount)
	}

	jobsByName := make(map[string]store.CreateJobParams)
	for _, p := range st.createJobParams {
		jobsByName[p.Name] = p
	}

	healJob, ok := jobsByName["heal-1-0"]
	if !ok {
		t.Fatalf("expected heal-codex-ai-1-0 job to be created")
	}
	if healJob.Status != store.JobStatusPending {
		t.Fatalf("expected heal-1-0 to be pending, got %s", healJob.Status)
	}
	if healJob.ModImage != "mods-codex:latest" {
		t.Fatalf("expected heal-1-0 image=mods-codex:latest, got %q", healJob.ModImage)
	}

	reGateJob, ok := jobsByName["re-gate-1"]
	if !ok {
		t.Fatalf("expected re-gate-codex-ai-1 job to be created")
	}
	if reGateJob.Status != store.JobStatusCreated {
		t.Fatalf("expected re-gate-1 to be created, got %s", reGateJob.Status)
	}

	if healJob.StepIndex <= 1000 || healJob.StepIndex >= 2000 {
		t.Fatalf("heal step_index=%f should be between 1000 and 2000", healJob.StepIndex)
	}
	if reGateJob.StepIndex <= healJob.StepIndex || reGateJob.StepIndex >= 2000 {
		t.Fatalf("re-gate step_index=%f should be between heal (%f) and 2000", reGateJob.StepIndex, healJob.StepIndex)
	}
}

// TestMaybeCompleteMultiStepRun_MultiBranchWinner_Succeeds verifies that when
// one healing branch wins (re-gate succeeds) and other branches are skipped,
// the run completes with succeeded status (not canceled).
func TestMaybeCompleteMultiStepRun_MultiBranchWinner_Succeeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runID := domaintypes.NewRunID()

	jobs := []store.Job{
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
		},
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "heal-branch-a-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1333,
		},
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "re-gate-branch-a-1",
			Status:    store.JobStatusSucceeded,
			ModType:   "re_gate",
			StepIndex: 1444,
		},
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "heal-branch-b-1-0",
			Status:    store.JobStatusSkipped, // Loser branch heal skipped after winner selected.
			ModType:   "heal",
			StepIndex: 1555,
		},
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "re-gate-branch-b-1",
			Status:    store.JobStatusSkipped, // Loser branch re-gate skipped after winner selected.
			ModType:   "re_gate",
			StepIndex: 1666,
		},
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "mod",
			StepIndex: 2000,
		},
	}

	run := store.Run{
		ID:   runID.String(),
		Spec: []byte(`{}`),
	}

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	if err := maybeCompleteMultiStepRun(ctx, st, nil, run, domaintypes.RunID(runID)); err != nil {
		t.Fatalf("maybeCompleteMultiStepRun returned error: %v", err)
	}

	if !st.updateRunCompletionCalled {
		t.Fatalf("expected UpdateRunCompletion to be called")
	}
	if st.updateRunCompletionParams.Status != store.RunStatusSucceeded {
		t.Fatalf("expected run status=succeeded, got %s", st.updateRunCompletionParams.Status)
	}
}

// No standalone parser: build_gate_healing.mod is read inline by maybeCreateHealingJobs.
