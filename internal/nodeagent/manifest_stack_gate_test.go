package nodeagent

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestValidateAndDeriveStackGateChaining tests the chaining validation logic.
func TestValidateAndDeriveStackGateChaining(t *testing.T) {
	t.Run("single step no chaining", func(t *testing.T) {
		steps := []StepMod{{
			Image: contracts.ModImage{Universal: "test:latest"},
			Stack: &contracts.StackGateSpec{
				Inbound: &contracts.StackGatePhaseSpec{
					Enabled: true,
					Expect:  &contracts.StackExpectation{Language: "java"},
				},
			},
		}}

		err := validateAndDeriveStackGateChaining(steps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("derives inbound from previous outbound", func(t *testing.T) {
		steps := []StepMod{
			{
				Image: contracts.ModImage{Universal: "mod1:latest"},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
			{
				Image: contracts.ModImage{Universal: "mod2:latest"},
				// No Stack config; should derive inbound from previous outbound.
			},
		}

		err := validateAndDeriveStackGateChaining(steps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify inbound was derived.
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
		steps := []StepMod{
			{
				Image: contracts.ModImage{Universal: "mod1:latest"},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "11"},
					},
				},
			},
			{
				Image: contracts.ModImage{Universal: "mod2:latest"},
				Stack: &contracts.StackGateSpec{
					// Inbound is nil but Stack exists.
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
		}

		err := validateAndDeriveStackGateChaining(steps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify inbound was derived.
		if steps[1].Stack.Inbound == nil {
			t.Fatal("steps[1].Stack.Inbound should have been derived")
		}
		if steps[1].Stack.Inbound.Expect.Release != "11" {
			t.Errorf("derived inbound.expect.release = %q, want 11", steps[1].Stack.Inbound.Expect.Release)
		}
	})

	t.Run("rejects mismatched explicit inbound", func(t *testing.T) {
		steps := []StepMod{
			{
				Image: contracts.ModImage{Universal: "mod1:latest"},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
			{
				Image: contracts.ModImage{Universal: "mod2:latest"},
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
		steps := []StepMod{
			{
				Image: contracts.ModImage{Universal: "mod1:latest"},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
			{
				Image: contracts.ModImage{Universal: "mod2:latest"},
				Stack: &contracts.StackGateSpec{
					Inbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"}, // Matches!
					},
				},
			},
		}

		err := validateAndDeriveStackGateChaining(steps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("skips chaining when previous outbound disabled", func(t *testing.T) {
		steps := []StepMod{
			{
				Image: contracts.ModImage{Universal: "mod1:latest"},
				Stack: &contracts.StackGateSpec{
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: false, // Disabled.
						Expect:  &contracts.StackExpectation{Language: "java"},
					},
				},
			},
			{
				Image: contracts.ModImage{Universal: "mod2:latest"},
				// No Stack; should not derive since previous outbound is disabled.
			},
		}

		err := validateAndDeriveStackGateChaining(steps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify no derivation occurred.
		if steps[1].Stack != nil {
			t.Error("steps[1].Stack should remain nil when previous outbound is disabled")
		}
	})

	t.Run("skips chaining when previous has no Stack", func(t *testing.T) {
		steps := []StepMod{
			{
				Image: contracts.ModImage{Universal: "mod1:latest"},
				// No Stack config.
			},
			{
				Image: contracts.ModImage{Universal: "mod2:latest"},
				// No Stack config.
			},
		}

		err := validateAndDeriveStackGateChaining(steps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify no derivation occurred.
		if steps[1].Stack != nil {
			t.Error("steps[1].Stack should remain nil")
		}
	})

	t.Run("three step chain", func(t *testing.T) {
		steps := []StepMod{
			{
				Image: contracts.ModImage{Universal: "mod1:latest"},
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
				Image: contracts.ModImage{Universal: "mod2:latest"},
				Stack: &contracts.StackGateSpec{
					// Inbound will be derived from step 0 outbound (release: 11).
					Outbound: &contracts.StackGatePhaseSpec{
						Enabled: true,
						Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
					},
				},
			},
			{
				Image: contracts.ModImage{Universal: "mod3:latest"},
				// Inbound will be derived from step 1 outbound (release: 17).
			},
		}

		err := validateAndDeriveStackGateChaining(steps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify step 1 inbound.
		if steps[1].Stack.Inbound == nil {
			t.Fatal("steps[1].Stack.Inbound should have been derived")
		}
		if steps[1].Stack.Inbound.Expect.Release != "11" {
			t.Errorf("steps[1] inbound.expect.release = %q, want 11", steps[1].Stack.Inbound.Expect.Release)
		}

		// Verify step 2 inbound.
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
	t.Run("nil input", func(t *testing.T) {
		result := stackGatePhaseSpecToStepGate(nil)
		if result != nil {
			t.Error("expected nil for nil input")
		}
	})

	t.Run("disabled phase", func(t *testing.T) {
		phase := &contracts.StackGatePhaseSpec{Enabled: false}
		result := stackGatePhaseSpecToStepGate(phase)
		if result != nil {
			t.Error("expected nil for disabled phase")
		}
	})

	t.Run("enabled phase with expect", func(t *testing.T) {
		phase := &contracts.StackGatePhaseSpec{
			Enabled: true,
			Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
		}
		result := stackGatePhaseSpecToStepGate(phase)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if !result.Enabled {
			t.Error("result.Enabled should be true")
		}
		if result.Expect == nil {
			t.Fatal("result.Expect should not be nil")
		}
		if result.Expect.Language != "java" {
			t.Errorf("result.Expect.Language = %q, want java", result.Expect.Language)
		}
		if result.Expect.Release != "17" {
			t.Errorf("result.Expect.Release = %q, want 17", result.Expect.Release)
		}
	})
}

// TestBuildGateManifestFromRequest_StackGateThreading tests that StackGate
// is correctly threaded into gate manifests via typedOpts.StackGate.
func TestBuildGateManifestFromRequest_StackGateThreading(t *testing.T) {
	t.Run("threads StackGate when set", func(t *testing.T) {
		req := baseStartRunRequest()
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
		req := baseStartRunRequest()
		typedOpts := RunOptions{
			StackGate: nil,
		}

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

	t.Run("outbound expectations for post gate", func(t *testing.T) {
		// Simulate post_gate scenario where outbound expectations are used.
		req := baseStartRunRequest()
		typedOpts := RunOptions{
			Steps: []StepMod{{
				Image: contracts.ModImage{Universal: "test:latest"},
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
			// Set StackGate to outbound expectations (as would be done by executeGateJob for post_gate).
			StackGate: &contracts.StepGateStackSpec{
				Enabled: true,
				Expect:  &contracts.StackExpectation{Language: "java", Release: "17"},
			},
		}

		manifest, err := buildGateManifestFromRequest(req, typedOpts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify outbound (release: 17) is threaded, not inbound (release: 11).
		if manifest.Gate.StackGate == nil {
			t.Fatal("manifest.Gate.StackGate should be set")
		}
		if manifest.Gate.StackGate.Expect.Release != "17" {
			t.Errorf("StackGate.Expect.Release = %q, want 17 (outbound)", manifest.Gate.StackGate.Expect.Release)
		}
	})
}

// baseStartRunRequest creates a minimal valid StartRunRequest for testing.
func baseStartRunRequest() StartRunRequest {
	return StartRunRequest{
		RunID:   "run-test-12345678",
		JobID:   "job-test-12345678",
		RepoURL: "https://github.com/example/repo.git",
	}
}
