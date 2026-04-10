package step

import (
	"context"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"gopkg.in/yaml.v3"
)

func TestDockerGateExecutor_DoesNotRemoveContainerAfterExecution(t *testing.T) {
	executor, rt, workspace := newDockerGateTestHarness(t)
	spec := &contracts.StepGateSpec{Enabled: true}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if rt.removeCalled {
		t.Fatalf("expected Remove not to be called on container runtime after gate execution")
	}
	if !rt.createCalled || !rt.startCalled || !rt.waitCalled || !rt.logsCalled {
		t.Fatalf("expected create/start/wait/logs to be called; got %+v", rt)
	}
}

func TestDockerGateExecutor_ReportsRuntimeImageBeforeContainerCreate(t *testing.T) {
	var (
		observerCalled bool
		observedImage  string
	)

	rt := &testContainerRuntime{
		createFn: func(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
			if !observerCalled {
				t.Fatalf("expected runtime image observer to be called before container Create")
			}
			return ContainerHandle("mock"), nil
		},
	}
	executor := NewDockerGateExecutor(rt)

	ctx := WithGateRuntimeImageObserver(context.Background(), func(_ context.Context, image string) {
		observerCalled = true
		observedImage = image
	})

	workspace := createMavenWorkspace(t, "17")
	spec := &contracts.StepGateSpec{Enabled: true}

	if _, err := executor.Execute(ctx, spec, workspace); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !observerCalled {
		t.Fatalf("expected runtime image observer to be called")
	}
	if observedImage == "" {
		t.Fatalf("observed image is empty")
	}
}

func TestDockerGateExecutor_PassesContainerLabelsFromContext(t *testing.T) {
	executor, rt, workspace := newDockerGateTestHarness(t)
	spec := &contracts.StepGateSpec{Enabled: true}
	ctx := WithGateContainerLabels(context.Background(), map[string]string{
		types.LabelRunID: "run-123",
		types.LabelJobID: "job-456",
	})

	if _, err := executor.Execute(ctx, spec, workspace); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	if got := rt.captured.Labels[types.LabelRunID]; got != "run-123" {
		t.Fatalf("label %q = %q, want %q", types.LabelRunID, got, "run-123")
	}
	if got := rt.captured.Labels[types.LabelJobID]; got != "job-456" {
		t.Fatalf("label %q = %q, want %q", types.LabelJobID, got, "job-456")
	}
}

// TestDockerGateExecutor_EnvPassthrough verifies that environment variables from
// StepGateSpec.Env are passed through to the Docker container. This ensures that
// global env vars injected by the control plane are available to image-level
// startup hooks.
func TestDockerGateExecutor_EnvPassthrough(t *testing.T) {
	t.Parallel()

	executor, rt, workspace := newDockerGateTestHarness(t)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Env: map[string]string{
			"APP_TLS_CERT":  "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
			"APP_AUTH_JSON": `{"token":"secret"}`,
			"CUSTOM_VAR":    "custom-value",
		},
	}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	// Verify all env vars from spec.Env are passed to the container spec.
	if rt.captured.Env == nil {
		t.Fatal("expected ContainerSpec.Env to be set, got nil")
	}
	if len(rt.captured.Env) != 3 {
		t.Fatalf("expected 3 env vars, got %d: %v", len(rt.captured.Env), rt.captured.Env)
	}

	// Check each expected key.
	expectedKeys := []string{"APP_TLS_CERT", "APP_AUTH_JSON", "CUSTOM_VAR"}
	for _, key := range expectedKeys {
		if _, ok := rt.captured.Env[key]; !ok {
			t.Errorf("expected env var %q to be present, but it's missing", key)
		}
	}

	// Verify values are correct.
	if rt.captured.Env["APP_TLS_CERT"] != spec.Env["APP_TLS_CERT"] {
		t.Errorf("APP_TLS_CERT mismatch: got %q, want %q",
			rt.captured.Env["APP_TLS_CERT"], spec.Env["APP_TLS_CERT"])
	}
	if rt.captured.Env["APP_AUTH_JSON"] != spec.Env["APP_AUTH_JSON"] {
		t.Errorf("APP_AUTH_JSON mismatch: got %q, want %q",
			rt.captured.Env["APP_AUTH_JSON"], spec.Env["APP_AUTH_JSON"])
	}
	if rt.captured.Env["CUSTOM_VAR"] != spec.Env["CUSTOM_VAR"] {
		t.Errorf("CUSTOM_VAR mismatch: got %q, want %q",
			rt.captured.Env["CUSTOM_VAR"], spec.Env["CUSTOM_VAR"])
	}
}

// TestDockerGateExecutor_EmptyEnv verifies that the gate executor handles
// empty or nil env maps gracefully without errors.
func TestDockerGateExecutor_EmptyEnv(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		env  map[string]string
	}{
		{"nil_env", nil},
		{"empty_env", map[string]string{}},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			executor, rt, workspace := newDockerGateTestHarness(t)

			spec := &contracts.StepGateSpec{
				Enabled: true,
				Env:     tc.env,
			}

			_, err := executor.Execute(context.Background(), spec, workspace)
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}

			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}

			// For nil/empty input, the container spec env should be nil or empty.
			if len(rt.captured.Env) != 0 {
				t.Errorf("expected empty env for %s, got %v", tc.name, rt.captured.Env)
			}
		})
	}
}

func TestDockerGateExecutor_FailureStructuredEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		workspaceFn      func(t *testing.T) string
		logs             string
		wantEvidence     bool
		wantEvidenceMode string
	}{
		{
			name:        "gradle failure emits structured evidence",
			workspaceFn: func(t *testing.T) string { return createGradleWorkspace(t, "17") },
			logs: `
* What went wrong:
An exception occurred applying plugin request [id: 'org.springframework.boot', version: '3.0.5']
> Failed to apply plugin 'org.springframework.boot'.
BUILD FAILED in 1s
`,
			wantEvidence:     true,
			wantEvidenceMode: "plugin_apply",
		},
		{
			name:        "maven failure has no structured evidence",
			workspaceFn: func(t *testing.T) string { return createMavenWorkspace(t, "17") },
			logs: `
[ERROR] COMPILATION ERROR :
[ERROR] /workspace/src/main/java/A.java:[1,1] cannot find symbol
`,
			wantEvidence: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			workspace := tt.workspaceFn(t)
			rt := &testContainerRuntime{
				waitFn: func(context.Context, ContainerHandle) (ContainerResult, error) {
					return ContainerResult{ExitCode: 1}, nil
				},
				logsFn: func(context.Context, ContainerHandle) ([]byte, error) {
					return []byte(tt.logs), nil
				},
			}
			executor := NewDockerGateExecutor(rt)
			meta, err := executor.Execute(context.Background(), &contracts.StepGateSpec{Enabled: true}, workspace)
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}
			if meta == nil || len(meta.LogFindings) == 0 {
				t.Fatalf("expected log findings, got %+v", meta)
			}

			finding := meta.LogFindings[0]
			if strings.TrimSpace(finding.Message) == "" {
				t.Fatal("expected non-empty finding message")
			}

			if !tt.wantEvidence {
				if strings.TrimSpace(finding.Evidence) != "" {
					t.Fatalf("expected empty evidence, got:\n%s", finding.Evidence)
				}
				return
			}

			if strings.TrimSpace(finding.Evidence) == "" {
				t.Fatal("expected non-empty evidence")
			}
			var payload map[string]any
			if err := yaml.Unmarshal([]byte(finding.Evidence), &payload); err != nil {
				t.Fatalf("invalid evidence yaml: %v", err)
			}
			if mode, _ := payload["mode"].(string); mode != tt.wantEvidenceMode {
				t.Fatalf("mode=%q, want %q; evidence:\n%s", mode, tt.wantEvidenceMode, finding.Evidence)
			}
		})
	}
}
