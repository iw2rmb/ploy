package nodeagent

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// assertStackInbound checks that a step has a derived/explicit inbound with
// the expected language and release values.
func assertStackInbound(t *testing.T, step contracts.MigStep, idx int, wantLang, wantRelease string) {
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

// stepMig is a shorthand for building canonical test steps.
func stepMig(image string, stack *contracts.StackGateSpec) contracts.MigStep {
	return contracts.MigStep{
		Image: contracts.JobImage{Universal: image},
		Stack: stack,
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
		steps   []contracts.MigStep
		wantErr string
		check   func(t *testing.T, steps []contracts.MigStep)
	}{
		{
			name: "single step no chaining",
			steps: []contracts.MigStep{stepMig("test:latest", &contracts.StackGateSpec{
				Inbound: inbound("java", ""),
			})},
		},
		{
			name: "derives inbound from previous outbound",
			steps: []contracts.MigStep{
				stepMig("mig1:latest", &contracts.StackGateSpec{Outbound: outbound("java", "17")}),
				stepMig("mig2:latest", nil),
			},
			check: func(t *testing.T, steps []contracts.MigStep) {
				assertStackInbound(t, steps[1], 1, "java", "17")
			},
		},
		{
			name: "derives inbound when Stack exists but Inbound is nil",
			steps: []contracts.MigStep{
				stepMig("mig1:latest", &contracts.StackGateSpec{Outbound: outbound("java", "11")}),
				stepMig("mig2:latest", &contracts.StackGateSpec{Outbound: outbound("java", "17")}),
			},
			check: func(t *testing.T, steps []contracts.MigStep) {
				assertStackInbound(t, steps[1], 1, "java", "11")
			},
		},
		{
			name: "rejects mismatched explicit inbound",
			steps: []contracts.MigStep{
				stepMig("mig1:latest", &contracts.StackGateSpec{Outbound: outbound("java", "17")}),
				stepMig("mig2:latest", &contracts.StackGateSpec{Inbound: inbound("java", "11")}),
			},
			wantErr: "mismatch",
		},
		{
			name: "matching explicit inbound passes",
			steps: []contracts.MigStep{
				stepMig("mig1:latest", &contracts.StackGateSpec{Outbound: outbound("java", "17")}),
				stepMig("mig2:latest", &contracts.StackGateSpec{Inbound: inbound("java", "17")}),
			},
		},
		{
			name: "skips chaining when previous outbound disabled",
			steps: []contracts.MigStep{
				stepMig("mig1:latest", &contracts.StackGateSpec{Outbound: disabledOutbound("java")}),
				stepMig("mig2:latest", nil),
			},
			check: func(t *testing.T, steps []contracts.MigStep) {
				if steps[1].Stack != nil {
					t.Error("steps[1].Stack should remain nil when previous outbound is disabled")
				}
			},
		},
		{
			name: "skips chaining when previous has no Stack",
			steps: []contracts.MigStep{
				stepMig("mig1:latest", nil),
				stepMig("mig2:latest", nil),
			},
			check: func(t *testing.T, steps []contracts.MigStep) {
				if steps[1].Stack != nil {
					t.Error("steps[1].Stack should remain nil")
				}
			},
		},
		{
			name: "three step chain",
			steps: []contracts.MigStep{
				stepMig("mig1:latest", &contracts.StackGateSpec{
					Inbound:  inbound("java", "8"),
					Outbound: outbound("java", "11"),
				}),
				stepMig("mig2:latest", &contracts.StackGateSpec{
					Outbound: outbound("java", "17"),
				}),
				stepMig("mig3:latest", nil),
			},
			check: func(t *testing.T, steps []contracts.MigStep) {
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

func TestStackGatePhaseForJobSelectsEnabledBoundaryPhase(t *testing.T) {
	t.Parallel()

	firstInbound := inbound("java", "11")
	lastOutbound := outbound("java", "21")
	steps := []contracts.MigStep{
		{Stack: &contracts.StackGateSpec{Inbound: firstInbound, Outbound: outbound("java", "17")}},
		{Stack: &contracts.StackGateSpec{Inbound: inbound("java", "17"), Outbound: lastOutbound}},
	}
	if got := stackGatePhaseForJob(steps, types.JobTypePreGate); got != firstInbound {
		t.Fatalf("pre-gate phase = %p, want first inbound %p", got, firstInbound)
	}
	if got := stackGatePhaseForJob(steps, types.JobTypePostGate); got != lastOutbound {
		t.Fatalf("post-gate phase = %p, want last outbound %p", got, lastOutbound)
	}

	steps[0].Stack.Inbound.Enabled = false
	steps[1].Stack.Outbound.Enabled = false
	if got := stackGatePhaseForJob(steps, types.JobTypePreGate); got != nil {
		t.Fatalf("disabled pre-gate phase = %+v, want nil", got)
	}
	if got := stackGatePhaseForJob(steps, types.JobTypePostGate); got != nil {
		t.Fatalf("disabled post-gate phase = %+v, want nil", got)
	}
}

// TestBuildGateManifestFromRequest_StackGateThreading tests gate-only settings.
func TestBuildGateManifestFromRequest_StackGateThreading(t *testing.T) {
	tests := []struct {
		name  string
		spec  *contracts.MigSpec
		phase *contracts.StackGatePhaseSpec
		check func(t *testing.T, m contracts.StepManifest)
	}{
		{
			name: "threads StackGate when set",
			phase: &contracts.StackGatePhaseSpec{
				Enabled: true,
				Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
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
			check: func(t *testing.T, m contracts.StepManifest) {
				if m.Gate.StackGate != nil {
					t.Error("manifest.Gate.StackGate should be nil when not set")
				}
			},
		},
		{
			name: "threads build_gate.images into Gate.ImageOverrides",
			spec: &contracts.MigSpec{
				BuildGate: &contracts.BuildGateConfig{
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
			spec: &contracts.MigSpec{
				Steps: []contracts.MigStep{{
					Image: contracts.JobImage{Universal: "test:latest"},
					Stack: &contracts.StackGateSpec{
						Inbound:  inbound("java", "11"),
						Outbound: outbound("java", "17"),
					},
				}},
			},
			phase: outbound("java", "17"),
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
			req.MigSpec = tc.spec
			manifest, err := buildGateManifest(req, tc.phase)
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

func TestApplyGatePhaseOverridesUsesCanonicalBuildGatePhase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		jobType types.JobType
		want    *contracts.BuildGateStackConfig
	}{
		{name: "pre", jobType: types.JobTypePreGate, want: &contracts.BuildGateStackConfig{Mode: contracts.BuildGateStackModeForced, Language: "java", Release: "11"}},
		{name: "post", jobType: types.JobTypePostGate, want: &contracts.BuildGateStackConfig{Mode: contracts.BuildGateStackModeStrict, Language: "java", Release: "17"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newStartRunRequest()
			req.JobType = tt.jobType
			req.MigSpec = &contracts.MigSpec{BuildGate: &contracts.BuildGateConfig{
				Pre:  &contracts.BuildGatePhaseConfig{Stack: tests[0].want},
				Post: &contracts.BuildGatePhaseConfig{Stack: tests[1].want},
			}}
			manifest, err := buildGateManifest(req, nil)
			if err != nil {
				t.Fatalf("buildGateManifest() error: %v", err)
			}
			applyGatePhaseOverrides(&manifest, req)
			if manifest.Gate.StackDetect != tt.want {
				t.Fatalf("StackDetect = %+v, want %+v", manifest.Gate.StackDetect, tt.want)
			}
		})
	}
}
