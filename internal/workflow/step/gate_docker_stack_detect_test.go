package step

import (
	"context"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestGateDocker_StackDetect_FallbackUsesConfiguredStackOnMissingVersion(t *testing.T) {
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
			Mode:     contracts.BuildGateStackModeFallback,
			Language: "java",
			Tool:     "maven",
			Release:  "17",
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

func TestGateDocker_StackDetect_StrictCancelsOnDetectionFailure(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspaceNoJavaVersion(t)

	executor, rt, _ := newDockerGateTestHarness(t)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackDetect: &contracts.BuildGateStackConfig{
			Mode:     contracts.BuildGateStackModeStrict,
			Language: "java",
			Tool:     "maven",
			Release:  "17",
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err == nil {
		t.Fatal("expected error")
	}
	if got, wantPrefix := err.Error(), "BUILD_GATE_STACK_DETECT_FAILED:"; !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("error = %q, want prefix %q", got, wantPrefix)
	}

	// Container must NOT be executed.
	if rt.createCalled {
		t.Fatal("expected container Create NOT to be called")
	}
	if meta == nil || len(meta.LogFindings) == 0 {
		t.Fatal("expected log findings in metadata")
	}
}
