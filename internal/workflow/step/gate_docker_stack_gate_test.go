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
			Image: "maven:3-eclipse-temurin-17",
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

	// Verify container was executed.
	if !rt.createCalled {
		t.Error("expected container Create to be called")
	}
	if !rt.startCalled {
		t.Error("expected container Start to be called")
	}

	// Verify RuntimeImage is set.
	if meta.StackGate.RuntimeImage != "maven:3-eclipse-temurin-17" {
		t.Errorf("RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, "maven:3-eclipse-temurin-17")
	}

	// Verify stack gate result.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "pass" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "pass")
	}
	if meta.StackGate.Enabled != true {
		t.Errorf("StackGate.Enabled = %v, want true", meta.StackGate.Enabled)
	}
	if meta.StackGate.Expected == nil || meta.StackGate.Expected.Release != "17" {
		t.Errorf("StackGate.Expected.Release = %v, want 17", meta.StackGate.Expected)
	}
	if meta.StackGate.Detected == nil || meta.StackGate.Detected.Release != "17" {
		t.Errorf("StackGate.Detected.Release = %v, want 17", meta.StackGate.Detected)
	}
}

// TestGateDocker_StackGate_PreCheckMismatch verifies that Stack Gate fails early
// when detected stack doesn't match expectations, without running container.
func TestGateDocker_StackGate_PreCheckMismatch(t *testing.T) {
	t.Parallel()
	expectedRuntimeImage := resolveContainerRegistryPrefix() + "/maven:3-eclipse-temurin-17"

	// Create workspace with Java 11, but expect Java 17.
	workspace := createMavenWorkspace(t, "11")

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

	// Verify container was NOT executed (early return).
	if rt.createCalled {
		t.Error("expected container Create NOT to be called on mismatch")
	}

	// Verify stack gate result.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "mismatch" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "mismatch")
	}
	if !strings.Contains(meta.StackGate.Reason, "release:") {
		t.Errorf("StackGate.Reason = %q, expected to contain 'release:'", meta.StackGate.Reason)
	}

	// Verify runtime image is still resolved for observability (even though the container is not run).
	if meta.RuntimeImage != expectedRuntimeImage {
		t.Errorf("RuntimeImage = %q, want %q", meta.RuntimeImage, expectedRuntimeImage)
	}
	if meta.StackGate.RuntimeImage != expectedRuntimeImage {
		t.Errorf("StackGate.RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, expectedRuntimeImage)
	}

	// Verify static check reports failure.
	if len(meta.StaticChecks) == 0 || meta.StaticChecks[0].Passed {
		t.Errorf("expected static check to report failure, got %+v", meta.StaticChecks)
	}
	if len(meta.StaticChecks) > 0 {
		if meta.StaticChecks[0].Tool != "stack-gate" {
			t.Errorf("StaticChecks[0].Tool = %q, want %q", meta.StaticChecks[0].Tool, "stack-gate")
		}
		if meta.StaticChecks[0].Language != "java" {
			t.Errorf("StaticChecks[0].Language = %q, want %q", meta.StaticChecks[0].Language, "java")
		}
	}

	// Verify log finding with STACK_GATE_MISMATCH code.
	found := false
	for _, f := range meta.LogFindings {
		if f.Code == "STACK_GATE_MISMATCH" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log finding with code STACK_GATE_MISMATCH, got %+v", meta.LogFindings)
	}
}

// TestGateDocker_StackGate_PreCheckUnknown_Ambiguous verifies that Stack Gate fails early
// when both Maven and Gradle files are present (ambiguous detection).
func TestGateDocker_StackGate_PreCheckUnknown_Ambiguous(t *testing.T) {
	t.Parallel()
	expectedRuntimeImage := resolveContainerRegistryPrefix() + "/maven:3-eclipse-temurin-17"

	// Create workspace with both pom.xml and build.gradle.
	tmpDir := t.TempDir()
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
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomContent), 0o644); err != nil {
		t.Fatalf("failed to create pom.xml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(gradleContent), 0o644); err != nil {
		t.Fatalf("failed to create build.gradle: %v", err)
	}

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

	meta, err := executor.Execute(context.Background(), spec, tmpDir)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify container was NOT executed.
	if rt.createCalled {
		t.Error("expected container Create NOT to be called on ambiguous")
	}

	// Verify stack gate result is unknown.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "unknown" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "unknown")
	}
	if !strings.Contains(meta.StackGate.Reason, "ambiguous") {
		t.Errorf("StackGate.Reason = %q, expected to contain 'ambiguous'", meta.StackGate.Reason)
	}

	// Verify runtime image is still resolved for observability (even though the container is not run).
	if meta.RuntimeImage != expectedRuntimeImage {
		t.Errorf("RuntimeImage = %q, want %q", meta.RuntimeImage, expectedRuntimeImage)
	}
	if meta.StackGate.RuntimeImage != expectedRuntimeImage {
		t.Errorf("StackGate.RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, expectedRuntimeImage)
	}

	// Verify log finding with STACK_GATE_UNKNOWN code.
	found := false
	for _, f := range meta.LogFindings {
		if f.Code == "STACK_GATE_UNKNOWN" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log finding with code STACK_GATE_UNKNOWN, got %+v", meta.LogFindings)
	}
	if len(meta.StaticChecks) > 0 {
		if meta.StaticChecks[0].Tool != "stack-gate" {
			t.Errorf("StaticChecks[0].Tool = %q, want %q", meta.StaticChecks[0].Tool, "stack-gate")
		}
		if meta.StaticChecks[0].Language != "java" {
			t.Errorf("StaticChecks[0].Language = %q, want %q", meta.StaticChecks[0].Language, "java")
		}
	}
}

// TestGateDocker_StackGate_PreCheckUnknown_NoFiles verifies that Stack Gate fails early
// when no Maven or Gradle build files are present.
func TestGateDocker_StackGate_PreCheckUnknown_NoFiles(t *testing.T) {
	t.Parallel()
	expectedRuntimeImage := resolveContainerRegistryPrefix() + "/maven:3-eclipse-temurin-17"

	// Empty workspace.
	tmpDir := t.TempDir()

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

	meta, err := executor.Execute(context.Background(), spec, tmpDir)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify container was NOT executed.
	if rt.createCalled {
		t.Error("expected container Create NOT to be called on unknown")
	}

	// Verify stack gate result is unknown.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "unknown" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "unknown")
	}

	// Verify runtime image is still resolved for observability (even though the container is not run).
	if meta.RuntimeImage != expectedRuntimeImage {
		t.Errorf("RuntimeImage = %q, want %q", meta.RuntimeImage, expectedRuntimeImage)
	}
	if meta.StackGate.RuntimeImage != expectedRuntimeImage {
		t.Errorf("StackGate.RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, expectedRuntimeImage)
	}
	if len(meta.StaticChecks) > 0 {
		if meta.StaticChecks[0].Tool != "stack-gate" {
			t.Errorf("StaticChecks[0].Tool = %q, want %q", meta.StaticChecks[0].Tool, "stack-gate")
		}
		if meta.StaticChecks[0].Language != "java" {
			t.Errorf("StaticChecks[0].Language = %q, want %q", meta.StaticChecks[0].Language, "java")
		}
	}
}

// TestGateDocker_StackGate_ImageResolution verifies correct image is used when Stack Gate matches.
// This test uses ImageOverrides to provide inline rules and verifies the complete resolver flow.
func TestGateDocker_StackGate_ImageResolution(t *testing.T) {
	t.Parallel()

	executor, rt, workspace := newDockerGateTestHarness(t)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		ImageOverrides: []contracts.BuildGateImageRule{{
			Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			Image: "custom-maven:java17",
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

	// Verify correct image was used.
	if rt.captured.Image != "custom-maven:java17" {
		t.Errorf("Image = %q, want %q", rt.captured.Image, "custom-maven:java17")
	}

	// Verify RuntimeImage is set in metadata.
	if meta.StackGate.RuntimeImage != "custom-maven:java17" {
		t.Errorf("RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, "custom-maven:java17")
	}

	// Verify stack gate result shows pass.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "pass" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "pass")
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
	// Should return metadata, not error.
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Container should NOT be executed.
	if rt.createCalled {
		t.Error("expected container Create NOT to be called")
	}

	// Verify stack gate result is unknown.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "unknown" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "unknown")
	}

	// Verify log finding with appropriate code.
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

func TestGateDocker_StackGate_UsesDefaultMappingFileByDefault(t *testing.T) {
	t.Parallel()
	expectedRuntimeImage := resolveContainerRegistryPrefix() + "/maven:3-eclipse-temurin-17"

	executor, rt, workspace := newDockerGateTestHarness(t)

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

	if rt.captured.Image != expectedRuntimeImage {
		t.Errorf("Image = %q, want %q", rt.captured.Image, expectedRuntimeImage)
	}
	if meta.StackGate == nil || meta.StackGate.RuntimeImage != expectedRuntimeImage {
		t.Fatalf("RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, expectedRuntimeImage)
	}
}

func TestGateDocker_StackGate_UsesStackMappedImage(t *testing.T) {
	t.Parallel()

	executor, rt, workspace := newDockerGateTestHarness(t)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		ImageOverrides: []contracts.BuildGateImageRule{{
			Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			Image: "resolved-from-mapping:17",
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

	if rt.captured.Image != "resolved-from-mapping:17" {
		t.Errorf("Image = %q, want %q", rt.captured.Image, "resolved-from-mapping:17")
	}

	// Verify RuntimeImage in metadata.
	if meta.StackGate.RuntimeImage != "resolved-from-mapping:17" {
		t.Errorf("RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, "resolved-from-mapping:17")
	}
}
