package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestApplyGatePhaseOverrides(t *testing.T) {
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

	cases := []struct {
		name                  string
		jobType               types.JobType
		buildPreConfig        *contracts.BuildGatePhaseConfig // nil for case 4
		buildPostConfig       *contracts.BuildGatePhaseConfig
		wantStackDetect       *contracts.BuildGateStackConfig
		wantGateProfile       *contracts.BuildGateProfileOverride
		wantTarget            string
		wantEnforceTargetLock bool
	}{
		{
			name:    "pre_gate uses pre stack",
			jobType: types.JobTypePreGate,
			buildPreConfig: &contracts.BuildGatePhaseConfig{
				Stack: pre, GateProfile: preGateProfile,
				Target: contracts.GateProfileTargetUnit,
			},
			buildPostConfig: &contracts.BuildGatePhaseConfig{
				Stack: post, GateProfile: postGateProfile,
				Target: contracts.GateProfileTargetAllTests,
			},
			wantStackDetect: pre,
			wantGateProfile: preGateProfile,
			wantTarget:      contracts.GateProfileTargetUnit,
		},
		{
			name:    "post_gate uses post stack",
			jobType: types.JobTypePostGate,
			buildPreConfig: &contracts.BuildGatePhaseConfig{
				Stack: pre, GateProfile: preGateProfile,
				Target: contracts.GateProfileTargetUnit,
			},
			buildPostConfig: &contracts.BuildGatePhaseConfig{
				Stack: post, GateProfile: postGateProfile,
				Target: contracts.GateProfileTargetAllTests,
			},
			wantStackDetect: post,
			wantGateProfile: postGateProfile,
			wantTarget:      contracts.GateProfileTargetAllTests,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
			typedOpts := RunOptions{}
			typedOpts.BuildGate.Pre = tc.buildPreConfig
			typedOpts.BuildGate.Post = tc.buildPostConfig

			req := StartRunRequest{
				JobType: tc.jobType,
			}

			applyGatePhaseOverrides(&manifest, req, typedOpts)

			if manifest.Gate.StackDetect != tc.wantStackDetect {
				t.Fatalf("Gate.StackDetect=%v; want %v", manifest.Gate.StackDetect, tc.wantStackDetect)
			}
			if tc.wantGateProfile != nil && manifest.Gate.GateProfile != tc.wantGateProfile {
				t.Fatalf("Gate.GateProfile=%v; want %v", manifest.Gate.GateProfile, tc.wantGateProfile)
			}
			if tc.wantTarget != "" {
				if got, want := manifest.Gate.Target, tc.wantTarget; got != want {
					t.Fatalf("Gate.Target=%q; want %q", got, want)
				}
			}
			if got, want := manifest.Gate.EnforceTargetLock, tc.wantEnforceTargetLock; got != want {
				t.Fatalf("Gate.EnforceTargetLock=%v; want %v", got, want)
			}
		})
	}
}
