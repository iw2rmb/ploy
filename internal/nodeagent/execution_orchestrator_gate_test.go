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
		recoveryCtx           *contracts.RecoveryClaimContext
		buildPreConfig        *contracts.BuildGatePhaseConfig // nil for case 4
		buildPostConfig       *contracts.BuildGatePhaseConfig
		wantStackDetect       *contracts.BuildGateStackConfig // nil for re_gate
		wantGateProfile       *contracts.BuildGateProfileOverride
		wantTarget            string
		wantEnforceTargetLock bool
		wantCA                []string
	}{
		{
			name:    "pre_gate uses pre stack",
			jobType: types.JobTypePreGate,
			buildPreConfig: &contracts.BuildGatePhaseConfig{
				Stack: pre, GateProfile: preGateProfile,
				CA:     []string{"aaaaaaa11111", "bbbbbbb22222"},
				Target: contracts.GateProfileTargetUnit,
			},
			buildPostConfig: &contracts.BuildGatePhaseConfig{
				Stack: post, GateProfile: postGateProfile,
				Target: contracts.GateProfileTargetAllTests,
			},
			wantStackDetect: pre,
			wantGateProfile: preGateProfile,
			wantTarget:      contracts.GateProfileTargetUnit,
			wantCA:          []string{"aaaaaaa11111", "bbbbbbb22222"},
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
				CA:     []string{"ccccccc33333"},
				Target: contracts.GateProfileTargetAllTests,
			},
			wantStackDetect: post,
			wantGateProfile: postGateProfile,
			wantTarget:      contracts.GateProfileTargetAllTests,
			wantCA:          []string{"ccccccc33333"},
		},
		{
			name:    "re_gate uses stack detection output and post gate_profile override",
			jobType: types.JobTypeReGate,
			recoveryCtx: &contracts.RecoveryClaimContext{
				GateProfileSchemaJSON: `{"type":"object"}`,
			},
			buildPreConfig: &contracts.BuildGatePhaseConfig{
				Stack: pre, GateProfile: preGateProfile,
				Target: contracts.GateProfileTargetUnit,
			},
			buildPostConfig: &contracts.BuildGatePhaseConfig{
				Stack: post, GateProfile: postGateProfile,
				CA:     []string{"ddddddd44444"},
				Target: contracts.GateProfileTargetAllTests,
			},
			wantStackDetect:       nil,
			wantGateProfile:       postGateProfile,
			wantTarget:            contracts.GateProfileTargetAllTests,
			wantEnforceTargetLock: true,
			wantCA:                []string{"ddddddd44444"},
		},
		{
			name:    "re_gate does not enforce target lock for non-infra recovery",
			jobType: types.JobTypeReGate,
			recoveryCtx: &contracts.RecoveryClaimContext{
				LoopKind: "healing",
			},
			buildPostConfig: &contracts.BuildGatePhaseConfig{
				CA:     []string{"eeeeeee55555"},
				Target: contracts.GateProfileTargetAllTests,
			},
			wantEnforceTargetLock: false,
			wantCA:                []string{"eeeeeee55555"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := contracts.StepManifest{Gate: &contracts.StepGateSpec{}}
			typedOpts := RunOptions{}
			typedOpts.BuildGate.Pre = tc.buildPreConfig
			typedOpts.BuildGate.Post = tc.buildPostConfig

			req := StartRunRequest{
				JobType:         tc.jobType,
				RecoveryContext: tc.recoveryCtx,
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
			if len(tc.wantCA) > 0 {
				if got, want := len(manifest.CA), len(tc.wantCA); got != want {
					t.Fatalf("manifest.CA length=%d; want %d (%v)", got, want, manifest.CA)
				}
				for i, want := range tc.wantCA {
					if manifest.CA[i] != want {
						t.Fatalf("manifest.CA[%d]=%q; want %q", i, manifest.CA[i], want)
					}
				}
			}
		})
	}
}
