package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestMaybeCreateHealingJobs_SecondAttemptUsesModType verifies that healing
// retries are counted using the jobs.mod_type column so that subsequent
// attempts receive incremented attempt numbers (heal-2-0, re-gate-2, ...),
// avoiding duplicate job names on re-gate failure.
func TestMaybeCreateHealingJobs_SecondAttemptUsesModType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// build_gate_healing with a single healing mod and retries=3
	spec := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(3),
			"mods": []any{
				map[string]any{"image": "heal:latest"},
			},
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	// Jobs for a run where:
	// - pre-gate (1000) failed previously
	// - heal-1-0 (1333.33) succeeded
	// - re-gate-1 (1666.66) has just failed (failedStepIndex)
	// - mod-0/post-gate are still created
	reGateStepIndex := 1666.6666666666665
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1333.3333333333333,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "re-gate-1",
			Status:    store.JobStatusFailed,
			ModType:   "re_gate",
			StepIndex: reGateStepIndex,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCreated,
			ModType:   "mod",
			StepIndex: 2000,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "post-gate",
			Status:    store.JobStatusCreated,
			ModType:   "post_gate",
			StepIndex: 3000,
			Meta:      []byte(`{}`),
		},
	}

	run := store.Run{
		ID:   runID,
		Spec: specBytes,
	}

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	if err := maybeCreateHealingJobs(ctx, st, run, runID, domaintypes.StepIndex(reGateStepIndex), jobs); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	// With one existing "heal" job and one healing mod configured, the second
	// invocation should use attempt=2 and create:
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
