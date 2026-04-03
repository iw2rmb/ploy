package step

import (
	"context"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestDockerGateExecutor_NoPreambleInCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		workspace   func(t *testing.T) string
		spec        func() *contracts.StepGateSpec
		expectInCmd string // substring expected in the shell command
	}{
		{
			name:        "maven",
			workspace:   func(t *testing.T) string { return createMavenWorkspace(t, "17") },
			spec:        func() *contracts.StepGateSpec { return &contracts.StepGateSpec{Enabled: true} },
			expectInCmd: "mvn --ff -B -q -e",
		},
		{
			name:        "gradle",
			workspace:   func(t *testing.T) string { return createGradleWorkspace(t, "17") },
			spec:        func() *contracts.StepGateSpec { return &contracts.StepGateSpec{Enabled: true} },
			expectInCmd: "gradle -q --stacktrace",
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
			expectInCmd: "go test ./...",
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
			if len(rt.captured.Command) != 3 {
				t.Fatalf("expected 3-element command, got %v", rt.captured.Command)
			}

			cmd := rt.captured.Command[2]

			// CA delivery is now via Hydra CA mounts; no env preamble should
			// be injected into gate commands.
			if strings.Contains(cmd, "PLOY_CA_CERTS") {
				t.Errorf("unexpected PLOY_CA_CERTS preamble in command: %q", cmd)
			}

			// The build command must still be present.
			if !strings.Contains(cmd, tc.expectInCmd) {
				t.Errorf("expected %q in command, got %q", tc.expectInCmd, cmd)
			}
		})
	}
}

// TestCAPreambleScript_ReturnsEmpty verifies that caPreambleScript returns an
// empty string after the PLOY_CA_CERTS materializer was removed in favor of
// Hydra CA mount delivery.
func TestCAPreambleScript_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	preamble := caPreambleScript()
	if preamble != "" {
		t.Errorf("expected empty preamble after PLOY_CA_CERTS materializer removal, got:\n%s", preamble)
	}
}

// TestEnvMaterializerPreamble_Empty verifies that envMaterializerPreamble
// returns an empty string and MaterializerForKey returns nil for all keys
// after the PLOY_CA_CERTS materializer was removed.
func TestEnvMaterializerPreamble_Empty(t *testing.T) {
	t.Parallel()

	preamble := envMaterializerPreamble()
	if preamble != "" {
		t.Errorf("expected empty preamble, got: %q", preamble)
	}

	// No materializer should be registered.
	if m := MaterializerForKey("PLOY_CA_CERTS"); m != nil {
		t.Error("expected nil materializer for PLOY_CA_CERTS after removal")
	}
	if m := MaterializerForKey("OPENAI_API_KEY"); m != nil {
		t.Error("expected nil materializer for plain key OPENAI_API_KEY")
	}
}

// TestDockerGateExecutor_NoPreambleOnPrepOverride verifies that prep override
// commands no longer contain the PLOY_CA_CERTS materializer preamble after
// CA delivery was switched to Hydra mount entries.
func TestDockerGateExecutor_NoPreambleOnPrepOverride(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		command contracts.CommandSpec
	}{
		{
			name:    "shell_form",
			command: contracts.CommandSpec{Shell: "echo prep-gate-test"},
		},
		{
			name:    "exec_form",
			command: contracts.CommandSpec{Exec: []string{"/usr/bin/echo", "prep-gate-test"}},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rt := &testContainerRuntime{}
			executor := NewDockerGateExecutor(rt)
			workspace := createMavenWorkspace(t, "17")

			spec := &contracts.StepGateSpec{
				Enabled: true,
				GateProfile: &contracts.BuildGateProfileOverride{
					Command: tc.command,
				},
			}

			_, err := executor.Execute(context.Background(), spec, workspace)
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}
			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}

			cmd := rt.captured.Command[len(rt.captured.Command)-1]

			if strings.Contains(cmd, "PLOY_CA_CERTS") {
				t.Errorf("unexpected PLOY_CA_CERTS preamble in prep override command: %q", cmd)
			}
			if !strings.Contains(cmd, "prep-gate-test") {
				t.Errorf("expected original command content in prep override command, got %q", cmd)
			}
		})
	}
}

// --- Stack Gate Pre-Check Tests ---

// createMavenWorkspace creates a workspace with a valid Maven pom.xml that has Java version.
