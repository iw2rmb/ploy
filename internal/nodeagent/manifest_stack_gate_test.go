package nodeagent

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// assertStackInbound checks that a step has a derived/explicit inbound with
// the expected language and release values.
func assertStackInbound(t *testing.T, step StepMig, idx int, wantLang, wantRelease string) {
	t.Helper()
	if step.Stack == nil {
		t.Fatalf("steps[%d].Stack should not be nil", idx)
	}
	if step.Stack.Inbound == nil {
		t.Fatalf("steps[%d].Stack.Inbound should not be nil", idx)
	}
	if !step.Stack.Inbound.Enabled {
		t.Errorf("steps[%d].Stack.Inbound.Enabled should be true", idx)
	}
	if step.Stack.Inbound.Expect == nil {
		t.Fatalf("steps[%d].Stack.Inbound.Expect should not be nil", idx)
	}
	if step.Stack.Inbound.Expect.Language != wantLang {
		t.Errorf("steps[%d] inbound.expect.language = %q, want %q", idx, step.Stack.Inbound.Expect.Language, wantLang)
	}
	if step.Stack.Inbound.Expect.Release != wantRelease {
		t.Errorf("steps[%d] inbound.expect.release = %q, want %q", idx, step.Stack.Inbound.Expect.Release, wantRelease)
	}
}

// stepMig is a shorthand for building test StepMig values.
func stepMig(image string, stack *contracts.StackGateSpec) StepMig {
	return StepMig{
		MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: image}},
		Stack:            stack,
	}
}

func outbound(lang, release string) *contracts.StackGatePhaseSpec {
	return &contracts.StackGatePhaseSpec{
		Enabled: true,
		Expect:  &contracts.StackExpectation{Language: lang, Release: release},
	}
}

func inbound(lang, release string) *contracts.StackGatePhaseSpec {
	return outbound(lang, release) // same shape
}

func disabledOutbound(lang string) *contracts.StackGatePhaseSpec {
	return &contracts.StackGatePhaseSpec{
		Enabled: false,
		Expect:  &contracts.StackExpectation{Language: lang},
	}
}

// TestValidateAndDeriveStackGateChaining tests the chaining validation logic.
func TestValidateAndDeriveStackGateChaining(t *testing.T) {
	tests := []struct {
		name    string
		steps   []StepMig
		wantErr string
		check   func(t *testing.T, steps []StepMig)
	}{
		{
			name: "single step no chaining",
			steps: []StepMig{stepMig("test:latest", &contracts.StackGateSpec{
				Inbound: inbound("java", ""),
			})},
		},
		{
			name: "derives inbound from previous outbound",
			steps: []StepMig{
				stepMig("mig1:latest", &contracts.StackGateSpec{Outbound: outbound("java", "17")}),
				stepMig("mig2:latest", nil),
			},
			check: func(t *testing.T, steps []StepMig) {
				assertStackInbound(t, steps[1], 1, "java", "17")
			},
		},
		{
			name: "derives inbound when Stack exists but Inbound is nil",
			steps: []StepMig{
				stepMig("mig1:latest", &contracts.StackGateSpec{Outbound: outbound("java", "11")}),
				stepMig("mig2:latest", &contracts.StackGateSpec{Outbound: outbound("java", "17")}),
			},
			check: func(t *testing.T, steps []StepMig) {
				assertStackInbound(t, steps[1], 1, "java", "11")
			},
		},
		{
			name: "rejects mismatched explicit inbound",
			steps: []StepMig{
				stepMig("mig1:latest", &contracts.StackGateSpec{Outbound: outbound("java", "17")}),
				stepMig("mig2:latest", &contracts.StackGateSpec{Inbound: inbound("java", "11")}),
			},
			wantErr: "mismatch",
		},
		{
			name: "matching explicit inbound passes",
			steps: []StepMig{
				stepMig("mig1:latest", &contracts.StackGateSpec{Outbound: outbound("java", "17")}),
				stepMig("mig2:latest", &contracts.StackGateSpec{Inbound: inbound("java", "17")}),
			},
		},
		{
			name: "skips chaining when previous outbound disabled",
			steps: []StepMig{
				stepMig("mig1:latest", &contracts.StackGateSpec{Outbound: disabledOutbound("java")}),
				stepMig("mig2:latest", nil),
			},
			check: func(t *testing.T, steps []StepMig) {
				if steps[1].Stack != nil {
					t.Error("steps[1].Stack should remain nil when previous outbound is disabled")
				}
			},
		},
		{
			name: "skips chaining when previous has no Stack",
			steps: []StepMig{
				stepMig("mig1:latest", nil),
				stepMig("mig2:latest", nil),
			},
			check: func(t *testing.T, steps []StepMig) {
				if steps[1].Stack != nil {
					t.Error("steps[1].Stack should remain nil")
				}
			},
		},
		{
			name: "three step chain",
			steps: []StepMig{
				stepMig("mig1:latest", &contracts.StackGateSpec{
					Inbound:  inbound("java", "8"),
					Outbound: outbound("java", "11"),
				}),
				stepMig("mig2:latest", &contracts.StackGateSpec{
					Outbound: outbound("java", "17"),
				}),
				stepMig("mig3:latest", nil),
			},
			check: func(t *testing.T, steps []StepMig) {
				assertStackInbound(t, steps[1], 1, "java", "11")
				assertStackInbound(t, steps[2], 2, "java", "17")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAndDeriveStackGateChaining(tc.steps)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, tc.steps)
			}
		})
	}
}

// TestStackGatePhaseSpecToStepGate tests the conversion helper.
func TestStackGatePhaseSpecToStepGate(t *testing.T) {
	tests := []struct {
		name        string
		phase       *contracts.StackGatePhaseSpec
		wantNil     bool
		wantLang    string
		wantRelease string
	}{
		{name: "nil input", phase: nil, wantNil: true},
		{name: "disabled phase", phase: &contracts.StackGatePhaseSpec{Enabled: false}, wantNil: true},
		{
			name:        "enabled phase with expect",
			phase:       &contracts.StackGatePhaseSpec{Enabled: true, Expect: &contracts.StackExpectation{Language: "java", Release: "17"}},
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
			if !result.Enabled {
				t.Error("Enabled should be true")
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
	tests := []struct {
		name  string
		opts  RunOptions
		check func(t *testing.T, m contracts.StepManifest)
	}{
		{
			name: "threads StackGate when set",
			opts: RunOptions{
				StackGate: &contracts.StepGateStackSpec{
					Enabled: true,
					Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
				},
			},
			check: func(t *testing.T, m contracts.StepManifest) {
				if m.Gate.StackGate == nil {
					t.Fatal("manifest.Gate.StackGate should be threaded")
				}
				if !m.Gate.StackGate.Enabled {
					t.Error("StackGate.Enabled should be true")
				}
				if m.Gate.StackGate.Expect.Language != "java" || m.Gate.StackGate.Expect.Release != "17" {
					t.Errorf("StackGate.Expect = %+v, want java/17", m.Gate.StackGate.Expect)
				}
			},
		},
		{
			name: "no StackGate when not set",
			opts: RunOptions{StackGate: nil},
			check: func(t *testing.T, m contracts.StepManifest) {
				if m.Gate.StackGate != nil {
					t.Error("manifest.Gate.StackGate should be nil when not set")
				}
			},
		},
		{
			name: "threads build_gate.images into Gate.ImageOverrides",
			opts: RunOptions{
				BuildGate: BuildGateOptions{
					Enabled: true,
					Images: []contracts.BuildGateImageRule{
						{Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"}, Image: "maven:jdk17"},
					},
				},
			},
			check: func(t *testing.T, m contracts.StepManifest) {
				if len(m.Gate.ImageOverrides) != 1 {
					t.Fatalf("len(Gate.ImageOverrides) = %d, want 1", len(m.Gate.ImageOverrides))
				}
				if m.Gate.ImageOverrides[0].Image != "maven:jdk17" {
					t.Errorf("ImageOverrides[0].Image = %q, want %q", m.Gate.ImageOverrides[0].Image, "maven:jdk17")
				}
			},
		},
		{
			name: "outbound expectations for post gate",
			opts: RunOptions{
				Steps: []StepMig{{
					MigContainerSpec: MigContainerSpec{Image: contracts.JobImage{Universal: "test:latest"}},
					Stack: &contracts.StackGateSpec{
						Inbound:  inbound("java", "11"),
						Outbound: outbound("java", "17"),
					},
				}},
				StackGate: &contracts.StepGateStackSpec{
					Enabled: true,
					Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
				},
			},
			check: func(t *testing.T, m contracts.StepManifest) {
				if m.Gate.StackGate == nil {
					t.Fatal("manifest.Gate.StackGate should be set")
				}
				if m.Gate.StackGate.Expect.Release != "17" {
					t.Errorf("StackGate.Expect.Release = %q, want 17", m.Gate.StackGate.Expect.Release)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newStartRunRequest()
			manifest, err := buildGateManifestFromRequest(req, tc.opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if manifest.Gate == nil {
				t.Fatal("manifest.Gate should not be nil")
			}
			tc.check(t, manifest)
		})
	}
}
