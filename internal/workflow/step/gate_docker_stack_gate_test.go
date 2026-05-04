package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestGateDocker_StackGate_PreCheckPass(t *testing.T) {
	t.Parallel()

	executor, rt, workspace := newDockerGateTestHarness(t)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		ImageOverrides: []contracts.BuildGateImageRule{{
			Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			Image: "maven:jdk17",
		}},
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Error("expected container Create to be called")
	}
	if !rt.startCalled {
		t.Error("expected container Start to be called")
	}
	if meta.StackGate.RuntimeImage != "maven:jdk17" {
		t.Errorf("RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, "maven:jdk17")
	}
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "pass" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "pass")
	}
	if !meta.StackGate.Enabled {
		t.Errorf("StackGate.Enabled = %v, want true", meta.StackGate.Enabled)
	}
	if meta.StackGate.Expected == nil || meta.StackGate.Expected.Release != "17" {
		t.Errorf("StackGate.Expected.Release = %v, want 17", meta.StackGate.Expected)
	}
	if meta.StackGate.Detected == nil || meta.StackGate.Detected.Release != "17" {
		t.Errorf("StackGate.Detected.Release = %v, want 17", meta.StackGate.Detected)
	}
}

// TestGateDocker_StackGate_PreCheckFailure consolidates mismatch, ambiguous, and unknown scenarios.
func TestGateDocker_StackGate_PreCheckFailure(t *testing.T) {
	t.Setenv("PLOY_CONTAINER_REGISTRY", "ghcr.io/iw2rmb/ploy")
	expectedRuntimeImage := "ghcr.io/iw2rmb/ploy/maven:jdk17"

	tests := []struct {
		name            string
		workspace       func(t *testing.T) string
		wantResult      string
		wantReasonPart  string
		wantFindingCode string
	}{
		{
			name: "mismatch (java 11 vs expected 17)",
			workspace: func(t *testing.T) string {
				return createMavenWorkspace(t, "11")
			},
			wantResult:      "mismatch",
			wantReasonPart:  "release:",
			wantFindingCode: "STACK_GATE_MISMATCH",
		},
		{
			name: "ambiguous (both maven and gradle)",
			workspace: func(t *testing.T) string {
				dir := t.TempDir()
				pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>test</groupId>
  <artifactId>test</artifactId>
  <version>1.0</version>
  <properties>
    <maven.compiler.release>17</maven.compiler.release>
  </properties>
</project>`
				gradleContent := `plugins { id 'java' }
java { toolchain { languageVersion = JavaLanguageVersion.of(17) } }`
				if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pomContent), 0o644); err != nil {
					t.Fatalf("write pom.xml: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(gradleContent), 0o644); err != nil {
					t.Fatalf("write build.gradle: %v", err)
				}
				return dir
			},
			wantResult:      "unknown",
			wantReasonPart:  "ambiguous",
			wantFindingCode: "STACK_GATE_UNKNOWN",
		},
		{
			name: "unknown (empty workspace, no build files)",
			workspace: func(t *testing.T) string {
				return t.TempDir()
			},
			wantResult:      "unknown",
			wantFindingCode: "STACK_GATE_UNKNOWN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workspace := tt.workspace(t)
			executor, rt, _ := newDockerGateTestHarness(t)

			spec := &contracts.StepGateSpec{
				Enabled: true,
				StackGate: &contracts.StepGateStackSpec{
					Enabled: true,
					Expect: &contracts.StackExpectation{
						Language: "java",
						Tool:     "maven",
						Release:  "17",
					},
				},
			}

			meta, err := executor.Execute(context.Background(), spec, workspace)
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}

			// Container should NOT be executed on pre-check failure.
			if rt.createCalled {
				t.Error("expected container Create NOT to be called")
			}

			// Stack gate result.
			if meta.StackGate == nil {
				t.Fatal("expected StackGate result in metadata")
			}
			if meta.StackGate.Result != tt.wantResult {
				t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, tt.wantResult)
			}
			if tt.wantReasonPart != "" && !strings.Contains(meta.StackGate.Reason, tt.wantReasonPart) {
				t.Errorf("StackGate.Reason = %q, want to contain %q", meta.StackGate.Reason, tt.wantReasonPart)
			}

			// Runtime image resolved for observability even though container not run.
			if meta.RuntimeImage != expectedRuntimeImage {
				t.Errorf("RuntimeImage = %q, want %q", meta.RuntimeImage, expectedRuntimeImage)
			}
			if meta.StackGate.RuntimeImage != expectedRuntimeImage {
				t.Errorf("StackGate.RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, expectedRuntimeImage)
			}

			// Log finding with expected code.
			found := false
			for _, f := range meta.LogFindings {
				if f.Code == tt.wantFindingCode {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected log finding with code %q, got %+v", tt.wantFindingCode, meta.LogFindings)
			}

			// Static check metadata must be present and marked failed.
			if len(meta.StaticChecks) == 0 {
				t.Fatalf("expected static check failure metadata, got none")
			}
			if meta.StaticChecks[0].Passed {
				t.Errorf("StaticChecks[0].Passed = %v, want %v", meta.StaticChecks[0].Passed, false)
			}
			if meta.StaticChecks[0].Tool != "stack-gate" {
				t.Errorf("StaticChecks[0].Tool = %q, want %q", meta.StaticChecks[0].Tool, "stack-gate")
			}
			if meta.StaticChecks[0].Language != "java" {
				t.Errorf("StaticChecks[0].Language = %q, want %q", meta.StaticChecks[0].Language, "java")
			}
		})
	}
}

// TestGateDocker_StackGate_ImageResolution consolidates image resolution scenarios.
func TestGateDocker_StackGate_ImageResolution(t *testing.T) {
	t.Setenv("PLOY_CONTAINER_REGISTRY", "ghcr.io/iw2rmb/ploy")

	tests := []struct {
		name           string
		imageOverrides []contracts.BuildGateImageRule
		wantImage      string
	}{
		{
			name: "custom inline rule",
			imageOverrides: []contracts.BuildGateImageRule{{
				Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
				Image: "custom-maven:java17",
			}},
			wantImage: "custom-maven:java17",
		},
		{
			name:      "default mapping file",
			wantImage: "ghcr.io/iw2rmb/ploy/maven:jdk17",
		},
		{
			name: "stack-mapped image",
			imageOverrides: []contracts.BuildGateImageRule{{
				Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
				Image: "resolved-from-mapping:17",
			}},
			wantImage: "resolved-from-mapping:17",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			executor, rt, workspace := newDockerGateTestHarness(t)

			spec := &contracts.StepGateSpec{
				Enabled:        true,
				ImageOverrides: tt.imageOverrides,
				StackGate: &contracts.StepGateStackSpec{
					Enabled: true,
					Expect: &contracts.StackExpectation{
						Language: "java",
						Tool:     "maven",
						Release:  "17",
					},
				},
			}

			meta, err := executor.Execute(context.Background(), spec, workspace)
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}

			if rt.captured.Image != tt.wantImage {
				t.Errorf("Image = %q, want %q", rt.captured.Image, tt.wantImage)
			}
			if meta.StackGate == nil {
				t.Fatal("expected StackGate result in metadata")
			}
			if meta.StackGate.RuntimeImage != tt.wantImage {
				t.Errorf("RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, tt.wantImage)
			}
			if meta.StackGate.Result != "pass" {
				t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "pass")
			}
		})
	}
}

func TestGateDocker_StackGate_NoMatchingDefaultRule_ReturnsNoImageRule(t *testing.T) {
	t.Parallel()

	workspace := createPythonWorkspace(t, "3.11")
	executor, rt, _ := newDockerGateTestHarness(t)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "python",
				Tool:     "pip",
				Release:  "3.11",
			},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if rt.createCalled {
		t.Error("expected container Create NOT to be called")
	}
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "unknown" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "unknown")
	}

	found := false
	for _, f := range meta.LogFindings {
		if f.Code == "STACK_GATE_NO_IMAGE_RULE" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log finding with code STACK_GATE_NO_IMAGE_RULE, got %+v", meta.LogFindings)
	}
}
