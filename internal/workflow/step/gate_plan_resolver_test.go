package step

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestGatePlanResolver_CommandTargetUnsupportedPolicy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		stackGate     bool
		enforceLock   bool
		wantCode      string
		wantCancelled bool
	}{
		{
			name:          "detected_stack_no_lock",
			stackGate:     false,
			enforceLock:   false,
			wantCode:      "BUILD_GATE_TARGET_UNSUPPORTED",
			wantCancelled: false,
		},
		{
			name:          "detected_stack_lock_enabled",
			stackGate:     false,
			enforceLock:   true,
			wantCode:      "BUILD_GATE_TARGET_UNSUPPORTED",
			wantCancelled: true,
		},
		{
			name:          "stack_gate_no_lock",
			stackGate:     true,
			enforceLock:   false,
			wantCode:      "STACK_GATE_TARGET_UNSUPPORTED",
			wantCancelled: false,
		},
		{
			name:          "stack_gate_lock_enabled",
			stackGate:     true,
			enforceLock:   true,
			wantCode:      "STACK_GATE_TARGET_UNSUPPORTED",
			wantCancelled: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workspace := createMavenWorkspace(t, "17")
			spec := &contracts.StepGateSpec{
				Enabled: true,
				ImageOverrides: []contracts.BuildGateImageRule{{
					Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
					Image: "planner-test:java17",
				}},
				Target:            contracts.GateProfileTargetUnit,
				EnforceTargetLock: tc.enforceLock,
				GateProfile: &contracts.BuildGateProfileOverride{
					Command: contracts.CommandSpec{Shell: "echo candidate"},
					Target:  contracts.GateProfileTargetAllTests,
				},
			}
			if tc.stackGate {
				spec.StackGate = &contracts.StepGateStackSpec{
					Enabled: true,
					Expect:  &contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
				}
			}

			plan, terminal := resolveGateExecutionPlan(context.Background(), workspace, spec, "")
			if terminal == nil {
				t.Fatal("expected terminal result")
			}
			if plan.image != "" || len(plan.cmd) != 0 {
				t.Fatalf("expected empty plan on terminal path, got %+v", plan)
			}
			if terminal.meta == nil || len(terminal.meta.LogFindings) == 0 {
				t.Fatalf("expected terminal metadata with log findings, got %+v", terminal.meta)
			}
			if got := terminal.meta.LogFindings[0].Code; got != tc.wantCode {
				t.Fatalf("log code = %q, want %q", got, tc.wantCode)
			}
			if got := errors.Is(terminal.err, ErrRepoCancelled); got != tc.wantCancelled {
				t.Fatalf("cancelled = %v, want %v (err=%v)", got, tc.wantCancelled, terminal.err)
			}
			if !terminal.reportRuntimeImage {
				t.Fatal("expected runtime image reporting on target-unsupported terminal")
			}
			if strings.TrimSpace(terminal.runtimeImage) == "" {
				t.Fatal("expected non-empty terminal runtime image")
			}
			if strings.TrimSpace(terminal.meta.RuntimeImage) == "" {
				t.Fatal("expected metadata RuntimeImage on target-unsupported terminal")
			}
		})
	}
}

func TestGatePlanResolver_StackDetectDefaultPolicy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		defaultValue  bool
		wantTerminal  bool
		wantCode      string
		wantCancelled bool
	}{
		{
			name:          "default_false_cancels",
			defaultValue:  false,
			wantTerminal:  true,
			wantCode:      "BUILD_GATE_STACK_DETECT_FAILED",
			wantCancelled: true,
		},
		{
			name:         "default_true_returns_plan",
			defaultValue: true,
			wantTerminal: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workspace := createMavenWorkspaceNoJavaVersion(t)
			spec := &contracts.StepGateSpec{
				Enabled: true,
				ImageOverrides: []contracts.BuildGateImageRule{{
					Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
					Image: "planner-test:java17",
				}},
				StackDetect: &contracts.BuildGateStackConfig{
					Enabled:  true,
					Language: "java",
					Release:  "17",
					Default:  tc.defaultValue,
				},
			}

			plan, terminal := resolveGateExecutionPlan(context.Background(), workspace, spec, "")
			if tc.wantTerminal {
				if terminal == nil {
					t.Fatal("expected terminal result")
				}
				if got := terminal.meta.LogFindings[0].Code; got != tc.wantCode {
					t.Fatalf("log code = %q, want %q", got, tc.wantCode)
				}
				if got := errors.Is(terminal.err, ErrRepoCancelled); got != tc.wantCancelled {
					t.Fatalf("cancelled = %v, want %v (err=%v)", got, tc.wantCancelled, terminal.err)
				}
				return
			}

			if terminal != nil {
				t.Fatalf("expected plan result, got terminal=%+v", terminal)
			}
			if got, want := plan.image, "planner-test:java17"; got != want {
				t.Fatalf("plan image = %q, want %q", got, want)
			}
			if got, want := plan.tool, "maven"; got != want {
				t.Fatalf("plan tool = %q, want %q", got, want)
			}
			if got, want := plan.language, "java"; got != want {
				t.Fatalf("plan language = %q, want %q", got, want)
			}
			if got, want := plan.release, "17"; got != want {
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
