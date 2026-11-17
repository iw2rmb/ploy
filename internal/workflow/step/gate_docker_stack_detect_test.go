package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// A settings-only Gradle root selects the catalog's Java release and starts the gate.
func TestGateDocker_StackDetect_SettingsOnlyGradleUsesCatalogRelease(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	for name, content := range map[string]string{
		"settings.gradle.kts":       "include(\"bom\", \"app\")\n",
		"gradle/libs.versions.toml": "[versions]\njvmTarget = \"21\"\n",
		"bom/build.gradle.kts":      "plugins { `java-platform` }\n",
		"app/build.gradle.kts":      "plugins { java }\n",
	} {
		path := filepath.Join(workspace, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rt := &testContainerRuntime{}
	executor := NewGateExecutor(rt)
	spec := &contracts.StepGateSpec{
		Enabled: true,
		ImageOverrides: []contracts.BuildGateImageRule{{
			Stack: contracts.StackExpectation{Language: "java", Tool: "gradle", Release: "21"},
			Image: "custom-gradle:java21",
		}},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace, newTestGateJobMounts(t))
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected container Create to be called")
	}
	if rt.captured.Image != "custom-gradle:java21" {
		t.Fatalf("Image = %q, want custom-gradle:java21", rt.captured.Image)
	}
	if meta == nil || len(meta.StaticChecks) == 0 {
		t.Fatal("expected non-empty metadata")
	}
	if meta.StaticChecks[0].Tool != "gradle" {
		t.Fatalf("tool = %q, want gradle", meta.StaticChecks[0].Tool)
	}
}

func TestGateDocker_StackDetect_FallbackUsesConfiguredStackOnMissingVersion(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspaceNoJavaVersion(t)

	executor, rt, _ := newGateTestHarness(t)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		// Provide inline mapping for the fallback expectation.
		ImageOverrides: []contracts.BuildGateImageRule{{
			Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			Image: "custom-maven:java17",
		}},
		Stack: &contracts.BuildGateStackConfig{
			Mode:     contracts.BuildGateStackModeFallback,
			Language: "java",
			Tool:     "maven",
			Release:  "17",
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace, newTestGateJobMounts(t))
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

	executor, rt, _ := newGateTestHarness(t)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Stack: &contracts.BuildGateStackConfig{
			Mode:     contracts.BuildGateStackModeStrict,
			Language: "java",
			Tool:     "maven",
			Release:  "17",
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace, newTestGateJobMounts(t))
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
