package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestApplyGateStackDetect_PrePostAndNotReGate(t *testing.T) {
	pre := &contracts.BuildGateStackConfig{Enabled: true, Language: "java", Release: "11"}
	post := &contracts.BuildGateStackConfig{Enabled: true, Language: "java", Release: "17"}

	t.Run("pre_gate uses pre stack", func(t *testing.T) {
		manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
		typedOpts := RunOptions{}
		typedOpts.BuildGate.PreStack = pre
		typedOpts.BuildGate.PostStack = post

		applyGateStackDetect(&manifest, types.ModTypePreGate, typedOpts)

		if manifest.Gate.StackDetect != pre {
			t.Fatalf("Gate.StackDetect=%v; want pre", manifest.Gate.StackDetect)
		}
	})

	t.Run("post_gate uses post stack", func(t *testing.T) {
		manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
		typedOpts := RunOptions{}
		typedOpts.BuildGate.PreStack = pre
		typedOpts.BuildGate.PostStack = post

		applyGateStackDetect(&manifest, types.ModTypePostGate, typedOpts)

		if manifest.Gate.StackDetect != post {
			t.Fatalf("Gate.StackDetect=%v; want post", manifest.Gate.StackDetect)
		}
	})

	t.Run("re_gate uses stack detection output (no StackDetect override)", func(t *testing.T) {
		manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
		typedOpts := RunOptions{}
		typedOpts.BuildGate.PreStack = pre
		typedOpts.BuildGate.PostStack = post

		applyGateStackDetect(&manifest, types.ModTypeReGate, typedOpts)

		if manifest.Gate.StackDetect != nil {
			t.Fatalf("Gate.StackDetect=%v; want nil", manifest.Gate.StackDetect)
		}
	})
}

