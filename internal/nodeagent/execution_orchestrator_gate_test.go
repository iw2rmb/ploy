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
		gateSkip              *contracts.BuildGateSkipMetadata
		recoveryCtx           *contracts.RecoveryClaimContext
		buildPreConfig        *contracts.BuildGatePhaseConfig  // nil for case 4
		buildPostConfig       *contracts.BuildGatePhaseConfig
		wantStackDetect       *contracts.BuildGateStackConfig  // nil for re_gate
		wantGateProfile       *contracts.BuildGateProfileOverride
		wantTarget            string
		wantAlways            bool
		wantEnforceTargetLock bool
		wantSkip              *contracts.BuildGateSkipMetadata
	}{
		{
			name:    "pre_gate uses pre stack",
			jobType: types.JobTypePreGate,
			gateSkip: &contracts.BuildGateSkipMetadata{
				Enabled: true, SourceProfileID: 11, MatchedTarget: contracts.GateProfileTargetUnit,
			},
			buildPreConfig: &contracts.BuildGatePhaseConfig{
				Stack: pre, GateProfile: preGateProfile,
				Target: contracts.GateProfileTargetUnit, Always: true,
			},
			buildPostConfig: &contracts.BuildGatePhaseConfig{
				Stack: post, GateProfile: postGateProfile,
				Target: contracts.GateProfileTargetAllTests, Always: false,
			},
			wantStackDetect: pre,
			wantGateProfile: preGateProfile,
			wantTarget:      contracts.GateProfileTargetUnit,
			wantAlways:      true,
			wantSkip: &contracts.BuildGateSkipMetadata{
				Enabled: true, SourceProfileID: 11, MatchedTarget: contracts.GateProfileTargetUnit,
			},
		},
		{
			name:    "post_gate uses post stack",
			jobType: types.JobTypePostGate,
			gateSkip: &contracts.BuildGateSkipMetadata{
				Enabled: true, SourceProfileID: 22, MatchedTarget: contracts.GateProfileTargetAllTests,
			},
			buildPreConfig: &contracts.BuildGatePhaseConfig{
				Stack: pre, GateProfile: preGateProfile,
				Target: contracts.GateProfileTargetUnit, Always: true,
			},
			buildPostConfig: &contracts.BuildGatePhaseConfig{
				Stack: post, GateProfile: postGateProfile,
				Target: contracts.GateProfileTargetAllTests, Always: false,
			},
			wantStackDetect: post,
			wantGateProfile: postGateProfile,
			wantTarget:      contracts.GateProfileTargetAllTests,
			wantAlways:      false,
			wantSkip: &contracts.BuildGateSkipMetadata{
				Enabled: true, SourceProfileID: 22, MatchedTarget: contracts.GateProfileTargetAllTests,
			},
		},
		{
			name:    "re_gate uses stack detection output and post gate_profile override",
			jobType: types.JobTypeReGate,
			recoveryCtx: &contracts.RecoveryClaimContext{
				SelectedErrorKind: "infra",
			},
			buildPreConfig: &contracts.BuildGatePhaseConfig{
				Stack: pre, GateProfile: preGateProfile,
				Target: contracts.GateProfileTargetUnit, Always: true,
			},
			buildPostConfig: &contracts.BuildGatePhaseConfig{
				Stack: post, GateProfile: postGateProfile,
				Target: contracts.GateProfileTargetAllTests, Always: false,
			},
			wantStackDetect:       nil,
			wantGateProfile:       postGateProfile,
			wantTarget:            contracts.GateProfileTargetAllTests,
			wantAlways:            false,
			wantEnforceTargetLock: true,
		},
		{
			name:    "re_gate does not enforce target lock for non-infra recovery",
			jobType: types.JobTypeReGate,
			recoveryCtx: &contracts.RecoveryClaimContext{
				SelectedErrorKind: "code",
			},
			buildPostConfig: &contracts.BuildGatePhaseConfig{
				Target: contracts.GateProfileTargetAllTests,
			},
			wantEnforceTargetLock: false,
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
				GateSkip:        tc.gateSkip,
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
			if got, want := manifest.Gate.Always, tc.wantAlways; got != want {
				t.Fatalf("Gate.Always=%v; want %v", got, want)
			}
			if got, want := manifest.Gate.EnforceTargetLock, tc.wantEnforceTargetLock; got != want {
				t.Fatalf("Gate.EnforceTargetLock=%v; want %v", got, want)
			}
			if tc.wantSkip != nil && manifest.Gate.Skip != tc.gateSkip {
				t.Fatalf("Gate.Skip=%v; want skip payload", manifest.Gate.Skip)
			}
		})
	}
}
