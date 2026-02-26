package step

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestRunnerRun_ContainerLabels(t *testing.T) {
	t.Parallel()

	manifest := contracts.StepManifest{
		ID:    types.StepID("step-xyz"),
		Name:  "Test Run",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{Name: "workspace", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite, Hydration: &contracts.StepInputHydration{}},
		},
	}

	tests := []struct {
		name     string
		runID    types.RunID
		jobID    types.JobID
		expected map[string]string
	}{
		{
			name:  "run and job labels",
			runID: types.RunID("run-123"),
			jobID: types.JobID("job-123"),
			expected: map[string]string{
				types.LabelRunID: "run-123",
				types.LabelJobID: "job-123",
			},
		},
		{
			name:  "run label only",
			runID: types.RunID("run-456"),
			expected: map[string]string{
				types.LabelRunID: "run-456",
			},
		},
		{
			name:  "job label only",
			jobID: types.JobID("job-456"),
			expected: map[string]string{
				types.LabelJobID: "job-456",
			},
		},
		{
			name:     "no labels",
			expected: map[string]string{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rt := &testContainerRuntime{}
			runner := Runner{Containers: rt}

			req := Request{
				RunID:     tc.runID,
				JobID:     tc.jobID,
				Manifest:  manifest,
				Workspace: "/tmp/workspace",
			}

			if _, err := runner.Run(context.Background(), req); err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}

			labels := rt.captured.Labels
			if len(tc.expected) == 0 {
				if len(labels) != 0 {
					t.Fatalf("expected no labels, got %v", labels)
				}
				return
			}

			for key, want := range tc.expected {
				if got := labels[key]; got != want {
					t.Fatalf("label %q=%q, want %q", key, got, want)
				}
			}
			if len(labels) != len(tc.expected) {
				t.Fatalf("label count=%d, want %d: %v", len(labels), len(tc.expected), labels)
			}
		})
	}
}
