package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// mockGateRuntimeMinimal implements the subset of ContainerRuntime used by
// dockerGateExecutor plus Remove, so we can verify cleanup behavior without
// depending on the real Docker client or runner mocks.
type mockGateRuntimeMinimal struct {
	createCalled bool
	startCalled  bool
	waitCalled   bool
	logsCalled   bool
	removeCalled bool
	lastSpec     ContainerSpec
}

func (m *mockGateRuntimeMinimal) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	m.createCalled = true
	m.lastSpec = spec
	return ContainerHandle{ID: "mock-id"}, nil
}

func (m *mockGateRuntimeMinimal) Start(ctx context.Context, h ContainerHandle) error {
	m.startCalled = true
	return nil
}

func (m *mockGateRuntimeMinimal) Wait(ctx context.Context, h ContainerHandle) (ContainerResult, error) {
	m.waitCalled = true
	return ContainerResult{ExitCode: 0}, nil
}

func (m *mockGateRuntimeMinimal) Logs(ctx context.Context, h ContainerHandle) ([]byte, error) {
	m.logsCalled = true
	return []byte("ok"), nil
}

func (m *mockGateRuntimeMinimal) Remove(ctx context.Context, h ContainerHandle) error {
	m.removeCalled = true
	return nil
}

func TestDockerGateExecutor_RemovesContainerAfterExecution(t *testing.T) {
	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: "java",
	}

	_, err := executor.Execute(context.Background(), spec, "/tmp/workspace")
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.removeCalled {
		t.Fatalf("expected Remove to be called on container runtime after gate execution")
	}
	if !rt.createCalled || !rt.startCalled || !rt.waitCalled || !rt.logsCalled {
		t.Fatalf("expected create/start/wait/logs to be called before remove; got %+v", rt)
	}
}

// TestDockerGateExecutor_EnvPassthrough verifies that environment variables from
// StepGateSpec.Env are passed through to the Docker container. This ensures that
// global env vars injected by the control plane (e.g., CA_CERTS_PEM_BUNDLE,
// CODEX_AUTH_JSON) are available to image-level startup hooks.
func TestDockerGateExecutor_EnvPassthrough(t *testing.T) {
	t.Parallel()

	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: "java",
		Env: map[string]string{
			"CA_CERTS_PEM_BUNDLE": "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
			"CODEX_AUTH_JSON":     `{"token":"secret"}`,
			"CUSTOM_VAR":          "custom-value",
		},
	}

	_, err := executor.Execute(context.Background(), spec, t.TempDir())
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	// Verify all env vars from spec.Env are passed to the container spec.
	if rt.lastSpec.Env == nil {
		t.Fatal("expected ContainerSpec.Env to be set, got nil")
	}
	if len(rt.lastSpec.Env) != 3 {
		t.Fatalf("expected 3 env vars, got %d: %v", len(rt.lastSpec.Env), rt.lastSpec.Env)
	}

	// Check each expected key.
	expectedKeys := []string{"CA_CERTS_PEM_BUNDLE", "CODEX_AUTH_JSON", "CUSTOM_VAR"}
	for _, key := range expectedKeys {
		if _, ok := rt.lastSpec.Env[key]; !ok {
			t.Errorf("expected env var %q to be present, but it's missing", key)
		}
	}

	// Verify values are correct.
	if rt.lastSpec.Env["CA_CERTS_PEM_BUNDLE"] != spec.Env["CA_CERTS_PEM_BUNDLE"] {
		t.Errorf("CA_CERTS_PEM_BUNDLE mismatch: got %q, want %q",
			rt.lastSpec.Env["CA_CERTS_PEM_BUNDLE"], spec.Env["CA_CERTS_PEM_BUNDLE"])
	}
	if rt.lastSpec.Env["CODEX_AUTH_JSON"] != spec.Env["CODEX_AUTH_JSON"] {
		t.Errorf("CODEX_AUTH_JSON mismatch: got %q, want %q",
			rt.lastSpec.Env["CODEX_AUTH_JSON"], spec.Env["CODEX_AUTH_JSON"])
	}
	if rt.lastSpec.Env["CUSTOM_VAR"] != spec.Env["CUSTOM_VAR"] {
		t.Errorf("CUSTOM_VAR mismatch: got %q, want %q",
			rt.lastSpec.Env["CUSTOM_VAR"], spec.Env["CUSTOM_VAR"])
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

			rt := &mockGateRuntimeMinimal{}
			executor := NewDockerGateExecutor(rt)

			spec := &contracts.StepGateSpec{
				Enabled: true,
				Profile: "java",
				Env:     tc.env,
			}

			_, err := executor.Execute(context.Background(), spec, t.TempDir())
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}

			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}

			// For nil/empty input, the container spec env should be nil or empty.
			if len(rt.lastSpec.Env) != 0 {
				t.Errorf("expected empty env for %s, got %v", tc.name, rt.lastSpec.Env)
			}
		})
	}
}

func TestDockerGateExecutor_GradleCommandOmitsFailFast(t *testing.T) {
	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	// Use an explicit java-gradle profile; workspace contents are irrelevant for this path.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte{}, 0o644); err != nil {
		t.Fatalf("failed to create dummy build.gradle: %v", err)
	}

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: "java-gradle",
	}

	if _, err := executor.Execute(context.Background(), spec, tmpDir); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	if len(rt.lastSpec.Command) != 3 {
		t.Fatalf("expected 3-element command, got %v", rt.lastSpec.Command)
	}

	cmd := rt.lastSpec.Command[2]
	if !strings.Contains(cmd, "gradle -q --stacktrace") {
		t.Fatalf("expected gradle command with -q --stacktrace, got %q", cmd)
	}
	if strings.Contains(cmd, "--fail-fast") {
		t.Fatalf("expected gradle command not to contain --fail-fast, got %q", cmd)
	}
	if !strings.Contains(cmd, "test -p /workspace") {
		t.Fatalf("expected gradle command to run tests in /workspace, got %q", cmd)
	}
}
