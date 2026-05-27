package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestGatePlanResolver_StackDetectModePolicy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		workspace     func(t *testing.T) string
		stackDetect   *contracts.BuildGateStackConfig
		wantTerminal  bool
		wantCode      string
		wantErrPrefix string
		wantImage     string
		wantRelease   string
	}{
		{
			name:          "no_explicit_fallback_returns_internal_error",
			workspace:     func(t *testing.T) string { return createMavenWorkspaceNoJavaVersion(t) },
			stackDetect:   nil,
			wantTerminal:  true,
			wantCode:      "BUILD_GATE_STACK_DETECT_FAILED",
			wantErrPrefix: "BUILD_GATE_STACK_DETECT_FAILED:",
		},
		{
			name:        "forced_skips_detection_and_returns_configured_plan",
			workspace:   func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing-workspace") },
			stackDetect: &contracts.BuildGateStackConfig{Mode: contracts.BuildGateStackModeForced, Language: "java", Tool: "maven", Release: "17"},
			wantImage:   "planner-test:java17",
			wantRelease: "17",
		},
		{
			name:          "strict_returns_internal_error_on_incomplete_detection",
			workspace:     func(t *testing.T) string { return createMavenWorkspaceNoJavaVersion(t) },
			stackDetect:   &contracts.BuildGateStackConfig{Mode: contracts.BuildGateStackModeStrict, Language: "java", Tool: "maven", Release: "17"},
			wantTerminal:  true,
			wantCode:      "BUILD_GATE_STACK_DETECT_FAILED",
			wantErrPrefix: "BUILD_GATE_STACK_DETECT_FAILED:",
		},
		{
			name:         "strict_returns_mismatch_on_different_detected_stack",
			workspace:    func(t *testing.T) string { return createMavenWorkspace(t, "11") },
			stackDetect:  &contracts.BuildGateStackConfig{Mode: contracts.BuildGateStackModeStrict, Language: "java", Tool: "maven", Release: "17"},
			wantTerminal: true,
			wantCode:     "BUILD_GATE_STACK_MISMATCH",
		},
		{
			name:        "fallback_returns_configured_plan_on_incomplete_detection",
			workspace:   func(t *testing.T) string { return createMavenWorkspaceNoJavaVersion(t) },
			stackDetect: &contracts.BuildGateStackConfig{Mode: contracts.BuildGateStackModeFallback, Language: "java", Tool: "maven", Release: "17"},
			wantImage:   "planner-test:java17",
			wantRelease: "17",
		},
		{
			name:        "fallback_returns_configured_plan_on_detection_failure",
			workspace:   func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing-workspace") },
			stackDetect: &contracts.BuildGateStackConfig{Mode: contracts.BuildGateStackModeFallback, Language: "java", Tool: "maven", Release: "17"},
			wantImage:   "planner-test:java17",
			wantRelease: "17",
		},
		{
			name:        "fallback_uses_detected_stack_on_success",
			workspace:   func(t *testing.T) string { return createMavenWorkspace(t, "11") },
			stackDetect: &contracts.BuildGateStackConfig{Mode: contracts.BuildGateStackModeFallback, Language: "java", Tool: "maven", Release: "17"},
			wantImage:   "planner-test:java11",
			wantRelease: "11",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spec := &contracts.StepGateSpec{
				Enabled: true,
				ImageOverrides: []contracts.BuildGateImageRule{
					{
						Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "11"},
						Image: "planner-test:java11",
					},
					{
						Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
						Image: "planner-test:java17",
					},
				},
				StackDetect: tc.stackDetect,
			}

			plan, terminal := resolveGateExecutionPlan(context.Background(), tc.workspace(t), spec, "")
			if tc.wantTerminal {
				if terminal == nil {
					t.Fatal("expected terminal result")
				}
				if got := terminal.meta.LogFindings[0].Code; got != tc.wantCode {
					t.Fatalf("log code = %q, want %q", got, tc.wantCode)
				}
				if tc.wantErrPrefix != "" {
					if terminal.err == nil {
						t.Fatal("terminal err = nil, want non-nil")
					}
					if got := terminal.err.Error(); !strings.HasPrefix(got, tc.wantErrPrefix) {
						t.Fatalf("terminal err = %q, want prefix %q", got, tc.wantErrPrefix)
					}
				} else if terminal.err != nil {
					t.Fatalf("terminal err = %v, want nil", terminal.err)
				}
				return
			}

			if terminal != nil {
				t.Fatalf("expected plan result, got terminal=%+v", terminal)
			}
			if got, want := plan.image, tc.wantImage; got != want {
				t.Fatalf("plan image = %q, want %q", got, want)
			}
			if got, want := plan.tool, "maven"; got != want {
				t.Fatalf("plan tool = %q, want %q", got, want)
			}
			if got, want := plan.language, "java"; got != want {
				t.Fatalf("plan language = %q, want %q", got, want)
			}
			if got, want := plan.release, tc.wantRelease; got != want {
				t.Fatalf("plan release = %q, want %q", got, want)
			}
		})
	}
}

func TestGatePlanResolver_StackGateTerminalRuntimeImage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		workspace func(t *testing.T) string
		wantCode  string
		wantState string
	}{
		{
			name:      "mismatch",
			workspace: func(t *testing.T) string { return createMavenWorkspace(t, "11") },
			wantCode:  "STACK_GATE_MISMATCH",
			wantState: "mismatch",
		},
		{
			name: "unknown_ambiguous",
			workspace: func(t *testing.T) string {
				dir := t.TempDir()
				pom := `<?xml version="1.0" encoding="UTF-8"?><project><modelVersion>4.0.0</modelVersion><groupId>test</groupId><artifactId>test</artifactId><version>1.0</version><properties><maven.compiler.release>17</maven.compiler.release></properties></project>`
				gradle := "plugins { id 'java' }\njava { toolchain { languageVersion = JavaLanguageVersion.of(17) } }"
				if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pom), 0o644); err != nil {
					t.Fatalf("write pom.xml: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(gradle), 0o644); err != nil {
					t.Fatalf("write build.gradle: %v", err)
				}
				return dir
			},
			wantCode:  "STACK_GATE_UNKNOWN",
			wantState: "unknown",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spec := &contracts.StepGateSpec{
				Enabled: true,
				ImageOverrides: []contracts.BuildGateImageRule{{
					Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
					Image: "planner-test:java17",
				}},
				StackGate: &contracts.StepGateStackSpec{
					Enabled: true,
					Expect:  &contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
				},
			}

			_, terminal := resolveGateExecutionPlan(context.Background(), tc.workspace(t), spec, "")
			if terminal == nil {
				t.Fatal("expected terminal result")
			}
			if terminal.meta == nil || terminal.meta.StackGate == nil {
				t.Fatalf("expected stack gate metadata, got %+v", terminal.meta)
			}
			if got := terminal.meta.LogFindings[0].Code; got != tc.wantCode {
				t.Fatalf("log code = %q, want %q", got, tc.wantCode)
			}
			if got := terminal.meta.StackGate.Result; got != tc.wantState {
				t.Fatalf("stack gate result = %q, want %q", got, tc.wantState)
			}
			if !terminal.reportRuntimeImage {
				t.Fatal("expected runtime image reporting on stack gate terminal")
			}
			if got, want := terminal.meta.RuntimeImage, "planner-test:java17"; got != want {
				t.Fatalf("meta runtime image = %q, want %q", got, want)
			}
			if got, want := terminal.meta.StackGate.RuntimeImage, "planner-test:java17"; got != want {
				t.Fatalf("stack gate runtime image = %q, want %q", got, want)
			}
		})
	}
}
