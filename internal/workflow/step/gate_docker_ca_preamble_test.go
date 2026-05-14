package step

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestDockerGateExecutor_UsesImageOwnedCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		workspace func(t *testing.T) string
		spec      func() *contracts.StepGateSpec
	}{
		{
			name:      "maven",
			workspace: func(t *testing.T) string { return createMavenWorkspace(t, "17") },
			spec:      func() *contracts.StepGateSpec { return &contracts.StepGateSpec{Enabled: true} },
		},
		{
			name:      "gradle",
			workspace: func(t *testing.T) string { return createGradleWorkspace(t, "17") },
			spec:      func() *contracts.StepGateSpec { return &contracts.StepGateSpec{Enabled: true} },
		},
		{
			name:      "go",
			workspace: func(t *testing.T) string { return createGoWorkspace(t, "1.25") },
			spec: func() *contracts.StepGateSpec {
				return &contracts.StepGateSpec{
					Enabled: true,
					ImageOverrides: []contracts.BuildGateImageRule{{
						Stack: contracts.StackExpectation{Language: "go", Release: "1.25"},
						Image: "golang:1.25",
					}},
				}
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rt := &testContainerRuntime{}
			executor := NewDockerGateExecutor(rt)

			tmpDir := tc.workspace(t)
			spec := tc.spec()
			_, err := executor.Execute(context.Background(), spec, tmpDir)
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}

			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}
			if len(rt.captured.Command) != 0 {
				t.Fatalf("expected no explicit command override, got %v", rt.captured.Command)
			}
		})
	}
}

// --- Stack Gate Pre-Check Tests ---

// createMavenWorkspace creates a workspace with a valid Maven pom.xml that has Java version.
