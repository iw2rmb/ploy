package step

import (
	"context"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestDockerGateExecutor_CAPreambleIncluded(t *testing.T) {
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
		{
			name:      "cargo",
			workspace: func(t *testing.T) string { return createCargoWorkspace(t, "1.76") },
			spec: func() *contracts.StepGateSpec {
				return &contracts.StepGateSpec{
					Enabled: true,
					ImageOverrides: []contracts.BuildGateImageRule{{
						Stack: contracts.StackExpectation{Language: "rust", Release: "1.76"},
						Image: "rust:1.76",
					}},
				}
			},
			expectInCmd: "cargo test",
		},
		{
			name:      "pip",
			workspace: func(t *testing.T) string { return createPythonWorkspace(t, "3.11") },
			spec: func() *contracts.StepGateSpec {
				return &contracts.StepGateSpec{
					Enabled: true,
					ImageOverrides: []contracts.BuildGateImageRule{{
						Stack: contracts.StackExpectation{Language: "python", Release: "3.11"},
						Image: "python:3.11",
					}},
				}
			},
			expectInCmd: "python -m compileall",
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

			// Verify PLOY_CA_CERTS materializer preamble is present.
			if !strings.Contains(cmd, "PLOY_CA_CERTS") {
				t.Errorf("expected PLOY_CA_CERTS in command, got %q", cmd)
			}
			if !strings.Contains(cmd, "update-ca-certificates") {
				t.Errorf("expected update-ca-certificates in command, got %q", cmd)
			}
			if !strings.Contains(cmd, "keytool -importcert") {
				t.Errorf("expected keytool -importcert in command, got %q", cmd)
			}
			if !strings.Contains(cmd, "ploy-gate") {
				t.Errorf("expected ploy-gate CA directory name in command, got %q", cmd)
			}

			// Verify the build command is still present after the preamble.
			if !strings.Contains(cmd, tc.expectInCmd) {
				t.Errorf("expected %q in command after preamble, got %q", tc.expectInCmd, cmd)
			}
		})
	}
}

// TestCAPreambleScript verifies the caPreambleScript function returns a valid
// shell script that handles CA bundle installation through the PLOY_CA_CERTS materializer.
func TestCAPreambleScript(t *testing.T) {
	t.Parallel()

	preamble := caPreambleScript()

	// Verify key components of the materializer preamble are present.
	expectedFragments := []string{
		"PLOY_CA_CERTS",                            // env var check
		"mktemp",                                   // temp file creation
		"awk",                                      // cert splitting
		"update-ca-certificates",                   // system CA update
		"keytool -importcert",                      // Java cacerts import
		"ploy_gate_pem_",                           // alias prefix
		"changeit",                                 // default keystore password
		"--- PLOY_CA_CERTS materializer preamble",  // start marker
		"--- End PLOY_CA_CERTS materializer preamble", // end marker
	}

	for _, fragment := range expectedFragments {
		if !strings.Contains(preamble, fragment) {
			t.Errorf("expected %q in CA preamble, got:\n%s", fragment, preamble)
		}
	}

	// Verify materializer supports both file path and inline PEM detection.
	if !strings.Contains(preamble, `-f "${PLOY_CA_CERTS}"`) {
		t.Errorf("expected file-path detection in preamble, got:\n%s", preamble)
	}
}

// TestEnvMaterializerPreamble verifies envMaterializerPreamble returns the
// PLOY_CA_CERTS materializer preamble and that MaterializerForKey correctly
// identifies special vs plain-passthrough keys.
func TestEnvMaterializerPreamble(t *testing.T) {
	t.Parallel()

	preamble := envMaterializerPreamble()
	if !strings.Contains(preamble, "PLOY_CA_CERTS") {
		t.Errorf("expected PLOY_CA_CERTS in materializer preamble")
	}

	// PLOY_CA_CERTS has a materializer.
	if m := MaterializerForKey("PLOY_CA_CERTS"); m == nil {
		t.Error("expected materializer for PLOY_CA_CERTS, got nil")
	}

	// Unknown keys use plain passthrough (no materializer).
	if m := MaterializerForKey("OPENAI_API_KEY"); m != nil {
		t.Error("expected nil materializer for plain key OPENAI_API_KEY")
	}
}

// --- Stack Gate Pre-Check Tests ---

// createMavenWorkspace creates a workspace with a valid Maven pom.xml that has Java version.
