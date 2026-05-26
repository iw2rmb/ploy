package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// mustMutateAndUnmarshal calls mutateClaimSpec and unmarshals the result.
func mustMutateAndUnmarshal(t *testing.T, input claimSpecMutatorInput) map[string]any {
	t.Helper()
	merged, err := mutateClaimSpec(input)
	if err != nil {
		t.Fatalf("mutateClaimSpec: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	return out
}

func TestMutateClaimSpec_BaseMutators(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	out := mustMutateAndUnmarshal(t, claimSpecMutatorInput{
		spec:      []byte(`{"envs":{"EXISTING":"1"}}`),
		job:       store.Job{ID: jobID},
		jobType:   domaintypes.JobTypePostGate,
		globalEnv: map[string][]GlobalEnvVar{"GLOBAL": {{Value: "g", Target: domaintypes.GlobalEnvTargetGates}}},
	})

	if got := out["job_id"]; got != jobID.String() {
		t.Fatalf("job_id=%v, want %s", got, jobID.String())
	}
	envs := out["envs"].(map[string]any)
	if got := envs["EXISTING"]; got != "1" {
		t.Fatalf("envs[EXISTING]=%v", got)
	}
	if got := envs["GLOBAL"]; got != "g" {
		t.Fatalf("envs[GLOBAL]=%v", got)
	}
}

func TestMutateClaimSpec_InvalidSpec(t *testing.T) {
	t.Parallel()

	_, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:    []byte(`[]`),
		job:     store.Job{ID: domaintypes.NewJobID(), Meta: []byte(`{}`)},
		jobType: domaintypes.JobTypeMig,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "merge job_id into spec") {
		t.Fatalf("expected merge job_id wrapper in error, got %q", got)
	}
}
