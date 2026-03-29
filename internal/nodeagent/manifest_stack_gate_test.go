package nodeagent

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestValidateAndDeriveStackGateChaining tests the chaining validation logic.
func TestValidateAndDeriveStackGateChaining(t *testing.T) {
	t.Run("single step no chaining", func(t *testing.T) {
		steps := []StepMig{{
			MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "test:latest"}},
			Stack: &contracts.StackGateSpec{
				Inbound: &contracts.StackGatePhaseSpec{
					Enabled: true,
					Expect:  &contracts.StackExpectation{Language: "java"},
				},
			},
		}}

		if err := validateAndDeriveStackGateChaining(steps); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("derives inbound from previous outbound", func(t *testing.T) {
		steps := []StepMig{
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod1:latest"}},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod2:latest"}},
			},
		}

		if err := validateAndDeriveStackGateChaining(steps); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if steps[1].Stack == nil {
			t.Fatal("steps[1].Stack should have been created")
		}
		if steps[1].Stack.Inbound == nil {
			t.Fatal("steps[1].Stack.Inbound should have been derived")
		}
		if !steps[1].Stack.Inbound.Enabled {
			t.Error("derived inbound.enabled should be true")
		}
		if steps[1].Stack.Inbound.Expect == nil {
			t.Fatal("derived inbound.expect should not be nil")
		}
		if steps[1].Stack.Inbound.Expect.Language != "java" {
			t.Errorf("derived inbound.expect.language = %q, want java", steps[1].Stack.Inbound.Expect.Language)
		}
		if steps[1].Stack.Inbound.Expect.Release != "17" {
			t.Errorf("derived inbound.expect.release = %q, want 17", steps[1].Stack.Inbound.Expect.Release)
		}
	})

	t.Run("derives inbound when Stack exists but Inbound is nil", func(t *testing.T) {
		steps := []StepMig{
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod1:latest"}},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "11"},
					},
				},
			},
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod2:latest"}},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
		}

		if err := validateAndDeriveStackGateChaining(steps); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if steps[1].Stack.Inbound == nil {
			t.Fatal("steps[1].Stack.Inbound should have been derived")
		}
		if steps[1].Stack.Inbound.Expect.Release != "11" {
			t.Errorf("derived inbound.expect.release = %q, want 11", steps[1].Stack.Inbound.Expect.Release)
		}
	})

	t.Run("rejects mismatched explicit inbound", func(t *testing.T) {
		steps := []StepMig{
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod1:latest"}},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod2:latest"}},
				Stack: &contracts.StackGateSpec{
					Inbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "11"}, // Mismatch!
					},
				},
			},
		}

		err := validateAndDeriveStackGateChaining(steps)
		if err == nil {
			t.Fatal("expected error for mismatched inbound")
		}

		if !strings.Contains(err.Error(), "mismatch") {
			t.Errorf("error should mention mismatch: %v", err)
		}
		if !strings.Contains(err.Error(), "steps[1].stack.inbound") {
			t.Errorf("error should reference steps[1]: %v", err)
		}
	})

	t.Run("matching explicit inbound passes", func(t *testing.T) {
		steps := []StepMig{
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod1:latest"}},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod2:latest"}},
				Stack: &contracts.StackGateSpec{
					Inbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
		}

		if err := validateAndDeriveStackGateChaining(steps); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("skips chaining when previous outbound disabled", func(t *testing.T) {
		steps := []StepMig{
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod1:latest"}},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: false,
						Expect:  &contracts.StackExpectation{Language: "java"},
					},
				},
			},
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod2:latest"}},
			},
		}

		if err := validateAndDeriveStackGateChaining(steps); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if steps[1].Stack != nil {
			t.Error("steps[1].Stack should remain nil when previous outbound is disabled")
		}
	})

	t.Run("skips chaining when previous has no Stack", func(t *testing.T) {
		steps := []StepMig{
			{MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod1:latest"}}},
			{MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod2:latest"}}},
		}

		if err := validateAndDeriveStackGateChaining(steps); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if steps[1].Stack != nil {
			t.Error("steps[1].Stack should remain nil")
		}
	})

	t.Run("three step chain", func(t *testing.T) {
		steps := []StepMig{
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod1:latest"}},
				Stack: &contracts.StackGateSpec{
					Inbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "8"},
					},
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "11"},
					},
				},
			},
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod2:latest"}},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
			{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "mod3:latest"}},
			},
		}

		if err := validateAndDeriveStackGateChaining(steps); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if steps[1].Stack.Inbound == nil {
			t.Fatal("steps[1].Stack.Inbound should have been derived")
		}
		if steps[1].Stack.Inbound.Expect.Release != "11" {
			t.Errorf("steps[1] inbound.expect.release = %q, want 11", steps[1].Stack.Inbound.Expect.Release)
		}

		if steps[2].Stack == nil || steps[2].Stack.Inbound == nil {
			t.Fatal("steps[2].Stack.Inbound should have been derived")
		}
		if steps[2].Stack.Inbound.Expect.Release != "17" {
			t.Errorf("steps[2] inbound.expect.release = %q, want 17", steps[2].Stack.Inbound.Expect.Release)
		}
	})
}

// TestStackGatePhaseSpecToStepGate tests the conversion helper.
func TestStackGatePhaseSpecToStepGate(t *testing.T) {
	tests := []struct {
		name        string
		phase       *contracts.StackGatePhaseSpec
		wantNil     bool
		wantEnabled bool
		wantLang    string
		wantRelease string
	}{
		{
			name:    "nil input",
			phase:   nil,
			wantNil: true,
		},
		{
			name:    "disabled phase",
			phase:   &contracts.StackGatePhaseSpec{Enabled: false},
			wantNil: true,
		},
		{
			name: "enabled phase with expect",
			phase: &contracts.StackGatePhaseSpec{
				Enabled: true,
				Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
			},
			wantNil:     false,
			wantEnabled: true,
			wantLang:    "java",
			wantRelease: "17",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stackGatePhaseSpecToStepGate(tc.phase, nil)
			if tc.wantNil {
				if result != nil {
					t.Fatalf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.Enabled != tc.wantEnabled {
				t.Errorf("Enabled = %v, want %v", result.Enabled, tc.wantEnabled)
			}
			if result.Expect == nil {
				t.Fatal("Expect should not be nil")
			}
			if result.Expect.Language != tc.wantLang {
				t.Errorf("Expect.Language = %q, want %q", result.Expect.Language, tc.wantLang)
			}
			if result.Expect.Release != tc.wantRelease {
				t.Errorf("Expect.Release = %q, want %q", result.Expect.Release, tc.wantRelease)
			}
		})
	}
}

// TestBuildGateManifestFromRequest_StackGateThreading tests that StackGate
// is correctly threaded into gate manifests via typedOpts.StackGate.
func TestBuildGateManifestFromRequest_StackGateThreading(t *testing.T) {
	t.Run("threads StackGate when set", func(t *testing.T) {
		req := newStartRunRequest()
		typedOpts := RunOptions{
			StackGate: &contracts.StepGateStackSpec{
				Enabled: true,
				Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
			},
		}

		manifest, err := buildGateManifestFromRequest(req, typedOpts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if manifest.Gate == nil {
			t.Fatal("manifest.Gate should not be nil")
		}
		if manifest.Gate.StackGate == nil {
			t.Fatal("manifest.Gate.StackGate should be threaded")
		}
		if !manifest.Gate.StackGate.Enabled {
			t.Error("StackGate.Enabled should be true")
		}
		if manifest.Gate.StackGate.Expect == nil {
			t.Fatal("StackGate.Expect should not be nil")
		}
		if manifest.Gate.StackGate.Expect.Language != "java" {
			t.Errorf("StackGate.Expect.Language = %q, want java", manifest.Gate.StackGate.Expect.Language)
		}
		if manifest.Gate.StackGate.Expect.Release != "17" {
			t.Errorf("StackGate.Expect.Release = %q, want 17", manifest.Gate.StackGate.Expect.Release)
		}
	})

	t.Run("no StackGate when not set", func(t *testing.T) {
		req := newStartRunRequest()
		typedOpts := RunOptions{StackGate: nil}

		manifest, err := buildGateManifestFromRequest(req, typedOpts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if manifest.Gate == nil {
			t.Fatal("manifest.Gate should not be nil")
		}
		if manifest.Gate.StackGate != nil {
			t.Error("manifest.Gate.StackGate should be nil when not set")
		}
	})

	t.Run("threads build_gate.images into Gate.ImageOverrides", func(t *testing.T) {
		req := newStartRunRequest()
		typedOpts := RunOptions{
			BuildGate: BuildGateOptions{
				Enabled: true,
				Images: []contracts.BuildGateImageRule{
					{Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"}, Image: "maven:3-eclipse-temurin-17"},
				},
			},
		}

		manifest, err := buildGateManifestFromRequest(req, typedOpts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if manifest.Gate == nil {
			t.Fatal("manifest.Gate should not be nil")
		}
		if len(manifest.Gate.ImageOverrides) != 1 {
			t.Fatalf("len(Gate.ImageOverrides) = %d, want 1", len(manifest.Gate.ImageOverrides))
		}
		if manifest.Gate.ImageOverrides[0].Image != "maven:3-eclipse-temurin-17" {
			t.Errorf("Gate.ImageOverrides[0].Image = %q, want %q", manifest.Gate.ImageOverrides[0].Image, "maven:3-eclipse-temurin-17")
		}
	})

	t.Run("outbound expectations for post gate", func(t *testing.T) {
		req := newStartRunRequest()
		typedOpts := RunOptions{
			Steps: []StepMig{{
				MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "test:latest"}},
				Stack: &contracts.StackGateSpec{
					Inbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "11"},
					},
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			}},
			StackGate: &contracts.StepGateStackSpec{
				Enabled: true,
				Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
			},
		}

		manifest, err := buildGateManifestFromRequest(req, typedOpts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if manifest.Gate.StackGate == nil {
			t.Fatal("manifest.Gate.StackGate should be set")
		}
		if manifest.Gate.StackGate.Expect.Release != "17" {
			t.Errorf("StackGate.Expect.Release = %q, want 17 (outbound)", manifest.Gate.StackGate.Expect.Release)
		}
	})
}
