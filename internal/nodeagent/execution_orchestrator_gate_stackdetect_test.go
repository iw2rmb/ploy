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
		typedOpts.BuildGate.PreStack = pre
		typedOpts.BuildGate.PostStack = post
		typedOpts.BuildGate.PreGateProfile = preGateProfile
		typedOpts.BuildGate.PostGateProfile = postGateProfile

		applyGatePhaseOverrides(&manifest, types.JobTypePreGate, typedOpts)

		if manifest.Gate.StackDetect != pre {
			t.Fatalf("Gate.StackDetect=%v; want pre", manifest.Gate.StackDetect)
		}
		if manifest.Gate.GateProfile != preGateProfile {
			t.Fatalf("Gate.GateProfile=%v; want pre gate_profile override", manifest.Gate.GateProfile)
		}
	})

	t.Run("post_gate uses post stack", func(t *testing.T) {
		manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
		typedOpts := RunOptions{}
		typedOpts.BuildGate.PreStack = pre
		typedOpts.BuildGate.PostStack = post
		typedOpts.BuildGate.PreGateProfile = preGateProfile
		typedOpts.BuildGate.PostGateProfile = postGateProfile

		applyGatePhaseOverrides(&manifest, types.JobTypePostGate, typedOpts)

		if manifest.Gate.StackDetect != post {
			t.Fatalf("Gate.StackDetect=%v; want post", manifest.Gate.StackDetect)
		}
		if manifest.Gate.GateProfile != postGateProfile {
			t.Fatalf("Gate.GateProfile=%v; want post gate_profile override", manifest.Gate.GateProfile)
		}
	})

	t.Run("re_gate uses stack detection output and post gate_profile override", func(t *testing.T) {
		manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
		typedOpts := RunOptions{}
		typedOpts.BuildGate.PreStack = pre
		typedOpts.BuildGate.PostStack = post
		typedOpts.BuildGate.PreGateProfile = preGateProfile
		typedOpts.BuildGate.PostGateProfile = postGateProfile

		applyGatePhaseOverrides(&manifest, types.JobTypeReGate, typedOpts)

		if manifest.Gate.StackDetect != nil {
			t.Fatalf("Gate.StackDetect=%v; want nil", manifest.Gate.StackDetect)
		}
		if manifest.Gate.GateProfile != postGateProfile {
			t.Fatalf("Gate.GateProfile=%v; want post gate_profile override", manifest.Gate.GateProfile)
		}
	})
}
