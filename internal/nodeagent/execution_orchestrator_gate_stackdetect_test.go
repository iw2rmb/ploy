package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestApplyGatePhaseOverrides_PrePostAndReGate(t *testing.T) {
	pre := &contracts.BuildGateStackConfig{Enabled: true, Language: "java", Release: "11"}
	post := &contracts.BuildGateStackConfig{Enabled: true, Language: "java", Release: "17"}
	preGateProfile := &contracts.BuildGateProfileOverride{
		Command: contracts.CommandSpec{Shell: "go test ./..."},
		Env:     map[string]string{"GOFLAGS": "-mod=readonly"},
	}
	postGateProfile := &contracts.BuildGateProfileOverride{
		Command: contracts.CommandSpec{Shell: "go test ./... -run TestUnit"},
		Env:     map[string]string{"CGO_ENABLED": "0"},
	}

	t.Run("pre_gate uses pre stack", func(t *testing.T) {
		manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
		typedOpts := RunOptions{}
		typedOpts.BuildGate.Pre = &contracts.BuildGatePhaseConfig{
			Stack:       pre,
			GateProfile: preGateProfile,
			Target:      contracts.GateProfileTargetUnit,
			Always:      true,
		}
		typedOpts.BuildGate.Post = &contracts.BuildGatePhaseConfig{
			Stack:       post,
			GateProfile: postGateProfile,
			Target:      contracts.GateProfileTargetAllTests,
			Always:      false,
		}

		skip := &contracts.BuildGateSkipMetadata{Enabled: true, SourceProfileID: 11, MatchedTarget: contracts.GateProfileTargetUnit}
		applyGatePhaseOverrides(&manifest, StartRunRequest{
			JobType:  types.JobTypePreGate,
			GateSkip: skip,
		}, typedOpts)

		if manifest.Gate.StackDetect != pre {
			t.Fatalf("Gate.StackDetect=%v; want pre", manifest.Gate.StackDetect)
		}
		if manifest.Gate.GateProfile != preGateProfile {
			t.Fatalf("Gate.GateProfile=%v; want pre gate_profile override", manifest.Gate.GateProfile)
		}
		if got, want := manifest.Gate.Target, contracts.GateProfileTargetUnit; got != want {
			t.Fatalf("Gate.Target=%q; want %q", got, want)
		}
		if !manifest.Gate.Always {
			t.Fatal("Gate.Always=false; want true")
		}
		if manifest.Gate.Skip != skip {
			t.Fatalf("Gate.Skip=%v; want skip payload", manifest.Gate.Skip)
		}
	})

	t.Run("post_gate uses post stack", func(t *testing.T) {
		manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
		typedOpts := RunOptions{}
		typedOpts.BuildGate.Pre = &contracts.BuildGatePhaseConfig{
			Stack:       pre,
			GateProfile: preGateProfile,
			Target:      contracts.GateProfileTargetUnit,
			Always:      true,
		}
		typedOpts.BuildGate.Post = &contracts.BuildGatePhaseConfig{
			Stack:       post,
			GateProfile: postGateProfile,
			Target:      contracts.GateProfileTargetAllTests,
			Always:      false,
		}

		skip := &contracts.BuildGateSkipMetadata{Enabled: true, SourceProfileID: 22, MatchedTarget: contracts.GateProfileTargetAllTests}
		applyGatePhaseOverrides(&manifest, StartRunRequest{
			JobType:  types.JobTypePostGate,
			GateSkip: skip,
		}, typedOpts)

		if manifest.Gate.StackDetect != post {
			t.Fatalf("Gate.StackDetect=%v; want post", manifest.Gate.StackDetect)
		}
		if manifest.Gate.GateProfile != postGateProfile {
			t.Fatalf("Gate.GateProfile=%v; want post gate_profile override", manifest.Gate.GateProfile)
		}
		if got, want := manifest.Gate.Target, contracts.GateProfileTargetAllTests; got != want {
			t.Fatalf("Gate.Target=%q; want %q", got, want)
		}
		if manifest.Gate.Always {
			t.Fatal("Gate.Always=true; want false")
		}
		if manifest.Gate.Skip != skip {
			t.Fatalf("Gate.Skip=%v; want skip payload", manifest.Gate.Skip)
		}
	})

	t.Run("re_gate uses stack detection output and post gate_profile override", func(t *testing.T) {
		manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
		typedOpts := RunOptions{}
		typedOpts.BuildGate.Pre = &contracts.BuildGatePhaseConfig{
			Stack:       pre,
			GateProfile: preGateProfile,
			Target:      contracts.GateProfileTargetUnit,
			Always:      true,
		}
		typedOpts.BuildGate.Post = &contracts.BuildGatePhaseConfig{
			Stack:       post,
			GateProfile: postGateProfile,
			Target:      contracts.GateProfileTargetAllTests,
			Always:      false,
		}

		applyGatePhaseOverrides(&manifest, StartRunRequest{
			JobType: types.JobTypeReGate,
			RecoveryContext: &contracts.RecoveryClaimContext{
				SelectedErrorKind: "infra",
			},
		}, typedOpts)

		if manifest.Gate.StackDetect != nil {
			t.Fatalf("Gate.StackDetect=%v; want nil", manifest.Gate.StackDetect)
		}
		if manifest.Gate.GateProfile != postGateProfile {
			t.Fatalf("Gate.GateProfile=%v; want post gate_profile override", manifest.Gate.GateProfile)
		}
		if got, want := manifest.Gate.Target, contracts.GateProfileTargetAllTests; got != want {
			t.Fatalf("Gate.Target=%q; want %q", got, want)
		}
		if manifest.Gate.Always {
			t.Fatal("Gate.Always=true; want false")
		}
		if !manifest.Gate.EnforceTargetLock {
			t.Fatal("Gate.EnforceTargetLock=false; want true for infra re_gate with pinned target")
		}
	})

	t.Run("re_gate does not enforce target lock for non-infra recovery", func(t *testing.T) {
		manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
		typedOpts := RunOptions{}
		typedOpts.BuildGate.Post = &contracts.BuildGatePhaseConfig{
			Target: contracts.GateProfileTargetAllTests,
		}

		applyGatePhaseOverrides(&manifest, StartRunRequest{
			JobType: types.JobTypeReGate,
			RecoveryContext: &contracts.RecoveryClaimContext{
				SelectedErrorKind: "code",
			},
		}, typedOpts)

		if manifest.Gate.EnforceTargetLock {
			t.Fatal("Gate.EnforceTargetLock=true; want false for non-infra re_gate")
		}
	})

}
