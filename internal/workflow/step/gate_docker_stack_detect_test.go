package step

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestGateDocker_StackDetect_DefaultTrue_FallsBackOnMissingVersion(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspaceNoJavaVersion(t)

	executor, rt, _ := newDockerGateTestHarness(t)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		// Provide inline mapping for the fallback expectation.
		ImageOverrides: []contracts.BuildGateImageRule{{
			Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			Image: "custom-maven:java17",
		}},
		StackDetect: &contracts.BuildGateStackConfig{
			Enabled:  true,
			Language: "java",
			Release:  "17",
			Default:  true,
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Container should be executed.
	if !rt.createCalled {
		t.Fatal("expected container Create to be called")
	}
	if rt.captured.Image != "custom-maven:java17" {
		t.Fatalf("Image = %q, want %q", rt.captured.Image, "custom-maven:java17")
	}
	if meta == nil || len(meta.StaticChecks) == 0 {
		t.Fatal("expected non-empty metadata")
	}
	if meta.StaticChecks[0].Tool != "maven" {
		t.Fatalf("tool = %q, want %q", meta.StaticChecks[0].Tool, "maven")
	}
}

func TestGateDocker_StackDetect_DefaultFalse_CancelsOnDetectionFailure(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspaceNoJavaVersion(t)

	executor, rt, _ := newDockerGateTestHarness(t)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackDetect: &contracts.BuildGateStackConfig{
			Enabled:  true,
			Language: "java",
			Release:  "17",
			Default:  false,
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrRepoCancelled) {
		t.Fatalf("error = %v, want ErrRepoCancelled", err)
	}

	// Container must NOT be executed.
	if rt.createCalled {
		t.Fatal("expected container Create NOT to be called")
	}
	if meta == nil || len(meta.LogFindings) == 0 {
		t.Fatal("expected log findings in metadata")
	}
}
